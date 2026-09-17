package modelserving_test

import (
	"encoding/base64"
	"testing"

	"github.com/blang/semver/v4"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	"github.com/opendatahub-io/odh-cli/pkg/migrate/action"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/action/result"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/actions/modelserving"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"

	. "github.com/onsi/gomega"
)

const (
	testStorageConfigPVC = `{"type":"pvc","bucket":"","endpoint_url":""}`
	testStorageConfigS3  = `{"type":"s3","bucket":"my-bucket","endpoint_url":"https://s3.example.com"}`
)

func newModelMeshISVC(namespace, name, runtimeName string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.InferenceService.APIVersion(),
			"kind":       resources.InferenceService.Kind,
			"metadata": map[string]any{
				"name":      name,
				"namespace": namespace,
				"uid":       "test-uid-mm-123",
				"annotations": map[string]any{
					"serving.kserve.io/deploymentMode": "ModelMesh",
				},
			},
			"spec": map[string]any{
				"predictor": map[string]any{
					"model": map[string]any{
						"runtime": runtimeName,
					},
				},
			},
		},
	}
}

func newServingRuntime(namespace, name string, multiModel bool) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.ServingRuntime.APIVersion(),
			"kind":       resources.ServingRuntime.Kind,
			"metadata": map[string]any{
				"name":      name,
				"namespace": namespace,
			},
			"spec": map[string]any{
				"multiModel": multiModel,
				"containers": []any{
					map[string]any{
						"name":  "ovms",
						"image": "openvino/model_server:latest",
					},
				},
			},
		},
	}
}

func newModelMeshISVCWithAuth(namespace, name, runtimeName string) *unstructured.Unstructured {
	isvc := newModelMeshISVC(namespace, name, runtimeName)

	annotations := isvc.GetAnnotations()
	annotations["security.opendatahub.io/enable-auth"] = "true"
	isvc.SetAnnotations(annotations)

	return isvc
}

func newNamespace(name string, labels map[string]string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.Namespace.APIVersion(),
			"kind":       resources.Namespace.Kind,
			"metadata": map[string]any{
				"name": name,
			},
		},
	}

	if labels != nil {
		labelsAny := make(map[string]any, len(labels))
		for k, v := range labels {
			labelsAny[k] = v
		}

		obj.Object["metadata"].(map[string]any)["labels"] = labelsAny
	}

	return obj
}

func newStorageConfigSecret(namespace string, entries map[string]string) *unstructured.Unstructured {
	data := make(map[string]any, len(entries))
	for key, jsonVal := range entries {
		data[key] = base64.StdEncoding.EncodeToString([]byte(jsonVal))
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

func newModelMeshISVCWithStorage(namespace, name, runtimeName, storageKey string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.InferenceService.APIVersion(),
			"kind":       resources.InferenceService.Kind,
			"metadata": map[string]any{
				"name":      name,
				"namespace": namespace,
				"uid":       "test-uid-mm-storage",
				"annotations": map[string]any{
					"serving.kserve.io/deploymentMode": "ModelMesh",
				},
			},
			"spec": map[string]any{
				"predictor": map[string]any{
					"model": map[string]any{
						"runtime": runtimeName,
						"storage": map[string]any{
							"key": storageKey,
						},
					},
				},
			},
		},
	}
}

func TestModelMeshToRawAction_ID(t *testing.T) {
	g := NewWithT(t)

	a := &modelserving.ModelMeshToRawAction{}
	g.Expect(a.ID()).To(Equal("modelserving.modelmesh-to-raw"))
}

func TestModelMeshToRawAction_CanApply(t *testing.T) {
	t.Run("should return true for version 2.25", func(t *testing.T) {
		g := NewWithT(t)

		a := &modelserving.ModelMeshToRawAction{}
		v := semver.MustParse("2.25.0")
		target := action.Target{CurrentVersion: &v}

		g.Expect(a.CanApply(target)).To(BeTrue())
	})

	t.Run("should return false for version 3.x", func(t *testing.T) {
		g := NewWithT(t)

		a := &modelserving.ModelMeshToRawAction{}
		v := semver.MustParse("3.0.0")
		target := action.Target{CurrentVersion: &v}

		g.Expect(a.CanApply(target)).To(BeFalse())
	})
}

