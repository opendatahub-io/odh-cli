//nolint:testpackage // Tests internal implementation (classification, checks, run task)
package verify

import (
	"bytes"
	"errors"
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

	"github.com/opendatahub-io/odh-cli/pkg/migrate/action"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/action/result"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/actions/workbenches"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
	"github.com/opendatahub-io/odh-cli/pkg/util/iostreams"
	"github.com/opendatahub-io/odh-cli/pkg/util/kube"

	. "github.com/onsi/gomega"
)

// --- Test Fixtures ---

//nolint:gochecknoglobals // test-only GVR→ListKind mapping
var testListKinds = map[schema.GroupVersionResource]string{
	resources.Notebook.GVR():    resources.Notebook.ListKind(),
	resources.Route.GVR():       resources.Route.ListKind(),
	resources.Service.GVR():     resources.Service.ListKind(),
	resources.Secret.GVR():      resources.Secret.ListKind(),
	resources.OAuthClient.GVR(): resources.OAuthClient.ListKind(),
	resources.Pod.GVR():         resources.Pod.ListKind(),
}

func newScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	_ = metav1.AddMetaToScheme(scheme)

	return scheme
}

type notebookOption func(map[string]any)

func withAnnotations(annotations map[string]string) notebookOption {
	return func(obj map[string]any) {
		metadata := obj["metadata"].(map[string]any)
		anyAnnotations := make(map[string]any, len(annotations))

		for k, v := range annotations {
			anyAnnotations[k] = v
		}

		metadata["annotations"] = anyAnnotations
	}
}

func withContainers(containers ...map[string]any) notebookOption {
	return func(obj map[string]any) {
		obj["spec"] = map[string]any{
			"template": map[string]any{
				"spec": map[string]any{
					"containers": toAnySlice(containers),
				},
			},
		}
	}
}

func toAnySlice(items []map[string]any) []any {
	result := make([]any, len(items))
	for i, item := range items {
		result[i] = item
	}

	return result
}

func container(name string) map[string]any {
	return map[string]any{
		"name":  name,
		"image": "registry.example.com/" + name + ":latest",
	}
}

func newNotebook(name, namespace string, opts ...notebookOption) *unstructured.Unstructured {
	obj := map[string]any{
		"apiVersion": resources.Notebook.APIVersion(),
		"kind":       resources.Notebook.Kind,
		"metadata": map[string]any{
			"name":      name,
			"namespace": namespace,
		},
	}

	for _, opt := range opts {
		opt(obj)
	}

	return &unstructured.Unstructured{Object: obj}
}

func newMigratedNotebook(name, namespace string) *unstructured.Unstructured {
	return newNotebook(name, namespace,
		withAnnotations(map[string]string{
			workbenches.AnnotationInjectAuth: "true",
		}),
		withContainers(
			container("my-notebook"),
			container(workbenches.ContainerKubeRBACProxy),
		))
}

func newLegacyNotebook(name, namespace string) *unstructured.Unstructured {
	return newNotebook(name, namespace,
		withAnnotations(map[string]string{
			workbenches.AnnotationInjectOAuth: "true",
		}),
		withContainers(
			container("my-notebook"),
			container(workbenches.ContainerOAuthProxy),
		))
}

func newStoppedNotebook(name, namespace string, opts ...notebookOption) *unstructured.Unstructured {
	allOpts := make([]notebookOption, 0, 2+len(opts))
	allOpts = append(allOpts,
		withAnnotations(map[string]string{
			workbenches.AnnotationInjectAuth: "true",
			"kubeflow-resource-stopped":      "2024-01-01T00:00:00Z",
		}),
		withContainers(
			container("my-notebook"),
			container(workbenches.ContainerKubeRBACProxy),
		),
	)
	allOpts = append(allOpts, opts...)

	return newNotebook(name, namespace, allOpts...)
}

