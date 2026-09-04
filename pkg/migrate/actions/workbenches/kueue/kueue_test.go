//nolint:testpackage // Tests internal implementation (unexported helpers)
package kueue

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/blang/semver/v4"
	"github.com/spf13/pflag"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	metadatafake "k8s.io/client-go/metadata/fake"
	k8stesting "k8s.io/client-go/testing"

	"github.com/opendatahub-io/odh-cli/pkg/constants"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/action"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/actions/workbenches"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
	"github.com/opendatahub-io/odh-cli/pkg/util/iostreams"
	"github.com/opendatahub-io/odh-cli/pkg/util/kube"

	. "github.com/onsi/gomega"
)

// --- Test Fixtures ---

//nolint:gochecknoglobals // test-only GVR→ListKind mapping for fake dynamic client
var testListKinds = map[schema.GroupVersionResource]string{
	resources.Notebook.GVR():  resources.Notebook.ListKind(),
	resources.Namespace.GVR(): resources.Namespace.ListKind(),
}

func newScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	_ = metav1.AddMetaToScheme(scheme)

	return scheme
}

func newNotebook(name, namespace string, labels map[string]string) *unstructured.Unstructured {
	metadata := map[string]any{
		"name":      name,
		"namespace": namespace,
	}

	if len(labels) > 0 {
		anyLabels := make(map[string]any, len(labels))
		for k, v := range labels {
			anyLabels[k] = v
		}

		metadata["labels"] = anyLabels
	}

	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.Notebook.APIVersion(),
			"kind":       resources.Notebook.Kind,
			"metadata":   metadata,
		},
	}
}

func newNamespace(name string, labels map[string]string) *unstructured.Unstructured {
	metadata := map[string]any{
		"name": name,
	}

	if len(labels) > 0 {
		anyLabels := make(map[string]any, len(labels))
		for k, v := range labels {
			anyLabels[k] = v
		}

		metadata["labels"] = anyLabels
	}

	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.Namespace.APIVersion(),
			"kind":       resources.Namespace.Kind,
			"metadata":   metadata,
		},
	}
}

func kueueManagedLabels() map[string]string {
	return map[string]string{
		constants.LabelKueueOpenshiftManaged: "true",
	}
}

func newFakeClient(objects []*unstructured.Unstructured, reactors ...k8stesting.Reactor) client.Client {
	scheme := newScheme()

	dynamicObjs := make([]runtime.Object, len(objects))
	for i, obj := range objects {
		dynamicObjs[i] = obj
	}

	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		scheme,
		testListKinds,
		dynamicObjs...,
	)

	for _, r := range reactors {
		dynamicClient.ReactionChain = append([]k8stesting.Reactor{r}, dynamicClient.ReactionChain...)
	}

	metadataClient := metadatafake.NewSimpleMetadataClient(
		scheme,
		kube.ToPartialObjectMetadata(objects...)...,
	)

	return client.NewForTesting(client.TestClientConfig{
		Dynamic:  dynamicClient,
		Metadata: metadataClient,
	})
}

func patchErrorReactor() k8stesting.Reactor {
	return &k8stesting.SimpleReactor{
		Verb:     "patch",
		Resource: "notebooks",
		Reaction: func(_ k8stesting.Action) (bool, runtime.Object, error) {
			return true, nil, errors.New("simulated patch error")
		},
	}
}

func listErrorReactor() k8stesting.Reactor {
	return &k8stesting.SimpleReactor{
		Verb:     "list",
		Resource: "notebooks",
		Reaction: func(_ k8stesting.Action) (bool, runtime.Object, error) {
			return true, nil, errors.New("simulated API server error")
		},
	}
}

func newTarget(k8sClient client.Client, opts ...func(*action.Target)) action.Target {
	targetVersion := semver.MustParse("3.0.0")

	io := iostreams.NewIOStreams(
		&bytes.Buffer{},
		&bytes.Buffer{},
		&bytes.Buffer{},
	)

	target := action.Target{
		Client:        k8sClient,
		TargetVersion: &targetVersion,
		DryRun:        false,
		SkipConfirm:   true,
		Recorder:      action.NewVerboseRootRecorder(io),
		IO:            io,
	}

	for _, opt := range opts {
		opt(&target)
	}

	return target
}

func withDryRun(t *action.Target) {
	t.DryRun = true
}

// --- Action Metadata Tests ---

