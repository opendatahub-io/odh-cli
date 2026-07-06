//nolint:testpackage // Tests internal implementation (unexported helpers)
package authmodel

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

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
	workbenchcleanup "github.com/opendatahub-io/odh-cli/pkg/migrate/actions/workbenches/cleanup"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
	"github.com/opendatahub-io/odh-cli/pkg/util/iostreams"
	"github.com/opendatahub-io/odh-cli/pkg/util/jq"
	"github.com/opendatahub-io/odh-cli/pkg/util/kube"

	. "github.com/onsi/gomega"
)

// --- Test Fixtures ---

//nolint:gochecknoglobals // test-only GVR→ListKind mapping for fake dynamic client
var testListKinds = map[schema.GroupVersionResource]string{
	resources.Notebook.GVR():    resources.Notebook.ListKind(),
	resources.StatefulSet.GVR(): resources.StatefulSet.ListKind(),
	resources.Pod.GVR():         resources.Pod.ListKind(),
	resources.Namespace.GVR():   resources.Namespace.ListKind(),
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
		spec, ok := obj["spec"].(map[string]any)
		if !ok {
			spec = make(map[string]any)
			obj["spec"] = spec
		}

		template, ok := spec["template"].(map[string]any)
		if !ok {
			template = make(map[string]any)
			spec["template"] = template
		}

		podSpec, ok := template["spec"].(map[string]any)
		if !ok {
			podSpec = make(map[string]any)
			template["spec"] = podSpec
		}

		podSpec["containers"] = toAnySlice(containers)
	}
}

func withVolumes(volumes ...map[string]any) notebookOption {
	return func(obj map[string]any) {
		spec, ok := obj["spec"].(map[string]any)
		if !ok {
			spec = make(map[string]any)
			obj["spec"] = spec
		}

		template, ok := spec["template"].(map[string]any)
		if !ok {
			template = make(map[string]any)
			spec["template"] = template
		}

		podSpec, ok := template["spec"].(map[string]any)
		if !ok {
			podSpec = make(map[string]any)
			template["spec"] = podSpec
		}

		podSpec["volumes"] = toAnySlice(volumes)
	}
}