func TestModelMeshToRawAction_RunValidate(t *testing.T) {
	t.Run("should report ModelMesh ISVCs found", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		isvc := newModelMeshISVC(testISVCNamespace, "mm-model", "ovms")

		scheme := runtime.NewScheme()

		listKinds := map[schema.GroupVersionResource]string{
			resources.InferenceService.GVR(): resources.InferenceService.ListKind(),
			resources.Secret.GVR():           resources.Secret.ListKind(),
		}

		dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
			scheme, listKinds, isvc,
		)

		target := newTestTarget(dynamicClient, "2.25.0", false)

		a := &modelserving.ModelMeshToRawAction{}
		actionResult, err := a.Run().Validate(ctx, target)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(actionResult).ToNot(BeNil())

		hasCompleted := false
		for _, step := range actionResult.Status.Steps {
			if step.Status == result.StepCompleted {
				hasCompleted = true
			}
		}

		g.Expect(hasCompleted).To(BeTrue())
	})

	t.Run("should detect PVC-backed ISVCs in validation", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		isvc := newModelMeshISVCWithStorage(testISVCNamespace, "pvc-model", "ovms", "my-pvc-key")
		secret := newStorageConfigSecret(testISVCNamespace, map[string]string{
			"my-pvc-key": testStorageConfigPVC,
		})

		scheme := runtime.NewScheme()

		listKinds := map[schema.GroupVersionResource]string{
			resources.InferenceService.GVR(): resources.InferenceService.ListKind(),
			resources.Secret.GVR():           resources.Secret.ListKind(),
		}

		dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
			scheme, listKinds, isvc, secret,
		)

		target := newTestTarget(dynamicClient, "2.25.0", false)

		a := &modelserving.ModelMeshToRawAction{}
		actionResult, err := a.Run().Validate(ctx, target)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(actionResult).ToNot(BeNil())

		hasCompleted := false
		hasSkipped := false

		for _, step := range actionResult.Status.Steps {
			if step.Status == result.StepCompleted {
				hasCompleted = true
			}

			if step.Status == result.StepSkipped {
				hasSkipped = true
			}
		}

		g.Expect(hasCompleted).To(BeTrue())
		g.Expect(hasSkipped).To(BeTrue())
	})
}