func TestAttachKueueLabelAction_Metadata(t *testing.T) {
	g := NewWithT(t)

	a := &AttachKueueLabelAction{Scope: &workbenches.SharedScopeOptions{}}

	g.Expect(a.ID()).To(Equal("workbenches.attach-kueue-label"))
	g.Expect(a.Name()).To(Equal("Attach Kueue queue-name label to workbenches"))
	g.Expect(a.Description()).To(ContainSubstring("queue-name"))
	g.Expect(a.Group()).To(Equal(action.GroupMigration))
	g.Expect(a.Phase()).To(Equal(action.PhasePostUpgrade))
}

func TestAttachKueueLabelAction_CanApply(t *testing.T) {
	v216 := semver.MustParse("2.16.0")
	v3 := semver.MustParse("3.0.0")
	v35 := semver.MustParse("3.5.0")

	tests := []struct {
		name          string
		targetVersion *semver.Version
		expected      bool
	}{
		{"does not apply for target 2.16", &v216, false},
		{"applies for target 3.0", &v3, true},
		{"applies for target 3.5", &v35, true},
		{"does not apply for nil target version", nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			a := &AttachKueueLabelAction{Scope: &workbenches.SharedScopeOptions{}}
			target := action.Target{
				TargetVersion: tt.targetVersion,
			}

			g.Expect(a.CanApply(target)).To(Equal(tt.expected))
		})
	}
}

func TestAttachKueueLabelAction_PrepareIsNil(t *testing.T) {
	g := NewWithT(t)

	a := &AttachKueueLabelAction{Scope: &workbenches.SharedScopeOptions{}}
	g.Expect(a.Prepare()).To(BeNil())
}

func TestAttachKueueLabelAction_RunNotNil(t *testing.T) {
	g := NewWithT(t)

	a := &AttachKueueLabelAction{Scope: &workbenches.SharedScopeOptions{}}
	g.Expect(a.Run()).ToNot(BeNil())
}

func TestAttachKueueLabelAction_AddFlags(t *testing.T) {
	g := NewWithT(t)

	a := &AttachKueueLabelAction{Scope: &workbenches.SharedScopeOptions{}}
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	a.AddFlags(fs)

	queueFlag := fs.Lookup("queue-name")
	g.Expect(queueFlag).ToNot(BeNil())
	g.Expect(queueFlag.DefValue).To(Equal("default"))

	nsFlag := fs.Lookup("workbench-namespace")
	g.Expect(nsFlag).ToNot(BeNil())
	g.Expect(nsFlag.DefValue).To(Equal(""))

	nameFlag := fs.Lookup("workbench-name")
	g.Expect(nameFlag).ToNot(BeNil())
	g.Expect(nameFlag.DefValue).To(Equal(""))
}

// --- Helper Function Tests ---

func TestHasQueueNameLabel(t *testing.T) {
	tests := []struct {
		name     string
		nb       *unstructured.Unstructured
		expected bool
	}{
		{
			"has queue-name label",
			newNotebook("nb1", "ns1", map[string]string{
				constants.LabelKueueQueueName: "default",
			}),
			true,
		},
		{
			"no labels",
			newNotebook("nb1", "ns1", nil),
			false,
		},
		{
			"has other labels but not queue-name",
			newNotebook("nb1", "ns1", map[string]string{
				"app": "jupyter",
			}),
			false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			g.Expect(hasQueueNameLabel(tt.nb)).To(Equal(tt.expected))
		})
	}
}

func TestQueueName_DefaultAndCustom(t *testing.T) {
	g := NewWithT(t)

	a := &AttachKueueLabelAction{Scope: &workbenches.SharedScopeOptions{}}
	g.Expect(a.queueName()).To(Equal("default"))

	a.QueueName = "custom-queue"
	g.Expect(a.queueName()).To(Equal("custom-queue"))

	a.QueueName = ""
	g.Expect(a.queueName()).To(Equal("default"))
}

func TestRunTask_Validate_InvalidQueueName(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	k8sClient := newFakeClient([]*unstructured.Unstructured{
		newNamespace("ns1", kueueManagedLabels()),
	})
	target := newTarget(k8sClient)

	tests := []struct {
		name      string
		queueName string
	}{
		{"leading dash", "-invalid"},
		{"contains spaces", "my queue"},
		{"too long", strings.Repeat("a", 64)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			a := &AttachKueueLabelAction{QueueName: tt.queueName, Scope: &workbenches.SharedScopeOptions{}}
			runTask := a.Run()

			_, err := runTask.Validate(ctx, target)
			g.Expect(err).To(HaveOccurred())
			g.Expect(err.Error()).To(ContainSubstring("invalid --queue-name"))
		})
	}

	a := &AttachKueueLabelAction{QueueName: "my queue", Scope: &workbenches.SharedScopeOptions{}}
	runTask := a.Run()

	_, err := runTask.Execute(ctx, target)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("invalid --queue-name"))
}