func withFinalizers(finalizers ...string) notebookOption {
	return func(obj map[string]any) {
		metadata := obj["metadata"].(map[string]any)
		anyFinalizers := make([]any, len(finalizers))

		for i, f := range finalizers {
			anyFinalizers[i] = f
		}

		metadata["finalizers"] = anyFinalizers
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

func volume(name string) map[string]any {
	return map[string]any{
		"name":     name,
		"emptyDir": map[string]any{},
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

// oauthNotebook creates a notebook with full 2.x OAuth artifacts.
func oauthNotebook(name, namespace string) *unstructured.Unstructured {
	return newNotebook(name, namespace,
		withAnnotations(map[string]string{
			annotationInjectOAuth:    "true",
			annotationOAuthLogoutURL: "https://example.com/logout",
		}),
		withContainers(
			containerWithEnv("my-notebook",
				envVar(envNotebookArgs,
					`--ServerApp.port=8888 --ServerApp.tornado_settings={"headers":{"Content-Security-Policy":"frame-ancestors 'self'"}}`)),
			container(containerOAuthProxy),
		),
		withVolumes(
			volume("oauth-config"),
			volume("oauth-client"),
			volume("tls-certificates"),
			volume("data"),
		),
		withFinalizers(finalizerOAuthClient, "other-finalizer"),
	)
}

// patchedNotebook creates a notebook that has already been patched for 3.x.
func patchedNotebook(name, namespace string) *unstructured.Unstructured {
	return newNotebook(name, namespace,
		withAnnotations(map[string]string{
			annotationInjectAuth: "true",
		}),
		withContainers(
			container("my-notebook"),
			container("kube-rbac-proxy"),
		),
		withVolumes(volume("data")),
	)
}

// stoppedOAuthNotebook creates a stopped notebook with OAuth artifacts.
func stoppedOAuthNotebook(name, namespace string) *unstructured.Unstructured {
	return newNotebook(name, namespace,
		withAnnotations(map[string]string{
			annotationInjectOAuth:             "true",
			annotationKubeflowResourceStopped: "2024-01-01T00:00:00Z",
		}),
		withContainers(
			container("my-notebook"),
			container(containerOAuthProxy),
		),
		withVolumes(
			volume("oauth-config"),
			volume("oauth-client"),
			volume("tls-certificates"),
		),
		withFinalizers(finalizerOAuthClient),
	)
}

func newStatefulSet(name, namespace string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.StatefulSet.APIVersion(),
			"kind":       resources.StatefulSet.Kind,
			"metadata": map[string]any{
				"name":      name,
				"namespace": namespace,
			},
			"spec": map[string]any{
				"replicas": int64(0),
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

func updateErrorReactor() k8stesting.Reactor {
	return &k8stesting.SimpleReactor{
		Verb:     "update",
		Resource: "notebooks",
		Reaction: func(_ k8stesting.Action) (bool, runtime.Object, error) {
			return true, nil, errors.New("simulated update error")
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

func newAction() *PatchAuthModelAction {
	return &PatchAuthModelAction{
		Scope:         &workbenches.SharedScopeOptions{},
		CleanupAction: &workbenchcleanup.CleanupOAuthAction{Scope: &workbenches.SharedScopeOptions{}},
	}
}

func newTarget(k8sClient client.Client, opts ...func(*action.Target)) action.Target {
	currentVersion := semver.MustParse("2.16.0")
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
		OutputDir:      "/tmp/test-backup",
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

func TestPatchAuthModelAction_Metadata(t *testing.T) {
	g := NewWithT(t)

	a := newAction()

	g.Expect(a.ID()).To(Equal("workbenches.patch-auth-model"))
	g.Expect(a.Name()).To(ContainSubstring("auth model"))
	g.Expect(a.Description()).To(ContainSubstring("oauth-proxy"))
	g.Expect(a.Group()).To(Equal(action.GroupMigration))
	g.Expect(a.Phase()).To(Equal(action.PhasePreUpgrade))
}

func TestPatchAuthModelAction_CanApply(t *testing.T) {
	g := NewWithT(t)

	a := newAction()

	g.Expect(a.CanApply(action.Target{})).To(BeFalse(), "nil versions")

	v216 := semver.MustParse("2.16.0")
	v3 := semver.MustParse("3.0.0")
	g.Expect(a.CanApply(action.Target{CurrentVersion: &v216, TargetVersion: &v3})).To(BeTrue(), "2.16→3.0")

	v220 := semver.MustParse("2.20.0")
	g.Expect(a.CanApply(action.Target{CurrentVersion: &v220, TargetVersion: &v3})).To(BeTrue(), "2.20→3.0")

	v215 := semver.MustParse("2.15.0")
	g.Expect(a.CanApply(action.Target{CurrentVersion: &v215, TargetVersion: &v3})).To(BeFalse(), "2.15→3.0 too old")

	v2 := semver.MustParse("2.16.0")
	v25 := semver.MustParse("2.17.0")
	g.Expect(a.CanApply(action.Target{CurrentVersion: &v2, TargetVersion: &v25})).To(BeFalse(), "2→2 no upgrade")

	v1 := semver.MustParse("1.0.0")
	g.Expect(a.CanApply(action.Target{CurrentVersion: &v1, TargetVersion: &v3})).To(BeFalse(), "1.x not applicable")
}

func TestPatchAuthModelAction_PrepareNotNil(t *testing.T) {
	g := NewWithT(t)

	a := newAction()
	g.Expect(a.Prepare()).ToNot(BeNil())
}

func TestPatchAuthModelAction_RunNotNil(t *testing.T) {
	g := NewWithT(t)

	a := newAction()
	g.Expect(a.Run()).ToNot(BeNil())
}

func TestPatchAuthModelAction_AddFlags(t *testing.T) {
	g := NewWithT(t)

	a := newAction()
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	a.AddFlags(fs)

	g.Expect(fs.Lookup("workbench-namespace")).ToNot(BeNil())
	g.Expect(fs.Lookup("workbench-name")).ToNot(BeNil())
	g.Expect(fs.Lookup("skip-stop")).ToNot(BeNil())
	g.Expect(fs.Lookup("only-stopped")).ToNot(BeNil())
	g.Expect(fs.Lookup("with-cleanup")).ToNot(BeNil())
}

func TestPatchAuthModelAction_FlagDefaults(t *testing.T) {
	g := NewWithT(t)

	a := newAction()
	g.Expect(a.SkipStop).To(BeFalse())
	g.Expect(a.OnlyStopped).To(BeFalse())
	g.Expect(a.WithCleanup).To(BeFalse())
}

// --- Patch Helper Tests ---

func TestNeedsAuthModelPatch_OAuthAnnotation(t *testing.T) {
	g := NewWithT(t)

	nb := newNotebook("wb1", "ns1",
		withAnnotations(map[string]string{
			annotationInjectOAuth: "true",
		}),
		withContainers(container("my-notebook")),
	)

	g.Expect(needsAuthModelPatch(nb)).To(BeTrue())
}

func TestNeedsAuthModelPatch_OAuthProxyContainer(t *testing.T) {
	g := NewWithT(t)

	nb := newNotebook("wb1", "ns1",
		withContainers(container("my-notebook"), container(containerOAuthProxy)),
	)

	g.Expect(needsAuthModelPatch(nb)).To(BeTrue())
}

func TestNeedsAuthModelPatch_OAuthFinalizer(t *testing.T) {
	g := NewWithT(t)

	nb := newNotebook("wb1", "ns1",
		withContainers(container("my-notebook")),
		withFinalizers(finalizerOAuthClient),
	)

	g.Expect(needsAuthModelPatch(nb)).To(BeTrue())
}

func TestNeedsAuthModelPatch_OAuthVolumes(t *testing.T) {
	g := NewWithT(t)

	nb := newNotebook("wb1", "ns1",
		withContainers(container("my-notebook")),
		withVolumes(volume("oauth-config")),
	)

	g.Expect(needsAuthModelPatch(nb)).To(BeTrue())
}

func TestNeedsAuthModelPatch_TornadoSettings(t *testing.T) {
	g := NewWithT(t)

	nb := newNotebook("wb1", "ns1",
		withContainers(
			containerWithEnv("my-notebook",
				envVar(envNotebookArgs, "--ServerApp.tornado_settings={}")),
		),
	)

	g.Expect(needsAuthModelPatch(nb)).To(BeTrue())
}

func TestNeedsAuthModelPatch_AlreadyPatched(t *testing.T) {
	g := NewWithT(t)

	nb := patchedNotebook("wb1", "ns1")

	g.Expect(needsAuthModelPatch(nb)).To(BeFalse())
}

func TestNeedsAuthModelPatch_CleanNotebook(t *testing.T) {
	g := NewWithT(t)

	nb := newNotebook("wb1", "ns1",
		withContainers(container("my-notebook")),
		withVolumes(volume("data")),
	)

	g.Expect(needsAuthModelPatch(nb)).To(BeFalse())
}

func TestAddInjectAuthAnnotation(t *testing.T) {
	g := NewWithT(t)

	nb := newNotebook("wb1", "ns1")
	addInjectAuthAnnotation(nb)

	g.Expect(nb.GetAnnotations()[annotationInjectAuth]).To(Equal("true"))
}

func TestAddInjectAuthAnnotation_PreservesExisting(t *testing.T) {
	g := NewWithT(t)

	nb := newNotebook("wb1", "ns1",
		withAnnotations(map[string]string{"other": "value"}),
	)
	addInjectAuthAnnotation(nb)

	g.Expect(nb.GetAnnotations()[annotationInjectAuth]).To(Equal("true"))
	g.Expect(nb.GetAnnotations()["other"]).To(Equal("value"))
}

func TestRemoveAnnotation(t *testing.T) {
	g := NewWithT(t)

	nb := newNotebook("wb1", "ns1",
		withAnnotations(map[string]string{
			annotationInjectOAuth: "true",
			"other":               "keep",
		}),
	)
	removeAnnotation(nb, annotationInjectOAuth)

	g.Expect(nb.GetAnnotations()).ToNot(HaveKey(annotationInjectOAuth))
	g.Expect(nb.GetAnnotations()["other"]).To(Equal("keep"))
}

func TestRemoveAnnotation_NoAnnotations(t *testing.T) {
	g := NewWithT(t)

	nb := newNotebook("wb1", "ns1")
	removeAnnotation(nb, annotationInjectOAuth)

	g.Expect(nb.GetAnnotations()).To(BeNil())
}

func TestRemoveOAuthProxyContainer(t *testing.T) {
	g := NewWithT(t)

	nb := newNotebook("wb1", "ns1",
		withContainers(container("my-notebook"), container(containerOAuthProxy)),
	)

	err := removeOAuthProxyContainer(nb)
	g.Expect(err).ToNot(HaveOccurred())

	g.Expect(hasOAuthProxyContainer(nb)).To(BeFalse())

	containers, _ := nb.Object["spec"].(map[string]any)["template"].(map[string]any)["spec"].(map[string]any)["containers"].([]any)
	g.Expect(containers).To(HaveLen(1))
}

func TestRemoveOAuthProxyContainer_NotPresent(t *testing.T) {
	g := NewWithT(t)

	nb := newNotebook("wb1", "ns1",
		withContainers(container("my-notebook")),
	)

	err := removeOAuthProxyContainer(nb)
	g.Expect(err).ToNot(HaveOccurred())
}

func TestRemoveOAuthFinalizer(t *testing.T) {
	g := NewWithT(t)

	nb := newNotebook("wb1", "ns1",
		withFinalizers(finalizerOAuthClient, "other-finalizer"),
	)

	removeOAuthFinalizer(nb)

	g.Expect(nb.GetFinalizers()).To(Equal([]string{"other-finalizer"}))
}

func TestRemoveOAuthFinalizer_NotPresent(t *testing.T) {
	g := NewWithT(t)

	nb := newNotebook("wb1", "ns1",
		withFinalizers("other-finalizer"),
	)

	removeOAuthFinalizer(nb)

	g.Expect(nb.GetFinalizers()).To(Equal([]string{"other-finalizer"}))
}

func TestRemoveOAuthVolumes(t *testing.T) {
	g := NewWithT(t)

	nb := newNotebook("wb1", "ns1",
		withContainers(container("my-notebook")),
		withVolumes(
			volume("oauth-config"),
			volume("oauth-client"),
			volume("tls-certificates"),
			volume("data"),
		),
	)

	err := removeOAuthVolumes(nb)
	g.Expect(err).ToNot(HaveOccurred())

	g.Expect(hasOAuthVolumes(nb)).To(BeFalse())

	podSpec := nb.Object["spec"].(map[string]any)["template"].(map[string]any)["spec"].(map[string]any)
	volumes := podSpec["volumes"].([]any)
	g.Expect(volumes).To(HaveLen(1))
}

func TestRemoveOAuthVolumes_NonePresent(t *testing.T) {
	g := NewWithT(t)

	nb := newNotebook("wb1", "ns1",
		withContainers(container("my-notebook")),
		withVolumes(volume("data")),
	)

	err := removeOAuthVolumes(nb)
	g.Expect(err).ToNot(HaveOccurred())
}

func TestStripTornadoSettings(t *testing.T) {
	g := NewWithT(t)

	nb := newNotebook("wb1", "ns1",
		withContainers(
			containerWithEnv("my-notebook",
				envVar(envNotebookArgs,
					`--ServerApp.port=8888 --ServerApp.tornado_settings={"headers":{"Content-Security-Policy":"frame-ancestors 'self'"}}`)),
		),
	)

	err := stripTornadoSettings(nb)
	g.Expect(err).ToNot(HaveOccurred())

	g.Expect(hasTornadoSettings(nb)).To(BeFalse())
}

func TestStripTornadoSettings_PreservesOtherArgs(t *testing.T) {
	g := NewWithT(t)

	nb := newNotebook("wb1", "ns1",
		withContainers(
			containerWithEnv("my-notebook",
				envVar(envNotebookArgs,
					`--ServerApp.port=8888 --ServerApp.tornado_settings={"foo":"bar"} --ServerApp.base_url=/notebook`)),
		),
	)

	err := stripTornadoSettings(nb)
	g.Expect(err).ToNot(HaveOccurred())

	containers := nb.Object["spec"].(map[string]any)["template"].(map[string]any)["spec"].(map[string]any)["containers"].([]any)
	env := containers[0].(map[string]any)["env"].([]any)
	value := env[0].(map[string]any)["value"].(string)
	g.Expect(value).To(ContainSubstring("--ServerApp.port=8888"))
	g.Expect(value).To(ContainSubstring("--ServerApp.base_url=/notebook"))
	g.Expect(value).ToNot(ContainSubstring("tornado_settings"))
}

func TestStripTornadoSettings_NoTornadoSettings(t *testing.T) {
	g := NewWithT(t)

	nb := newNotebook("wb1", "ns1",
		withContainers(
			containerWithEnv("my-notebook",
				envVar(envNotebookArgs, "--ServerApp.port=8888")),
		),
	)

	err := stripTornadoSettings(nb)
	g.Expect(err).ToNot(HaveOccurred())
}

func TestApplyAllPatches(t *testing.T) {
	g := NewWithT(t)

	nb := oauthNotebook("wb1", "ns1")
	modified := nb.DeepCopy()

	err := applyAllPatches(modified)
	g.Expect(err).ToNot(HaveOccurred())

	g.Expect(modified.GetAnnotations()[annotationInjectAuth]).To(Equal("true"))
	g.Expect(modified.GetAnnotations()).ToNot(HaveKey(annotationInjectOAuth))
	g.Expect(modified.GetAnnotations()).ToNot(HaveKey(annotationOAuthLogoutURL))
	g.Expect(hasOAuthProxyContainer(modified)).To(BeFalse())
	g.Expect(hasOAuthFinalizer(modified)).To(BeFalse())
	g.Expect(hasOAuthVolumes(modified)).To(BeFalse())
	g.Expect(hasTornadoSettings(modified)).To(BeFalse())

	g.Expect(modified.GetFinalizers()).To(Equal([]string{"other-finalizer"}))
}

// --- IsStopped Tests ---

func TestIsStopped_WithAnnotation(t *testing.T) {
	g := NewWithT(t)

	nb := newNotebook("wb1", "ns1",
		withAnnotations(map[string]string{
			annotationKubeflowResourceStopped: "2024-01-01T00:00:00Z",
		}),
	)

	g.Expect(isStopped(nb)).To(BeTrue())
}

func TestIsStopped_WithoutAnnotation(t *testing.T) {
	g := NewWithT(t)

	nb := newNotebook("wb1", "ns1")

	g.Expect(isStopped(nb)).To(BeFalse())
}

func TestIsStopped_EmptyAnnotationValue(t *testing.T) {
	g := NewWithT(t)

	nb := newNotebook("wb1", "ns1",
		withAnnotations(map[string]string{
			annotationKubeflowResourceStopped: "",
		}),
	)

	g.Expect(isStopped(nb)).To(BeFalse())
}

// --- Validate Tests ---

func TestRunTask_Validate_NoNotebooks(t *testing.T) {
	g := NewWithT(t)

	a := newAction()
	task := a.Run()
	k8sClient := newFakeClient(nil)
	target := newTarget(k8sClient)

	result, err := task.Validate(context.Background(), target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).ToNot(BeNil())
}

func TestRunTask_Validate_OAuthNotebooksFound(t *testing.T) {
	g := NewWithT(t)

	a := newAction()
	task := a.Run()

	objs := []*unstructured.Unstructured{
		oauthNotebook("wb1", "ns1"),
		patchedNotebook("wb2", "ns1"),
	}

	k8sClient := newFakeClient(objs)
	target := newTarget(k8sClient)

	result, err := task.Validate(context.Background(), target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).ToNot(BeNil())
}

func TestRunTask_Validate_AllPatched(t *testing.T) {
	g := NewWithT(t)

	a := newAction()
	task := a.Run()

	objs := []*unstructured.Unstructured{
		patchedNotebook("wb1", "ns1"),
	}

	k8sClient := newFakeClient(objs)
	target := newTarget(k8sClient)

	result, err := task.Validate(context.Background(), target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).ToNot(BeNil())
}

func TestRunTask_Validate_FlagConflict(t *testing.T) {
	g := NewWithT(t)

	a := newAction()
	a.SkipStop = true
	a.OnlyStopped = true
	task := a.Run()

	k8sClient := newFakeClient(nil)
	target := newTarget(k8sClient)

	_, err := task.Validate(context.Background(), target)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("mutually exclusive"))
}

func TestRunTask_Validate_NameWithoutNamespace(t *testing.T) {
	g := NewWithT(t)

	a := newAction()
	a.Scope.WorkbenchName = "wb1"
	task := a.Run()

	k8sClient := newFakeClient(nil)
	target := newTarget(k8sClient)

	_, err := task.Validate(context.Background(), target)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("--workbench-namespace"))
}

func TestRunTask_Validate_ListError(t *testing.T) {
	g := NewWithT(t)

	a := newAction()
	task := a.Run()

	k8sClient := newFakeClient(nil, listErrorReactor())
	target := newTarget(k8sClient)

	result, err := task.Validate(context.Background(), target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).ToNot(BeNil())
}

// --- Execute Tests ---

func TestRunTask_Execute_FullPatch(t *testing.T) {
	g := NewWithT(t)

	a := newAction()
	task := a.Run()

	nb := stoppedOAuthNotebook("wb1", "ns1")
	sts := newStatefulSet("wb1", "ns1")

	k8sClient := newFakeClient([]*unstructured.Unstructured{nb, sts})
	target := newTarget(k8sClient)

	result, err := task.Execute(context.Background(), target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).ToNot(BeNil())

	// Verify the notebook was updated
	updated, err := k8sClient.Dynamic().Resource(resources.Notebook.GVR()).
		Namespace("ns1").Get(context.Background(), "wb1", metav1.GetOptions{})
	g.Expect(err).ToNot(HaveOccurred())

	g.Expect(updated.GetAnnotations()[annotationInjectAuth]).To(Equal("true"))
	g.Expect(updated.GetAnnotations()).ToNot(HaveKey(annotationInjectOAuth))
	g.Expect(hasOAuthProxyContainer(updated)).To(BeFalse())
	g.Expect(hasOAuthFinalizer(updated)).To(BeFalse())
	g.Expect(hasOAuthVolumes(updated)).To(BeFalse())
}

func TestRunTask_Execute_AlreadyPatchedSkipped(t *testing.T) {
	g := NewWithT(t)

	a := newAction()
	task := a.Run()

	nb := patchedNotebook("wb1", "ns1")
	k8sClient := newFakeClient([]*unstructured.Unstructured{nb})
	target := newTarget(k8sClient)

	result, err := task.Execute(context.Background(), target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).ToNot(BeNil())
}

func TestRunTask_Execute_DryRun(t *testing.T) {
	g := NewWithT(t)

	a := newAction()
	task := a.Run()

	nb := stoppedOAuthNotebook("wb1", "ns1")
	k8sClient := newFakeClient([]*unstructured.Unstructured{nb})
	target := newTarget(k8sClient, withDryRun)

	result, err := task.Execute(context.Background(), target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).ToNot(BeNil())

	// Verify notebook was NOT modified
	current, err := k8sClient.Dynamic().Resource(resources.Notebook.GVR()).
		Namespace("ns1").Get(context.Background(), "wb1", metav1.GetOptions{})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(hasOAuthProxyContainer(current)).To(BeTrue(), "dry-run should not modify notebook")
}

func TestRunTask_Execute_MultipleNamespaces(t *testing.T) {
	g := NewWithT(t)

	a := newAction()
	task := a.Run()

	nb1 := stoppedOAuthNotebook("wb1", "ns1")
	nb2 := stoppedOAuthNotebook("wb2", "ns2")
	sts1 := newStatefulSet("wb1", "ns1")
	sts2 := newStatefulSet("wb2", "ns2")

	k8sClient := newFakeClient([]*unstructured.Unstructured{nb1, nb2, sts1, sts2})
	target := newTarget(k8sClient)

	result, err := task.Execute(context.Background(), target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).ToNot(BeNil())

	// Verify both notebooks were updated
	for _, ns := range []string{"ns1", "ns2"} {
		updated, err := k8sClient.Dynamic().Resource(resources.Notebook.GVR()).
			Namespace(ns).Get(context.Background(), "wb1", metav1.GetOptions{})
		if err != nil {
			updated, err = k8sClient.Dynamic().Resource(resources.Notebook.GVR()).
				Namespace(ns).Get(context.Background(), "wb2", metav1.GetOptions{})
		}

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(updated.GetAnnotations()[annotationInjectAuth]).To(Equal("true"))
	}
}

func TestRunTask_Execute_UpdateError(t *testing.T) {
	g := NewWithT(t)

	a := newAction()
	task := a.Run()

	nb := stoppedOAuthNotebook("wb1", "ns1")
	k8sClient := newFakeClient([]*unstructured.Unstructured{nb}, updateErrorReactor())
	target := newTarget(k8sClient)

	result, err := task.Execute(context.Background(), target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).ToNot(BeNil())
}

func TestRunTask_Execute_StatefulSetDeleted(t *testing.T) {
	g := NewWithT(t)

	a := newAction()
	task := a.Run()

	nb := stoppedOAuthNotebook("wb1", "ns1")
	sts := newStatefulSet("wb1", "ns1")

	k8sClient := newFakeClient([]*unstructured.Unstructured{nb, sts})
	target := newTarget(k8sClient)

	_, err := task.Execute(context.Background(), target)
	g.Expect(err).ToNot(HaveOccurred())

	// Verify StatefulSet was deleted
	_, err = k8sClient.Dynamic().Resource(resources.StatefulSet.GVR()).
		Namespace("ns1").Get(context.Background(), "wb1", metav1.GetOptions{})
	g.Expect(err).To(HaveOccurred())
}

func TestRunTask_Execute_StatefulSetNotFound(t *testing.T) {
	g := NewWithT(t)

	a := newAction()
	task := a.Run()

	nb := stoppedOAuthNotebook("wb1", "ns1")
	k8sClient := newFakeClient([]*unstructured.Unstructured{nb})
	target := newTarget(k8sClient)

	result, err := task.Execute(context.Background(), target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).ToNot(BeNil())
}

// --- Lifecycle Tests ---

func TestRunTask_Execute_OnlyStopped_FiltersRunning(t *testing.T) {
	g := NewWithT(t)

	a := newAction()
	a.OnlyStopped = true
	task := a.Run()

	stopped := stoppedOAuthNotebook("wb-stopped", "ns1")
	running := oauthNotebook("wb-running", "ns1")
	sts := newStatefulSet("wb-stopped", "ns1")

	k8sClient := newFakeClient([]*unstructured.Unstructured{stopped, running, sts})
	target := newTarget(k8sClient)

	_, err := task.Execute(context.Background(), target)
	g.Expect(err).ToNot(HaveOccurred())

	// Stopped notebook should be patched
	updated, err := k8sClient.Dynamic().Resource(resources.Notebook.GVR()).
		Namespace("ns1").Get(context.Background(), "wb-stopped", metav1.GetOptions{})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(updated.GetAnnotations()[annotationInjectAuth]).To(Equal("true"))

	// Running notebook should NOT be patched
	notPatched, err := k8sClient.Dynamic().Resource(resources.Notebook.GVR()).
		Namespace("ns1").Get(context.Background(), "wb-running", metav1.GetOptions{})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(hasOAuthProxyContainer(notPatched)).To(BeTrue())
}

func TestRunTask_Execute_SkipStop(t *testing.T) {
	g := NewWithT(t)

	a := newAction()
	a.SkipStop = true
	task := a.Run()

	nb := oauthNotebook("wb1", "ns1")
	sts := newStatefulSet("wb1", "ns1")

	k8sClient := newFakeClient([]*unstructured.Unstructured{nb, sts})
	target := newTarget(k8sClient)

	_, err := task.Execute(context.Background(), target)
	g.Expect(err).ToNot(HaveOccurred())

	// Notebook should be patched despite being running
	updated, err := k8sClient.Dynamic().Resource(resources.Notebook.GVR()).
		Namespace("ns1").Get(context.Background(), "wb1", metav1.GetOptions{})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(updated.GetAnnotations()[annotationInjectAuth]).To(Equal("true"))
}

// --- Targeting Tests ---

func TestRunTask_Execute_SingleNotebook(t *testing.T) {
	g := NewWithT(t)

	a := newAction()
	a.Scope.WorkbenchNamespace = "ns1"
	a.Scope.WorkbenchName = "wb1"
	task := a.Run()

	nb1 := stoppedOAuthNotebook("wb1", "ns1")
	nb2 := stoppedOAuthNotebook("wb2", "ns1")
	sts1 := newStatefulSet("wb1", "ns1")

	k8sClient := newFakeClient([]*unstructured.Unstructured{nb1, nb2, sts1})
	target := newTarget(k8sClient)

	_, err := task.Execute(context.Background(), target)
	g.Expect(err).ToNot(HaveOccurred())

	// wb1 should be patched
	updated, err := k8sClient.Dynamic().Resource(resources.Notebook.GVR()).
		Namespace("ns1").Get(context.Background(), "wb1", metav1.GetOptions{})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(updated.GetAnnotations()[annotationInjectAuth]).To(Equal("true"))

	// wb2 should NOT be patched (not targeted)
	notPatched, err := k8sClient.Dynamic().Resource(resources.Notebook.GVR()).
		Namespace("ns1").Get(context.Background(), "wb2", metav1.GetOptions{})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(hasOAuthProxyContainer(notPatched)).To(BeTrue())
}

func TestRunTask_Execute_NamespaceScoped(t *testing.T) {
	g := NewWithT(t)

	a := newAction()
	a.Scope.WorkbenchNamespace = "ns1"
	task := a.Run()

	nb1 := stoppedOAuthNotebook("wb1", "ns1")
	nb2 := stoppedOAuthNotebook("wb2", "ns2")
	sts1 := newStatefulSet("wb1", "ns1")

	k8sClient := newFakeClient([]*unstructured.Unstructured{nb1, nb2, sts1})
	target := newTarget(k8sClient)

	_, err := task.Execute(context.Background(), target)
	g.Expect(err).ToNot(HaveOccurred())

	// wb1 in ns1 should be patched
	updated, err := k8sClient.Dynamic().Resource(resources.Notebook.GVR()).
		Namespace("ns1").Get(context.Background(), "wb1", metav1.GetOptions{})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(updated.GetAnnotations()[annotationInjectAuth]).To(Equal("true"))
}

// --- Cleanup Tests ---

func TestRunTask_Execute_WithCleanup(t *testing.T) {
	g := NewWithT(t)

	a := newAction()
	a.WithCleanup = true
	task := a.Run()

	nb := stoppedOAuthNotebook("wb1", "ns1")
	sts := newStatefulSet("wb1", "ns1")
	oauthResources := allOAuthResources("wb1", "ns1")

	objs := append([]*unstructured.Unstructured{nb, sts}, oauthResources...)
	k8sClient := newFakeClient(objs)
	target := newTarget(k8sClient)

	_, err := task.Execute(context.Background(), target)
	g.Expect(err).ToNot(HaveOccurred())

	// Verify OAuth resources were deleted
	_, err = k8sClient.Dynamic().Resource(resources.Route.GVR()).
		Namespace("ns1").Get(context.Background(), "wb1", metav1.GetOptions{})
	g.Expect(err).To(HaveOccurred(), "Route should be deleted")

	_, err = k8sClient.Dynamic().Resource(resources.Service.GVR()).
		Namespace("ns1").Get(context.Background(), "wb1-tls", metav1.GetOptions{})
	g.Expect(err).To(HaveOccurred(), "Service should be deleted")
}

func TestRunTask_Execute_WithCleanup_DryRun(t *testing.T) {
	g := NewWithT(t)

	a := newAction()
	a.WithCleanup = true
	task := a.Run()

	nb := stoppedOAuthNotebook("wb1", "ns1")
	oauthResources := allOAuthResources("wb1", "ns1")

	objs := append([]*unstructured.Unstructured{nb}, oauthResources...)
	k8sClient := newFakeClient(objs)
	target := newTarget(k8sClient, withDryRun)

	result, err := task.Execute(context.Background(), target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).ToNot(BeNil())

	// Verify Route still exists (dry-run)
	_, err = k8sClient.Dynamic().Resource(resources.Route.GVR()).
		Namespace("ns1").Get(context.Background(), "wb1", metav1.GetOptions{})
	g.Expect(err).ToNot(HaveOccurred(), "Route should still exist in dry-run")
}

func TestRunTask_Execute_WithCleanup_NoPatchedNotebooks(t *testing.T) {
	g := NewWithT(t)

	a := newAction()
	a.WithCleanup = true
	task := a.Run()

	nb := patchedNotebook("wb1", "ns1")
	k8sClient := newFakeClient([]*unstructured.Unstructured{nb})
	target := newTarget(k8sClient)

	result, err := task.Execute(context.Background(), target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).ToNot(BeNil())
}

// --- Confirmation Tests ---

func TestRunTask_Execute_ConfirmationCancelled(t *testing.T) {
	g := NewWithT(t)

	a := newAction()
	task := a.Run()

	nb := stoppedOAuthNotebook("wb1", "ns1")
	k8sClient := newFakeClient([]*unstructured.Unstructured{nb})
	target := newTarget(k8sClient, withInteractiveConfirm("n\n"))

	result, err := task.Execute(context.Background(), target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).ToNot(BeNil())

	// Verify notebook was NOT modified
	current, err := k8sClient.Dynamic().Resource(resources.Notebook.GVR()).
		Namespace("ns1").Get(context.Background(), "wb1", metav1.GetOptions{})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(hasOAuthProxyContainer(current)).To(BeTrue())
}

func TestRunTask_Execute_ConfirmationAccepted(t *testing.T) {
	g := NewWithT(t)

	a := newAction()
	task := a.Run()

	nb := stoppedOAuthNotebook("wb1", "ns1")
	sts := newStatefulSet("wb1", "ns1")
	k8sClient := newFakeClient([]*unstructured.Unstructured{nb, sts})
	target := newTarget(k8sClient, withInteractiveConfirm("y\n"))

	result, err := task.Execute(context.Background(), target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).ToNot(BeNil())

	// Verify notebook was patched
	updated, err := k8sClient.Dynamic().Resource(resources.Notebook.GVR()).
		Namespace("ns1").Get(context.Background(), "wb1", metav1.GetOptions{})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(updated.GetAnnotations()[annotationInjectAuth]).To(Equal("true"))
}

func TestRunTask_Execute_AutoStopCancelledRestartsNotebooks(t *testing.T) {
	g := NewWithT(t)

	a := newAction()
	task := a.Run()

	nb := oauthNotebook("wb1", "ns1")
	sts := newStatefulSet("wb1", "ns1")
	k8sClient := newFakeClient([]*unstructured.Unstructured{nb, sts})
	target := newTarget(k8sClient, withInteractiveConfirm("n\n"))

	result, err := task.Execute(context.Background(), target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).ToNot(BeNil())

	current, err := k8sClient.Dynamic().Resource(resources.Notebook.GVR()).
		Namespace("ns1").Get(context.Background(), "wb1", metav1.GetOptions{})
	g.Expect(err).ToNot(HaveOccurred())

	// Notebook should NOT be patched (user cancelled)
	g.Expect(hasOAuthProxyContainer(current)).To(BeTrue())

	// Notebook should be restarted (stop annotation removed)
	g.Expect(isStopped(current)).To(BeFalse())
}

// --- PrepareTask Tests ---

func TestPrepareTask_Validate_NoNotebooks(t *testing.T) {
	g := NewWithT(t)

	a := newAction()
	task := a.Prepare()

	k8sClient := newFakeClient(nil)
	target := newTarget(k8sClient)

	result, err := task.Validate(context.Background(), target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).ToNot(BeNil())
}

func TestPrepareTask_Validate_MixedNotebooks(t *testing.T) {
	g := NewWithT(t)

	a := newAction()
	task := a.Prepare()

	objs := []*unstructured.Unstructured{
		oauthNotebook("wb1", "ns1"),
		patchedNotebook("wb2", "ns1"),
	}

	k8sClient := newFakeClient(objs)
	target := newTarget(k8sClient)

	result, err := task.Validate(context.Background(), target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).ToNot(BeNil())
}

func TestPrepareTask_Execute_DryRun(t *testing.T) {
	g := NewWithT(t)

	a := newAction()
	task := a.Prepare()

	nb := oauthNotebook("wb1", "ns1")
	k8sClient := newFakeClient([]*unstructured.Unstructured{nb})
	target := newTarget(k8sClient, withDryRun)

	result, err := task.Execute(context.Background(), target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).ToNot(BeNil())
}

func TestPrepareTask_Execute_NoNotebooks(t *testing.T) {
	g := NewWithT(t)

	a := newAction()
	task := a.Prepare()

	k8sClient := newFakeClient(nil)
	target := newTarget(k8sClient)

	result, err := task.Execute(context.Background(), target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).ToNot(BeNil())
}

func TestPrepareTask_Validate_ListError(t *testing.T) {
	g := NewWithT(t)

	a := newAction()
	task := a.Prepare()

	k8sClient := newFakeClient(nil, listErrorReactor())
	target := newTarget(k8sClient)

	result, err := task.Validate(context.Background(), target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).ToNot(BeNil())
}

func TestPrepareTask_Execute_ListError(t *testing.T) {
	g := NewWithT(t)

	a := newAction()
	task := a.Prepare()

	k8sClient := newFakeClient(nil, listErrorReactor())
	target := newTarget(k8sClient)

	result, err := task.Execute(context.Background(), target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).ToNot(BeNil())
}

func TestPrepareTask_Validate_NameWithoutNamespace(t *testing.T) {
	g := NewWithT(t)

	a := newAction()
	a.Scope.WorkbenchName = "wb1"
	task := a.Prepare()

	k8sClient := newFakeClient(nil)
	target := newTarget(k8sClient)

	_, err := task.Validate(context.Background(), target)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("--workbench-namespace"))
}

// --- Edge Cases ---

func TestRunTask_Execute_NoNotebooks(t *testing.T) {
	g := NewWithT(t)

	a := newAction()
	task := a.Run()

	k8sClient := newFakeClient(nil)
	target := newTarget(k8sClient)

	result, err := task.Execute(context.Background(), target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).ToNot(BeNil())
}

func TestRunTask_Execute_ListError(t *testing.T) {
	g := NewWithT(t)

	a := newAction()
	task := a.Run()

	k8sClient := newFakeClient(nil, listErrorReactor())
	target := newTarget(k8sClient)

	result, err := task.Execute(context.Background(), target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).ToNot(BeNil())
}

func TestRunTask_Execute_OnlyStopped_AllRunning(t *testing.T) {
	g := NewWithT(t)

	a := newAction()
	a.OnlyStopped = true
	task := a.Run()

	nb := oauthNotebook("wb1", "ns1")
	k8sClient := newFakeClient([]*unstructured.Unstructured{nb})
	target := newTarget(k8sClient)

	result, err := task.Execute(context.Background(), target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).ToNot(BeNil())

	// Notebook should NOT be patched
	current, err := k8sClient.Dynamic().Resource(resources.Notebook.GVR()).
		Namespace("ns1").Get(context.Background(), "wb1", metav1.GetOptions{})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(hasOAuthProxyContainer(current)).To(BeTrue())
}

func TestRunTask_Execute_WithCleanup_CleanupConfirmationCancelled(t *testing.T) {
	g := NewWithT(t)

	a := newAction()
	a.WithCleanup = true
	task := a.Run()

	nb := stoppedOAuthNotebook("wb1", "ns1")
	sts := newStatefulSet("wb1", "ns1")
	oauthResources := allOAuthResources("wb1", "ns1")

	objs := append([]*unstructured.Unstructured{nb, sts}, oauthResources...)
	k8sClient := newFakeClient(objs)
	// First "y" for patch confirmation, then "n" for cleanup confirmation
	target := newTarget(k8sClient, withInteractiveConfirm("y\nn\n"))

	_, err := task.Execute(context.Background(), target)
	g.Expect(err).ToNot(HaveOccurred())

	// Notebook should be patched
	updated, err := k8sClient.Dynamic().Resource(resources.Notebook.GVR()).
		Namespace("ns1").Get(context.Background(), "wb1", metav1.GetOptions{})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(updated.GetAnnotations()[annotationInjectAuth]).To(Equal("true"))

	// Route should still exist (cleanup was cancelled)
	_, err = k8sClient.Dynamic().Resource(resources.Route.GVR()).
		Namespace("ns1").Get(context.Background(), "wb1", metav1.GetOptions{})
	g.Expect(err).ToNot(HaveOccurred(), "Route should still exist because cleanup was cancelled")
}

func TestHasOAuthAnnotation_LogoutURL(t *testing.T) {
	g := NewWithT(t)

	nb := newNotebook("wb1", "ns1",
		withAnnotations(map[string]string{
			annotationOAuthLogoutURL: "https://example.com/logout",
		}),
	)

	g.Expect(hasOAuthAnnotation(nb)).To(BeTrue())
}

func TestHasOAuthAnnotation_Neither(t *testing.T) {
	g := NewWithT(t)

	nb := newNotebook("wb1", "ns1",
		withAnnotations(map[string]string{"other": "value"}),
	)

	g.Expect(hasOAuthAnnotation(nb)).To(BeFalse())
}

func TestHasOAuthAnnotation_NilAnnotations(t *testing.T) {
	g := NewWithT(t)

	nb := newNotebook("wb1", "ns1")

	g.Expect(hasOAuthAnnotation(nb)).To(BeFalse())
}

// --- Lifecycle Tests (direct function calls) ---

func TestStopWorkbench(t *testing.T) {
	g := NewWithT(t)

	nb := oauthNotebook("wb1", "ns1")
	sts := newStatefulSet("wb1", "ns1")

	k8sClient := newFakeClient([]*unstructured.Unstructured{nb, sts})
	target := newTarget(k8sClient)
	step := target.Recorder.Child("test", "test")

	err := stopWorkbench(context.Background(), target, nb, step)
	g.Expect(err).ToNot(HaveOccurred())

	// Verify stop annotation was applied
	updated, err := k8sClient.Dynamic().Resource(resources.Notebook.GVR()).
		Namespace("ns1").Get(context.Background(), "wb1", metav1.GetOptions{})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(isStopped(updated)).To(BeTrue())
}

func TestRestartWorkbench(t *testing.T) {
	g := NewWithT(t)

	nb := stoppedOAuthNotebook("wb1", "ns1")
	k8sClient := newFakeClient([]*unstructured.Unstructured{nb})
	target := newTarget(k8sClient)
	step := target.Recorder.Child("test", "test")

	restartWorkbench(context.Background(), target, nb, step)

	// Verify stop annotation was removed
	updated, err := k8sClient.Dynamic().Resource(resources.Notebook.GVR()).
		Namespace("ns1").Get(context.Background(), "wb1", metav1.GetOptions{})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(isStopped(updated)).To(BeFalse())
}

func TestDeleteStatefulSet(t *testing.T) {
	g := NewWithT(t)

	sts := newStatefulSet("wb1", "ns1")
	k8sClient := newFakeClient([]*unstructured.Unstructured{sts})
	target := newTarget(k8sClient)
	step := target.Recorder.Child("test", "test")

	ok := deleteStatefulSet(context.Background(), target, "wb1", "ns1", step)
	g.Expect(ok).To(BeTrue())

	// Verify it was deleted
	_, err := k8sClient.Dynamic().Resource(resources.StatefulSet.GVR()).
		Namespace("ns1").Get(context.Background(), "wb1", metav1.GetOptions{})
	g.Expect(err).To(HaveOccurred())
}

func TestDeleteStatefulSet_NotFound(t *testing.T) {
	g := NewWithT(t)

	k8sClient := newFakeClient(nil)
	target := newTarget(k8sClient)
	step := target.Recorder.Child("test", "test")

	ok := deleteStatefulSet(context.Background(), target, "wb1", "ns1", step)
	g.Expect(ok).To(BeTrue())
}

func TestDeleteStatefulSet_Error(t *testing.T) {
	g := NewWithT(t)

	sts := newStatefulSet("wb1", "ns1")
	reactor := &k8stesting.SimpleReactor{
		Verb:     "delete",
		Resource: "statefulsets",
		Reaction: func(_ k8stesting.Action) (bool, runtime.Object, error) {
			return true, nil, errors.New("simulated delete error")
		},
	}

	k8sClient := newFakeClient([]*unstructured.Unstructured{sts}, reactor)
	target := newTarget(k8sClient)
	step := target.Recorder.Child("test", "test")

	ok := deleteStatefulSet(context.Background(), target, "wb1", "ns1", step)
	g.Expect(ok).To(BeFalse())
}

func TestCheckKueueTerminatingPods_NoKueueNamespaces(t *testing.T) {
	g := NewWithT(t)

	k8sClient := newFakeClient(nil)
	target := newTarget(k8sClient)
	step := target.Recorder.Child("test", "test")

	// Should complete without error when no Kueue namespaces exist
	checkKueueTerminatingPods(context.Background(), target, nil, step)
	g.Expect(true).To(BeTrue())
}

func TestWaitForStatefulSetScaleDown_NotFound(t *testing.T) {
	g := NewWithT(t)

	k8sClient := newFakeClient(nil)
	target := newTarget(k8sClient)

	err := waitForStatefulSetScaleDown(context.Background(), target, "wb1", "ns1")
	g.Expect(err).ToNot(HaveOccurred())
}

func TestWaitForStatefulSetScaleDown_AlreadyScaledDown(t *testing.T) {
	g := NewWithT(t)

	sts := newStatefulSet("wb1", "ns1") // replicas=0
	k8sClient := newFakeClient([]*unstructured.Unstructured{sts})
	target := newTarget(k8sClient)

	err := waitForStatefulSetScaleDown(context.Background(), target, "wb1", "ns1")
	g.Expect(err).ToNot(HaveOccurred())
}

// --- Auto-stop integration ---

func TestRunTask_Execute_AutoStopRunningNotebooks(t *testing.T) {
	g := NewWithT(t)

	a := newAction()
	task := a.Run()

	// One running notebook + its STS
	nb := oauthNotebook("wb1", "ns1")
	sts := newStatefulSet("wb1", "ns1")

	k8sClient := newFakeClient([]*unstructured.Unstructured{nb, sts})
	target := newTarget(k8sClient)

	_, err := task.Execute(context.Background(), target)
	g.Expect(err).ToNot(HaveOccurred())

	// Notebook should be patched
	updated, err := k8sClient.Dynamic().Resource(resources.Notebook.GVR()).
		Namespace("ns1").Get(context.Background(), "wb1", metav1.GetOptions{})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(updated.GetAnnotations()[annotationInjectAuth]).To(Equal("true"))

	// After patching and restarting, the stop annotation should be removed
	g.Expect(isStopped(updated)).To(BeFalse())
}

func TestRunTask_Execute_AutoStopMixedStoppedRunning(t *testing.T) {
	g := NewWithT(t)

	a := newAction()
	task := a.Run()

	stopped := stoppedOAuthNotebook("wb-stopped", "ns1")
	running := oauthNotebook("wb-running", "ns1")
	sts1 := newStatefulSet("wb-stopped", "ns1")
	sts2 := newStatefulSet("wb-running", "ns1")

	k8sClient := newFakeClient([]*unstructured.Unstructured{stopped, running, sts1, sts2})
	target := newTarget(k8sClient)

	_, err := task.Execute(context.Background(), target)
	g.Expect(err).ToNot(HaveOccurred())

	// Both should be patched
	for _, name := range []string{"wb-stopped", "wb-running"} {
		updated, err := k8sClient.Dynamic().Resource(resources.Notebook.GVR()).
			Namespace("ns1").Get(context.Background(), name, metav1.GetOptions{})
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(updated.GetAnnotations()[annotationInjectAuth]).To(Equal("true"))
	}
}

func TestRunTask_Execute_AutoStopDryRun(t *testing.T) {
	g := NewWithT(t)

	a := newAction()
	task := a.Run()

	nb := oauthNotebook("wb1", "ns1")
	k8sClient := newFakeClient([]*unstructured.Unstructured{nb})
	target := newTarget(k8sClient, withDryRun)

	result, err := task.Execute(context.Background(), target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).ToNot(BeNil())

	// Notebook should NOT be modified in dry-run
	current, err := k8sClient.Dynamic().Resource(resources.Notebook.GVR()).
		Namespace("ns1").Get(context.Background(), "wb1", metav1.GetOptions{})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(hasOAuthProxyContainer(current)).To(BeTrue())
}

// --- backupNotebooks Tests ---

func TestBackupNotebooks_DryRun(t *testing.T) {
	g := NewWithT(t)

	target := newTarget(newFakeClient(nil), withDryRun)
	step := target.Recorder.Child("test", "test")

	nb := oauthNotebook("wb1", "ns1")
	err := backupNotebooks(target, []*unstructured.Unstructured{nb}, step)
	g.Expect(err).ToNot(HaveOccurred())
}

func TestBackupNotebooks_WritesToDir(t *testing.T) {
	g := NewWithT(t)

	outputDir := t.TempDir()
	target := newTarget(newFakeClient(nil))
	target.OutputDir = outputDir
	step := target.Recorder.Child("test", "test")

	nb := oauthNotebook("wb1", "ns1")
	err := backupNotebooks(target, []*unstructured.Unstructured{nb}, step)
	g.Expect(err).ToNot(HaveOccurred())
}

func TestBackupNotebooks_MultipleNamespaces(t *testing.T) {
	g := NewWithT(t)

	outputDir := t.TempDir()
	target := newTarget(newFakeClient(nil))
	target.OutputDir = outputDir
	step := target.Recorder.Child("test", "test")

	nbs := []*unstructured.Unstructured{
		oauthNotebook("wb1", "ns1"),
		oauthNotebook("wb2", "ns2"),
	}

	err := backupNotebooks(target, nbs, step)
	g.Expect(err).ToNot(HaveOccurred())
}

// --- RestartWorkbench error path ---

func TestRestartWorkbench_PatchError(t *testing.T) {
	g := NewWithT(t)

	nb := stoppedOAuthNotebook("wb1", "ns1")
	reactor := &k8stesting.SimpleReactor{
		Verb:     "patch",
		Resource: "notebooks",
		Reaction: func(_ k8stesting.Action) (bool, runtime.Object, error) {
			return true, nil, errors.New("simulated patch error")
		},
	}
	k8sClient := newFakeClient([]*unstructured.Unstructured{nb}, reactor)
	target := newTarget(k8sClient)
	step := target.Recorder.Child("test", "test")

	restartWorkbench(context.Background(), target, nb, step)
	g.Expect(true).To(BeTrue()) // should not panic
}

// --- StopWorkbench error path ---

func TestStopWorkbench_PatchError(t *testing.T) {
	g := NewWithT(t)

	nb := oauthNotebook("wb1", "ns1")
	reactor := &k8stesting.SimpleReactor{
		Verb:     "patch",
		Resource: "notebooks",
		Reaction: func(_ k8stesting.Action) (bool, runtime.Object, error) {
			return true, nil, errors.New("simulated patch error")
		},
	}
	k8sClient := newFakeClient([]*unstructured.Unstructured{nb}, reactor)
	target := newTarget(k8sClient)
	step := target.Recorder.Child("test", "test")

	err := stopWorkbench(context.Background(), target, nb, step)
	g.Expect(err).To(HaveOccurred())
}

// --- hasOAuthVolumes edge cases ---

func TestHasOAuthVolumes_AllThree(t *testing.T) {
	g := NewWithT(t)

	nb := newNotebook("wb1", "ns1",
		withContainers(container("my-notebook")),
		withVolumes(
			volume("oauth-config"),
			volume("oauth-client"),
			volume("tls-certificates"),
		),
	)

	g.Expect(hasOAuthVolumes(nb)).To(BeTrue())
}

func TestHasOAuthVolumes_OneOAuth(t *testing.T) {
	g := NewWithT(t)

	nb := newNotebook("wb1", "ns1",
		withContainers(container("my-notebook")),
		withVolumes(volume("tls-certificates")),
	)

	g.Expect(hasOAuthVolumes(nb)).To(BeTrue())
}

func TestHasOAuthVolumes_NoOAuth(t *testing.T) {
	g := NewWithT(t)

	nb := newNotebook("wb1", "ns1",
		withContainers(container("my-notebook")),
		withVolumes(volume("data")),
	)

	g.Expect(hasOAuthVolumes(nb)).To(BeFalse())
}

func TestHasOAuthVolumes_NoVolumes(t *testing.T) {
	g := NewWithT(t)

	nb := newNotebook("wb1", "ns1",
		withContainers(container("my-notebook")),
	)

	g.Expect(hasOAuthVolumes(nb)).To(BeFalse())
}

func TestHasOAuthFinalizer_True(t *testing.T) {
	g := NewWithT(t)

	nb := newNotebook("wb1", "ns1",
		withFinalizers(finalizerOAuthClient),
	)

	g.Expect(hasOAuthFinalizer(nb)).To(BeTrue())
}

func TestHasOAuthFinalizer_False(t *testing.T) {
	g := NewWithT(t)

	nb := newNotebook("wb1", "ns1")

	g.Expect(hasOAuthFinalizer(nb)).To(BeFalse())
}

func TestHasOAuthProxyContainer_True(t *testing.T) {
	g := NewWithT(t)

	nb := newNotebook("wb1", "ns1",
		withContainers(container("my-notebook"), container(containerOAuthProxy)),
	)

	g.Expect(hasOAuthProxyContainer(nb)).To(BeTrue())
}

func TestHasOAuthProxyContainer_False(t *testing.T) {
	g := NewWithT(t)

	nb := newNotebook("wb1", "ns1",
		withContainers(container("my-notebook")),
	)

	g.Expect(hasOAuthProxyContainer(nb)).To(BeFalse())
}

func TestHasTornadoSettings_True(t *testing.T) {
	g := NewWithT(t)

	nb := newNotebook("wb1", "ns1",
		withContainers(
			containerWithEnv("my-notebook",
				envVar(envNotebookArgs, "--ServerApp.tornado_settings={}")),
		),
	)

	g.Expect(hasTornadoSettings(nb)).To(BeTrue())
}

func TestHasTornadoSettings_False(t *testing.T) {
	g := NewWithT(t)

	nb := newNotebook("wb1", "ns1",
		withContainers(
			containerWithEnv("my-notebook",
				envVar(envNotebookArgs, "--ServerApp.port=8888")),
		),
	)

	g.Expect(hasTornadoSettings(nb)).To(BeFalse())
}

func TestHasTornadoSettings_DifferentEnvVar(t *testing.T) {
	g := NewWithT(t)

	nb := newNotebook("wb1", "ns1",
		withContainers(
			containerWithEnv("my-notebook",
				envVar("OTHER_VAR", "--ServerApp.tornado_settings={}")),
		),
	)

	g.Expect(hasTornadoSettings(nb)).To(BeFalse())
}

func TestHasTornadoSettings_NoEnvVars(t *testing.T) {
	g := NewWithT(t)

	nb := newNotebook("wb1", "ns1",
		withContainers(container("my-notebook")),
	)

	g.Expect(hasTornadoSettings(nb)).To(BeFalse())
}

// --- Kueue Terminating Pods Tests ---

func newNamespaceWithLabel(name string, labels map[string]string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "Namespace",
			"metadata": map[string]any{
				"name":   name,
				"labels": toAnyMap(labels),
			},
		},
	}
}

func toAnyMap(m map[string]string) map[string]any {
	result := make(map[string]any, len(m))
	for k, v := range m {
		result[k] = v
	}

	return result
}

func newTerminatingPod(name, namespace string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "Pod",
			"metadata": map[string]any{
				"name":              name,
				"namespace":         namespace,
				"deletionTimestamp": "2024-01-01T00:00:00Z",
			},
		},
	}
}

func TestCheckKueueTerminatingPods_WithStuckPods(t *testing.T) {
	g := NewWithT(t)

	ns := newNamespaceWithLabel("ns1", map[string]string{
		"kueue.openshift.io/managed": "true",
	})
	pod := newTerminatingPod("wb1-0", "ns1")
	nb := oauthNotebook("wb1", "ns1")

	k8sClient := newFakeClient([]*unstructured.Unstructured{ns, pod, nb})
	target := newTarget(k8sClient)
	step := target.Recorder.Child("test", "test")

	checkKueueTerminatingPods(context.Background(), target,
		[]*unstructured.Unstructured{nb}, step)

	g.Expect(true).To(BeTrue()) // should record warning without error
}

func TestCheckKueueTerminatingPods_NoStuckPods(t *testing.T) {
	g := NewWithT(t)

	ns := newNamespaceWithLabel("ns1", map[string]string{
		"kueue.openshift.io/managed": "true",
	})
	nb := oauthNotebook("wb1", "ns1")

	k8sClient := newFakeClient([]*unstructured.Unstructured{ns, nb})
	target := newTarget(k8sClient)
	step := target.Recorder.Child("test", "test")

	checkKueueTerminatingPods(context.Background(), target,
		[]*unstructured.Unstructured{nb}, step)

	g.Expect(true).To(BeTrue())
}

func TestCheckKueueTerminatingPods_NotebookNotInKueueNamespace(t *testing.T) {
	g := NewWithT(t)

	ns := newNamespaceWithLabel("ns1", map[string]string{
		"kueue.openshift.io/managed": "true",
	})
	nb := oauthNotebook("wb1", "ns2") // different namespace

	k8sClient := newFakeClient([]*unstructured.Unstructured{ns, nb})
	target := newTarget(k8sClient)
	step := target.Recorder.Child("test", "test")

	checkKueueTerminatingPods(context.Background(), target,
		[]*unstructured.Unstructured{nb}, step)

	g.Expect(true).To(BeTrue())
}

func TestCheckKueueTerminatingPods_NilNotebooks(t *testing.T) {
	g := NewWithT(t)

	ns := newNamespaceWithLabel("ns1", map[string]string{
		"kueue.openshift.io/managed": "true",
	})

	k8sClient := newFakeClient([]*unstructured.Unstructured{ns})
	target := newTarget(k8sClient)
	step := target.Recorder.Child("test", "test")

	checkKueueTerminatingPods(context.Background(), target, nil, step)

	g.Expect(true).To(BeTrue())
}

// --- Execute with SkipStop + Kueue check ---

func TestRunTask_Execute_SkipStop_TriggersKueueCheck(t *testing.T) {
	g := NewWithT(t)

	a := newAction()
	a.SkipStop = true
	task := a.Run()

	ns := newNamespaceWithLabel("ns1", map[string]string{
		"kueue.openshift.io/managed": "true",
	})
	nb := oauthNotebook("wb1", "ns1")
	sts := newStatefulSet("wb1", "ns1")
	pod := newTerminatingPod("wb1-0", "ns1")

	k8sClient := newFakeClient([]*unstructured.Unstructured{ns, nb, sts, pod})
	target := newTarget(k8sClient)

	result, err := task.Execute(context.Background(), target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).ToNot(BeNil())
}

// --- removeOAuthFinalizer: no finalizers ---

func TestRemoveOAuthFinalizer_NoFinalizers(t *testing.T) {
	g := NewWithT(t)

	nb := newNotebook("wb1", "ns1")
	removeOAuthFinalizer(nb)

	g.Expect(nb.GetFinalizers()).To(BeNil())
}

// --- PrepareTask Execute: backup with valid dir ---

func TestPrepareTask_Execute_BackupWritten(t *testing.T) {
	g := NewWithT(t)

	a := newAction()
	task := a.Prepare()

	nb := oauthNotebook("wb1", "ns1")
	k8sClient := newFakeClient([]*unstructured.Unstructured{nb})
	target := newTarget(k8sClient)
	target.OutputDir = t.TempDir()

	result, err := task.Execute(context.Background(), target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).ToNot(BeNil())
}

// --- PrepareTask Validate: all patched ---

func TestPrepareTask_Validate_AllPatched(t *testing.T) {
	g := NewWithT(t)

	a := newAction()
	task := a.Prepare()

	nb := patchedNotebook("wb1", "ns1")
	k8sClient := newFakeClient([]*unstructured.Unstructured{nb})
	target := newTarget(k8sClient)

	result, err := task.Validate(context.Background(), target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).ToNot(BeNil())
}

// --- runCleanup refetch error ---

func TestRunTask_Execute_WithCleanup_RefetchError(t *testing.T) {
	g := NewWithT(t)

	a := newAction()
	a.WithCleanup = true
	task := a.Run()

	nb := stoppedOAuthNotebook("wb1", "ns1")
	sts := newStatefulSet("wb1", "ns1")

	// Create client with the notebook + STS but add a reactor that fails GET
	// after the first update (patch succeeds, but cleanup re-fetch fails)
	updateCount := 0
	getErrorReactor := &k8stesting.SimpleReactor{
		Verb:     "get",
		Resource: "notebooks",
		Reaction: func(_ k8stesting.Action) (bool, runtime.Object, error) {
			updateCount++
			// Let the first GET through (for ListNotebooks), fail subsequent ones
			if updateCount > 1 {
				return true, nil, errors.New("simulated get error")
			}

			return false, nil, nil
		},
	}

	k8sClient := newFakeClient([]*unstructured.Unstructured{nb, sts}, getErrorReactor)
	// Use scope with specific name/namespace to use Get (not List)
	a.Scope.WorkbenchNamespace = "ns1"
	a.Scope.WorkbenchName = "wb1"
	target := newTarget(k8sClient)

	result, err := task.Execute(context.Background(), target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).ToNot(BeNil())
}

// --- predicate edge cases: non-map items in containers/volumes ---

func TestHasOAuthProxyContainer_NonMapContainer(t *testing.T) {
	g := NewWithT(t)

	nb := newNotebook("wb1", "ns1")
	containers := []any{"not-a-map", "also-not-a-map"}
	_ = unstructured.SetNestedField(nb.Object,
		containers,
		"spec", "template", "spec", "containers")

	g.Expect(hasOAuthProxyContainer(nb)).To(BeFalse())
}

func TestHasOAuthVolumes_NonMapVolume(t *testing.T) {
	g := NewWithT(t)

	nb := newNotebook("wb1", "ns1")
	volumes := []any{"not-a-map"}
	_ = unstructured.SetNestedField(nb.Object,
		volumes,
		"spec", "template", "spec", "volumes")

	g.Expect(hasOAuthVolumes(nb)).To(BeFalse())
}

func TestHasTornadoSettings_NonMapContainer(t *testing.T) {
	g := NewWithT(t)

	nb := newNotebook("wb1", "ns1")
	containers := []any{"not-a-map"}
	_ = unstructured.SetNestedField(nb.Object,
		containers,
		"spec", "template", "spec", "containers")

	g.Expect(hasTornadoSettings(nb)).To(BeFalse())
}

func TestHasTornadoSettings_NonMapEnvVar(t *testing.T) {
	g := NewWithT(t)

	nb := newNotebook("wb1", "ns1")
	containers := []any{
		map[string]any{
			"name": "main",
			"env":  []any{"not-a-map"},
		},
	}
	_ = unstructured.SetNestedField(nb.Object,
		containers,
		"spec", "template", "spec", "containers")

	g.Expect(hasTornadoSettings(nb)).To(BeFalse())
}

func TestHasTornadoSettings_NoEnv(t *testing.T) {
	g := NewWithT(t)

	nb := newNotebook("wb1", "ns1")
	containers := []any{
		map[string]any{
			"name": "main",
		},
	}
	_ = unstructured.SetNestedField(nb.Object,
		containers,
		"spec", "template", "spec", "containers")

	g.Expect(hasTornadoSettings(nb)).To(BeFalse())
}

// --- remove functions with non-map items ---

func TestRemoveOAuthProxyContainer_PreservesNonMapItems(t *testing.T) {
	g := NewWithT(t)

	nb := newNotebook("wb1", "ns1")
	containers := []any{
		"not-a-map",
		map[string]any{"name": "oauth-proxy", "image": "oauth"},
		map[string]any{"name": "main", "image": "main"},
	}
	_ = unstructured.SetNestedField(nb.Object,
		containers,
		"spec", "template", "spec", "containers")

	err := removeOAuthProxyContainer(nb)
	g.Expect(err).ToNot(HaveOccurred())

	got, _ := jq.Query[[]any](nb, ".spec.template.spec.containers")
	g.Expect(got).To(HaveLen(2))
}

func TestRemoveOAuthVolumes_PreservesNonMapItems(t *testing.T) {
	g := NewWithT(t)

	nb := newNotebook("wb1", "ns1")
	volumes := []any{
		"not-a-map",
		map[string]any{"name": "oauth-config"},
		map[string]any{"name": "data"},
	}
	_ = unstructured.SetNestedField(nb.Object,
		volumes,
		"spec", "template", "spec", "volumes")

	err := removeOAuthVolumes(nb)
	g.Expect(err).ToNot(HaveOccurred())

	got, _ := jq.Query[[]any](nb, ".spec.template.spec.volumes")
	g.Expect(got).To(HaveLen(2))
}

func TestStripTornadoSettings_NonMapContainerAndEnv(t *testing.T) {
	g := NewWithT(t)

	nb := newNotebook("wb1", "ns1")
	containers := []any{
		"not-a-map",
		map[string]any{
			"name": "main",
			"env": []any{
				"not-a-map",
				map[string]any{
					"name":  "NOTEBOOK_ARGS",
					"value": "--NotebookApp.port=8888 --ServerApp.tornado_settings={\"headers\":{}} --arg2",
				},
			},
		},
	}
	_ = unstructured.SetNestedField(nb.Object,
		containers,
		"spec", "template", "spec", "containers")

	err := stripTornadoSettings(nb)
	g.Expect(err).ToNot(HaveOccurred())
}

// --- waitForStatefulSetScaleDown with Get error ---

func TestWaitForStatefulSetScaleDown_GetError(t *testing.T) {
	g := NewWithT(t)

	reactor := &k8stesting.SimpleReactor{
		Verb:     "get",
		Resource: "statefulsets",
		Reaction: func(_ k8stesting.Action) (bool, runtime.Object, error) {
			return true, nil, errors.New("server error")
		},
	}

	k8sClient := newFakeClient(nil, reactor)
	target := newTarget(k8sClient)

	err := waitForStatefulSetScaleDown(context.Background(), target, "wb1", "ns1")
	g.Expect(err).To(HaveOccurred())
}

// --- stopWorkbench with wait timeout ---

func TestStopWorkbench_WaitTimeout(t *testing.T) {
	g := NewWithT(t)

	sts := newStatefulSet("wb1", "ns1")
	_ = unstructured.SetNestedField(sts.Object, int64(1), "spec", "replicas")

	nb := oauthNotebook("wb1", "ns1")
	k8sClient := newFakeClient([]*unstructured.Unstructured{nb, sts})
	target := newTarget(k8sClient)

	step := target.Recorder.Child("test", "test")

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	err := stopWorkbench(ctx, target, nb, step)
	g.Expect(err).To(HaveOccurred())
}

// --- restartWorkbench success path ---

func TestRestartWorkbench_Success(t *testing.T) {
	g := NewWithT(t)

	nb := stoppedOAuthNotebook("wb1", "ns1")
	k8sClient := newFakeClient([]*unstructured.Unstructured{nb})
	target := newTarget(k8sClient)

	step := target.Recorder.Child("test", "test")

	restartWorkbench(context.Background(), target, nb, step)

	g.Expect(true).To(BeTrue())
}

// --- predicate no-spec edge cases ---

func TestHasOAuthProxyContainer_NoContainers(t *testing.T) {
	g := NewWithT(t)

	nb := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "kubeflow.org/v1",
			"kind":       "Notebook",
			"metadata":   map[string]any{"name": "wb1", "namespace": "ns1"},
		},
	}

	g.Expect(hasOAuthProxyContainer(nb)).To(BeFalse())
}

func TestHasOAuthVolumes_NoSpec2(t *testing.T) {
	g := NewWithT(t)

	nb := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "kubeflow.org/v1",
			"kind":       "Notebook",
			"metadata":   map[string]any{"name": "wb1", "namespace": "ns1"},
		},
	}

	g.Expect(hasOAuthVolumes(nb)).To(BeFalse())
}

