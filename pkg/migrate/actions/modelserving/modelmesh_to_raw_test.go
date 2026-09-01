package modelserving_test

import (
	"testing"

	"github.com/blang/semver/v4"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/opendatahub-io/odh-cli/pkg/migrate/action"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/action/result"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/actions/modelserving"
	"github.com/opendatahub-io/odh-cli/pkg/resources"

	. "github.com/onsi/gomega"
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

		dynamicClient := newModelServingDynamicClient(isvc)

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
}

func TestModelMeshToRawAction_RunExecute(t *testing.T) {
	t.Run("should convert ModelMesh ISVCs to RawDeployment and rename container", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		isvc := newModelMeshISVC(testISVCNamespace, "mm-model", "ovms-runtime")
		sr := newServingRuntime(testISVCNamespace, "ovms-runtime", true)
		ns := newNamespace(testISVCNamespace, map[string]string{"modelmesh-enabled": "true"})

		dynamicClient := newModelServingDynamicClient(isvc, sr, ns)

		target := newTestTarget(dynamicClient, "2.25.0", false)

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

		dynamicClient := newModelServingDynamicClient(isvc, sr, ns)

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

		dynamicClient := newModelServingDynamicClient(isvc, sr, ns)

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

		dynamicClient := newModelServingDynamicClient()

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

	t.Run("should not mutate in dry-run mode", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		isvc := newModelMeshISVC(testISVCNamespace, "mm-model", "ovms-runtime")
		sr := newServingRuntime(testISVCNamespace, "ovms-runtime", true)
		ns := newNamespace(testISVCNamespace, map[string]string{"modelmesh-enabled": "true"})

		dynamicClient := newModelServingDynamicClient(isvc, sr, ns)

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

func TestModelMeshToRawAction_PVCConversion(t *testing.T) {
	t.Run("should convert PVC-backed ISVCs with storageUri rewrite", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		isvc := newModelMeshISVCWithStorage(testISVCNamespace, "pvc-model", "ovms-runtime", "my-pvc-key", "models/my-model")
		sr := newServingRuntime(testISVCNamespace, "ovms-runtime", true)
		ns := newNamespace(testISVCNamespace, map[string]string{"modelmesh-enabled": "true"})
		secret := newStorageConfigSecret(testISVCNamespace, map[string]storageConfigEntryJSON{
			"my-pvc-key": {Type: "pvc", Bucket: "my-pvc-volume", LocalPath: "/models/my-model"},
		})

		dynamicClient := newModelServingDynamicClient(isvc, sr, ns, secret)

		target := newTestTarget(dynamicClient, "2.25.0", false)

		a := &modelserving.ModelMeshToRawAction{}
		actionResult, err := a.Run().Execute(ctx, target)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(actionResult).ToNot(BeNil())

		// Verify ISVC was patched to RawDeployment with storageUri
		updated, err := dynamicClient.Resource(resources.InferenceService.GVR()).
			Namespace(testISVCNamespace).
			Get(ctx, "pvc-model", metav1.GetOptions{})

		g.Expect(err).ToNot(HaveOccurred())

		annotations := updated.GetAnnotations()
		g.Expect(annotations).To(HaveKeyWithValue("serving.kserve.io/deploymentMode", "RawDeployment"))

		// Verify storageUri was set
		storageURI, found, err := unstructured.NestedString(updated.Object, "spec", "predictor", "model", "storageUri")
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(found).To(BeTrue())
		g.Expect(storageURI).To(Equal("pvc://my-pvc-volume/models/my-model"))

		// Verify storage key was removed
		_, storageKeyFound, _ := unstructured.NestedString(updated.Object, "spec", "predictor", "model", "storage", "key")
		g.Expect(storageKeyFound).To(BeFalse())
	})

	t.Run("should patch OVMS args and port for PVC-backed ISVCs", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		isvc := newModelMeshISVCWithStorage(testISVCNamespace, "pvc-model", "ovms-runtime", "pvc-key", "model-dir")
		sr := newServingRuntime(testISVCNamespace, "ovms-runtime", true)
		ns := newNamespace(testISVCNamespace, nil)
		secret := newStorageConfigSecret(testISVCNamespace, map[string]storageConfigEntryJSON{
			"pvc-key": {Type: "pvc", Bucket: "data-pvc", LocalPath: "/model-dir"},
		})

		dynamicClient := newModelServingDynamicClient(isvc, sr, ns, secret)

		target := newTestTarget(dynamicClient, "2.25.0", false)

		a := &modelserving.ModelMeshToRawAction{}
		_, err := a.Run().Execute(ctx, target)

		g.Expect(err).ToNot(HaveOccurred())

		// Verify ServingRuntime was updated
		updatedSR, err := dynamicClient.Resource(resources.ServingRuntime.GVR()).
			Namespace(testISVCNamespace).
			Get(ctx, "ovms-runtime", metav1.GetOptions{})

		g.Expect(err).ToNot(HaveOccurred())

		// Verify multiModel=false
		multiModel, _, _ := unstructured.NestedBool(updatedSR.Object, "spec", "multiModel")
		g.Expect(multiModel).To(BeFalse())

		// Verify container name
		containers, _, _ := unstructured.NestedSlice(updatedSR.Object, "spec", "containers")
		g.Expect(containers).To(HaveLen(1))

		container := containers[0].(map[string]any)
		g.Expect(container).To(HaveKeyWithValue("name", "kserve-container"))

		// Verify OVMS single-model args
		args, ok := container["args"].([]any)
		g.Expect(ok).To(BeTrue())
		g.Expect(args).To(ContainElement("--model_name=pvc-model"))
		g.Expect(args).To(ContainElement("--model_path=/mnt/models"))
		g.Expect(args).To(ContainElement("--port=8001"))
		g.Expect(args).To(ContainElement("--rest_port=8888"))

		// Verify port 8888
		ports, ok := container["ports"].([]any)
		g.Expect(ok).To(BeTrue())
		g.Expect(ports).To(HaveLen(1))

		port := ports[0].(map[string]any)
		g.Expect(port).To(HaveKeyWithValue("containerPort", int64(8888)))
		g.Expect(port).To(HaveKeyWithValue("protocol", "TCP"))

		// Verify readiness probe
		probe, ok := container["readinessProbe"].(map[string]any)
		g.Expect(ok).To(BeTrue())

		tcpSocket, ok := probe["tcpSocket"].(map[string]any)
		g.Expect(ok).To(BeTrue())
		g.Expect(tcpSocket).To(HaveKeyWithValue("port", int64(8888)))
	})

	t.Run("should convert S3-backed ISVCs with storage key normally", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		isvc := newModelMeshISVCWithStorage(testISVCNamespace, "s3-model", "ovms-runtime", "s3-key", "model-path")
		sr := newServingRuntime(testISVCNamespace, "ovms-runtime", true)
		ns := newNamespace(testISVCNamespace, nil)
		secret := newStorageConfigSecret(testISVCNamespace, map[string]storageConfigEntryJSON{
			"s3-key": {Type: "s3", Bucket: "my-bucket"},
		})

		dynamicClient := newModelServingDynamicClient(isvc, sr, ns, secret)

		target := newTestTarget(dynamicClient, "2.25.0", false)

		a := &modelserving.ModelMeshToRawAction{}
		_, err := a.Run().Execute(ctx, target)

		g.Expect(err).ToNot(HaveOccurred())

		// Verify ISVC was patched to RawDeployment (standard path, no storageUri rewrite)
		updated, err := dynamicClient.Resource(resources.InferenceService.GVR()).
			Namespace(testISVCNamespace).
			Get(ctx, "s3-model", metav1.GetOptions{})

		g.Expect(err).ToNot(HaveOccurred())

		annotations := updated.GetAnnotations()
		g.Expect(annotations).To(HaveKeyWithValue("serving.kserve.io/deploymentMode", "RawDeployment"))

		// Verify storageUri was NOT set (S3 uses existing storage path)
		_, storageURIFound, _ := unstructured.NestedString(updated.Object, "spec", "predictor", "model", "storageUri")
		g.Expect(storageURIFound).To(BeFalse())

		// Verify storage key is still present
		_, storageKeyFound, _ := unstructured.NestedString(updated.Object, "spec", "predictor", "model", "storage", "key")
		g.Expect(storageKeyFound).To(BeTrue())
	})

	t.Run("should handle mixed S3 and PVC ISVCs", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		s3ISVC := newModelMeshISVCWithStorage(testISVCNamespace, "s3-model", "ovms-runtime", "s3-key", "s3-path")
		pvcISVC := newModelMeshISVCWithStorage(testISVCNamespace, "pvc-model", "ovms-runtime", "pvc-key", "pvc-path")
		sr := newServingRuntime(testISVCNamespace, "ovms-runtime", true)
		ns := newNamespace(testISVCNamespace, nil)
		secret := newStorageConfigSecret(testISVCNamespace, map[string]storageConfigEntryJSON{
			"s3-key":  {Type: "s3", Bucket: "s3-bucket"},
			"pvc-key": {Type: "pvc", Bucket: "pvc-volume", LocalPath: "/pvc-path"},
		})

		dynamicClient := newModelServingDynamicClient(s3ISVC, pvcISVC, sr, ns, secret)

		target := newTestTarget(dynamicClient, "2.25.0", false)

		a := &modelserving.ModelMeshToRawAction{}
		_, err := a.Run().Execute(ctx, target)

		g.Expect(err).ToNot(HaveOccurred())

		// Both should be RawDeployment
		for _, name := range []string{"s3-model", "pvc-model"} {
			updated, getErr := dynamicClient.Resource(resources.InferenceService.GVR()).
				Namespace(testISVCNamespace).
				Get(ctx, name, metav1.GetOptions{})

			g.Expect(getErr).ToNot(HaveOccurred())

			annotations := updated.GetAnnotations()
			g.Expect(annotations).To(HaveKeyWithValue("serving.kserve.io/deploymentMode", "RawDeployment"))
		}

		// PVC ISVC should have storageUri
		pvcUpdated, err := dynamicClient.Resource(resources.InferenceService.GVR()).
			Namespace(testISVCNamespace).
			Get(ctx, "pvc-model", metav1.GetOptions{})

		g.Expect(err).ToNot(HaveOccurred())

		storageURI, found, _ := unstructured.NestedString(pvcUpdated.Object, "spec", "predictor", "model", "storageUri")
		g.Expect(found).To(BeTrue())
		g.Expect(storageURI).To(Equal("pvc://pvc-volume/pvc-path"))

		// S3 ISVC should NOT have storageUri
		s3Updated, err := dynamicClient.Resource(resources.InferenceService.GVR()).
			Namespace(testISVCNamespace).
			Get(ctx, "s3-model", metav1.GetOptions{})

		g.Expect(err).ToNot(HaveOccurred())

		_, s3URIFound, _ := unstructured.NestedString(s3Updated.Object, "spec", "predictor", "model", "storageUri")
		g.Expect(s3URIFound).To(BeFalse())
	})

	t.Run("should surface PVC detection in dry-run mode", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		isvc := newModelMeshISVCWithStorage(testISVCNamespace, "pvc-model", "ovms-runtime", "pvc-key", "model-dir")
		sr := newServingRuntime(testISVCNamespace, "ovms-runtime", true)
		ns := newNamespace(testISVCNamespace, map[string]string{"modelmesh-enabled": "true"})
		secret := newStorageConfigSecret(testISVCNamespace, map[string]storageConfigEntryJSON{
			"pvc-key": {Type: "pvc", Bucket: "my-pvc", LocalPath: "/model-dir"},
		})

		dynamicClient := newModelServingDynamicClient(isvc, sr, ns, secret)

		target := newTestTarget(dynamicClient, "2.25.0", true)

		a := &modelserving.ModelMeshToRawAction{}
		actionResult, err := a.Run().Execute(ctx, target)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(actionResult).ToNot(BeNil())

		// Verify ISVC was NOT mutated
		original, err := dynamicClient.Resource(resources.InferenceService.GVR()).
			Namespace(testISVCNamespace).
			Get(ctx, "pvc-model", metav1.GetOptions{})

		g.Expect(err).ToNot(HaveOccurred())

		annotations := original.GetAnnotations()
		g.Expect(annotations).To(HaveKeyWithValue("serving.kserve.io/deploymentMode", "ModelMesh"))

		// Verify storage key still present (not mutated in dry-run)
		_, storageKeyFound, _ := unstructured.NestedString(original.Object, "spec", "predictor", "model", "storage", "key")
		g.Expect(storageKeyFound).To(BeTrue())

		// Verify PVC-specific dry-run step was recorded
		g.Expect(hasStepMessageContaining(
			actionResult.Status.Steps, result.StepSkipped, "Would set storageUri=",
		)).To(BeTrue())

		// Verify detection step recorded PVC-backed count
		g.Expect(hasStepMessageContaining(
			actionResult.Status.Steps, result.StepCompleted, "PVC-backed",
		)).To(BeTrue())
	})

	t.Run("should handle missing storage-config secret gracefully", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		// ISVC has storage key but no storage-config secret exists
		isvc := newModelMeshISVCWithStorage(testISVCNamespace, "no-secret-model", "ovms-runtime", "missing-key", "path")
		sr := newServingRuntime(testISVCNamespace, "ovms-runtime", true)
		ns := newNamespace(testISVCNamespace, nil)

		dynamicClient := newModelServingDynamicClient(isvc, sr, ns)

		target := newTestTarget(dynamicClient, "2.25.0", false)

		a := &modelserving.ModelMeshToRawAction{}
		_, err := a.Run().Execute(ctx, target)

		g.Expect(err).ToNot(HaveOccurred())

		// Should be treated as standard and converted normally
		updated, err := dynamicClient.Resource(resources.InferenceService.GVR()).
			Namespace(testISVCNamespace).
			Get(ctx, "no-secret-model", metav1.GetOptions{})

		g.Expect(err).ToNot(HaveOccurred())

		annotations := updated.GetAnnotations()
		g.Expect(annotations).To(HaveKeyWithValue("serving.kserve.io/deploymentMode", "RawDeployment"))
	})

	t.Run("should handle missing storage key in secret", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		// ISVC references a key not in the storage-config secret
		isvc := newModelMeshISVCWithStorage(testISVCNamespace, "orphan-key-model", "ovms-runtime", "nonexistent-key", "path")
		sr := newServingRuntime(testISVCNamespace, "ovms-runtime", true)
		ns := newNamespace(testISVCNamespace, nil)
		secret := newStorageConfigSecret(testISVCNamespace, map[string]storageConfigEntryJSON{
			"other-key": {Type: "s3", Bucket: "some-bucket"},
		})

		dynamicClient := newModelServingDynamicClient(isvc, sr, ns, secret)

		target := newTestTarget(dynamicClient, "2.25.0", false)

		a := &modelserving.ModelMeshToRawAction{}
		_, err := a.Run().Execute(ctx, target)

		g.Expect(err).ToNot(HaveOccurred())

		// Should be treated as standard and converted normally
		updated, err := dynamicClient.Resource(resources.InferenceService.GVR()).
			Namespace(testISVCNamespace).
			Get(ctx, "orphan-key-model", metav1.GetOptions{})

		g.Expect(err).ToNot(HaveOccurred())

		annotations := updated.GetAnnotations()
		g.Expect(annotations).To(HaveKeyWithValue("serving.kserve.io/deploymentMode", "RawDeployment"))
	})

	t.Run("should skip ISVC when storage-config entry is malformed", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		isvc := newModelMeshISVCWithStorage(testISVCNamespace, "bad-json-model", "ovms-runtime", "corrupt-key", "path")
		sr := newServingRuntime(testISVCNamespace, "ovms-runtime", true)
		ns := newNamespace(testISVCNamespace, nil)

		// Create secret with invalid base64 data for the storage key
		secret := &unstructured.Unstructured{
			Object: map[string]any{
				"apiVersion": resources.Secret.APIVersion(),
				"kind":       resources.Secret.Kind,
				"metadata": map[string]any{
					"name":      "storage-config",
					"namespace": testISVCNamespace,
				},
				"data": map[string]any{
					"corrupt-key": "not-valid-base64!!!",
				},
			},
		}

		dynamicClient := newModelServingDynamicClient(isvc, sr, ns, secret)

		target := newTestTarget(dynamicClient, "2.25.0", false)

		a := &modelserving.ModelMeshToRawAction{}
		actionResult, err := a.Run().Execute(ctx, target)

		g.Expect(err).ToNot(HaveOccurred())

		// ISVC should NOT be converted — detection failure means skip, not convert
		updated, err := dynamicClient.Resource(resources.InferenceService.GVR()).
			Namespace(testISVCNamespace).
			Get(ctx, "bad-json-model", metav1.GetOptions{})

		g.Expect(err).ToNot(HaveOccurred())

		annotations := updated.GetAnnotations()
		g.Expect(annotations).To(HaveKeyWithValue("serving.kserve.io/deploymentMode", "ModelMesh"))

		// Detection step should report failure
		g.Expect(hasStepMessageContaining(
			actionResult.Status.Steps, result.StepFailed, "Failed to detect storage type",
		)).To(BeTrue())
	})

	t.Run("should set deploymentMode in single update with storageUri", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		isvc := newModelMeshISVCWithStorage(testISVCNamespace, "pvc-single-update", "ovms-runtime", "pvc-key", "path")
		sr := newServingRuntime(testISVCNamespace, "ovms-runtime", true)
		ns := newNamespace(testISVCNamespace, nil)
		secret := newStorageConfigSecret(testISVCNamespace, map[string]storageConfigEntryJSON{
			"pvc-key": {Type: "pvc", Bucket: "vol", LocalPath: "/path"},
		})

		dynamicClient := newModelServingDynamicClient(isvc, sr, ns, secret)

		target := newTestTarget(dynamicClient, "2.25.0", false)

		a := &modelserving.ModelMeshToRawAction{}
		_, err := a.Run().Execute(ctx, target)

		g.Expect(err).ToNot(HaveOccurred())

		updated, err := dynamicClient.Resource(resources.InferenceService.GVR()).
			Namespace(testISVCNamespace).
			Get(ctx, "pvc-single-update", metav1.GetOptions{})

		g.Expect(err).ToNot(HaveOccurred())

		// Both storageUri and deploymentMode should be set
		storageURI, found, _ := unstructured.NestedString(updated.Object, "spec", "predictor", "model", "storageUri")
		g.Expect(found).To(BeTrue())
		g.Expect(storageURI).To(Equal("pvc://vol/path"))

		annotations := updated.GetAnnotations()
		g.Expect(annotations).To(HaveKeyWithValue("serving.kserve.io/deploymentMode", "RawDeployment"))
	})

	t.Run("should warn when multiple PVC ISVCs share a ServingRuntime", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		isvc1 := newModelMeshISVCWithStorage(testISVCNamespace, "pvc-model-1", "shared-runtime", "pvc-key-1", "path1")
		isvc2 := newModelMeshISVCWithStorage(testISVCNamespace, "pvc-model-2", "shared-runtime", "pvc-key-2", "path2")
		sr := newServingRuntime(testISVCNamespace, "shared-runtime", true)
		ns := newNamespace(testISVCNamespace, nil)
		secret := newStorageConfigSecret(testISVCNamespace, map[string]storageConfigEntryJSON{
			"pvc-key-1": {Type: "pvc", Bucket: "vol1", LocalPath: "/path1"},
			"pvc-key-2": {Type: "pvc", Bucket: "vol2", LocalPath: "/path2"},
		})

		dynamicClient := newModelServingDynamicClient(isvc1, isvc2, sr, ns, secret)

		target := newTestTarget(dynamicClient, "2.25.0", false)

		a := &modelserving.ModelMeshToRawAction{}
		actionResult, err := a.Run().Execute(ctx, target)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(actionResult).ToNot(BeNil())

		// Both ISVCs should still be converted to RawDeployment
		for _, name := range []string{"pvc-model-1", "pvc-model-2"} {
			updated, getErr := dynamicClient.Resource(resources.InferenceService.GVR()).
				Namespace(testISVCNamespace).
				Get(ctx, name, metav1.GetOptions{})

			g.Expect(getErr).ToNot(HaveOccurred())

			ann := updated.GetAnnotations()
			g.Expect(ann).To(HaveKeyWithValue("serving.kserve.io/deploymentMode", "RawDeployment"))
		}

		// Result should contain a failed step for the shared runtime
		g.Expect(actionResult.HasFailedSteps()).To(BeTrue())

		// Verify runtime keeps --model_name from first ISVC processed
		updatedSR, err := dynamicClient.Resource(resources.ServingRuntime.GVR()).
			Namespace(testISVCNamespace).
			Get(ctx, "shared-runtime", metav1.GetOptions{})

		g.Expect(err).ToNot(HaveOccurred())

		containers, _, _ := unstructured.NestedSlice(updatedSR.Object, "spec", "containers")
		g.Expect(containers).To(HaveLen(1))

		container := containers[0].(map[string]any)
		args, ok := container["args"].([]any)
		g.Expect(ok).To(BeTrue())
		g.Expect(args).To(ContainElement("--model_name=pvc-model-1"))
	})

	t.Run("should reject PVC ISVC with empty bucket in storage-config", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		isvc := newModelMeshISVCWithStorage(testISVCNamespace, "empty-bucket-model", "ovms-runtime", "bad-key", "path")
		sr := newServingRuntime(testISVCNamespace, "ovms-runtime", true)
		ns := newNamespace(testISVCNamespace, nil)
		secret := newStorageConfigSecret(testISVCNamespace, map[string]storageConfigEntryJSON{
			"bad-key": {Type: "pvc", Bucket: "", LocalPath: "/model"},
		})

		dynamicClient := newModelServingDynamicClient(isvc, sr, ns, secret)

		target := newTestTarget(dynamicClient, "2.25.0", false)

		a := &modelserving.ModelMeshToRawAction{}
		actionResult, err := a.Run().Execute(ctx, target)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(actionResult.HasFailedSteps()).To(BeTrue())

		// ISVC should NOT have storageUri set (validation failed)
		updated, err := dynamicClient.Resource(resources.InferenceService.GVR()).
			Namespace(testISVCNamespace).
			Get(ctx, "empty-bucket-model", metav1.GetOptions{})

		g.Expect(err).ToNot(HaveOccurred())

		_, storageURIFound, _ := unstructured.NestedString(updated.Object, "spec", "predictor", "model", "storageUri")
		g.Expect(storageURIFound).To(BeFalse())
	})

	t.Run("should reject PVC ISVC with path traversal in localPath", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		isvc := newModelMeshISVCWithStorage(testISVCNamespace, "traversal-model", "ovms-runtime", "bad-key", "path")
		sr := newServingRuntime(testISVCNamespace, "ovms-runtime", true)
		ns := newNamespace(testISVCNamespace, nil)
		secret := newStorageConfigSecret(testISVCNamespace, map[string]storageConfigEntryJSON{
			"bad-key": {Type: "pvc", Bucket: "my-pvc", LocalPath: "/../../etc/passwd"},
		})

		dynamicClient := newModelServingDynamicClient(isvc, sr, ns, secret)

		target := newTestTarget(dynamicClient, "2.25.0", false)

		a := &modelserving.ModelMeshToRawAction{}
		actionResult, err := a.Run().Execute(ctx, target)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(actionResult.HasFailedSteps()).To(BeTrue())

		// ISVC should NOT have storageUri set (validation failed)
		updated, err := dynamicClient.Resource(resources.InferenceService.GVR()).
			Namespace(testISVCNamespace).
			Get(ctx, "traversal-model", metav1.GetOptions{})

		g.Expect(err).ToNot(HaveOccurred())

		_, storageURIFound, _ := unstructured.NestedString(updated.Object, "spec", "predictor", "model", "storageUri")
		g.Expect(storageURIFound).To(BeFalse())
	})
}
