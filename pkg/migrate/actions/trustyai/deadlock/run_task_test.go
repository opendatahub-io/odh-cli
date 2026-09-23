//nolint:testpackage // Tests internal implementation
package deadlock

import (
	"bytes"
	"testing"

	"github.com/blang/semver/v4"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sfake "k8s.io/client-go/kubernetes/fake"

	"github.com/opendatahub-io/odh-cli/pkg/migrate/action"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
	"github.com/opendatahub-io/odh-cli/pkg/util/iostreams"

	. "github.com/onsi/gomega"
)

func newTestClient(objects ...runtime.Object) client.Client {
	k8sClient := k8sfake.NewSimpleClientset(objects...)

	return client.NewForTesting(client.TestClientConfig{
		Kubernetes: k8sClient,
	})
}

func newPredictorPod(name, namespace, isvc string, phase corev1.PodPhase) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				"component":                          "predictor",
				"serving.kserve.io/inferenceservice": isvc,
			},
		},
		Status: corev1.PodStatus{
			Phase: phase,
		},
	}
}

func newTarget(k8sClient client.Client, opts ...func(*action.Target)) action.Target {
	currentVersion := semver.MustParse("2.25.0")
	targetVersion := semver.MustParse("3.0.0")

	io := iostreams.NewIOStreams(
		&bytes.Buffer{},
		&bytes.Buffer{},
		&bytes.Buffer{},
	)

	target := action.Target{
		Client:         k8sClient,
		CurrentVersion: &currentVersion,
		TargetVersion:  &targetVersion,
		DryRun:         false,
		SkipConfirm:    true,
		Recorder:       action.NewVerboseRootRecorder(io),
		IO:             io,
	}

	for _, opt := range opts {
		opt(&target)
	}

	return target
}

func withDryRun(t *action.Target) {
	t.DryRun = true
}

func TestBreakGPUDeadlockAction_Metadata(t *testing.T) {
	g := NewWithT(t)

	a := &BreakGPUDeadlockAction{}

	g.Expect(a.ID()).To(Equal("trustyai.break-gpu-deadlock"))
	g.Expect(a.Name()).To(Equal("Break GPU deployment deadlocks"))
	g.Expect(a.Description()).To(ContainSubstring("GPU"))
	g.Expect(a.Group()).To(Equal(action.GroupMigration))
	g.Expect(a.Phase()).To(Equal(action.PhasePostUpgrade))
	g.Expect(a.Prepare()).To(BeNil())
	g.Expect(a.Run()).ToNot(BeNil())
}

func TestBreakGPUDeadlockAction_CanApply(t *testing.T) {
	g := NewWithT(t)

	a := &BreakGPUDeadlockAction{}
	g.Expect(a.CanApply(action.Target{})).To(BeTrue())
}

func TestRunTask_Validate_NoPods(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	target := newTarget(newTestClient())

	a := &BreakGPUDeadlockAction{}
	result, err := a.Run().Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Completed).To(BeTrue())
}

func TestRunTask_Validate_WithDeadlocks(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	target := newTarget(newTestClient(
		newPredictorPod("model-a-running", "ns1", "model-a", corev1.PodRunning),
		newPredictorPod("model-a-pending", "ns1", "model-a", corev1.PodPending),
	))

	a := &BreakGPUDeadlockAction{}
	result, err := a.Run().Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Completed).To(BeTrue())
}

func TestRunTask_Execute_NoDeadlocks(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	target := newTarget(newTestClient(
		newPredictorPod("model-a-running", "ns1", "model-a", corev1.PodRunning),
	))

	a := &BreakGPUDeadlockAction{}
	result, err := a.Run().Execute(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Completed).To(BeTrue())
}

func TestRunTask_Execute_FixesDeadlocks(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	k8sClient := k8sfake.NewSimpleClientset(
		newPredictorPod("model-a-running", "ns1", "model-a", corev1.PodRunning),
		newPredictorPod("model-a-pending", "ns1", "model-a", corev1.PodPending),
	)

	testClient := client.NewForTesting(client.TestClientConfig{
		Kubernetes: k8sClient,
	})
	target := newTarget(testClient)

	a := &BreakGPUDeadlockAction{}
	result, err := a.Run().Execute(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Completed).To(BeTrue())

	pods, err := k8sClient.CoreV1().Pods("ns1").List(ctx, metav1.ListOptions{})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(pods.Items).To(HaveLen(1))
	g.Expect(pods.Items[0].Name).To(Equal("model-a-pending"))
}