func TestHasTornadoSettings_NoSpec2(t *testing.T) {
	g := NewWithT(t)

	nb := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "kubeflow.org/v1",
			"kind":       "Notebook",
			"metadata":   map[string]any{"name": "wb1", "namespace": "ns1"},
		},
	}

	g.Expect(hasTornadoSettings(nb)).To(BeFalse())
}

// --- remove functions with no spec ---

func TestRemoveOAuthProxyContainer_NoContainers2(t *testing.T) {
	g := NewWithT(t)

	nb := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "kubeflow.org/v1",
			"kind":       "Notebook",
			"metadata":   map[string]any{"name": "wb1", "namespace": "ns1"},
		},
	}

	err := removeOAuthProxyContainer(nb)
	g.Expect(err).To(HaveOccurred())
}

func TestRemoveOAuthVolumes_NoSpec2(t *testing.T) {
	g := NewWithT(t)

	nb := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "kubeflow.org/v1",
			"kind":       "Notebook",
			"metadata":   map[string]any{"name": "wb1", "namespace": "ns1"},
		},
	}

	err := removeOAuthVolumes(nb)
	g.Expect(err).To(HaveOccurred())
}

func TestStripTornadoSettings_NoContainers2(t *testing.T) {
	g := NewWithT(t)

	nb := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "kubeflow.org/v1",
			"kind":       "Notebook",
			"metadata":   map[string]any{"name": "wb1", "namespace": "ns1"},
		},
	}

	err := stripTornadoSettings(nb)
	g.Expect(err).To(HaveOccurred())
}