// --- Validate Tests ---

func TestRunTask_Validate_NoNotebooks(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	k8sClient := newFakeClient([]*unstructured.Unstructured{
		newNamespace("ns1", kueueManagedLabels()),
	})
	target := newTarget(k8sClient)

	a := &AttachKueueLabelAction{Scope: &workbenches.SharedScopeOptions{}}
	runTask := a.Run()

	result, err := runTask.Validate(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).ToNot(BeNil())
	g.Expect(result.Status.Completed).To(BeTrue())
}

func TestRunTask_Validate_NoKueueNamespaces(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	k8sClient := newFakeClient([]*unstructured.Unstructured{
		newNamespace("ns1", nil),
		newNotebook("nb1", "ns1", nil),
	})
	target := newTarget(k8sClient)

	a := &AttachKueueLabelAction{Scope: &workbenches.SharedScopeOptions{}}
	runTask := a.Run()

	result, err := runTask.Validate(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).ToNot(BeNil())
	g.Expect(result.Status.Completed).To(BeTrue())
}

func TestRunTask_Validate_NotebooksMissingLabel(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	k8sClient := newFakeClient([]*unstructured.Unstructured{
		newNamespace("ns1", kueueManagedLabels()),
		newNotebook("nb1", "ns1", nil),
		newNotebook("nb2", "ns1", nil),
	})
	target := newTarget(k8sClient)

	a := &AttachKueueLabelAction{Scope: &workbenches.SharedScopeOptions{}}
	runTask := a.Run()

	result, err := runTask.Validate(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).ToNot(BeNil())
	g.Expect(result.Status.Completed).To(BeTrue())
}

func TestRunTask_Validate_AllNotebooksAlreadyLabeled(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	k8sClient := newFakeClient([]*unstructured.Unstructured{
		newNamespace("ns1", kueueManagedLabels()),
		newNotebook("nb1", "ns1", map[string]string{
			constants.LabelKueueQueueName: "default",
		}),
	})
	target := newTarget(k8sClient)

	a := &AttachKueueLabelAction{Scope: &workbenches.SharedScopeOptions{}}
	runTask := a.Run()

	result, err := runTask.Validate(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).ToNot(BeNil())
	g.Expect(result.Status.Completed).To(BeTrue())
}

func TestRunTask_Validate_ListError(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	k8sClient := newFakeClient([]*unstructured.Unstructured{
		newNamespace("ns1", kueueManagedLabels()),
	}, listErrorReactor())
	target := newTarget(k8sClient)

	a := &AttachKueueLabelAction{Scope: &workbenches.SharedScopeOptions{}}
	runTask := a.Run()

	result, err := runTask.Validate(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).ToNot(BeNil())

	hasFailedStep := false

	for _, step := range result.Status.Steps {
		if step.Status == "Failed" {
			hasFailedStep = true
		}
	}

	g.Expect(hasFailedStep).To(BeTrue())
}

// --- Execute Tests ---

func TestRunTask_Execute_NoNotebooks(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	k8sClient := newFakeClient([]*unstructured.Unstructured{
		newNamespace("ns1", kueueManagedLabels()),
	})
	target := newTarget(k8sClient)

	a := &AttachKueueLabelAction{Scope: &workbenches.SharedScopeOptions{}}
	runTask := a.Run()

	result, err := runTask.Execute(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).ToNot(BeNil())
	g.Expect(result.Status.Completed).To(BeTrue())
}

func TestRunTask_Execute_LabelApplied(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	k8sClient := newFakeClient([]*unstructured.Unstructured{
		newNamespace("ns1", kueueManagedLabels()),
		newNotebook("nb1", "ns1", nil),
	})
	target := newTarget(k8sClient)

	a := &AttachKueueLabelAction{Scope: &workbenches.SharedScopeOptions{}}
	runTask := a.Run()

	result, err := runTask.Execute(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).ToNot(BeNil())
	g.Expect(result.Status.Completed).To(BeTrue())

	updated, err := k8sClient.Dynamic().Resource(resources.Notebook.GVR()).
		Namespace("ns1").Get(context.Background(), "nb1", metav1.GetOptions{})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(updated.GetLabels()).To(HaveKeyWithValue(constants.LabelKueueQueueName, "default"))
}

