package modelserving_test

import (
	"github.com/blang/semver/v4"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	"github.com/opendatahub-io/odh-cli/pkg/migrate/action"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
)

const (
	testISVCNamespace         = "test-ns"
	testISVCName              = "my-model"
	testApplicationsNamespace = "redhat-ods-applications"
	testConfigMapName         = "inferenceservice-config"
)

func newISVC(namespace, name, deploymentMode string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.InferenceService.APIVersion(),
			"kind":       resources.InferenceService.Kind,
			"metadata": map[string]any{
				"name":      name,
				"namespace": namespace,
				"uid":       "test-uid-123",
				"annotations": map[string]any{
					"serving.kserve.io/deploymentMode": deploymentMode,
				},
			},
		},
	}
}

func newDSCI(appNamespace string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.DSCInitialization.APIVersion(),
			"kind":       resources.DSCInitialization.Kind,
			"metadata": map[string]any{
				"name": "default-dsci",
			},
			"spec": map[string]any{
				"applicationsNamespace": appNamespace,
			},
		},
	}
}

func newISVCConfigMap(namespace string, annotations map[string]string, isvcConfigJSON string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]any{
				"name":      testConfigMapName,
				"namespace": namespace,
			},
			"data": map[string]any{},
		},
	}

	if annotations != nil {
		metaAnnotations := make(map[string]any, len(annotations))
		for k, v := range annotations {
			metaAnnotations[k] = v
		}

		obj.Object["metadata"].(map[string]any)["annotations"] = metaAnnotations
	}

	if isvcConfigJSON != "" {
		obj.Object["data"].(map[string]any)["inferenceService"] = isvcConfigJSON
	}

	return obj
}

func newDeployment(namespace, name string) *unstructured.Unstructured {
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
					"metadata": map[string]any{
						"annotations": map[string]any{},
					},
				},
			},
		},
	}
}

func newISVCWithAuth(namespace, name, deploymentMode string) *unstructured.Unstructured {
	isvc := newISVC(namespace, name, deploymentMode)
	annotations := isvc.GetAnnotations()
	annotations["security.opendatahub.io/enable-auth"] = "true"
	isvc.SetAnnotations(annotations)

	return isvc
}

func newISVCWithIstioAnnotations(namespace, name, deploymentMode string) *unstructured.Unstructured {
	isvc := newISVC(namespace, name, deploymentMode)
	annotations := isvc.GetAnnotations()
	annotations["istio.io/rev"] = "default"
	annotations["sidecar.istio.io/inject"] = "true"
	annotations["serving.knative.dev/creator"] = "system"
	isvc.SetAnnotations(annotations)

	labels := make(map[string]string)
	labels["networking.istio.io/gateway"] = "default"
	labels["networking.knative.dev/visibility"] = "cluster-local"
	isvc.SetLabels(labels)

	return isvc
}

func newVirtualService(namespace, name string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.VirtualService.APIVersion(),
			"kind":       resources.VirtualService.Kind,
			"metadata": map[string]any{
				"name":      name,
				"namespace": namespace,
			},
		},
	}
}

func newTestTarget(dynamicClient *dynamicfake.FakeDynamicClient, currentVersion string, dryRun bool) action.Target {
	v := semver.MustParse(currentVersion)
	tv := semver.MustParse("3.0.0")

	testClient := client.NewForTesting(client.TestClientConfig{
		Dynamic: dynamicClient,
	})

	return action.Target{
		Client:         testClient,
		CurrentVersion: &v,
		TargetVersion:  &tv,
		DryRun:         dryRun,
		SkipConfirm:    true,
		Recorder:       action.NewRootRecorder(),
	}
}