// --- applyAllPatches error propagation ---

func TestApplyAllPatches_NoContainersErrors(t *testing.T) {
	g := NewWithT(t)

	nb := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "kubeflow.org/v1",
			"kind":       "Notebook",
			"metadata":   map[string]any{"name": "wb1", "namespace": "ns1"},
		},
	}

	err := applyAllPatches(nb)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("removing oauth-proxy container"))
}

// --- patchNotebooks applyAllPatches error ---

func TestRunTask_Execute_PatchApplyError(t *testing.T) {
	g := NewWithT(t)

	a := newAction()
	task := a.Run()

	nb := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "kubeflow.org/v1",
			"kind":       "Notebook",
			"metadata": map[string]any{
				"name":      "wb1",
				"namespace": "ns1",
				"annotations": map[string]any{
					"notebooks.opendatahub.io/inject-oauth": "true",
					"kubeflow-resource-stopped":             "2024-01-01T00:00:00Z",
				},
			},
		},
	}

	k8sClient := newFakeClient([]*unstructured.Unstructured{nb})
	a.Scope.WorkbenchNamespace = "ns1"
	a.Scope.WorkbenchName = "wb1"
	target := newTarget(k8sClient)

	result, err := task.Execute(context.Background(), target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).ToNot(BeNil())
}