func TestRunTask_Execute_CustomQueueName(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	k8sClient := newFakeClient([]*unstructured.Unstructured{
		newNamespace("ns1", kueueManagedLabels()),
		newNotebook("nb1", "ns1", nil),
	})
	target := newTarget(k8sClient)

	a := &AttachKueueLabelAction{QueueName: "my-queue", Scope: &workbenches.SharedScopeOptions{}}
	runTask := a.Run()

	result, err := runTask.Execute(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).ToNot(BeNil())
	g.Expect(result.Status.Completed).To(BeTrue())

	updated, err := k8sClient.Dynamic().Resource(resources.Notebook.GVR()).
		Namespace("ns1").Get(context.Background(), "nb1", metav1.GetOptions{})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(updated.GetLabels()).To(HaveKeyWithValue(constants.LabelKueueQueueName, "my-queue"))
}

func TestRunTask_Execute_SkipsAlreadyLabeledNotebook(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	k8sClient := newFakeClient([]*unstructured.Unstructured{
		newNamespace("ns1", kueueManagedLabels()),
		newNotebook("labeled-nb", "ns1", map[string]string{
			constants.LabelKueueQueueName: "existing-queue",
		}),
		newNotebook("unlabeled-nb", "ns1", nil),
	})
	target := newTarget(k8sClient)

	a := &AttachKueueLabelAction{Scope: &workbenches.SharedScopeOptions{}}
	runTask := a.Run()

	result, err := runTask.Execute(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).ToNot(BeNil())
	g.Expect(result.Status.Completed).To(BeTrue())

	labeled, err := k8sClient.Dynamic().Resource(resources.Notebook.GVR()).
		Namespace("ns1").Get(context.Background(), "labeled-nb", metav1.GetOptions{})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(labeled.GetLabels()).To(HaveKeyWithValue(constants.LabelKueueQueueName, "existing-queue"))

	unlabeled, err := k8sClient.Dynamic().Resource(resources.Notebook.GVR()).
		Namespace("ns1").Get(context.Background(), "unlabeled-nb", metav1.GetOptions{})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(unlabeled.GetLabels()).To(HaveKeyWithValue(constants.LabelKueueQueueName, "default"))
}

func TestRunTask_Execute_SkipsNonKueueNamespace(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	k8sClient := newFakeClient([]*unstructured.Unstructured{
		newNamespace("kueue-ns", kueueManagedLabels()),
		newNamespace("regular-ns", nil),
		newNotebook("nb-kueue", "kueue-ns", nil),
		newNotebook("nb-regular", "regular-ns", nil),
	})
	target := newTarget(k8sClient)

	a := &AttachKueueLabelAction{Scope: &workbenches.SharedScopeOptions{}}
	runTask := a.Run()

	result, err := runTask.Execute(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).ToNot(BeNil())
	g.Expect(result.Status.Completed).To(BeTrue())

	kueueNb, err := k8sClient.Dynamic().Resource(resources.Notebook.GVR()).
		Namespace("kueue-ns").Get(context.Background(), "nb-kueue", metav1.GetOptions{})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(kueueNb.GetLabels()).To(HaveKeyWithValue(constants.LabelKueueQueueName, "default"))

	regularNb, err := k8sClient.Dynamic().Resource(resources.Notebook.GVR()).
		Namespace("regular-ns").Get(context.Background(), "nb-regular", metav1.GetOptions{})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(regularNb.GetLabels()).ToNot(HaveKey(constants.LabelKueueQueueName))
}

func TestRunTask_Execute_MultipleNamespaces(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	k8sClient := newFakeClient([]*unstructured.Unstructured{
		newNamespace("ns-a", kueueManagedLabels()),
		newNamespace("ns-b", kueueManagedLabels()),
		newNotebook("nb1", "ns-a", nil),
		newNotebook("nb2", "ns-b", nil),
	})
	target := newTarget(k8sClient)

	a := &AttachKueueLabelAction{Scope: &workbenches.SharedScopeOptions{}}
	runTask := a.Run()

	result, err := runTask.Execute(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).ToNot(BeNil())
	g.Expect(result.Status.Completed).To(BeTrue())

	nb1, err := k8sClient.Dynamic().Resource(resources.Notebook.GVR()).
		Namespace("ns-a").Get(context.Background(), "nb1", metav1.GetOptions{})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(nb1.GetLabels()).To(HaveKeyWithValue(constants.LabelKueueQueueName, "default"))

	nb2, err := k8sClient.Dynamic().Resource(resources.Notebook.GVR()).
		Namespace("ns-b").Get(context.Background(), "nb2", metav1.GetOptions{})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(nb2.GetLabels()).To(HaveKeyWithValue(constants.LabelKueueQueueName, "default"))
}

