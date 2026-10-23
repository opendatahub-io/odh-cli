//nolint:testpackage // Tests internal implementation
package guardrails

import (
	"bytes"
	"context"
	"testing"

	"github.com/blang/semver/v4"
	"github.com/spf13/pflag"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	"github.com/opendatahub-io/odh-cli/pkg/migrate/action"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
	"github.com/opendatahub-io/odh-cli/pkg/util/iostreams"

	. "github.com/onsi/gomega"
)

func newScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	_ = metav1.AddMetaToScheme(scheme)

	return scheme
}

func newFakeClient(scheme *runtime.Scheme, objects ...runtime.Object) client.Client {
	listKinds := map[schema.GroupVersionResource]string{
		resources.GuardrailsOrchestrator.GVR(): resources.GuardrailsOrchestrator.ListKind(),
		resources.Deployment.GVR():             resources.Deployment.ListKind(),
	}

	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, listKinds, objects...)

	return client.NewForTesting(client.TestClientConfig{
		Dynamic: dynamicClient,
	})
}

func newGORCH(name, namespace string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.GuardrailsOrchestrator.APIVersion(),
			"kind":       resources.GuardrailsOrchestrator.Kind,
			"metadata": map[string]any{
				"name":      name,
				"namespace": namespace,
			},
		},
	}
}

func newDeployment(name, namespace string, hasProbe bool) *unstructured.Unstructured {
	container := map[string]any{
		"name":  name,
		"image": "guardrails:latest",
	}

	if hasProbe {
		container["readinessProbe"] = map[string]any{
			"httpGet": map[string]any{
				"path": "/health",
				"port": int64(8034),
			},
		}
	}

	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.Deployment.APIVersion(),
			"kind":       resources.Deployment.Kind,
			"metadata": map[string]any{
				"name":      name,
				"namespace": namespace,
			},
			"spec": map[string]any{
				"template": map[string]any{
					"spec": map[string]any{
						"containers": []any{container},
					},
				},
			},
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

func TestPatchGuardrailsAction_Metadata(t *testing.T) {
	g := NewWithT(t)

	a := &PatchGuardrailsAction{}

	g.Expect(a.ID()).To(Equal("trustyai.patch-guardrails"))
	g.Expect(a.Name()).To(ContainSubstring("Patch"))
	g.Expect(a.Description()).To(ContainSubstring("readinessProbe"))
	g.Expect(a.Group()).To(Equal(action.GroupMigration))
	g.Expect(a.Phase()).To(Equal(action.PhasePostUpgrade))
	g.Expect(a.Prepare()).To(BeNil())
	g.Expect(a.Run()).ToNot(BeNil())
}

func TestPatchGuardrailsAction_AddFlags(t *testing.T) {
	g := NewWithT(t)

	a := &PatchGuardrailsAction{}
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	a.AddFlags(fs)

	flag := fs.Lookup("gorch-name")
	g.Expect(flag).ToNot(BeNil())
	g.Expect(flag.DefValue).To(Equal(""))
}

func TestPatchGuardrailsAction_CanApply(t *testing.T) {
	tests := []struct {
		name     string
		current  string
		target   string
		expected bool
	}{
		{"2.25 to 3.0", "2.25.0", "3.0.0", true},
		{"3.0 to 3.1", "3.0.0", "3.1.0", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			a := &PatchGuardrailsAction{}
			cv := semver.MustParse(tt.current)
			tv := semver.MustParse(tt.target)
			g.Expect(a.CanApply(action.Target{CurrentVersion: &cv, TargetVersion: &tv})).To(Equal(tt.expected))
		})
	}
}

func TestPatchGuardrailsAction_CanApply_NilVersions(t *testing.T) {
	g := NewWithT(t)

	a := &PatchGuardrailsAction{}
	g.Expect(a.CanApply(action.Target{})).To(BeFalse())
}

func TestRunTask_Validate_NoGORCHs(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	target := newTarget(newFakeClient(newScheme()))

	a := &PatchGuardrailsAction{}
	result, err := a.Run().Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Completed).To(BeTrue())
}

func TestRunTask_Validate_ProbeOK(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	gorch := newGORCH("my-gorch", "ns1")
	deploy := newDeployment("my-gorch", "ns1", true)

	target := newTarget(newFakeClient(newScheme(), gorch, deploy))

	a := &PatchGuardrailsAction{}
	result, err := a.Run().Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Completed).To(BeTrue())
}

func TestRunTask_Validate_NeedsPatch(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	gorch := newGORCH("my-gorch", "ns1")
	deploy := newDeployment("my-gorch", "ns1", false)

	target := newTarget(newFakeClient(newScheme(), gorch, deploy))

	a := &PatchGuardrailsAction{}
	result, err := a.Run().Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Completed).To(BeTrue())
}

func TestRunTask_Validate_DeploymentNotFound(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	gorch := newGORCH("my-gorch", "ns1")
	target := newTarget(newFakeClient(newScheme(), gorch))

	a := &PatchGuardrailsAction{}
	result, err := a.Run().Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Completed).To(BeTrue())
}

func TestRunTask_Execute_DeploymentNotFound(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	gorch := newGORCH("my-gorch", "ns1")
	target := newTarget(newFakeClient(newScheme(), gorch))

	a := &PatchGuardrailsAction{}
	result, err := a.Run().Execute(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Completed).To(BeTrue())
}

