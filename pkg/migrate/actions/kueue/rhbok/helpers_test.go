package rhbok_test

import (
	"bytes"
	"testing"
	"time"

	"github.com/blang/semver/v4"
	operatorsv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	operatorfake "github.com/operator-framework/operator-lifecycle-manager/pkg/api/client/clientset/versioned/fake"

	authorizationv1 "k8s.io/api/authorization/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextensionsfake "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/fake"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	kubefake "k8s.io/client-go/kubernetes/fake"
	metadatafake "k8s.io/client-go/metadata/fake"
	k8stesting "k8s.io/client-go/testing"

	"github.com/opendatahub-io/odh-cli/pkg/migrate/action"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/actions/kueue/rhbok"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
	"github.com/opendatahub-io/odh-cli/pkg/util/iostreams"
	"github.com/opendatahub-io/odh-cli/pkg/util/kube"
)

//nolint:gochecknoglobals // test-only GVR→ListKind mapping for fake dynamic client
var testListKinds = map[schema.GroupVersionResource]string{
	resources.DataScienceClusterV1.GVR(): resources.DataScienceClusterV1.ListKind(),
	resources.ClusterQueue.GVR():         resources.ClusterQueue.ListKind(),
	resources.LocalQueue.GVR():           resources.LocalQueue.ListKind(),
	resources.Subscription.GVR():         resources.Subscription.ListKind(),
	resources.OperatorGroup.GVR():        resources.OperatorGroup.ListKind(),
	resources.ConfigMap.GVR():            resources.ConfigMap.ListKind(),
	resources.PackageManifest.GVR():      resources.PackageManifest.ListKind(),
	resources.Deployment.GVR():           resources.Deployment.ListKind(),
	resources.Namespace.GVR():            resources.Namespace.ListKind(),
	resources.Notebook.GVR():             resources.Notebook.ListKind(),
	resources.InferenceService.GVR():     resources.InferenceService.ListKind(),
	resources.LLMInferenceService.GVR():  resources.LLMInferenceService.ListKind(),
	resources.RayCluster.GVR():           resources.RayCluster.ListKind(),
	resources.RayJob.GVR():               resources.RayJob.ListKind(),
	resources.PyTorchJob.GVR():           resources.PyTorchJob.ListKind(),
}

type targetOpts struct {
	dryRun         bool
	skipConfirm    bool
	outputDir      string
	olmObjects     []runtime.Object
	kubeObjects    []runtime.Object
	rbacAllowed    bool
	dynamicReactor func(k8stesting.Action) (bool, runtime.Object, error)
	olmReactor     func(k8stesting.Action) (bool, runtime.Object, error)
}

func newTarget(t *testing.T, objects []*unstructured.Unstructured, opts targetOpts) action.Target {
	t.Helper()

	rhbok.SetTestPollConfig(5*time.Millisecond, 50*time.Millisecond)

	scheme := runtime.NewScheme()
	_ = metav1.AddMetaToScheme(scheme)

	dynamicObjs := make([]runtime.Object, 0, len(objects)+1)
	for _, obj := range objects {
		dynamicObjs = append(dynamicObjs, obj)
	}

	dynamicObjs = append(dynamicObjs, makeKueuePackageManifest())

	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		scheme,
		testListKinds,
		dynamicObjs...,
	)

	dynamicClient.PrependReactor("update", "datascienceclusters", dscUpdateReactor)

	if opts.dynamicReactor != nil {
		dynamicClient.PrependReactor("*", "*", opts.dynamicReactor)
	}

	olmClient := operatorfake.NewSimpleClientset(opts.olmObjects...) //nolint:staticcheck // NewClientset requires generated apply configs not available in OLM

	if opts.olmReactor != nil {
		olmClient.PrependReactor("*", "*", opts.olmReactor)
	}

	kubeObjs := make([]runtime.Object, 0, 3+len(opts.kubeObjects))
	kubeObjs = append(kubeObjs,
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: rhbok.ExportOperatorNamespace}},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "kueue-controller-manager",
				Namespace: rhbok.ExportOperatorNamespace,
			},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				Conditions: []corev1.PodCondition{
					{
						Type:   corev1.PodReady,
						Status: corev1.ConditionTrue,
					},
				},
			},
		},
	)
	kubeObjs = append(kubeObjs, opts.kubeObjects...)

	kubeClient := kubefake.NewSimpleClientset(kubeObjs...) //nolint:staticcheck // Need PrependReactor for SelfSubjectAccessReview responses
	kubeClient.PrependReactor("create", "selfsubjectaccessreviews",
		func(_ k8stesting.Action) (bool, runtime.Object, error) {
			return true, &authorizationv1.SelfSubjectAccessReview{
				Status: authorizationv1.SubjectAccessReviewStatus{Allowed: opts.rbacAllowed},
			}, nil
		},
	)

	metadataClient := metadatafake.NewSimpleMetadataClient(
		scheme,
		kube.ToPartialObjectMetadata(append(objects, makeKueuePackageManifest())...)...,
	)

	testClient := client.NewForTesting(client.TestClientConfig{
		Dynamic:       dynamicClient,
		OLM:           olmClient,
		Kubernetes:    kubeClient,
		APIExtensions: apiextensionsfake.NewSimpleClientset(certManagerCRD()), //nolint:staticcheck // apply configs not available for apiextensions fake
		Metadata:      metadataClient,
	})

	outputDir := opts.outputDir
	if outputDir == "" {
		outputDir = t.TempDir()
	}

	currentVersion := semver.MustParse("2.25.0")
	targetVersion := semver.MustParse("3.0.0")

	return action.Target{
		Client:         testClient,
		CurrentVersion: &currentVersion,
		TargetVersion:  &targetVersion,
		DryRun:         opts.dryRun,
		SkipConfirm:    opts.skipConfirm,
		OutputDir:      outputDir,
		Recorder:       action.NewRootRecorder(),
		IO:             iostreams.NewIOStreams(&bytes.Buffer{}, &bytes.Buffer{}, &bytes.Buffer{}),
	}
}