func TestRunTask_Execute_NoKueueNamespaces(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	k8sClient := newFakeClient([]*unstructured.Unstructured{
		newNamespace("ns1", nil),
		newNotebook("nb1", "ns1", nil),
	})
	target := newTarget(k8sClient)

	a := &AttachKueueLabelAction{Scope: &workbenches.SharedScopeOptions{}}
	runTask := a.Run()

	result, err := runTask.Execute(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).ToNot(BeNil())
	g.Expect(result.Status.Completed).To(BeTrue())

	nb, err := k8sClient.Dynamic().Resource(resources.Notebook.GVR()).
		Namespace("ns1").Get(context.Background(), "nb1", metav1.GetOptions{})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(nb.GetLabels()).ToNot(HaveKey(constants.LabelKueueQueueName))
}

// --- Dry-Run Tests ---

func TestRunTask_Execute_DryRun_NoChanges(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	k8sClient := newFakeClient([]*unstructured.Unstructured{
		newNamespace("ns1", kueueManagedLabels()),
		newNotebook("nb1", "ns1", nil),
	})
	target := newTarget(k8sClient, withDryRun)

	a := &AttachKueueLabelAction{Scope: &workbenches.SharedScopeOptions{}}
	runTask := a.Run()

	result, err := runTask.Execute(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).ToNot(BeNil())
	g.Expect(result.Status.Completed).To(BeTrue())

	nb, err := k8sClient.Dynamic().Resource(resources.Notebook.GVR()).
		Namespace("ns1").Get(context.Background(), "nb1", metav1.GetOptions{})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(nb.GetLabels()).ToNot(HaveKey(constants.LabelKueueQueueName))
}

func TestRunTask_Execute_DryRun_MultipleNotebooks(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	k8sClient := newFakeClient([]*unstructured.Unstructured{
		newNamespace("ns1", kueueManagedLabels()),
		newNotebook("nb1", "ns1", nil),
		newNotebook("nb2", "ns1", nil),
	})
	target := newTarget(k8sClient, withDryRun)

	a := &AttachKueueLabelAction{Scope: &workbenches.SharedScopeOptions{}}
	runTask := a.Run()

	result, err := runTask.Execute(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).ToNot(BeNil())
	g.Expect(result.Status.Completed).To(BeTrue())

	for _, name := range []string{"nb1", "nb2"} {
		nb, err := k8sClient.Dynamic().Resource(resources.Notebook.GVR()).
			Namespace("ns1").Get(context.Background(), name, metav1.GetOptions{})
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(nb.GetLabels()).ToNot(HaveKey(constants.LabelKueueQueueName))
	}
}

// --- Error Path Tests ---

func TestRunTask_Execute_PatchError(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	k8sClient := newFakeClient([]*unstructured.Unstructured{
		newNamespace("ns1", kueueManagedLabels()),
		newNotebook("nb1", "ns1", nil),
	}, patchErrorReactor())
	target := newTarget(k8sClient)

	a := &AttachKueueLabelAction{Scope: &workbenches.SharedScopeOptions{}}
	runTask := a.Run()

	result, err := runTask.Execute(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).ToNot(BeNil())
	g.Expect(result.Status.Completed).To(BeFalse())
}

func TestRunTask_Execute_PatchError_ContinuesWithRemaining(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	scheme := newScheme()
	objects := []*unstructured.Unstructured{
		newNamespace("ns1", kueueManagedLabels()),
		newNotebook("nb-fail", "ns1", nil),
		newNotebook("nb-ok", "ns1", nil),
	}

	dynamicObjs := make([]runtime.Object, len(objects))
	for i, obj := range objects {
		dynamicObjs[i] = obj
	}

	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		scheme, testListKinds, dynamicObjs...,
	)

	dynamicClient.PrependReactor("patch", "notebooks", func(a k8stesting.Action) (bool, runtime.Object, error) {
		patchAction := a.(k8stesting.PatchAction)
		if patchAction.GetName() == "nb-fail" {
			return true, nil, errors.New("simulated patch error")
		}

		return false, nil, nil
	})

	metadataClient := metadatafake.NewSimpleMetadataClient(
		scheme,
		kube.ToPartialObjectMetadata(objects...)...,
	)

	k8sClient := client.NewForTesting(client.TestClientConfig{
		Dynamic:  dynamicClient,
		Metadata: metadataClient,
	})

	target := newTarget(k8sClient)

	act := &AttachKueueLabelAction{Scope: &workbenches.SharedScopeOptions{}}
	runTask := act.Run()

	result, err := runTask.Execute(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).ToNot(BeNil())
	g.Expect(result.Status.Completed).To(BeFalse())

	nbOk, err := k8sClient.Dynamic().Resource(resources.Notebook.GVR()).
		Namespace("ns1").Get(context.Background(), "nb-ok", metav1.GetOptions{})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(nbOk.GetLabels()).To(HaveKeyWithValue(constants.LabelKueueQueueName, "default"))
}