func newPod(name, namespace, phase string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.Pod.APIVersion(),
			"kind":       resources.Pod.Kind,
			"metadata": map[string]any{
				"name":      name,
				"namespace": namespace,
			},
			"status": map[string]any{
				"phase": phase,
			},
		},
	}
}

func newRoute(name, namespace string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.Route.APIVersion(),
			"kind":       resources.Route.Kind,
			"metadata": map[string]any{
				"name":      name,
				"namespace": namespace,
			},
		},
	}
}

func newService(name, namespace string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.Service.APIVersion(),
			"kind":       resources.Service.Kind,
			"metadata": map[string]any{
				"name":      name,
				"namespace": namespace,
			},
		},
	}
}

func newSecret(name, namespace string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.Secret.APIVersion(),
			"kind":       resources.Secret.Kind,
			"metadata": map[string]any{
				"name":      name,
				"namespace": namespace,
			},
		},
	}
}

func newOAuthClient(name string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.OAuthClient.APIVersion(),
			"kind":       resources.OAuthClient.Kind,
			"metadata": map[string]any{
				"name": name,
			},
		},
	}
}

func allOAuthResources(nbName, namespace string) []*unstructured.Unstructured {
	return []*unstructured.Unstructured{
		newRoute(nbName, namespace),
		newService(nbName+"-tls", namespace),
		newSecret(nbName+"-oauth-client", namespace),
		newSecret(nbName+"-oauth-config", namespace),
		newSecret(nbName+"-tls", namespace),
		newOAuthClient(nbName + "-" + namespace + "-oauth-client"),
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

func listErrorReactor() k8stesting.Reactor {
	return &k8stesting.SimpleReactor{
		Verb:     "list",
		Resource: "notebooks",
		Reaction: func(_ k8stesting.Action) (bool, runtime.Object, error) {
			return true, nil, errors.New("simulated API server error")
		},
	}
}

func newTarget(k8sClient client.Client) action.Target {
	targetVersion := semver.MustParse("3.0.0")

	io := iostreams.NewIOStreams(
		&bytes.Buffer{},
		&bytes.Buffer{},
		&bytes.Buffer{},
	)

	return action.Target{
		Client:        k8sClient,
		TargetVersion: &targetVersion,
		DryRun:        false,
		SkipConfirm:   true,
		Recorder:      action.NewVerboseRootRecorder(io),
		IO:            io,
	}
}

// --- Action Metadata Tests ---

func TestVerifyMigrationAction_Metadata(t *testing.T) {
	g := NewWithT(t)

	a := &VerifyMigrationAction{Scope: &workbenches.SharedScopeOptions{}}

	g.Expect(a.ID()).To(Equal("workbenches.verify-migration"))
	g.Expect(a.Name()).To(Equal("Verify workbench migration status"))
	g.Expect(a.Description()).To(ContainSubstring("migration state"))
	g.Expect(a.Group()).To(Equal(action.GroupValidation))
	g.Expect(a.Phase()).To(Equal(action.PhasePostUpgrade))
}

func TestVerifyMigrationAction_CanApply(t *testing.T) {
	g := NewWithT(t)

	a := &VerifyMigrationAction{Scope: &workbenches.SharedScopeOptions{}}

	g.Expect(a.CanApply(action.Target{})).To(BeFalse(), "nil target version")

	v2 := semver.MustParse("2.16.0")
	g.Expect(a.CanApply(action.Target{TargetVersion: &v2})).To(BeFalse(), "target 2.x")

	v3 := semver.MustParse("3.0.0")
	g.Expect(a.CanApply(action.Target{TargetVersion: &v3})).To(BeTrue(), "target 3.0")

	v35 := semver.MustParse("3.5.0")
	g.Expect(a.CanApply(action.Target{TargetVersion: &v35})).To(BeTrue(), "target 3.5")
}

func TestVerifyMigrationAction_PrepareIsNil(t *testing.T) {
	g := NewWithT(t)

	a := &VerifyMigrationAction{Scope: &workbenches.SharedScopeOptions{}}
	g.Expect(a.Prepare()).To(BeNil())
}

func TestVerifyMigrationAction_RunNotNil(t *testing.T) {
	g := NewWithT(t)

	a := &VerifyMigrationAction{Scope: &workbenches.SharedScopeOptions{}}
	g.Expect(a.Run()).ToNot(BeNil())
}

func TestVerifyMigrationAction_AddFlags(t *testing.T) {
	g := NewWithT(t)

	a := &VerifyMigrationAction{Scope: &workbenches.SharedScopeOptions{}}
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	a.AddFlags(fs)

	g.Expect(fs.Lookup("workbench-namespace")).ToNot(BeNil())
	g.Expect(fs.Lookup("workbench-name")).ToNot(BeNil())
	g.Expect(fs.Lookup("verify-phase")).ToNot(BeNil())

	phaseFlag := fs.Lookup("verify-phase")
	g.Expect(phaseFlag.DefValue).To(Equal("migration"))
}

// --- Classification Tests ---

func TestClassifyNotebook_Legacy(t *testing.T) {
	g := NewWithT(t)

	nb := newLegacyNotebook("wb1", "ns1")
	status, detail := ClassifyNotebook(nb)

	g.Expect(status).To(Equal(StatusLegacy))
	g.Expect(detail).To(ContainSubstring("migration"))
}

func TestClassifyNotebook_Migrated(t *testing.T) {
	g := NewWithT(t)

	nb := newMigratedNotebook("wb1", "ns1")
	status, _ := ClassifyNotebook(nb)

	g.Expect(status).To(Equal(StatusMigrated))
}

func TestClassifyNotebook_MigratedWithLeftoverAnnotation(t *testing.T) {
	g := NewWithT(t)

	nb := newNotebook("wb1", "ns1",
		withAnnotations(map[string]string{
			workbenches.AnnotationInjectAuth:  "true",
			workbenches.AnnotationInjectOAuth: "true",
		}),
		withContainers(
			container("my-notebook"),
			container(workbenches.ContainerKubeRBACProxy)))

	status, detail := ClassifyNotebook(nb)

	g.Expect(status).To(Equal(StatusMigrated))
	g.Expect(detail).To(ContainSubstring("leftover"))
}

func TestClassifyNotebook_Unreconciled(t *testing.T) {
	g := NewWithT(t)

	nb := newNotebook("wb1", "ns1",
		withAnnotations(map[string]string{
			workbenches.AnnotationInjectAuth: "true",
		}),
		withContainers(
			container("my-notebook"),
			container(workbenches.ContainerOAuthProxy)))

	status, detail := ClassifyNotebook(nb)

	g.Expect(status).To(Equal(StatusUnreconciled))
	g.Expect(detail).To(ContainSubstring("oauth-proxy"))
}

func TestClassifyNotebook_Invalid(t *testing.T) {
	g := NewWithT(t)

	nb := newNotebook("wb1", "ns1",
		withAnnotations(map[string]string{
			workbenches.AnnotationInjectAuth: "true",
		}),
		withContainers(
			container("my-notebook"),
			container(workbenches.ContainerKubeRBACProxy),
			container(workbenches.ContainerOAuthProxy)))

	status, detail := ClassifyNotebook(nb)

	g.Expect(status).To(Equal(StatusInvalid))
	g.Expect(detail).To(ContainSubstring("both"))
}

func TestClassifyNotebook_Unknown(t *testing.T) {
	g := NewWithT(t)

	nb := newNotebook("wb1", "ns1",
		withContainers(container("my-notebook")))

	status, detail := ClassifyNotebook(nb)

	g.Expect(status).To(Equal(StatusUnknown))
	g.Expect(detail).To(ContainSubstring("no auth"))
}

func TestClassifyNotebook_NoContainers(t *testing.T) {
	g := NewWithT(t)

	nb := newNotebook("wb1", "ns1")

	status, detail := ClassifyNotebook(nb)

	g.Expect(status).To(Equal(StatusUnknown))
	g.Expect(detail).To(ContainSubstring("could not read"))
}

// --- Running State Tests ---

func TestGetRunningState_Stopped(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	nb := newStoppedNotebook("wb1", "ns1")
	k8sClient := newFakeClient([]*unstructured.Unstructured{nb})
	target := newTarget(k8sClient)

	state, detail := GetRunningState(ctx, target, nb)

	g.Expect(state).To(Equal(StateStopped))
	g.Expect(detail).To(ContainSubstring("since"))
}

func TestGetRunningState_Running(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	nb := newMigratedNotebook("wb1", "ns1")
	pod := newPod("wb1-0", "ns1", "Running")
	k8sClient := newFakeClient([]*unstructured.Unstructured{nb, pod})
	target := newTarget(k8sClient)

	state, _ := GetRunningState(ctx, target, nb)

	g.Expect(state).To(Equal(StateRunning))
}

func TestGetRunningState_Pending(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	nb := newMigratedNotebook("wb1", "ns1")
	pod := newPod("wb1-0", "ns1", "Pending")
	k8sClient := newFakeClient([]*unstructured.Unstructured{nb, pod})
	target := newTarget(k8sClient)

	state, detail := GetRunningState(ctx, target, nb)

	g.Expect(state).To(Equal(StateStarting))
	g.Expect(detail).To(ContainSubstring("pending"))
}

func TestGetRunningState_NoPod(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	nb := newMigratedNotebook("wb1", "ns1")
	k8sClient := newFakeClient([]*unstructured.Unstructured{nb})
	target := newTarget(k8sClient)

	state, detail := GetRunningState(ctx, target, nb)

	g.Expect(state).To(Equal(StateStarting))
	g.Expect(detail).To(ContainSubstring("no pod"))
}

func TestGetRunningState_PodGetError(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	nb := newMigratedNotebook("wb1", "ns1")

	podGetErrorReactor := &k8stesting.SimpleReactor{
		Verb:     "get",
		Resource: "pods",
		Reaction: func(_ k8stesting.Action) (bool, runtime.Object, error) {
			return true, nil, errors.New("simulated forbidden error")
		},
	}

	k8sClient := newFakeClient([]*unstructured.Unstructured{nb}, podGetErrorReactor)
	target := newTarget(k8sClient)

	state, detail := GetRunningState(ctx, target, nb)

	g.Expect(state).To(Equal(StateError))
	g.Expect(detail).To(ContainSubstring("simulated forbidden error"))
}

// --- Cleanup Check Tests ---

func TestCheckCleanupState_AllAbsent(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	nb := newMigratedNotebook("wb1", "ns1")
	k8sClient := newFakeClient([]*unstructured.Unstructured{nb})
	target := newTarget(k8sClient)

	passed, failures := CheckCleanupState(ctx, target, nb)

	g.Expect(passed).To(BeTrue())
	g.Expect(failures).To(BeEmpty())
}

func TestCheckCleanupState_AllPresent(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	objects := append(
		[]*unstructured.Unstructured{newMigratedNotebook("wb1", "ns1")},
		allOAuthResources("wb1", "ns1")...,
	)
	k8sClient := newFakeClient(objects)
	target := newTarget(k8sClient)

	nb := objects[0]
	passed, failures := CheckCleanupState(ctx, target, nb)

	g.Expect(passed).To(BeFalse())
	g.Expect(failures).To(HaveLen(6))
	g.Expect(failures).To(ContainElement(ContainSubstring("Route")))
	g.Expect(failures).To(ContainElement(ContainSubstring("Service")))
	g.Expect(failures).To(ContainElement(ContainSubstring("oauth-client")))
	g.Expect(failures).To(ContainElement(ContainSubstring("oauth-config")))
	g.Expect(failures).To(ContainElement(ContainSubstring("tls")))
	g.Expect(failures).To(ContainElement(ContainSubstring("OAuthClient")))
}

func TestCheckCleanupState_MixedPresence(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	objects := []*unstructured.Unstructured{
		newMigratedNotebook("wb1", "ns1"),
		newRoute("wb1", "ns1"),
		newSecret("wb1-oauth-client", "ns1"),
	}
	k8sClient := newFakeClient(objects)
	target := newTarget(k8sClient)

	passed, failures := CheckCleanupState(ctx, target, objects[0])

	g.Expect(passed).To(BeFalse())
	g.Expect(failures).To(HaveLen(2))
	g.Expect(failures).To(ContainElement(ContainSubstring("Route")))
	g.Expect(failures).To(ContainElement(ContainSubstring("oauth-client")))
}

// --- Phase Validation Tests ---

func TestValidatePhase_Valid(t *testing.T) {
	g := NewWithT(t)

	for _, phase := range []string{"migration", "cleanup", "all", ""} {
		a := &VerifyMigrationAction{
			Scope:       &workbenches.SharedScopeOptions{},
			VerifyPhase: phase,
		}
		g.Expect(a.validatePhase()).ToNot(HaveOccurred(), "phase: %q", phase)
	}
}

func TestValidatePhase_Invalid(t *testing.T) {
	g := NewWithT(t)

	a := &VerifyMigrationAction{
		Scope:       &workbenches.SharedScopeOptions{},
		VerifyPhase: "invalid",
	}

	err := a.validatePhase()
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("invalid"))
}

func TestIncludeMigration(t *testing.T) {
	g := NewWithT(t)

	tests := []struct {
		phase    string
		expected bool
	}{
		{"migration", true},
		{"cleanup", false},
		{"all", true},
		{"", true}, // default
	}

	for _, tt := range tests {
		a := &VerifyMigrationAction{
			Scope:       &workbenches.SharedScopeOptions{},
			VerifyPhase: tt.phase,
		}
		g.Expect(a.includeMigration()).To(Equal(tt.expected), "phase: %q", tt.phase)
	}
}

func TestIncludeCleanup(t *testing.T) {
	g := NewWithT(t)

	tests := []struct {
		phase    string
		expected bool
	}{
		{"migration", false},
		{"cleanup", true},
		{"all", true},
		{"", false}, // default (migration)
	}

	for _, tt := range tests {
		a := &VerifyMigrationAction{
			Scope:       &workbenches.SharedScopeOptions{},
			VerifyPhase: tt.phase,
		}
		g.Expect(a.includeCleanup()).To(Equal(tt.expected), "phase: %q", tt.phase)
	}
}

// --- Execute Tests ---

func TestRunTask_Execute_NoNotebooks(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	k8sClient := newFakeClient(nil)
	target := newTarget(k8sClient)

	a := &VerifyMigrationAction{Scope: &workbenches.SharedScopeOptions{}}
	task := a.Run()

	res, err := task.Execute(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(res).ToNot(BeNil())
	g.Expect(res.Status.Completed).To(BeTrue())
}

func TestRunTask_Execute_SingleMigratedNotebook(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	k8sClient := newFakeClient([]*unstructured.Unstructured{
		newMigratedNotebook("wb1", "ns1"),
	})
	target := newTarget(k8sClient)

	a := &VerifyMigrationAction{Scope: &workbenches.SharedScopeOptions{}}
	task := a.Run()

	res, err := task.Execute(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(res).ToNot(BeNil())
	g.Expect(res.Status.Completed).To(BeTrue())

	summaryStep := findStep(res.Status.Steps, "summary")
	g.Expect(summaryStep).ToNot(BeNil())
	g.Expect(summaryStep.Status).To(Equal(result.StepCompleted))
	g.Expect(summaryStep.Message).To(ContainSubstring("all checks passed"))
}

func TestRunTask_Execute_SingleLegacyNotebook(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	k8sClient := newFakeClient([]*unstructured.Unstructured{
		newLegacyNotebook("wb1", "ns1"),
	})
	target := newTarget(k8sClient)

	a := &VerifyMigrationAction{Scope: &workbenches.SharedScopeOptions{}}
	task := a.Run()

	res, err := task.Execute(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(res).ToNot(BeNil())

	summaryStep := findStep(res.Status.Steps, "summary")
	g.Expect(summaryStep).ToNot(BeNil())
	g.Expect(summaryStep.Status).To(Equal(result.StepFailed))
	g.Expect(summaryStep.Message).To(ContainSubstring("failures"))
}

func TestRunTask_Execute_MixedNotebooks(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	k8sClient := newFakeClient([]*unstructured.Unstructured{
		newMigratedNotebook("wb-good", "ns1"),
		newLegacyNotebook("wb-legacy", "ns1"),
		newMigratedNotebook("wb-also-good", "ns2"),
	})
	target := newTarget(k8sClient)

	a := &VerifyMigrationAction{Scope: &workbenches.SharedScopeOptions{}}
	task := a.Run()

	res, err := task.Execute(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(res).ToNot(BeNil())

	summaryStep := findStep(res.Status.Steps, "summary")
	g.Expect(summaryStep).ToNot(BeNil())
	g.Expect(summaryStep.Status).To(Equal(result.StepFailed))

	byStatus, ok := summaryStep.Details["byStatus"].(map[string]int)
	g.Expect(ok).To(BeTrue())
	g.Expect(byStatus["migrated"]).To(Equal(2))
	g.Expect(byStatus["legacy"]).To(Equal(1))
}

func TestRunTask_Execute_PhaseMigrationOnly(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	objects := append(
		[]*unstructured.Unstructured{newMigratedNotebook("wb1", "ns1")},
		allOAuthResources("wb1", "ns1")...,
	)
	k8sClient := newFakeClient(objects)
	target := newTarget(k8sClient)

	a := &VerifyMigrationAction{
		Scope:       &workbenches.SharedScopeOptions{},
		VerifyPhase: "migration",
	}
	task := a.Run()

	res, err := task.Execute(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(res).ToNot(BeNil())

	// Migration checks pass (migrated notebook), cleanup not checked
	summaryStep := findStep(res.Status.Steps, "summary")
	g.Expect(summaryStep).ToNot(BeNil())
	g.Expect(summaryStep.Status).To(Equal(result.StepCompleted))

	// Cleanup details should not be present
	_, hasCleanupPass := summaryStep.Details["cleanupPass"]
	g.Expect(hasCleanupPass).To(BeFalse())
}

func TestRunTask_Execute_PhaseCleanupOnly(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	objects := append(
		[]*unstructured.Unstructured{newMigratedNotebook("wb1", "ns1")},
		allOAuthResources("wb1", "ns1")...,
	)
	k8sClient := newFakeClient(objects)
	target := newTarget(k8sClient)

	a := &VerifyMigrationAction{
		Scope:       &workbenches.SharedScopeOptions{},
		VerifyPhase: "cleanup",
	}
	task := a.Run()

	res, err := task.Execute(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(res).ToNot(BeNil())

	// Cleanup checks fail (OAuth resources still present)
	summaryStep := findStep(res.Status.Steps, "summary")
	g.Expect(summaryStep).ToNot(BeNil())
	g.Expect(summaryStep.Status).To(Equal(result.StepFailed))

	// Migration details should not be present
	_, hasMigrationPass := summaryStep.Details["migrationPass"]
	g.Expect(hasMigrationPass).To(BeFalse())
}

func TestRunTask_Execute_PhaseCleanupOnly_InvalidNotebookFails(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	invalidNb := newNotebook("wb-invalid", "ns1",
		withAnnotations(map[string]string{
			workbenches.AnnotationInjectAuth: "true",
		}),
		withContainers(
			container("my-notebook"),
			container(workbenches.ContainerKubeRBACProxy),
			container(workbenches.ContainerOAuthProxy),
		))

	k8sClient := newFakeClient([]*unstructured.Unstructured{invalidNb})
	target := newTarget(k8sClient)

	a := &VerifyMigrationAction{
		Scope:       &workbenches.SharedScopeOptions{},
		VerifyPhase: "cleanup",
	}
	task := a.Run()

	res, err := task.Execute(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(res).ToNot(BeNil())

	summaryStep := findStep(res.Status.Steps, "summary")
	g.Expect(summaryStep).ToNot(BeNil())
	g.Expect(summaryStep.Status).To(Equal(result.StepFailed))
	g.Expect(summaryStep.Details["classificationFail"]).To(Equal(1))
}

func TestRunTask_Execute_PhaseAll(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	k8sClient := newFakeClient([]*unstructured.Unstructured{
		newMigratedNotebook("wb1", "ns1"),
	})
	target := newTarget(k8sClient)

	a := &VerifyMigrationAction{
		Scope:       &workbenches.SharedScopeOptions{},
		VerifyPhase: "all",
	}
	task := a.Run()

	res, err := task.Execute(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(res).ToNot(BeNil())

	summaryStep := findStep(res.Status.Steps, "summary")
	g.Expect(summaryStep).ToNot(BeNil())
	g.Expect(summaryStep.Status).To(Equal(result.StepCompleted))

	// Both phase details should be present
	_, hasMigrationPass := summaryStep.Details["migrationPass"]
	g.Expect(hasMigrationPass).To(BeTrue())

	_, hasCleanupPass := summaryStep.Details["cleanupPass"]
	g.Expect(hasCleanupPass).To(BeTrue())
}

func TestRunTask_Execute_InvalidPhase(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	k8sClient := newFakeClient(nil)
	target := newTarget(k8sClient)

	a := &VerifyMigrationAction{
		Scope:       &workbenches.SharedScopeOptions{},
		VerifyPhase: "bogus",
	}
	task := a.Run()

	_, err := task.Execute(ctx, target)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("invalid"))
}

func TestRunTask_Execute_NamespaceScopedTargeting(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	k8sClient := newFakeClient([]*unstructured.Unstructured{
		newMigratedNotebook("wb1", "ns1"),
		newMigratedNotebook("wb2", "ns2"),
	})
	target := newTarget(k8sClient)

	a := &VerifyMigrationAction{
		Scope: &workbenches.SharedScopeOptions{
			WorkbenchNamespace: "ns1",
		},
	}
	task := a.Run()

	res, err := task.Execute(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(res).ToNot(BeNil())

	summaryStep := findStep(res.Status.Steps, "summary")
	g.Expect(summaryStep).ToNot(BeNil())

	total, ok := summaryStep.Details["total"].(int)
	g.Expect(ok).To(BeTrue())
	g.Expect(total).To(Equal(1))
}

func TestRunTask_Execute_SingleNotebookTargeting(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	k8sClient := newFakeClient([]*unstructured.Unstructured{
		newMigratedNotebook("wb1", "ns1"),
		newMigratedNotebook("wb2", "ns1"),
	})
	target := newTarget(k8sClient)

	a := &VerifyMigrationAction{
		Scope: &workbenches.SharedScopeOptions{
			WorkbenchNamespace: "ns1",
			WorkbenchName:      "wb1",
		},
	}
	task := a.Run()

	res, err := task.Execute(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(res).ToNot(BeNil())

	summaryStep := findStep(res.Status.Steps, "summary")
	g.Expect(summaryStep).ToNot(BeNil())

	total, ok := summaryStep.Details["total"].(int)
	g.Expect(ok).To(BeTrue())
	g.Expect(total).To(Equal(1))
}

func TestRunTask_Execute_NameRequiresNamespace(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	k8sClient := newFakeClient(nil)
	target := newTarget(k8sClient)

	a := &VerifyMigrationAction{
		Scope: &workbenches.SharedScopeOptions{WorkbenchName: "wb1"},
	}
	task := a.Run()

	_, err := task.Execute(ctx, target)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("--workbench-name requires --workbench-namespace"))
}

func TestRunTask_Execute_ListError(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	k8sClient := newFakeClient(nil, listErrorReactor())
	target := newTarget(k8sClient)

	a := &VerifyMigrationAction{Scope: &workbenches.SharedScopeOptions{}}
	task := a.Run()

	res, err := task.Execute(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(res).ToNot(BeNil())

	hasFailedStep := false

	for _, step := range res.Status.Steps {
		if step.Status == result.StepFailed {
			hasFailedStep = true
		}
	}

	g.Expect(hasFailedStep).To(BeTrue())
}

func TestRunTask_Validate_DelegatesToExecute(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	k8sClient := newFakeClient([]*unstructured.Unstructured{
		newMigratedNotebook("wb1", "ns1"),
	})
	target := newTarget(k8sClient)

	a := &VerifyMigrationAction{Scope: &workbenches.SharedScopeOptions{}}
	task := a.Run()

	res, err := task.Validate(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(res).ToNot(BeNil())
	g.Expect(res.Status.Completed).To(BeTrue())
}

func TestRunTask_Execute_SummaryCountsCorrectness(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	nb1 := newMigratedNotebook("wb-migrated", "ns1")
	nb2 := newLegacyNotebook("wb-legacy", "ns1")
	nb3 := newStoppedNotebook("wb-stopped", "ns1")
	pod1 := newPod("wb-migrated-0", "ns1", "Running")
	pod2 := newPod("wb-legacy-0", "ns1", "Running")

	k8sClient := newFakeClient([]*unstructured.Unstructured{nb1, nb2, nb3, pod1, pod2})
	target := newTarget(k8sClient)

	a := &VerifyMigrationAction{Scope: &workbenches.SharedScopeOptions{}}
	task := a.Run()

	res, err := task.Execute(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())

	summaryStep := findStep(res.Status.Steps, "summary")
	g.Expect(summaryStep).ToNot(BeNil())

	total, ok := summaryStep.Details["total"].(int)
	g.Expect(ok).To(BeTrue())
	g.Expect(total).To(Equal(3))

	byStatus, ok := summaryStep.Details["byStatus"].(map[string]int)
	g.Expect(ok).To(BeTrue())
	g.Expect(byStatus["migrated"]).To(Equal(2)) // wb-migrated + wb-stopped (both have kube-rbac-proxy)
	g.Expect(byStatus["legacy"]).To(Equal(1))

	byRunState, ok := summaryStep.Details["byRunState"].(map[string]int)
	g.Expect(ok).To(BeTrue())
	g.Expect(byRunState["running"]).To(Equal(2))
	g.Expect(byRunState["stopped"]).To(Equal(1))
}

func TestRunTask_Execute_CleanupChecksWithResources(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	objects := append(
		[]*unstructured.Unstructured{newMigratedNotebook("wb1", "ns1")},
		allOAuthResources("wb1", "ns1")...,
	)
	k8sClient := newFakeClient(objects)
	target := newTarget(k8sClient)

	a := &VerifyMigrationAction{
		Scope:       &workbenches.SharedScopeOptions{},
		VerifyPhase: "all",
	}
	task := a.Run()

	res, err := task.Execute(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())

	summaryStep := findStep(res.Status.Steps, "summary")
	g.Expect(summaryStep).ToNot(BeNil())

	cleanupFail, ok := summaryStep.Details["cleanupFail"].(int)
	g.Expect(ok).To(BeTrue())
	g.Expect(cleanupFail).To(Equal(1))

	migrationPass, ok := summaryStep.Details["migrationPass"].(int)
	g.Expect(ok).To(BeTrue())
	g.Expect(migrationPass).To(Equal(1))
}

// --- Helpers ---

func findStep(steps []result.ActionStep, name string) *result.ActionStep {
	for i := range steps {
		if steps[i].Name == name {
			return &steps[i]
		}

		if found := findStep(steps[i].Children, name); found != nil {
			return found
		}
	}

	return nil
}