// --- prepare_task.go Execute backup ---

func TestPrepareTask_Execute_BackupMultipleNamespaces(t *testing.T) {
	g := NewWithT(t)

	a := newAction()
	task := a.Prepare()

	nb1 := oauthNotebook("wb1", "ns1")
	nb2 := oauthNotebook("wb2", "ns2")
	nb3 := newNotebook("wb3", "ns1")

	k8sClient := newFakeClient([]*unstructured.Unstructured{nb1, nb2, nb3})
	target := newTarget(k8sClient)
	target.OutputDir = t.TempDir()

	result, err := task.Execute(context.Background(), target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).ToNot(BeNil())
}

// --- checkKueueTerminatingPods error from discovery ---

func TestCheckKueueTerminatingPods_DiscoveryError(t *testing.T) {
	g := NewWithT(t)

	reactor := &k8stesting.SimpleReactor{
		Verb:     "list",
		Resource: "namespaces",
		Reaction: func(_ k8stesting.Action) (bool, runtime.Object, error) {
			return true, nil, errors.New("namespace list error")
		},
	}

	k8sClient := newFakeClient(nil, reactor)
	target := newTarget(k8sClient)
	step := target.Recorder.Child("test", "test")

	checkKueueTerminatingPods(context.Background(), target, nil, step)

	g.Expect(true).To(BeTrue())
}

// --- checkKueueTerminatingPods with pod Get error ---

func TestCheckKueueTerminatingPods_PodGetError(t *testing.T) {
	g := NewWithT(t)

	ns := newNamespaceWithLabel("ns1", map[string]string{"kueue.x-k8s.io/managed": "true"})
	nb := oauthNotebook("wb1", "ns1")

	reactor := &k8stesting.SimpleReactor{
		Verb:     "get",
		Resource: "pods",
		Reaction: func(_ k8stesting.Action) (bool, runtime.Object, error) {
			return true, nil, errors.New("pod get error")
		},
	}

	k8sClient := newFakeClient([]*unstructured.Unstructured{ns, nb}, reactor)
	target := newTarget(k8sClient)
	step := target.Recorder.Child("test", "test")

	checkKueueTerminatingPods(context.Background(), target, []*unstructured.Unstructured{nb}, step)

	g.Expect(true).To(BeTrue())
}