func TestRunTask_Execute_UserCancels(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	gorch := newGORCH("my-gorch", "ns1")
	deploy := newDeployment("my-gorch", "ns1", false)

	inBuf := bytes.NewBufferString("n\n")
	io := iostreams.NewIOStreams(inBuf, &bytes.Buffer{}, &bytes.Buffer{})

	k8sClient := newFakeClient(newScheme(), gorch, deploy)
	target := newTarget(k8sClient, func(t *action.Target) {
		t.SkipConfirm = false
		t.IO = io
		t.Recorder = action.NewVerboseRootRecorder(io)
	})

	a := &PatchGuardrailsAction{}
	result, err := a.Run().Execute(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Completed).To(BeTrue())
}

func TestRunTask_Execute_NoGORCHs(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	target := newTarget(newFakeClient(newScheme()))

	a := &PatchGuardrailsAction{}
	result, err := a.Run().Execute(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Completed).To(BeTrue())
}

func TestRunTask_Execute_PatchesMissingProbe(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	gorch := newGORCH("my-gorch", "ns1")
	deploy := newDeployment("my-gorch", "ns1", false)

	scheme := newScheme()
	k8sClient := newFakeClient(scheme, gorch, deploy)
	target := newTarget(k8sClient)

	a := &PatchGuardrailsAction{}
	result, err := a.Run().Execute(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Completed).To(BeFalse())

	updated, err := k8sClient.Dynamic().Resource(resources.Deployment.GVR()).
		Namespace("ns1").Get(context.Background(), "my-gorch", metav1.GetOptions{})
	g.Expect(err).ToNot(HaveOccurred())

	containers, _, _ := unstructured.NestedSlice(updated.Object, "spec", "template", "spec", "containers")
	g.Expect(containers).ToNot(BeEmpty())
}

func TestRunTask_Execute_SkipsExistingProbe(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	gorch := newGORCH("my-gorch", "ns1")
	deploy := newDeployment("my-gorch", "ns1", true)

	target := newTarget(newFakeClient(newScheme(), gorch, deploy))

	a := &PatchGuardrailsAction{}
	result, err := a.Run().Execute(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Completed).To(BeTrue())
}

func TestRunTask_Execute_DryRun(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	gorch := newGORCH("my-gorch", "ns1")
	deploy := newDeployment("my-gorch", "ns1", false)

	target := newTarget(newFakeClient(newScheme(), gorch, deploy), withDryRun)

	a := &PatchGuardrailsAction{}
	result, err := a.Run().Execute(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Completed).To(BeTrue())
}

func TestRunTask_Execute_GorchNameFilter(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	gorch1 := newGORCH("gorch-1", "ns1")
	gorch2 := newGORCH("gorch-2", "ns1")
	deploy1 := newDeployment("gorch-1", "ns1", false)
	deploy2 := newDeployment("gorch-2", "ns1", false)

	scheme := newScheme()
	k8sClient := newFakeClient(scheme, gorch1, gorch2, deploy1, deploy2)
	target := newTarget(k8sClient)

	a := &PatchGuardrailsAction{GorchName: "gorch-1"}
	result, err := a.Run().Execute(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Completed).To(BeFalse())
}

func TestRunTask_Execute_MultiNamespace(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	gorch1 := newGORCH("gorch-1", "ns1")
	gorch2 := newGORCH("gorch-2", "ns2")
	deploy1 := newDeployment("gorch-1", "ns1", false)
	deploy2 := newDeployment("gorch-2", "ns2", false)

	scheme := newScheme()
	k8sClient := newFakeClient(scheme, gorch1, gorch2, deploy1, deploy2)
	target := newTarget(k8sClient)

	a := &PatchGuardrailsAction{}
	result, err := a.Run().Execute(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Completed).To(BeFalse())
}

func TestCheckProbe(t *testing.T) {
	tests := []struct {
		name     string
		hasProbe bool
		expected probeStatus
	}{
		{"with probe", true, probeOK},
		{"without probe", false, probeNeedsPatch},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			deploy := newDeployment("test", "ns1", tt.hasProbe)
			g.Expect(checkProbe(deploy, "test")).To(Equal(tt.expected))
		})
	}
}

func TestCheckProbe_NoContainers(t *testing.T) {
	g := NewWithT(t)

	deploy := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.Deployment.APIVersion(),
			"kind":       resources.Deployment.Kind,
			"metadata":   map[string]any{"name": "test", "namespace": "ns1"},
			"spec": map[string]any{
				"template": map[string]any{
					"spec": map[string]any{},
				},
			},
		},
	}

	g.Expect(checkProbe(deploy, "test")).To(Equal(probeNeedsPatch))
}

func TestCheckProbe_WrongContainerName(t *testing.T) {
	g := NewWithT(t)

	deploy := newDeployment("other-container", "ns1", true)
	g.Expect(checkProbe(deploy, "nonexistent")).To(Equal(probeNeedsPatch))
}

func TestToInt64(t *testing.T) {
	g := NewWithT(t)

	g.Expect(toInt64(int64(42))).To(Equal(int64(42)))
	g.Expect(toInt64(float64(42))).To(Equal(int64(42)))
	g.Expect(toInt64("not a number")).To(Equal(int64(0)))
	g.Expect(toInt64(nil)).To(Equal(int64(0)))
}

func TestBuildReadinessProbePatch(t *testing.T) {
	g := NewWithT(t)

	patch, err := buildReadinessProbePatch("my-container")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(string(patch)).To(ContainSubstring("my-container"))
	g.Expect(string(patch)).To(ContainSubstring("/health"))
	g.Expect(string(patch)).To(ContainSubstring("8034"))
}