// --- Step Recording Tests ---

func TestRunTask_Execute_StepRecording(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	k8sClient := newFakeClient([]*unstructured.Unstructured{
		newNamespace("ns1", kueueManagedLabels()),
		newNotebook("nb1", "ns1", nil),
	})
	target := newTarget(k8sClient)

	a := &AttachKueueLabelAction{Scope: &workbenches.SharedScopeOptions{}}
	runTask := a.Run()

	actionResult, err := runTask.Execute(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(actionResult.Status.Steps).ToNot(BeEmpty())

	hasLabelStep := false

	for _, step := range actionResult.Status.Steps {
		if step.Name == "apply-kueue-labels" {
			hasLabelStep = true
		}
	}

	g.Expect(hasLabelStep).To(BeTrue())
}

func TestRunTask_Validate_StepRecording(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	k8sClient := newFakeClient([]*unstructured.Unstructured{
		newNamespace("ns1", kueueManagedLabels()),
		newNotebook("nb1", "ns1", nil),
	})
	target := newTarget(k8sClient)

	a := &AttachKueueLabelAction{Scope: &workbenches.SharedScopeOptions{}}
	runTask := a.Run()

	actionResult, err := runTask.Validate(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(actionResult.Status.Steps).ToNot(BeEmpty())

	hasValidateStep := false

	for _, step := range actionResult.Status.Steps {
		if step.Name == "validate-kueue-labels" {
			hasValidateStep = true
		}
	}

	g.Expect(hasValidateStep).To(BeTrue())
}

// --- Targeting Mode Tests ---

func TestRunTask_Validate_NameRequiresNamespace(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	k8sClient := newFakeClient([]*unstructured.Unstructured{
		newNamespace("ns1", kueueManagedLabels()),
		newNotebook("nb1", "ns1", nil),
	})
	target := newTarget(k8sClient)

	a := &AttachKueueLabelAction{Scope: &workbenches.SharedScopeOptions{WorkbenchName: "nb1"}}
	runTask := a.Run()

	_, err := runTask.Validate(ctx, target)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("--workbench-name requires --workbench-namespace"))
}

func TestRunTask_Execute_NameRequiresNamespace(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	k8sClient := newFakeClient([]*unstructured.Unstructured{
		newNamespace("ns1", kueueManagedLabels()),
		newNotebook("nb1", "ns1", nil),
	})
	target := newTarget(k8sClient)

	a := &AttachKueueLabelAction{Scope: &workbenches.SharedScopeOptions{WorkbenchName: "nb1"}}
	runTask := a.Run()

	_, err := runTask.Execute(ctx, target)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("--workbench-name requires --workbench-namespace"))
}

func TestRunTask_Execute_NamespaceScopedTargeting(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	k8sClient := newFakeClient([]*unstructured.Unstructured{
		newNamespace("ns-a", kueueManagedLabels()),
		newNamespace("ns-b", kueueManagedLabels()),
		newNotebook("nb1", "ns-a", nil),
		newNotebook("nb2", "ns-b", nil),
	})
	target := newTarget(k8sClient)

	a := &AttachKueueLabelAction{Scope: &workbenches.SharedScopeOptions{WorkbenchNamespace: "ns-a"}}
	runTask := a.Run()

	result, err := runTask.Execute(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).ToNot(BeNil())
	g.Expect(result.Status.Completed).To(BeTrue())

	nb1, err := k8sClient.Dynamic().Resource(resources.Notebook.GVR()).
		Namespace("ns-a").Get(context.Background(), "nb1", metav1.GetOptions{})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(nb1.GetLabels()).To(HaveKeyWithValue(constants.LabelKueueQueueName, "default"))

	nb2, err := k8sClient.Dynamic().Resource(resources.Notebook.GVR()).
		Namespace("ns-b").Get(context.Background(), "nb2", metav1.GetOptions{})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(nb2.GetLabels()).ToNot(HaveKey(constants.LabelKueueQueueName))
}

