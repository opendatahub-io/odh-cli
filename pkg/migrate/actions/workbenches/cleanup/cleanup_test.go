//nolint:testpackage // Tests internal implementation (unexported helpers)
package cleanup

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
	resources.Notebook.GVR():    resources.Notebook.ListKind(),
	resources.Route.GVR():       resources.Route.ListKind(),
	resources.Service.GVR():     resources.Service.ListKind(),
	resources.Secret.GVR():      resources.Secret.ListKind(),
	resources.OAuthClient.GVR(): resources.OAuthClient.ListKind(),
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

func containerWithEnv(name string, envVars ...map[string]any) map[string]any {
	return map[string]any{
		"name":  name,
		"image": "registry.example.com/" + name + ":latest",
		"env":   toAnySlice(envVars),
	}
}

func envVar(name, value string) map[string]any {
	return map[string]any{
		"name":  name,
		"value": value,
	}
}

func migratedAnnotations() map[string]string {
	return map[string]string{
		workbenches.AnnotationInjectAuth: "true",
	}
}

func migratedContainers() []map[string]any {
	return []map[string]any{
		container("my-notebook"),
		container(workbenches.ContainerKubeRBACProxy),
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
		withAnnotations(migratedAnnotations()),
		withContainers(migratedContainers()...))
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

func deleteErrorReactor(resource string) k8stesting.Reactor {
	return &k8stesting.SimpleReactor{
		Verb:     "delete",
		Resource: resource,
		Reaction: func(_ k8stesting.Action) (bool, runtime.Object, error) {
			return true, nil, errors.New("simulated delete error")
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

func withInteractiveConfirm(input string) func(*action.Target) {
	return func(t *action.Target) {
		t.SkipConfirm = false
		t.IO = iostreams.NewIOStreams(
			strings.NewReader(input),
			&bytes.Buffer{},
			&bytes.Buffer{},
		)
		t.Recorder = action.NewVerboseRootRecorder(t.IO)
	}
}

// --- Action Metadata Tests ---

func TestCleanupOAuthAction_Metadata(t *testing.T) {
	g := NewWithT(t)

	a := &CleanupOAuthAction{Scope: &workbenches.SharedScopeOptions{}}

	g.Expect(a.ID()).To(Equal("workbenches.cleanup-oauth"))
	g.Expect(a.Name()).To(Equal("Clean up legacy OAuth resources from workbenches"))
	g.Expect(a.Description()).To(ContainSubstring("OAuth-proxy"))
	g.Expect(a.Group()).To(Equal(action.GroupMigration))
	g.Expect(a.Phase()).To(Equal(action.PhasePostUpgrade))
}

func TestCleanupOAuthAction_CanApply(t *testing.T) {
	g := NewWithT(t)

	a := &CleanupOAuthAction{Scope: &workbenches.SharedScopeOptions{}}

	g.Expect(a.CanApply(action.Target{})).To(BeFalse(), "nil target version")

	v2 := semver.MustParse("2.16.0")
	g.Expect(a.CanApply(action.Target{TargetVersion: &v2})).To(BeFalse(), "target 2.x")

	v3 := semver.MustParse("3.0.0")
	g.Expect(a.CanApply(action.Target{TargetVersion: &v3})).To(BeTrue(), "target 3.0")

	v35 := semver.MustParse("3.5.0")
	g.Expect(a.CanApply(action.Target{TargetVersion: &v35})).To(BeTrue(), "target 3.5")
}

func TestCleanupOAuthAction_PrepareIsNil(t *testing.T) {
	g := NewWithT(t)

	a := &CleanupOAuthAction{Scope: &workbenches.SharedScopeOptions{}}
	g.Expect(a.Prepare()).To(BeNil())
}

func TestCleanupOAuthAction_RunNotNil(t *testing.T) {
	g := NewWithT(t)

	a := &CleanupOAuthAction{Scope: &workbenches.SharedScopeOptions{}}
	g.Expect(a.Run()).ToNot(BeNil())
}

func TestCleanupOAuthAction_AddFlags(t *testing.T) {
	g := NewWithT(t)

	a := &CleanupOAuthAction{Scope: &workbenches.SharedScopeOptions{}}
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	a.AddFlags(fs)

	g.Expect(fs.Lookup("workbench-namespace")).ToNot(BeNil())
	g.Expect(fs.Lookup("workbench-name")).ToNot(BeNil())
}

// --- Pre-check Tests ---

func TestCheckMigrationState_AllPassed(t *testing.T) {
	g := NewWithT(t)

	nb := newMigratedNotebook("wb1", "ns1")
	passed, failures := CheckMigrationState(nb)

	g.Expect(passed).To(BeTrue())
	g.Expect(failures).To(BeEmpty())
}

func TestCheckMigrationState_InjectAuthMissing(t *testing.T) {
	g := NewWithT(t)

	nb := newNotebook("wb1", "ns1",
		withContainers(migratedContainers()...))

	passed, failures := CheckMigrationState(nb)

	g.Expect(passed).To(BeFalse())
	g.Expect(failures).To(ContainElement(ContainSubstring("inject-auth")))
}

func TestCheckMigrationState_KubeRBACProxyMissing(t *testing.T) {
	g := NewWithT(t)

	nb := newNotebook("wb1", "ns1",
		withAnnotations(migratedAnnotations()),
		withContainers(container("my-notebook")))

	passed, failures := CheckMigrationState(nb)

	g.Expect(passed).To(BeFalse())
	g.Expect(failures).To(ContainElement(ContainSubstring("kube-rbac-proxy")))
}

func TestCheckMigrationState_OAuthProxyStillPresent(t *testing.T) {
	g := NewWithT(t)

	nb := newNotebook("wb1", "ns1",
		withAnnotations(migratedAnnotations()),
		withContainers(
			container("my-notebook"),
			container(workbenches.ContainerKubeRBACProxy),
			container(workbenches.ContainerOAuthProxy)))

	passed, failures := CheckMigrationState(nb)

	g.Expect(passed).To(BeFalse())
	g.Expect(failures).To(ContainElement(ContainSubstring("oauth-proxy")))
}

func TestCheckMigrationState_InjectOAuthTolerated(t *testing.T) {
	g := NewWithT(t)

	nb := newNotebook("wb1", "ns1",
		withAnnotations(map[string]string{
			workbenches.AnnotationInjectAuth:  "true",
			workbenches.AnnotationInjectOAuth: "true",
		}),
		withContainers(
			container("my-notebook"),
			container(workbenches.ContainerKubeRBACProxy)))

	passed, failures := CheckMigrationState(nb)

	g.Expect(passed).To(BeTrue(), "inject-oauth tolerated when kube-rbac-proxy present and oauth-proxy absent")
	g.Expect(failures).To(BeEmpty())
}

func TestCheckMigrationState_InjectOAuthFails(t *testing.T) {
	g := NewWithT(t)

	nb := newNotebook("wb1", "ns1",
		withAnnotations(map[string]string{
			workbenches.AnnotationInjectAuth:  "true",
			workbenches.AnnotationInjectOAuth: "true",
		}),
		withContainers(container("my-notebook")))

	passed, failures := CheckMigrationState(nb)

	g.Expect(passed).To(BeFalse())
	g.Expect(failures).To(ContainElement(ContainSubstring("inject-oauth")))
}

func TestCheckMigrationState_TornadoSettingsPresent(t *testing.T) {
	g := NewWithT(t)

	nb := newNotebook("wb1", "ns1",
		withAnnotations(migratedAnnotations()),
		withContainers(
			containerWithEnv("my-notebook",
				envVar("NOTEBOOK_ARGS", "--ServerApp.tornado_settings={\"xsrf_cookies\": false}")),
			container(workbenches.ContainerKubeRBACProxy)))

	passed, failures := CheckMigrationState(nb)

	g.Expect(passed).To(BeFalse())
	g.Expect(failures).To(ContainElement(ContainSubstring("tornado_settings")))
}

func TestCheckMigrationState_NoContainers(t *testing.T) {
	g := NewWithT(t)

	nb := newNotebook("wb1", "ns1",
		withAnnotations(migratedAnnotations()))

	passed, failures := CheckMigrationState(nb)

	g.Expect(passed).To(BeFalse())
	g.Expect(failures).To(ContainElement(ContainSubstring("could not read containers")))
}

func TestCheckMigrationState_TornadoSettingsInDifferentEnvVar(t *testing.T) {
	g := NewWithT(t)

	nb := newNotebook("wb1", "ns1",
		withAnnotations(migratedAnnotations()),
		withContainers(
			containerWithEnv("my-notebook",
				envVar("OTHER_VAR", "--ServerApp.tornado_settings={\"xsrf_cookies\": false}")),
			container(workbenches.ContainerKubeRBACProxy)))

	passed, failures := CheckMigrationState(nb)

	g.Expect(passed).To(BeTrue(), "tornado_settings in a non-NOTEBOOK_ARGS env var should be ignored")
	g.Expect(failures).To(BeEmpty())
}

// --- Validate Tests ---

func TestRunTask_Validate_MixedPassFail(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	k8sClient := newFakeClient([]*unstructured.Unstructured{
		newMigratedNotebook("wb-good", "ns1"),
		newNotebook("wb-bad", "ns1", withContainers(container("my-notebook"))),
	})
	target := newTarget(k8sClient)

	a := &CleanupOAuthAction{Scope: &workbenches.SharedScopeOptions{}}
	task := a.Run()

	result, err := task.Validate(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).ToNot(BeNil())

	hasFailedStep := false

	for _, step := range result.Status.Steps {
		if step.Status == "Failed" {
			hasFailedStep = true

			g.Expect(step.Message).To(ContainSubstring("1/2"))
		}
	}

	g.Expect(hasFailedStep).To(BeTrue())
}

func TestRunTask_Validate_NoNotebooks(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	k8sClient := newFakeClient(nil)
	target := newTarget(k8sClient)

	a := &CleanupOAuthAction{Scope: &workbenches.SharedScopeOptions{}}
	task := a.Run()

	result, err := task.Validate(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).ToNot(BeNil())
	g.Expect(result.Status.Completed).To(BeTrue())
}

func TestRunTask_Validate_PassingChecks(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	k8sClient := newFakeClient([]*unstructured.Unstructured{
		newMigratedNotebook("wb1", "ns1"),
	})
	target := newTarget(k8sClient)

	a := &CleanupOAuthAction{Scope: &workbenches.SharedScopeOptions{}}
	task := a.Run()

	result, err := task.Validate(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).ToNot(BeNil())
	g.Expect(result.Status.Completed).To(BeTrue())
}

func TestRunTask_Validate_FailingChecks(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	nb := newNotebook("wb1", "ns1",
		withContainers(container("my-notebook")))

	k8sClient := newFakeClient([]*unstructured.Unstructured{nb})
	target := newTarget(k8sClient)

	a := &CleanupOAuthAction{Scope: &workbenches.SharedScopeOptions{}}
	task := a.Run()

	result, err := task.Validate(ctx, target)
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

func TestRunTask_Validate_NameRequiresNamespace(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	k8sClient := newFakeClient(nil)
	target := newTarget(k8sClient)

	a := &CleanupOAuthAction{Scope: &workbenches.SharedScopeOptions{WorkbenchName: "wb1"}}
	task := a.Run()

	_, err := task.Validate(ctx, target)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("--workbench-name requires --workbench-namespace"))
}

func TestRunTask_Validate_ListError(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	k8sClient := newFakeClient(nil, listErrorReactor())
	target := newTarget(k8sClient)

	a := &CleanupOAuthAction{Scope: &workbenches.SharedScopeOptions{}}
	task := a.Run()

	result, err := task.Validate(ctx, target)
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

	k8sClient := newFakeClient(nil)
	target := newTarget(k8sClient)

	a := &CleanupOAuthAction{Scope: &workbenches.SharedScopeOptions{}}
	task := a.Run()

	result, err := task.Execute(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).ToNot(BeNil())
	g.Expect(result.Status.Completed).To(BeTrue())
}

func TestRunTask_Execute_CleanupSuccess(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	objects := append(
		[]*unstructured.Unstructured{newMigratedNotebook("wb1", "ns1")},
		allOAuthResources("wb1", "ns1")...,
	)
	k8sClient := newFakeClient(objects)
	target := newTarget(k8sClient)

	a := &CleanupOAuthAction{Scope: &workbenches.SharedScopeOptions{}}
	task := a.Run()

	result, err := task.Execute(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).ToNot(BeNil())
	g.Expect(result.Status.Completed).To(BeTrue())

	// Verify resources are deleted
	_, err = k8sClient.Dynamic().Resource(resources.Route.GVR()).
		Namespace("ns1").Get(context.Background(), "wb1", metav1.GetOptions{})
	g.Expect(err).To(HaveOccurred(), "route should be deleted")

	_, err = k8sClient.Dynamic().Resource(resources.Service.GVR()).
		Namespace("ns1").Get(context.Background(), "wb1-tls", metav1.GetOptions{})
	g.Expect(err).To(HaveOccurred(), "service should be deleted")

	_, err = k8sClient.Dynamic().Resource(resources.Secret.GVR()).
		Namespace("ns1").Get(context.Background(), "wb1-oauth-client", metav1.GetOptions{})
	g.Expect(err).To(HaveOccurred(), "secret oauth-client should be deleted")

	_, err = k8sClient.Dynamic().Resource(resources.Secret.GVR()).
		Namespace("ns1").Get(context.Background(), "wb1-oauth-config", metav1.GetOptions{})
	g.Expect(err).To(HaveOccurred(), "secret oauth-config should be deleted")

	_, err = k8sClient.Dynamic().Resource(resources.Secret.GVR()).
		Namespace("ns1").Get(context.Background(), "wb1-tls", metav1.GetOptions{})
	g.Expect(err).To(HaveOccurred(), "secret tls should be deleted")

	_, err = k8sClient.Dynamic().Resource(resources.OAuthClient.GVR()).
		Get(context.Background(), "wb1-ns1-oauth-client", metav1.GetOptions{})
	g.Expect(err).To(HaveOccurred(), "oauthclient should be deleted")
}

func TestRunTask_Execute_ResourcesAlreadyAbsent(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	k8sClient := newFakeClient([]*unstructured.Unstructured{
		newMigratedNotebook("wb1", "ns1"),
	})
	target := newTarget(k8sClient)

	a := &CleanupOAuthAction{Scope: &workbenches.SharedScopeOptions{}}
	task := a.Run()

	result, err := task.Execute(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).ToNot(BeNil())
	g.Expect(result.Status.Completed).To(BeTrue())
}

func TestRunTask_Execute_DeleteError(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	objects := append(
		[]*unstructured.Unstructured{newMigratedNotebook("wb1", "ns1")},
		allOAuthResources("wb1", "ns1")...,
	)
	k8sClient := newFakeClient(objects, deleteErrorReactor("routes"))
	target := newTarget(k8sClient)

	a := &CleanupOAuthAction{Scope: &workbenches.SharedScopeOptions{}}
	task := a.Run()

	result, err := task.Execute(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).ToNot(BeNil())

	hasFailedStep := false

	for _, step := range result.Status.Steps {
		if step.Status == "Failed" {
			hasFailedStep = true
		}

		for _, child := range step.Children {
			if child.Status == "Failed" {
				hasFailedStep = true
			}
		}
	}

	g.Expect(hasFailedStep).To(BeTrue())
}

func TestRunTask_Execute_DryRun(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	objects := append(
		[]*unstructured.Unstructured{newMigratedNotebook("wb1", "ns1")},
		allOAuthResources("wb1", "ns1")...,
	)
	k8sClient := newFakeClient(objects)
	target := newTarget(k8sClient, withDryRun)

	a := &CleanupOAuthAction{Scope: &workbenches.SharedScopeOptions{}}
	task := a.Run()

	result, err := task.Execute(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).ToNot(BeNil())

	// Verify resources are NOT deleted in dry-run
	_, err = k8sClient.Dynamic().Resource(resources.Route.GVR()).
		Namespace("ns1").Get(context.Background(), "wb1", metav1.GetOptions{})
	g.Expect(err).ToNot(HaveOccurred(), "route should still exist in dry-run")

	_, err = k8sClient.Dynamic().Resource(resources.OAuthClient.GVR()).
		Get(context.Background(), "wb1-ns1-oauth-client", metav1.GetOptions{})
	g.Expect(err).ToNot(HaveOccurred(), "oauthclient should still exist in dry-run")
}

func TestRunTask_Execute_PreCheckFails_SkipConfirm(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	nb := newNotebook("wb1", "ns1",
		withContainers(container("my-notebook")))

	objects := append(
		[]*unstructured.Unstructured{nb},
		allOAuthResources("wb1", "ns1")...,
	)
	k8sClient := newFakeClient(objects)
	target := newTarget(k8sClient)

	a := &CleanupOAuthAction{Scope: &workbenches.SharedScopeOptions{}}
	task := a.Run()

	result, err := task.Execute(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).ToNot(BeNil())
	g.Expect(result.Status.Completed).To(BeTrue())

	// SkipConfirm=true means pre-check failure is overridden and cleanup proceeds
	_, err = k8sClient.Dynamic().Resource(resources.Route.GVR()).
		Namespace("ns1").Get(context.Background(), "wb1", metav1.GetOptions{})
	g.Expect(err).To(HaveOccurred(), "route should be deleted even with failed pre-check (SkipConfirm)")
}

func TestRunTask_Execute_PreCheckFails_UserSkips(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	nb := newNotebook("wb1", "ns1",
		withContainers(container("my-notebook")))

	objects := append(
		[]*unstructured.Unstructured{nb},
		allOAuthResources("wb1", "ns1")...,
	)
	k8sClient := newFakeClient(objects)
	target := newTarget(k8sClient, withInteractiveConfirm("n\ny\n"))

	a := &CleanupOAuthAction{Scope: &workbenches.SharedScopeOptions{}}
	task := a.Run()

	result, err := task.Execute(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).ToNot(BeNil())

	// User said "n" to per-notebook pre-check prompt, so resources should still exist
	_, err = k8sClient.Dynamic().Resource(resources.Route.GVR()).
		Namespace("ns1").Get(context.Background(), "wb1", metav1.GetOptions{})
	g.Expect(err).ToNot(HaveOccurred(), "route should still exist when user skips")
}

func TestRunTask_Execute_MultipleNotebooks(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	objects := make([]*unstructured.Unstructured, 0, 14)
	objects = append(objects, newMigratedNotebook("wb1", "ns1"))
	objects = append(objects, allOAuthResources("wb1", "ns1")...)
	objects = append(objects, newMigratedNotebook("wb2", "ns2"))
	objects = append(objects, allOAuthResources("wb2", "ns2")...)

	k8sClient := newFakeClient(objects)
	target := newTarget(k8sClient)

	a := &CleanupOAuthAction{Scope: &workbenches.SharedScopeOptions{}}
	task := a.Run()

	result, err := task.Execute(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).ToNot(BeNil())
	g.Expect(result.Status.Completed).To(BeTrue())

	// Both notebooks' resources should be deleted
	_, err = k8sClient.Dynamic().Resource(resources.Route.GVR()).
		Namespace("ns1").Get(context.Background(), "wb1", metav1.GetOptions{})
	g.Expect(err).To(HaveOccurred(), "wb1 route should be deleted")

	_, err = k8sClient.Dynamic().Resource(resources.Route.GVR()).
		Namespace("ns2").Get(context.Background(), "wb2", metav1.GetOptions{})
	g.Expect(err).To(HaveOccurred(), "wb2 route should be deleted")
}

func TestRunTask_Execute_SingleNotebookTargeting(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	objects := make([]*unstructured.Unstructured, 0, 14)
	objects = append(objects, newMigratedNotebook("wb1", "ns1"))
	objects = append(objects, allOAuthResources("wb1", "ns1")...)
	objects = append(objects, newMigratedNotebook("wb2", "ns1"))
	objects = append(objects, allOAuthResources("wb2", "ns1")...)

	k8sClient := newFakeClient(objects)
	target := newTarget(k8sClient)

	a := &CleanupOAuthAction{Scope: &workbenches.SharedScopeOptions{
		WorkbenchNamespace: "ns1",
		WorkbenchName:      "wb1",
	}}
	task := a.Run()

	result, err := task.Execute(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).ToNot(BeNil())
	g.Expect(result.Status.Completed).To(BeTrue())

	// wb1 resources should be deleted
	_, err = k8sClient.Dynamic().Resource(resources.Route.GVR()).
		Namespace("ns1").Get(context.Background(), "wb1", metav1.GetOptions{})
	g.Expect(err).To(HaveOccurred(), "wb1 route should be deleted")

	// wb2 resources should still exist (not targeted)
	_, err = k8sClient.Dynamic().Resource(resources.Route.GVR()).
		Namespace("ns1").Get(context.Background(), "wb2", metav1.GetOptions{})
	g.Expect(err).ToNot(HaveOccurred(), "wb2 route should still exist")
}

func TestRunTask_Execute_NamespaceScopedTargeting(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	objects := make([]*unstructured.Unstructured, 0, 14)
	objects = append(objects, newMigratedNotebook("wb1", "ns1"))
	objects = append(objects, allOAuthResources("wb1", "ns1")...)
	objects = append(objects, newMigratedNotebook("wb2", "ns2"))
	objects = append(objects, allOAuthResources("wb2", "ns2")...)

	k8sClient := newFakeClient(objects)
	target := newTarget(k8sClient)

	a := &CleanupOAuthAction{Scope: &workbenches.SharedScopeOptions{
		WorkbenchNamespace: "ns1",
	}}
	task := a.Run()

	result, err := task.Execute(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).ToNot(BeNil())
	g.Expect(result.Status.Completed).To(BeTrue())

	// ns1 resources should be deleted
	_, err = k8sClient.Dynamic().Resource(resources.Route.GVR()).
		Namespace("ns1").Get(context.Background(), "wb1", metav1.GetOptions{})
	g.Expect(err).To(HaveOccurred(), "ns1 route should be deleted")

	// ns2 resources should still exist
	_, err = k8sClient.Dynamic().Resource(resources.Route.GVR()).
		Namespace("ns2").Get(context.Background(), "wb2", metav1.GetOptions{})
	g.Expect(err).ToNot(HaveOccurred(), "ns2 route should still exist")
}

func TestRunTask_Execute_OAuthClientClusterScoped(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	objects := append(
		[]*unstructured.Unstructured{newMigratedNotebook("wb1", "ns1")},
		newOAuthClient("wb1-ns1-oauth-client"),
	)
	k8sClient := newFakeClient(objects)
	target := newTarget(k8sClient)

	a := &CleanupOAuthAction{Scope: &workbenches.SharedScopeOptions{}}
	task := a.Run()

	result, err := task.Execute(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).ToNot(BeNil())
	g.Expect(result.Status.Completed).To(BeTrue())

	// Cluster-scoped OAuthClient should be deleted (no namespace in Get)
	_, err = k8sClient.Dynamic().Resource(resources.OAuthClient.GVR()).
		Get(context.Background(), "wb1-ns1-oauth-client", metav1.GetOptions{})
	g.Expect(err).To(HaveOccurred(), "oauthclient should be deleted")
}

func TestRunTask_Execute_ListError(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	k8sClient := newFakeClient(nil, listErrorReactor())
	target := newTarget(k8sClient)

	a := &CleanupOAuthAction{Scope: &workbenches.SharedScopeOptions{}}
	task := a.Run()

	result, err := task.Execute(ctx, target)
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

func TestRunTask_Execute_NameRequiresNamespace(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	k8sClient := newFakeClient(nil)
	target := newTarget(k8sClient)

	a := &CleanupOAuthAction{Scope: &workbenches.SharedScopeOptions{WorkbenchName: "wb1"}}
	task := a.Run()

	_, err := task.Execute(ctx, target)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("--workbench-name requires --workbench-namespace"))
}

func TestRunTask_Execute_ConfirmationAccepted(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	objects := append(
		[]*unstructured.Unstructured{newMigratedNotebook("wb1", "ns1")},
		allOAuthResources("wb1", "ns1")...,
	)
	k8sClient := newFakeClient(objects)
	target := newTarget(k8sClient, withInteractiveConfirm("y\n"))

	a := &CleanupOAuthAction{Scope: &workbenches.SharedScopeOptions{}}
	task := a.Run()

	result, err := task.Execute(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).ToNot(BeNil())
	g.Expect(result.Status.Completed).To(BeTrue())

	_, err = k8sClient.Dynamic().Resource(resources.Route.GVR()).
		Namespace("ns1").Get(context.Background(), "wb1", metav1.GetOptions{})
	g.Expect(err).To(HaveOccurred(), "route should be deleted after confirmation")
}

func TestRunTask_Execute_ConfirmationCancelled(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	objects := append(
		[]*unstructured.Unstructured{newMigratedNotebook("wb1", "ns1")},
		allOAuthResources("wb1", "ns1")...,
	)
	k8sClient := newFakeClient(objects)
	target := newTarget(k8sClient, withInteractiveConfirm("n\n"))

	a := &CleanupOAuthAction{Scope: &workbenches.SharedScopeOptions{}}
	task := a.Run()

	result, err := task.Execute(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).ToNot(BeNil())

	// Resources should still exist since user cancelled
	_, err = k8sClient.Dynamic().Resource(resources.Route.GVR()).
		Namespace("ns1").Get(context.Background(), "wb1", metav1.GetOptions{})
	g.Expect(err).ToNot(HaveOccurred(), "route should still exist after cancellation")
}

func TestRunTask_Execute_MixedPresent(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	// Only route and one secret present; others absent
	objects := []*unstructured.Unstructured{
		newMigratedNotebook("wb1", "ns1"),
		newRoute("wb1", "ns1"),
		newSecret("wb1-oauth-client", "ns1"),
	}
	k8sClient := newFakeClient(objects)
	target := newTarget(k8sClient)

	a := &CleanupOAuthAction{Scope: &workbenches.SharedScopeOptions{}}
	task := a.Run()

	result, err := task.Execute(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).ToNot(BeNil())
	g.Expect(result.Status.Completed).To(BeTrue())

	// Present resources should be deleted
	_, err = k8sClient.Dynamic().Resource(resources.Route.GVR()).
		Namespace("ns1").Get(context.Background(), "wb1", metav1.GetOptions{})
	g.Expect(err).To(HaveOccurred(), "route should be deleted")

	_, err = k8sClient.Dynamic().Resource(resources.Secret.GVR()).
		Namespace("ns1").Get(context.Background(), "wb1-oauth-client", metav1.GetOptions{})
	g.Expect(err).To(HaveOccurred(), "secret should be deleted")
}