func TestRunTask_Execute_DryRun(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	k8sClient := k8sfake.NewSimpleClientset(
		newPredictorPod("model-a-running", "ns1", "model-a", corev1.PodRunning),
		newPredictorPod("model-a-pending", "ns1", "model-a", corev1.PodPending),
	)

	testClient := client.NewForTesting(client.TestClientConfig{
		Kubernetes: k8sClient,
	})
	target := newTarget(testClient, withDryRun)

	a := &BreakGPUDeadlockAction{}
	result, err := a.Run().Execute(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Completed).To(BeTrue())

	pods, err := k8sClient.CoreV1().Pods("ns1").List(ctx, metav1.ListOptions{})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(pods.Items).To(HaveLen(2))
}

func TestRunTask_Execute_MultiNamespace(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	k8sClient := k8sfake.NewSimpleClientset(
		newPredictorPod("model-a-running", "ns1", "model-a", corev1.PodRunning),
		newPredictorPod("model-a-pending", "ns1", "model-a", corev1.PodPending),
		newPredictorPod("model-b-running", "ns2", "model-b", corev1.PodRunning),
		newPredictorPod("model-b-pending", "ns2", "model-b", corev1.PodPending),
		newPredictorPod("model-c-running", "ns2", "model-c", corev1.PodRunning),
	)

	testClient := client.NewForTesting(client.TestClientConfig{
		Kubernetes: k8sClient,
	})
	target := newTarget(testClient)

	a := &BreakGPUDeadlockAction{}
	result, err := a.Run().Execute(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Completed).To(BeTrue())

	ns1Pods, _ := k8sClient.CoreV1().Pods("ns1").List(ctx, metav1.ListOptions{})
	g.Expect(ns1Pods.Items).To(HaveLen(1))
	g.Expect(ns1Pods.Items[0].Name).To(Equal("model-a-pending"))

	ns2Pods, _ := k8sClient.CoreV1().Pods("ns2").List(ctx, metav1.ListOptions{})
	g.Expect(ns2Pods.Items).To(HaveLen(2))
}

func TestRunTask_Execute_PodWithoutISVCLabel(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	podNoLabel := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "orphan-pod",
			Namespace: "ns1",
			Labels: map[string]string{
				"component": "predictor",
			},
		},
		Status: corev1.PodStatus{Phase: corev1.PodRunning},
	}

	target := newTarget(newTestClient(podNoLabel))

	a := &BreakGPUDeadlockAction{}
	result, err := a.Run().Execute(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Completed).To(BeTrue())
}

func TestRunTask_Execute_OnlyPending(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	target := newTarget(newTestClient(
		newPredictorPod("model-a-pending", "ns1", "model-a", corev1.PodPending),
	))

	a := &BreakGPUDeadlockAction{}
	result, err := a.Run().Execute(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Completed).To(BeTrue())
}

func TestRunTask_Execute_UserCancels(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	inBuf := bytes.NewBufferString("n\n")
	io := iostreams.NewIOStreams(inBuf, &bytes.Buffer{}, &bytes.Buffer{})

	currentVersion := semver.MustParse("2.25.0")
	targetVersion := semver.MustParse("3.0.0")

	target := action.Target{
		Client: newTestClient(
			newPredictorPod("model-a-running", "ns1", "model-a", corev1.PodRunning),
			newPredictorPod("model-a-pending", "ns1", "model-a", corev1.PodPending),
		),
		CurrentVersion: &currentVersion,
		TargetVersion:  &targetVersion,
		DryRun:         false,
		SkipConfirm:    false,
		Recorder:       action.NewVerboseRootRecorder(io),
		IO:             io,
	}

	a := &BreakGPUDeadlockAction{}
	result, err := a.Run().Execute(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Completed).To(BeTrue())
	g.Expect(result.HasSkippedSteps()).To(BeTrue())
}