func TestModelMeshToRawAction_RunExecute(t *testing.T) {
	t.Run("should convert ModelMesh ISVCs to RawDeployment and rename container", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		isvc := newModelMeshISVC(testISVCNamespace, "mm-model", "ovms-runtime")
		sr := newServingRuntime(testISVCNamespace, "ovms-runtime", true)
		ns := newNamespace(testISVCNamespace, map[string]string{"modelmesh-enabled": "true"})

		scheme := runtime.NewScheme()

		listKinds := map[schema.GroupVersionResource]string{
			resources.InferenceService.GVR(): resources.InferenceService.ListKind(),
			resources.ServingRuntime.GVR():   resources.ServingRuntime.ListKind(),
			resources.ServiceAccount.GVR():   resources.ServiceAccount.ListKind(),
			resources.Role.GVR():             resources.Role.ListKind(),
			resources.RoleBinding.GVR():      resources.RoleBinding.ListKind(),
			resources.Namespace.GVR():        resources.Namespace.ListKind(),
			resources.Secret.GVR():           resources.Secret.ListKind(),
		}

		dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
			scheme, listKinds, isvc, sr, ns,
		)

		testClient := client.NewForTesting(client.TestClientConfig{
			Dynamic: dynamicClient,
		})

		v := semver.MustParse("2.25.0")
		tv := semver.MustParse("3.0.0")

		target := action.Target{
			Client:         testClient,
			CurrentVersion: &v,
			TargetVersion:  &tv,
			DryRun:         false,
			SkipConfirm:    true,
			Recorder:       action.NewRootRecorder(),
		}

		a := &modelserving.ModelMeshToRawAction{}
		actionResult, err := a.Run().Execute(ctx, target)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(actionResult).ToNot(BeNil())

		// Verify ISVC was patched to RawDeployment
		updated, err := dynamicClient.Resource(resources.InferenceService.GVR()).
			Namespace(testISVCNamespace).
			Get(ctx, "mm-model", metav1.GetOptions{})

		g.Expect(err).ToNot(HaveOccurred())

		annotations := updated.GetAnnotations()
		g.Expect(annotations).To(HaveKeyWithValue("serving.kserve.io/deploymentMode", "RawDeployment"))

		// Verify ServingRuntime container was renamed to kserve-container
		updatedSR, err := dynamicClient.Resource(resources.ServingRuntime.GVR()).
			Namespace(testISVCNamespace).
			Get(ctx, "ovms-runtime", metav1.GetOptions{})

		g.Expect(err).ToNot(HaveOccurred())

		containers, _, _ := unstructured.NestedSlice(updatedSR.Object, "spec", "containers")
		g.Expect(containers).To(HaveLen(1))

		firstContainer := containers[0].(map[string]any)
		g.Expect(firstContainer).To(HaveKeyWithValue("name", "kserve-container"))

		// Verify modelmesh-enabled label was removed from namespace
		updatedNS, err := dynamicClient.Resource(resources.Namespace.GVR()).
			Get(ctx, testISVCNamespace, metav1.GetOptions{})

		g.Expect(err).ToNot(HaveOccurred())

		nsLabels := updatedNS.GetLabels()
		g.Expect(nsLabels).ToNot(HaveKey("modelmesh-enabled"))
	})

	t.Run("should skip auth resources when enable-auth annotation is not set", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		isvc := newModelMeshISVC(testISVCNamespace, "mm-model-noauth", "ovms-runtime")
		sr := newServingRuntime(testISVCNamespace, "ovms-runtime", true)
		ns := newNamespace(testISVCNamespace, nil)

		scheme := runtime.NewScheme()

		listKinds := map[schema.GroupVersionResource]string{
			resources.InferenceService.GVR(): resources.InferenceService.ListKind(),
			resources.ServingRuntime.GVR():   resources.ServingRuntime.ListKind(),
			resources.ServiceAccount.GVR():   resources.ServiceAccount.ListKind(),
			resources.Role.GVR():             resources.Role.ListKind(),
			resources.RoleBinding.GVR():      resources.RoleBinding.ListKind(),
			resources.Namespace.GVR():        resources.Namespace.ListKind(),
			resources.Secret.GVR():           resources.Secret.ListKind(),
		}

		dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
			scheme, listKinds, isvc, sr, ns,
		)

		target := newTestTarget(dynamicClient, "2.25.0", false)

		a := &modelserving.ModelMeshToRawAction{}
		_, err := a.Run().Execute(ctx, target)

		g.Expect(err).ToNot(HaveOccurred())

		// Verify no ServiceAccount was created (auth not enabled)
		_, saErr := dynamicClient.Resource(resources.ServiceAccount.GVR()).
			Namespace(testISVCNamespace).
			Get(ctx, "mm-model-noauth-sa", metav1.GetOptions{})

		g.Expect(saErr).To(HaveOccurred())
	})

	t.Run("should create auth resources when enable-auth annotation is set", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		isvc := newModelMeshISVCWithAuth(testISVCNamespace, "mm-model-auth", "ovms-runtime")
		sr := newServingRuntime(testISVCNamespace, "ovms-runtime", true)
		ns := newNamespace(testISVCNamespace, nil)

		scheme := runtime.NewScheme()

		listKinds := map[schema.GroupVersionResource]string{
			resources.InferenceService.GVR(): resources.InferenceService.ListKind(),
			resources.ServingRuntime.GVR():   resources.ServingRuntime.ListKind(),
			resources.ServiceAccount.GVR():   resources.ServiceAccount.ListKind(),
			resources.Role.GVR():             resources.Role.ListKind(),
			resources.RoleBinding.GVR():      resources.RoleBinding.ListKind(),
			resources.Namespace.GVR():        resources.Namespace.ListKind(),
			resources.Secret.GVR():           resources.Secret.ListKind(),
		}

		dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
			scheme, listKinds, isvc, sr, ns,
		)

		target := newTestTarget(dynamicClient, "2.25.0", false)

		a := &modelserving.ModelMeshToRawAction{}
		_, err := a.Run().Execute(ctx, target)

		g.Expect(err).ToNot(HaveOccurred())

		// Verify ServiceAccount was created (auth enabled)
		_, saErr := dynamicClient.Resource(resources.ServiceAccount.GVR()).
			Namespace(testISVCNamespace).
			Get(ctx, "mm-model-auth-sa", metav1.GetOptions{})

		g.Expect(saErr).ToNot(HaveOccurred())
	})

	t.Run("should skip when no ModelMesh ISVCs exist", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		scheme := runtime.NewScheme()

		listKinds := map[schema.GroupVersionResource]string{
			resources.InferenceService.GVR(): resources.InferenceService.ListKind(),
		}

		dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, listKinds)

		target := newTestTarget(dynamicClient, "2.25.0", false)

		a := &modelserving.ModelMeshToRawAction{}
		actionResult, err := a.Run().Execute(ctx, target)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(actionResult).ToNot(BeNil())

		hasSkipped := false
		for _, step := range actionResult.Status.Steps {
			if step.Status == result.StepSkipped {
				hasSkipped = true
			}
		}

		g.Expect(hasSkipped).To(BeTrue())
	})

	t.Run("should skip PVC-backed ISVCs with warning", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		isvc := newModelMeshISVCWithStorage(testISVCNamespace, "pvc-model", "ovms-runtime", "my-pvc-key")
		sr := newServingRuntime(testISVCNamespace, "ovms-runtime", true)
		ns := newNamespace(testISVCNamespace, map[string]string{"modelmesh-enabled": "true"})
		secret := newStorageConfigSecret(testISVCNamespace, map[string]string{
			"my-pvc-key": testStorageConfigPVC,
		})

		scheme := runtime.NewScheme()

		listKinds := map[schema.GroupVersionResource]string{
			resources.InferenceService.GVR(): resources.InferenceService.ListKind(),
			resources.ServingRuntime.GVR():   resources.ServingRuntime.ListKind(),
			resources.ServiceAccount.GVR():   resources.ServiceAccount.ListKind(),
			resources.Role.GVR():             resources.Role.ListKind(),
			resources.RoleBinding.GVR():      resources.RoleBinding.ListKind(),
			resources.Namespace.GVR():        resources.Namespace.ListKind(),
			resources.Secret.GVR():           resources.Secret.ListKind(),
		}

		dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
			scheme, listKinds, isvc, sr, ns, secret,
		)

		target := newTestTarget(dynamicClient, "2.25.0", false)

		a := &modelserving.ModelMeshToRawAction{}
		actionResult, err := a.Run().Execute(ctx, target)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(actionResult).ToNot(BeNil())

		// Verify ISVC was NOT patched (still ModelMesh)
		original, err := dynamicClient.Resource(resources.InferenceService.GVR()).
			Namespace(testISVCNamespace).
			Get(ctx, "pvc-model", metav1.GetOptions{})

		g.Expect(err).ToNot(HaveOccurred())

		annotations := original.GetAnnotations()
		g.Expect(annotations).To(HaveKeyWithValue("serving.kserve.io/deploymentMode", "ModelMesh"))

		// Verify step recorded a skip for the PVC ISVC
		hasSkipped := false
		for _, step := range actionResult.Status.Steps {
			if step.Status == result.StepSkipped {
				hasSkipped = true
			}
		}

		g.Expect(hasSkipped).To(BeTrue())
	})

	t.Run("should convert S3-backed ISVCs normally", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		isvc := newModelMeshISVCWithStorage(testISVCNamespace, "s3-model", "ovms-runtime", "my-s3-key")
		sr := newServingRuntime(testISVCNamespace, "ovms-runtime", true)
		ns := newNamespace(testISVCNamespace, map[string]string{"modelmesh-enabled": "true"})
		secret := newStorageConfigSecret(testISVCNamespace, map[string]string{
			"my-s3-key": testStorageConfigS3,
		})

		scheme := runtime.NewScheme()

		listKinds := map[schema.GroupVersionResource]string{
			resources.InferenceService.GVR(): resources.InferenceService.ListKind(),
			resources.ServingRuntime.GVR():   resources.ServingRuntime.ListKind(),
			resources.ServiceAccount.GVR():   resources.ServiceAccount.ListKind(),
			resources.Role.GVR():             resources.Role.ListKind(),
			resources.RoleBinding.GVR():      resources.RoleBinding.ListKind(),
			resources.Namespace.GVR():        resources.Namespace.ListKind(),
			resources.Secret.GVR():           resources.Secret.ListKind(),
		}

		dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
			scheme, listKinds, isvc, sr, ns, secret,
		)

		target := newTestTarget(dynamicClient, "2.25.0", false)

		a := &modelserving.ModelMeshToRawAction{}
		actionResult, err := a.Run().Execute(ctx, target)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(actionResult).ToNot(BeNil())

		// Verify ISVC was patched to RawDeployment
		updated, err := dynamicClient.Resource(resources.InferenceService.GVR()).
			Namespace(testISVCNamespace).
			Get(ctx, "s3-model", metav1.GetOptions{})

		g.Expect(err).ToNot(HaveOccurred())

		annotations := updated.GetAnnotations()
		g.Expect(annotations).To(HaveKeyWithValue("serving.kserve.io/deploymentMode", "RawDeployment"))
	})

	t.Run("should handle mixed PVC and S3 ISVCs", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		pvcISVC := newModelMeshISVCWithStorage(testISVCNamespace, "pvc-model", "ovms-runtime", "pvc-key")
		s3ISVC := newModelMeshISVCWithStorage(testISVCNamespace, "s3-model", "ovms-runtime", "s3-key")
		sr := newServingRuntime(testISVCNamespace, "ovms-runtime", true)
		ns := newNamespace(testISVCNamespace, map[string]string{"modelmesh-enabled": "true"})
		secret := newStorageConfigSecret(testISVCNamespace, map[string]string{
			"pvc-key": testStorageConfigPVC,
			"s3-key":  testStorageConfigS3,
		})

		scheme := runtime.NewScheme()

		listKinds := map[schema.GroupVersionResource]string{
			resources.InferenceService.GVR(): resources.InferenceService.ListKind(),
			resources.ServingRuntime.GVR():   resources.ServingRuntime.ListKind(),
			resources.ServiceAccount.GVR():   resources.ServiceAccount.ListKind(),
			resources.Role.GVR():             resources.Role.ListKind(),
			resources.RoleBinding.GVR():      resources.RoleBinding.ListKind(),
			resources.Namespace.GVR():        resources.Namespace.ListKind(),
			resources.Secret.GVR():           resources.Secret.ListKind(),
		}

		dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
			scheme, listKinds, pvcISVC, s3ISVC, sr, ns, secret,
		)

		target := newTestTarget(dynamicClient, "2.25.0", false)

		a := &modelserving.ModelMeshToRawAction{}
		actionResult, err := a.Run().Execute(ctx, target)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(actionResult).ToNot(BeNil())

		// Verify S3 ISVC was converted
		s3Updated, err := dynamicClient.Resource(resources.InferenceService.GVR()).
			Namespace(testISVCNamespace).
			Get(ctx, "s3-model", metav1.GetOptions{})

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(s3Updated.GetAnnotations()).To(HaveKeyWithValue("serving.kserve.io/deploymentMode", "RawDeployment"))

		// Verify PVC ISVC was NOT converted
		pvcOriginal, err := dynamicClient.Resource(resources.InferenceService.GVR()).
			Namespace(testISVCNamespace).
			Get(ctx, "pvc-model", metav1.GetOptions{})

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(pvcOriginal.GetAnnotations()).To(HaveKeyWithValue("serving.kserve.io/deploymentMode", "ModelMesh"))
	})

	t.Run("should surface PVC warning in dry-run mode", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		isvc := newModelMeshISVCWithStorage(testISVCNamespace, "pvc-model", "ovms-runtime", "my-pvc-key")
		sr := newServingRuntime(testISVCNamespace, "ovms-runtime", true)
		ns := newNamespace(testISVCNamespace, map[string]string{"modelmesh-enabled": "true"})
		secret := newStorageConfigSecret(testISVCNamespace, map[string]string{
			"my-pvc-key": testStorageConfigPVC,
		})

		scheme := runtime.NewScheme()

		listKinds := map[schema.GroupVersionResource]string{
			resources.InferenceService.GVR(): resources.InferenceService.ListKind(),
			resources.ServingRuntime.GVR():   resources.ServingRuntime.ListKind(),
			resources.ServiceAccount.GVR():   resources.ServiceAccount.ListKind(),
			resources.Role.GVR():             resources.Role.ListKind(),
			resources.RoleBinding.GVR():      resources.RoleBinding.ListKind(),
			resources.Namespace.GVR():        resources.Namespace.ListKind(),
			resources.Secret.GVR():           resources.Secret.ListKind(),
		}

		dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
			scheme, listKinds, isvc, sr, ns, secret,
		)

		target := newTestTarget(dynamicClient, "2.25.0", true)

		a := &modelserving.ModelMeshToRawAction{}
		actionResult, err := a.Run().Execute(ctx, target)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(actionResult).ToNot(BeNil())

		// Verify ISVC was NOT patched (dry-run + PVC skip)
		original, err := dynamicClient.Resource(resources.InferenceService.GVR()).
			Namespace(testISVCNamespace).
			Get(ctx, "pvc-model", metav1.GetOptions{})

		g.Expect(err).ToNot(HaveOccurred())

		annotations := original.GetAnnotations()
		g.Expect(annotations).To(HaveKeyWithValue("serving.kserve.io/deploymentMode", "ModelMesh"))

		// Verify steps contain skip status
		hasSkipped := false
		for _, step := range actionResult.Status.Steps {
			if step.Status == result.StepSkipped {
				hasSkipped = true
			}
		}

		g.Expect(hasSkipped).To(BeTrue())
	})

	t.Run("should not mutate in dry-run mode", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		isvc := newModelMeshISVC(testISVCNamespace, "mm-model", "ovms-runtime")
		sr := newServingRuntime(testISVCNamespace, "ovms-runtime", true)
		ns := newNamespace(testISVCNamespace, map[string]string{"modelmesh-enabled": "true"})

		scheme := runtime.NewScheme()

		listKinds := map[schema.GroupVersionResource]string{
			resources.InferenceService.GVR(): resources.InferenceService.ListKind(),
			resources.ServingRuntime.GVR():   resources.ServingRuntime.ListKind(),
			resources.ServiceAccount.GVR():   resources.ServiceAccount.ListKind(),
			resources.Role.GVR():             resources.Role.ListKind(),
			resources.RoleBinding.GVR():      resources.RoleBinding.ListKind(),
			resources.Namespace.GVR():        resources.Namespace.ListKind(),
			resources.Secret.GVR():           resources.Secret.ListKind(),
		}

		dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
			scheme, listKinds, isvc, sr, ns,
		)

		target := newTestTarget(dynamicClient, "2.25.0", true)

		a := &modelserving.ModelMeshToRawAction{}
		actionResult, err := a.Run().Execute(ctx, target)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(actionResult).ToNot(BeNil())

		// Verify ISVC was NOT patched
		original, err := dynamicClient.Resource(resources.InferenceService.GVR()).
			Namespace(testISVCNamespace).
			Get(ctx, "mm-model", metav1.GetOptions{})

		g.Expect(err).ToNot(HaveOccurred())

		annotations := original.GetAnnotations()
		g.Expect(annotations).To(HaveKeyWithValue("serving.kserve.io/deploymentMode", "ModelMesh"))

		// Verify ServingRuntime container was NOT renamed
		originalSR, err := dynamicClient.Resource(resources.ServingRuntime.GVR()).
			Namespace(testISVCNamespace).
			Get(ctx, "ovms-runtime", metav1.GetOptions{})

		g.Expect(err).ToNot(HaveOccurred())

		containers, _, _ := unstructured.NestedSlice(originalSR.Object, "spec", "containers")
		g.Expect(containers).To(HaveLen(1))

		firstContainer := containers[0].(map[string]any)
		g.Expect(firstContainer).To(HaveKeyWithValue("name", "ovms"))
	})
}