type objOption func(*unstructured.Unstructured)

func withComponent(name, state string) objOption {
	return func(obj *unstructured.Unstructured) {
		components, _, _ := unstructured.NestedMap(obj.Object, "spec", "components")
		if components == nil {
			components = map[string]any{}
		}

		components[name] = map[string]any{"managementState": state}
		_ = unstructured.SetNestedField(obj.Object, components, "spec", "components")
	}
}

func inNamespace(ns string) objOption {
	return func(obj *unstructured.Unstructured) {
		obj.SetNamespace(ns)
	}
}

func withLabel(key, value string) objOption {
	return func(obj *unstructured.Unstructured) {
		labels := obj.GetLabels()
		if labels == nil {
			labels = map[string]string{}
		}

		labels[key] = value
		obj.SetLabels(labels)
	}
}

func withDSCCondition(condType, status, reason string) objOption {
	return func(obj *unstructured.Unstructured) {
		_ = unstructured.SetNestedSlice(obj.Object, []any{
			map[string]any{
				"type":   condType,
				"status": status,
				"reason": reason,
			},
		}, "status", "conditions")
	}
}

func makeNamespace(name string, labels map[string]string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "Namespace",
			"metadata": map[string]any{
				"name": name,
			},
		},
	}
	if len(labels) > 0 {
		_ = unstructured.SetNestedStringMap(obj.Object, labels, "metadata", "labels")
	}

	return obj
}

func makeKubeNamespace(name string, labels map[string]string) *corev1.Namespace {
	return &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: labels,
		},
	}
}

func makeNotebook(name string, opts ...objOption) *unstructured.Unstructured {
	return makeObj(resources.Notebook, name, opts...)
}

func withAnnotation(key, value string) objOption {
	return func(obj *unstructured.Unstructured) {
		annotations := obj.GetAnnotations()
		if annotations == nil {
			annotations = map[string]string{}
		}

		annotations[key] = value
		obj.SetAnnotations(annotations)
	}
}

func makeObj(rt resources.ResourceType, name string, opts ...objOption) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": rt.APIVersion(),
			"kind":       rt.Kind,
			"metadata": map[string]any{
				"name": name,
			},
		},
	}

	for _, opt := range opts {
		opt(obj)
	}

	return obj
}

func makeDSCV1(name string, opts ...objOption) *unstructured.Unstructured {
	return makeObj(resources.DataScienceClusterV1, name, opts...)
}

func makeConfigMap(name string, opts ...objOption) *unstructured.Unstructured {
	obj := makeObj(resources.ConfigMap, name, opts...)
	if _, ok, _ := unstructured.NestedMap(obj.Object, "data"); !ok {
		_ = unstructured.SetNestedField(obj.Object, map[string]any{"config.yaml": "test-config-data"}, "data")
	}

	return obj
}

func makeClusterQueue(name string, opts ...objOption) *unstructured.Unstructured {
	return makeObj(resources.ClusterQueue, name, opts...)
}

func makeLocalQueue(name string, opts ...objOption) *unstructured.Unstructured {
	return makeObj(resources.LocalQueue, name, opts...)
}

func makeSubscription(name string, opts ...objOption) *unstructured.Unstructured {
	return makeObj(resources.Subscription, name, opts...)
}

func makeDeployment(name string, opts ...objOption) *unstructured.Unstructured {
	return makeObj(resources.Deployment, name, opts...)
}

func newOLMSubscription(name, namespace string) *operatorsv1alpha1.Subscription {
	return &operatorsv1alpha1.Subscription{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}
}

func newOLMCSV(name, namespace string) *operatorsv1alpha1.ClusterServiceVersion {
	return &operatorsv1alpha1.ClusterServiceVersion{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Status: operatorsv1alpha1.ClusterServiceVersionStatus{
			Phase: operatorsv1alpha1.CSVPhaseSucceeded,
		},
	}
}

func makeKueuePackageManifest() *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": resources.PackageManifest.APIVersion(),
		"kind":       resources.PackageManifest.Kind,
		"metadata": map[string]any{
			"name":      "kueue-operator",
			"namespace": "openshift-marketplace",
		},
		"status": map[string]any{
			"catalogSource": "redhat-operators",
			"channels": []any{
				map[string]any{
					"name": "stable-v1.2",
					"entries": []any{
						map[string]any{"name": "kueue-operator.v1.2.0"},
					},
				},
			},
		},
	}}
}

func certManagerCRD() *apiextensionsv1.CustomResourceDefinition {
	return &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: "certificates.cert-manager.io"},
	}
}

func dscUpdateReactor(action k8stesting.Action) (bool, runtime.Object, error) {
	updateAction, ok := action.(k8stesting.UpdateAction)
	if !ok {
		return false, nil, nil
	}

	obj, ok := updateAction.GetObject().(*unstructured.Unstructured)
	if !ok {
		return false, nil, nil
	}

	state, _, _ := unstructured.NestedString(obj.Object, "spec", "components", "kueue", "managementState")

	var cond map[string]any

	switch state {
	case "Removed":
		cond = map[string]any{
			"type":   "KueueReady",
			"status": "False",
			"reason": "Removed",
		}
	case "Unmanaged":
		cond = map[string]any{
			"type":   "KueueReady",
			"status": "True",
			"reason": "Ready",
		}
	}

	if cond != nil {
		_ = unstructured.SetNestedSlice(obj.Object, []any{cond}, "status", "conditions")
	}

	return false, nil, nil
}