func TestRunTask_Execute_NamespaceScopedTargeting_NonKueueNamespace(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	k8sClient := newFakeClient([]*unstructured.Unstructured{
		newNamespace("regular-ns", nil),
		newNotebook("nb1", "regular-ns", nil),
	})
	target := newTarget(k8sClient)

	a := &AttachKueueLabelAction{Scope: &workbenches.SharedScopeOptions{WorkbenchNamespace: "regular-ns"}}
	runTask := a.Run()

	result, err := runTask.Execute(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).ToNot(BeNil())
	g.Expect(result.Status.Completed).To(BeTrue())

	nb1, err := k8sClient.Dynamic().Resource(resources.Notebook.GVR()).
		Namespace("regular-ns").Get(context.Background(), "nb1", metav1.GetOptions{})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(nb1.GetLabels()).ToNot(HaveKey(constants.LabelKueueQueueName))
}

func TestRunTask_Execute_SingleNotebookTargeting(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	k8sClient := newFakeClient([]*unstructured.Unstructured{
		newNamespace("ns1", kueueManagedLabels()),
		newNotebook("target-nb", "ns1", nil),
		newNotebook("other-nb", "ns1", nil),
	})
	target := newTarget(k8sClient)

	a := &AttachKueueLabelAction{Scope: &workbenches.SharedScopeOptions{
		WorkbenchNamespace: "ns1",
		WorkbenchName:      "target-nb",
	}}
	runTask := a.Run()

	result, err := runTask.Execute(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).ToNot(BeNil())
	g.Expect(result.Status.Completed).To(BeTrue())

	targetNb, err := k8sClient.Dynamic().Resource(resources.Notebook.GVR()).
		Namespace("ns1").Get(context.Background(), "target-nb", metav1.GetOptions{})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(targetNb.GetLabels()).To(HaveKeyWithValue(constants.LabelKueueQueueName, "default"))

	otherNb, err := k8sClient.Dynamic().Resource(resources.Notebook.GVR()).
		Namespace("ns1").Get(context.Background(), "other-nb", metav1.GetOptions{})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(otherNb.GetLabels()).ToNot(HaveKey(constants.LabelKueueQueueName))
}

func TestRunTask_Execute_SingleNotebookTargeting_NotFound(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	k8sClient := newFakeClient([]*unstructured.Unstructured{
		newNamespace("ns1", kueueManagedLabels()),
		newNotebook("other-nb", "ns1", nil),
	})
	target := newTarget(k8sClient)

	a := &AttachKueueLabelAction{Scope: &workbenches.SharedScopeOptions{
		WorkbenchNamespace: "ns1",
		WorkbenchName:      "nonexistent-nb",
	}}
	runTask := a.Run()

	result, err := runTask.Execute(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).ToNot(BeNil())

	hasFailedStep := false

	for _, step := range result.Status.Steps {
		if step.Status == "Failed" {
			hasFailedStep = true
		}
	}

	g.Expect(hasFailedStep).To(BeTrue())

	otherNb, err := k8sClient.Dynamic().Resource(resources.Notebook.GVR()).
		Namespace("ns1").Get(context.Background(), "other-nb", metav1.GetOptions{})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(otherNb.GetLabels()).ToNot(HaveKey(constants.LabelKueueQueueName))
}

func TestRunTask_Execute_SingleNotebookTargeting_NonKueueNamespace(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	k8sClient := newFakeClient([]*unstructured.Unstructured{
		newNamespace("regular-ns", nil),
		newNotebook("nb1", "regular-ns", nil),
	})
	target := newTarget(k8sClient)

	a := &AttachKueueLabelAction{Scope: &workbenches.SharedScopeOptions{
		WorkbenchNamespace: "regular-ns",
		WorkbenchName:      "nb1",
	}}
	runTask := a.Run()

	result, err := runTask.Execute(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).ToNot(BeNil())
	g.Expect(result.Status.Completed).To(BeTrue())

	nb1, err := k8sClient.Dynamic().Resource(resources.Notebook.GVR()).
		Namespace("regular-ns").Get(context.Background(), "nb1", metav1.GetOptions{})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(nb1.GetLabels()).ToNot(HaveKey(constants.LabelKueueQueueName))
}

// --- Mixed Scenario Tests ---

