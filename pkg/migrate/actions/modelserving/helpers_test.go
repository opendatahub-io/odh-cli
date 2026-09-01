package modelserving_test

import (
	"encoding/base64"
	"encoding/json"
	"strings"

	"github.com/blang/semver/v4"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	"github.com/opendatahub-io/odh-cli/pkg/migrate/action"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/action/result"
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

// newModelServingDynamicClient builds a fake dynamic client with list kinds used by model-serving actions.
func newModelServingDynamicClient(objects ...runtime.Object) *dynamicfake.FakeDynamicClient {
	listKinds := map[schema.GroupVersionResource]string{
		resources.InferenceService.GVR(): resources.InferenceService.ListKind(),
		resources.ServingRuntime.GVR():   resources.ServingRuntime.ListKind(),
		resources.ServiceAccount.GVR():   resources.ServiceAccount.ListKind(),
		resources.Role.GVR():             resources.Role.ListKind(),
		resources.RoleBinding.GVR():      resources.RoleBinding.ListKind(),
		resources.Namespace.GVR():        resources.Namespace.ListKind(),
		resources.Secret.GVR():           resources.Secret.ListKind(),
	}

	return dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(), listKinds, objects...,
	)
}

// hasStepMessageContaining walks the step tree recursively and returns true
// if any step with the given status has a message containing substr.
func hasStepMessageContaining(steps []result.ActionStep, status result.StepStatus, substr string) bool {
	for _, s := range steps {
		if s.Status == status && strings.Contains(s.Message, substr) {
			return true
		}

		if hasStepMessageContaining(s.Children, status, substr) {
			return true
		}
	}

	return false
}

// newModelMeshISVCWithStorage creates a ModelMesh ISVC with a storage key reference.
func newModelMeshISVCWithStorage(namespace, name, runtimeName, storageKey, storagePath string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.InferenceService.APIVersion(),
			"kind":       resources.InferenceService.Kind,
			"metadata": map[string]any{
				"name":      name,
				"namespace": namespace,
				"uid":       "test-uid-mm-storage-123",
				"annotations": map[string]any{
					"serving.kserve.io/deploymentMode": "ModelMesh",
				},
			},
			"spec": map[string]any{
				"predictor": map[string]any{
					"model": map[string]any{
						"runtime": runtimeName,
						"storage": map[string]any{
							"key":  storageKey,
							"path": storagePath,
						},
					},
				},
			},
		},
	}
}

// storageConfigEntryJSON is a helper to build storage-config secret entries.
type storageConfigEntryJSON struct {
	Type      string `json:"type"`
	Bucket    string `json:"bucket,omitempty"`
	LocalPath string `json:"localPath,omitempty"`
}

// newStorageConfigSecret creates a storage-config secret with the given entries.
// Each key maps to a storageConfigEntryJSON that gets JSON-marshaled and base64-encoded.
func newStorageConfigSecret(namespace string, entries map[string]storageConfigEntryJSON) *unstructured.Unstructured {
	data := make(map[string]any, len(entries))

	for key, entry := range entries {
		jsonBytes, err := json.Marshal(entry)
		if err != nil {
			panic("test helper: failed to marshal storage-config entry: " + err.Error())
		}

		data[key] = base64.StdEncoding.EncodeToString(jsonBytes)
	}

	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.Secret.APIVersion(),
			"kind":       resources.Secret.Kind,
			"metadata": map[string]any{
				"name":      "storage-config",
				"namespace": namespace,
			},
			"data": data,
		},
	}
}