func TestRunTask_Execute_MixedManagedAndNonManaged(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	k8sClient := newFakeClient([]*unstructured.Unstructured{
		newNamespace("managed-ns", kueueManagedLabels()),
		newNamespace("regular-ns", nil),
		newNotebook("nb-managed-unlabeled", "managed-ns", nil),
		newNotebook("nb-managed-labeled", "managed-ns", map[string]string{
			constants.LabelKueueQueueName: "existing",
		}),
		newNotebook("nb-regular", "regular-ns", nil),
	})
	target := newTarget(k8sClient)

	a := &AttachKueueLabelAction{Scope: &workbenches.SharedScopeOptions{}}
	runTask := a.Run()

	result, err := runTask.Execute(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).ToNot(BeNil())
	g.Expect(result.Status.Completed).To(BeTrue())

	unlabeled, err := k8sClient.Dynamic().Resource(resources.Notebook.GVR()).
		Namespace("managed-ns").Get(context.Background(), "nb-managed-unlabeled", metav1.GetOptions{})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(unlabeled.GetLabels()).To(HaveKeyWithValue(constants.LabelKueueQueueName, "default"))

	labeled, err := k8sClient.Dynamic().Resource(resources.Notebook.GVR()).
		Namespace("managed-ns").Get(context.Background(), "nb-managed-labeled", metav1.GetOptions{})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(labeled.GetLabels()).To(HaveKeyWithValue(constants.LabelKueueQueueName, "existing"))

	regular, err := k8sClient.Dynamic().Resource(resources.Notebook.GVR()).
		Namespace("regular-ns").Get(context.Background(), "nb-regular", metav1.GetOptions{})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(regular.GetLabels()).ToNot(HaveKey(constants.LabelKueueQueueName))
}

func TestRunTask_Execute_ListError(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	k8sClient := newFakeClient([]*unstructured.Unstructured{
		newNamespace("ns1", kueueManagedLabels()),
	}, listErrorReactor())
	target := newTarget(k8sClient)

	a := &AttachKueueLabelAction{Scope: &workbenches.SharedScopeOptions{}}
	runTask := a.Run()

	result, err := runTask.Execute(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).ToNot(BeNil())

	hasFailedStep := false

	for _, step := range result.Status.Steps {
		if step.Status == "Failed" {
			hasFailedStep = true
		}
	}

	g.Expect(hasFailedStep).To(BeTrue())
}

// --- Confirmation Tests ---

func TestRunTask_Execute_ConfirmationCancelled(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	k8sClient := newFakeClient([]*unstructured.Unstructured{
		newNamespace("ns1", kueueManagedLabels()),
		newNotebook("nb1", "ns1", nil),
	})

	stdinBuf := bytes.NewBufferString("n\n")

	io := iostreams.NewIOStreams(
		stdinBuf,
		&bytes.Buffer{},
		&bytes.Buffer{},
	)

	targetVersion := semver.MustParse("3.0.0")

	target := action.Target{
		Client:        k8sClient,
		TargetVersion: &targetVersion,
		DryRun:        false,
		SkipConfirm:   false,
		Recorder:      action.NewVerboseRootRecorder(io),
		IO:            io,
	}

	a := &AttachKueueLabelAction{Scope: &workbenches.SharedScopeOptions{}}
	runTask := a.Run()

	result, err := runTask.Execute(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).ToNot(BeNil())
	g.Expect(result.Status.Completed).To(BeTrue())

	nb, err := k8sClient.Dynamic().Resource(resources.Notebook.GVR()).
		Namespace("ns1").Get(context.Background(), "nb1", metav1.GetOptions{})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(nb.GetLabels()).ToNot(HaveKey(constants.LabelKueueQueueName))
}

func TestRunTask_Execute_ConfirmationAccepted(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	k8sClient := newFakeClient([]*unstructured.Unstructured{
		newNamespace("ns1", kueueManagedLabels()),
		newNotebook("nb1", "ns1", nil),
	})

	stdinBuf := bytes.NewBufferString("y\n")

	io := iostreams.NewIOStreams(
		stdinBuf,
		&bytes.Buffer{},
		&bytes.Buffer{},
	)

	targetVersion := semver.MustParse("3.0.0")

	target := action.Target{
		Client:        k8sClient,
		TargetVersion: &targetVersion,
		DryRun:        false,
		SkipConfirm:   false,
		Recorder:      action.NewVerboseRootRecorder(io),
		IO:            io,
	}

	a := &AttachKueueLabelAction{Scope: &workbenches.SharedScopeOptions{}}
	runTask := a.Run()

	result, err := runTask.Execute(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).ToNot(BeNil())
	g.Expect(result.Status.Completed).To(BeTrue())

	nb, err := k8sClient.Dynamic().Resource(resources.Notebook.GVR()).
		Namespace("ns1").Get(context.Background(), "nb1", metav1.GetOptions{})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(nb.GetLabels()).To(HaveKeyWithValue(constants.LabelKueueQueueName, "default"))
}
