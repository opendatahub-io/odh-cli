package modelserving_test

import (
	"errors"
	"testing"

	"github.com/blang/semver/v4"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	k8stesting "k8s.io/client-go/testing"

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
	return newServingRuntimeWithArgs(namespace, name, multiModel)
}

func newServingRuntimeWithArgs(namespace, name string, multiModel bool, args ...string) *unstructured.Unstructured {
	container := map[string]any{
		"name":  "ovms",
		"image": "openvino/model_server:latest",
	}
	if len(args) > 0 {
		containerArgs := make([]any, len(args))
		for i, arg := range args {
			containerArgs[i] = arg
		}
		container["args"] = containerArgs
	}

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
					container,
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
		ns := newNamespace(testISVCNamespace, map[string]string{"modelmesh-enabled": "true"})

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

	t.Run("should not convert standard ISVC when ServingRuntime update fails", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		isvc := newModelMeshISVC(testISVCNamespace, "standard-runtime-failure", "ovms-runtime")
		sr := newServingRuntime(testISVCNamespace, "ovms-runtime", true)
		ns := newNamespace(testISVCNamespace, nil)

		dynamicClient := newModelServingDynamicClient(isvc, sr, ns)
		dynamicClient.PrependReactor("update", "servingruntimes", func(_ k8stesting.Action) (bool, k8sruntime.Object, error) {
			return true, nil, apierrors.NewInternalError(errors.New(testRuntimeUpdateFailure))
		})

		target := newTestTarget(dynamicClient, "2.25.0", false)

		a := &modelserving.ModelMeshToRawAction{}
		actionResult, err := a.Run().Execute(ctx, target)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(actionResult.HasFailedSteps()).To(BeTrue())

		updated, err := dynamicClient.Resource(resources.InferenceService.GVR()).
			Namespace(testISVCNamespace).
			Get(ctx, "standard-runtime-failure", metav1.GetOptions{})

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(updated.GetAnnotations()).To(HaveKeyWithValue("serving.kserve.io/deploymentMode", "ModelMesh"))
	})

	t.Run("should fail standard conversion on ServingRuntime API errors", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		isvc := newModelMeshISVC(testISVCNamespace, "standard-runtime-get-failure", "ovms-runtime")
		sr := newServingRuntime(testISVCNamespace, "ovms-runtime", true)
		ns := newNamespace(testISVCNamespace, nil)

		dynamicClient := newModelServingDynamicClient(isvc, sr, ns)
		dynamicClient.PrependReactor("get", "servingruntimes", func(_ k8stesting.Action) (bool, k8sruntime.Object, error) {
			return true, nil, apierrors.NewInternalError(errors.New(testRuntimeGetFailure))
		})

		target := newTestTarget(dynamicClient, "2.25.0", false)

		a := &modelserving.ModelMeshToRawAction{}
		actionResult, err := a.Run().Execute(ctx, target)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(actionResult.HasFailedSteps()).To(BeTrue())

		updated, err := dynamicClient.Resource(resources.InferenceService.GVR()).
			Namespace(testISVCNamespace).
			Get(ctx, "standard-runtime-get-failure", metav1.GetOptions{})

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(updated.GetAnnotations()).To(HaveKeyWithValue("serving.kserve.io/deploymentMode", "ModelMesh"))
	})

	t.Run("should not convert ISVC when auth resource creation fails", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		isvc := newModelMeshISVCWithAuth(testISVCNamespace, "auth-resource-failure", "ovms-runtime")
		sr := newServingRuntime(testISVCNamespace, "ovms-runtime", true)
		ns := newNamespace(testISVCNamespace, nil)

		dynamicClient := newModelServingDynamicClient(isvc, sr, ns)
		dynamicClient.PrependReactor("create", "serviceaccounts", func(_ k8stesting.Action) (bool, k8sruntime.Object, error) {
			return true, nil, apierrors.NewInternalError(errors.New(testAuthResourceFailure))
		})

		target := newTestTarget(dynamicClient, "2.25.0", false)

		a := &modelserving.ModelMeshToRawAction{}
		actionResult, err := a.Run().Execute(ctx, target)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(actionResult.HasFailedSteps()).To(BeTrue())

		updated, err := dynamicClient.Resource(resources.InferenceService.GVR()).
			Namespace(testISVCNamespace).
			Get(ctx, "auth-resource-failure", metav1.GetOptions{})

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(updated.GetAnnotations()).To(HaveKeyWithValue("serving.kserve.io/deploymentMode", "ModelMesh"))
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
			"my-pvc-key": {Type: "pvc", Name: "my-pvc-volume"},
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
		sr := newServingRuntimeWithArgs(
			testISVCNamespace,
			"ovms-runtime",
			true,
			testOVMSFileSystemPollArg,
			"--model_name=old-model",
			testOVMSAddressArg,
			testOVMSTargetDeviceArg,
			testOVMSMetricsArg,
			testOVMSConfigPathArg,
			testOVMSGRPCBindArg,
			testOVMSRESTBindArg,
			testOVMSConfigPathFlag,
			testOVMSConfigPathValue,
			testOVMSGRPCBindFlag,
			testOVMSGRPCBindValue,
			testOVMSRESTBindFlag,
			testOVMSRESTBindValue,
		)
		ns := newNamespace(testISVCNamespace, nil)
		secret := newStorageConfigSecret(testISVCNamespace, map[string]storageConfigEntryJSON{
			"pvc-key": {Type: "pvc", Name: "data-pvc"},
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
		g.Expect(args).To(ContainElement(testOVMSFileSystemPollArg))
		g.Expect(args).To(ContainElement(testOVMSAddressArg))
		g.Expect(args).To(ContainElement(testOVMSTargetDeviceArg))
		g.Expect(args).To(ContainElement(testOVMSMetricsArg))
		g.Expect(args).ToNot(ContainElement("--model_name=old-model"))
		g.Expect(args).ToNot(ContainElement(testOVMSConfigPathArg))
		g.Expect(args).ToNot(ContainElement(testOVMSGRPCBindArg))
		g.Expect(args).ToNot(ContainElement(testOVMSRESTBindArg))
		g.Expect(args).ToNot(ContainElement(testOVMSConfigPathFlag))
		g.Expect(args).ToNot(ContainElement(testOVMSConfigPathValue))
		g.Expect(args).ToNot(ContainElement(testOVMSGRPCBindFlag))
		g.Expect(args).ToNot(ContainElement(testOVMSGRPCBindValue))
		g.Expect(args).ToNot(ContainElement(testOVMSRESTBindFlag))
		g.Expect(args).ToNot(ContainElement(testOVMSRESTBindValue))

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

	t.Run("should reject mixed S3 and PVC ISVCs sharing a ServingRuntime before mutation", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		s3ISVC := newModelMeshISVCWithStorage(testISVCNamespace, "s3-model", "ovms-runtime", "s3-key", "s3-path")
		pvcISVC := newModelMeshISVCWithStorage(testISVCNamespace, "pvc-model", "ovms-runtime", "pvc-key", "pvc-path")
		sr := newServingRuntime(testISVCNamespace, "ovms-runtime", true)
		ns := newNamespace(testISVCNamespace, nil)
		secret := newStorageConfigSecret(testISVCNamespace, map[string]storageConfigEntryJSON{
			"s3-key":  {Type: "s3", Bucket: "s3-bucket"},
			"pvc-key": {Type: "pvc", Name: "pvc-volume"},
		})

		dynamicClient := newModelServingDynamicClient(s3ISVC, pvcISVC, sr, ns, secret)

		target := newTestTarget(dynamicClient, "2.25.0", false)

		a := &modelserving.ModelMeshToRawAction{}
		actionResult, err := a.Run().Execute(ctx, target)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(actionResult).ToNot(BeNil())

		// Neither ISVC should be mutated when the preflight detects a conflict.
		for _, name := range []string{"s3-model", "pvc-model"} {
			updated, getErr := dynamicClient.Resource(resources.InferenceService.GVR()).
				Namespace(testISVCNamespace).
				Get(ctx, name, metav1.GetOptions{})

			g.Expect(getErr).ToNot(HaveOccurred())

			annotations := updated.GetAnnotations()
			g.Expect(annotations).To(HaveKeyWithValue("serving.kserve.io/deploymentMode", "ModelMesh"))
		}

		g.Expect(actionResult.HasFailedSteps()).To(BeTrue())
		g.Expect(hasStepMessageContaining(
			actionResult.Status.Steps, result.StepFailed, "ovms-runtime",
		)).To(BeTrue())
	})

	t.Run("should surface PVC detection in dry-run mode", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		isvc := newModelMeshISVCWithStorage(testISVCNamespace, "pvc-model", "ovms-runtime", "pvc-key", "model-dir")
		sr := newServingRuntime(testISVCNamespace, "ovms-runtime", true)
		ns := newNamespace(testISVCNamespace, map[string]string{"modelmesh-enabled": "true"})
		secret := newStorageConfigSecret(testISVCNamespace, map[string]storageConfigEntryJSON{
			"pvc-key": {Type: "pvc", Name: "my-pvc"},
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
		g.Expect(hasStepMessageContaining(
			actionResult.Status.Steps, result.StepSkipped, "pvc://my-pvc/model-dir",
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
					"corrupt-key": testInvalidStorageConfig,
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

	t.Run("should retain namespace label when a sibling ISVC is skipped", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		standardISVC := newModelMeshISVC(testISVCNamespace, "standard-model", "standard-runtime")
		skippedISVC := newModelMeshISVCWithStorage(testISVCNamespace, "skipped-model", "skipped-runtime", "invalid-key", "path")
		standardRuntime := newServingRuntime(testISVCNamespace, "standard-runtime", true)
		ns := newNamespace(testISVCNamespace, map[string]string{"modelmesh-enabled": "true"})
		secret := newStorageConfigSecret(testISVCNamespace, nil)
		secret.Object["data"].(map[string]any)["invalid-key"] = testInvalidStorageConfig

		dynamicClient := newModelServingDynamicClient(standardISVC, skippedISVC, standardRuntime, ns, secret)

		target := newTestTarget(dynamicClient, "2.25.0", false)

		a := &modelserving.ModelMeshToRawAction{}
		actionResult, err := a.Run().Execute(ctx, target)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(actionResult.HasFailedSteps()).To(BeTrue())

		updatedNS, err := dynamicClient.Resource(resources.Namespace.GVR()).
			Get(ctx, testISVCNamespace, metav1.GetOptions{})

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(updatedNS.GetLabels()).To(HaveKeyWithValue("modelmesh-enabled", "true"))
	})

	t.Run("should reject standard ISVCs sharing a runtime with a skipped ISVC", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		standardISVC := newModelMeshISVC(testISVCNamespace, "standard-model", "shared-runtime")
		skippedISVC := newModelMeshISVCWithStorage(testISVCNamespace, "skipped-model", "shared-runtime", "invalid-key", "path")
		sr := newServingRuntime(testISVCNamespace, "shared-runtime", true)
		ns := newNamespace(testISVCNamespace, nil)
		secret := newStorageConfigSecret(testISVCNamespace, nil)
		secret.Object["data"].(map[string]any)["invalid-key"] = testInvalidStorageConfig

		dynamicClient := newModelServingDynamicClient(standardISVC, skippedISVC, sr, ns, secret)

		target := newTestTarget(dynamicClient, "2.25.0", false)

		a := &modelserving.ModelMeshToRawAction{}
		actionResult, err := a.Run().Execute(ctx, target)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(actionResult.HasFailedSteps()).To(BeTrue())

		for _, name := range []string{"standard-model", "skipped-model"} {
			updated, getErr := dynamicClient.Resource(resources.InferenceService.GVR()).
				Namespace(testISVCNamespace).
				Get(ctx, name, metav1.GetOptions{})

			g.Expect(getErr).ToNot(HaveOccurred())
			g.Expect(updated.GetAnnotations()).To(HaveKeyWithValue("serving.kserve.io/deploymentMode", "ModelMesh"))
		}
	})

	t.Run("should set deploymentMode in single update with storageUri", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		isvc := newModelMeshISVCWithStorage(testISVCNamespace, "pvc-single-update", "ovms-runtime", "pvc-key", "path")
		sr := newServingRuntime(testISVCNamespace, "ovms-runtime", true)
		ns := newNamespace(testISVCNamespace, nil)
		secret := newStorageConfigSecret(testISVCNamespace, map[string]storageConfigEntryJSON{
			"pvc-key": {Type: "pvc", Name: "vol"},
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

	t.Run("should reject multiple PVC ISVCs sharing a ServingRuntime before mutation", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		isvc1 := newModelMeshISVCWithStorage(testISVCNamespace, "pvc-model-1", "shared-runtime", "pvc-key-1", "path1")
		isvc2 := newModelMeshISVCWithStorage(testISVCNamespace, "pvc-model-2", "shared-runtime", "pvc-key-2", "path2")
		sr := newServingRuntime(testISVCNamespace, "shared-runtime", true)
		ns := newNamespace(testISVCNamespace, nil)
		secret := newStorageConfigSecret(testISVCNamespace, map[string]storageConfigEntryJSON{
			"pvc-key-1": {Type: "pvc", Name: "vol1"},
			"pvc-key-2": {Type: "pvc", Name: "vol2"},
		})

		dynamicClient := newModelServingDynamicClient(isvc1, isvc2, sr, ns, secret)

		target := newTestTarget(dynamicClient, "2.25.0", false)

		a := &modelserving.ModelMeshToRawAction{}
		actionResult, err := a.Run().Execute(ctx, target)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(actionResult).ToNot(BeNil())

		// Neither ISVC should be mutated when the preflight detects a conflict.
		for _, name := range []string{"pvc-model-1", "pvc-model-2"} {
			updated, getErr := dynamicClient.Resource(resources.InferenceService.GVR()).
				Namespace(testISVCNamespace).
				Get(ctx, name, metav1.GetOptions{})

			g.Expect(getErr).ToNot(HaveOccurred())

			ann := updated.GetAnnotations()
			g.Expect(ann).To(HaveKeyWithValue("serving.kserve.io/deploymentMode", "ModelMesh"))
		}

		// Result should contain a failed step for the shared runtime
		g.Expect(actionResult.HasFailedSteps()).To(BeTrue())

		// Verify the runtime was not partially patched.
		updatedSR, err := dynamicClient.Resource(resources.ServingRuntime.GVR()).
			Namespace(testISVCNamespace).
			Get(ctx, "shared-runtime", metav1.GetOptions{})

		g.Expect(err).ToNot(HaveOccurred())

		containers, _, _ := unstructured.NestedSlice(updatedSR.Object, "spec", "containers")
		g.Expect(containers).To(HaveLen(1))

		container := containers[0].(map[string]any)
		g.Expect(container).ToNot(HaveKey("args"))
		g.Expect(hasStepMessageContaining(
			actionResult.Status.Steps, result.StepFailed, "shared-runtime",
		)).To(BeTrue())
	})

	t.Run("should reject PVC ISVCs sharing a runtime with a skipped ISVC before mutation", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		pvcISVC := newModelMeshISVCWithStorage(testISVCNamespace, "pvc-model", "shared-runtime", "pvc-key", "pvc-path")
		skippedISVC := newModelMeshISVCWithStorage(testISVCNamespace, "skipped-model", "shared-runtime", "invalid-key", "path")
		sr := newServingRuntime(testISVCNamespace, "shared-runtime", true)
		ns := newNamespace(testISVCNamespace, nil)
		secret := newStorageConfigSecret(testISVCNamespace, map[string]storageConfigEntryJSON{
			"pvc-key": {Type: "pvc", Name: "pvc-volume"},
		})
		secret.Object["data"].(map[string]any)["invalid-key"] = testInvalidStorageConfig

		dynamicClient := newModelServingDynamicClient(pvcISVC, skippedISVC, sr, ns, secret)

		target := newTestTarget(dynamicClient, "2.25.0", false)

		a := &modelserving.ModelMeshToRawAction{}
		actionResult, err := a.Run().Execute(ctx, target)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(actionResult.HasFailedSteps()).To(BeTrue())
		g.Expect(hasStepMessageContaining(
			actionResult.Status.Steps, result.StepFailed, "shared-runtime",
		)).To(BeTrue())

		for _, name := range []string{"pvc-model", "skipped-model"} {
			updated, getErr := dynamicClient.Resource(resources.InferenceService.GVR()).
				Namespace(testISVCNamespace).
				Get(ctx, name, metav1.GetOptions{})

			g.Expect(getErr).ToNot(HaveOccurred())
			g.Expect(updated.GetAnnotations()).To(HaveKeyWithValue("serving.kserve.io/deploymentMode", "ModelMesh"))
		}
	})

	t.Run("should reject PVC ISVC with empty name in storage-config", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		isvc := newModelMeshISVCWithStorage(testISVCNamespace, "empty-name-model", "ovms-runtime", "bad-key", "path")
		sr := newServingRuntime(testISVCNamespace, "ovms-runtime", true)
		ns := newNamespace(testISVCNamespace, nil)
		secret := newStorageConfigSecret(testISVCNamespace, map[string]storageConfigEntryJSON{
			"bad-key": {Type: "pvc", Name: ""},
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
			Get(ctx, "empty-name-model", metav1.GetOptions{})

		g.Expect(err).ToNot(HaveOccurred())

		_, storageURIFound, _ := unstructured.NestedString(updated.Object, "spec", "predictor", "model", "storageUri")
		g.Expect(storageURIFound).To(BeFalse())
		g.Expect(hasStepMessageContaining(
			actionResult.Status.Steps, result.StepFailed, "Processed 0 of 1 InferenceService(s)",
		)).To(BeTrue())
	})

	t.Run("should reject PVC ISVC with path traversal in storage path", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		isvc := newModelMeshISVCWithStorage(testISVCNamespace, "traversal-model", "ovms-runtime", "bad-key", "//../../etc/passwd")
		sr := newServingRuntime(testISVCNamespace, "ovms-runtime", true)
		ns := newNamespace(testISVCNamespace, nil)
		secret := newStorageConfigSecret(testISVCNamespace, map[string]storageConfigEntryJSON{
			"bad-key": {Type: "pvc", Name: "my-pvc"},
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

	t.Run("should reject PVC ISVC with invalid PVC name", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		isvc := newModelMeshISVCWithStorage(testISVCNamespace, "invalid-name-model", "ovms-runtime", "bad-key", "path")
		sr := newServingRuntime(testISVCNamespace, "ovms-runtime", true)
		ns := newNamespace(testISVCNamespace, nil)
		secret := newStorageConfigSecret(testISVCNamespace, map[string]storageConfigEntryJSON{
			"bad-key": {Type: "pvc", Name: testInvalidPVCName},
		})

		dynamicClient := newModelServingDynamicClient(isvc, sr, ns, secret)

		target := newTestTarget(dynamicClient, "2.25.0", false)

		a := &modelserving.ModelMeshToRawAction{}
		actionResult, err := a.Run().Execute(ctx, target)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(actionResult.HasFailedSteps()).To(BeTrue())
		g.Expect(hasStepMessageContaining(
			actionResult.Status.Steps, result.StepFailed, "Processed 0 of 1 InferenceService(s)",
		)).To(BeTrue())

		updated, err := dynamicClient.Resource(resources.InferenceService.GVR()).
			Namespace(testISVCNamespace).
			Get(ctx, "invalid-name-model", metav1.GetOptions{})

		g.Expect(err).ToNot(HaveOccurred())
		_, storageURIFound, _ := unstructured.NestedString(updated.Object, "spec", "predictor", "model", "storageUri")
		g.Expect(storageURIFound).To(BeFalse())
	})

	t.Run("should fail when PVC ServingRuntime update fails", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		isvc := newModelMeshISVCWithStorage(testISVCNamespace, "runtime-update-failure", "ovms-runtime", "pvc-key", "path")
		sr := newServingRuntime(testISVCNamespace, "ovms-runtime", true)
		ns := newNamespace(testISVCNamespace, map[string]string{"modelmesh-enabled": "true"})
		secret := newStorageConfigSecret(testISVCNamespace, map[string]storageConfigEntryJSON{
			"pvc-key": {Type: "pvc", Name: "model-pvc"},
		})

		dynamicClient := newModelServingDynamicClient(isvc, sr, ns, secret)
		dynamicClient.PrependReactor("update", "servingruntimes", func(_ k8stesting.Action) (bool, k8sruntime.Object, error) {
			return true, nil, apierrors.NewInternalError(errors.New(testRuntimeUpdateFailure))
		})

		target := newTestTarget(dynamicClient, "2.25.0", false)

		a := &modelserving.ModelMeshToRawAction{}
		actionResult, err := a.Run().Execute(ctx, target)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(actionResult.HasFailedSteps()).To(BeTrue())
		g.Expect(hasStepMessageContaining(
			actionResult.Status.Steps, result.StepFailed, "Processed 0 of 1 InferenceService(s)",
		)).To(BeTrue())

		updated, err := dynamicClient.Resource(resources.InferenceService.GVR()).
			Namespace(testISVCNamespace).
			Get(ctx, "runtime-update-failure", metav1.GetOptions{})

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(updated.GetAnnotations()).To(HaveKeyWithValue("serving.kserve.io/deploymentMode", "ModelMesh"))
		_, storageKeyFound, err := unstructured.NestedString(updated.Object, "spec", "predictor", "model", "storage", "key")
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(storageKeyFound).To(BeTrue())

		updatedNS, err := dynamicClient.Resource(resources.Namespace.GVR()).
			Get(ctx, testISVCNamespace, metav1.GetOptions{})

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(updatedNS.GetLabels()).To(HaveKeyWithValue("modelmesh-enabled", "true"))
	})

	t.Run("should restore ServingRuntime when PVC ISVC update fails", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		isvc := newModelMeshISVCWithStorage(testISVCNamespace, "isvc-update-failure", "ovms-runtime", "pvc-key", "path")
		sr := newServingRuntime(testISVCNamespace, "ovms-runtime", true)
		ns := newNamespace(testISVCNamespace, nil)
		secret := newStorageConfigSecret(testISVCNamespace, map[string]storageConfigEntryJSON{
			"pvc-key": {Type: "pvc", Name: "model-pvc"},
		})

		dynamicClient := newModelServingDynamicClient(isvc, sr, ns, secret)
		dynamicClient.PrependReactor("update", "inferenceservices", func(_ k8stesting.Action) (bool, k8sruntime.Object, error) {
			return true, nil, apierrors.NewInternalError(errors.New(testISVCUpdateFailure))
		})

		target := newTestTarget(dynamicClient, "2.25.0", false)

		a := &modelserving.ModelMeshToRawAction{}
		actionResult, err := a.Run().Execute(ctx, target)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(actionResult.HasFailedSteps()).To(BeTrue())

		updatedISVC, err := dynamicClient.Resource(resources.InferenceService.GVR()).
			Namespace(testISVCNamespace).
			Get(ctx, "isvc-update-failure", metav1.GetOptions{})

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(updatedISVC.GetAnnotations()).To(HaveKeyWithValue("serving.kserve.io/deploymentMode", "ModelMesh"))
		_, storageKeyFound, err := unstructured.NestedString(updatedISVC.Object, "spec", "predictor", "model", "storage", "key")
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(storageKeyFound).To(BeTrue())

		updatedSR, err := dynamicClient.Resource(resources.ServingRuntime.GVR()).
			Namespace(testISVCNamespace).
			Get(ctx, "ovms-runtime", metav1.GetOptions{})

		g.Expect(err).ToNot(HaveOccurred())
		multiModel, _, err := unstructured.NestedBool(updatedSR.Object, "spec", "multiModel")
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(multiModel).To(BeTrue())
		containers, _, err := unstructured.NestedSlice(updatedSR.Object, "spec", "containers")
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(containers).To(HaveLen(1))
		g.Expect(containers[0].(map[string]any)).To(HaveKeyWithValue("name", "ovms"))
	})

	t.Run("should reject PVC ISVC when its ServingRuntime is missing", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		isvc := newModelMeshISVCWithStorage(testISVCNamespace, "missing-runtime", "missing-runtime", "pvc-key", "path")
		ns := newNamespace(testISVCNamespace, nil)
		secret := newStorageConfigSecret(testISVCNamespace, map[string]storageConfigEntryJSON{
			"pvc-key": {Type: "pvc", Name: "model-pvc"},
		})

		dynamicClient := newModelServingDynamicClient(isvc, ns, secret)

		target := newTestTarget(dynamicClient, "2.25.0", false)

		a := &modelserving.ModelMeshToRawAction{}
		actionResult, err := a.Run().Execute(ctx, target)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(actionResult.HasFailedSteps()).To(BeTrue())
		g.Expect(hasStepMessageContaining(
			actionResult.Status.Steps, result.StepFailed, "missing-runtime is unavailable",
		)).To(BeTrue())

		updated, err := dynamicClient.Resource(resources.InferenceService.GVR()).
			Namespace(testISVCNamespace).
			Get(ctx, "missing-runtime", metav1.GetOptions{})

		g.Expect(err).ToNot(HaveOccurred())
		_, storageURIFound, _ := unstructured.NestedString(updated.Object, "spec", "predictor", "model", "storageUri")
		g.Expect(storageURIFound).To(BeFalse())
	})

	t.Run("should fail when standard ISVC deployment mode patch fails", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		isvc := newModelMeshISVC(testISVCNamespace, "standard-patch-failure", "ovms-runtime")
		sr := newServingRuntime(testISVCNamespace, "ovms-runtime", true)
		ns := newNamespace(testISVCNamespace, nil)

		dynamicClient := newModelServingDynamicClient(isvc, sr, ns)
		dynamicClient.PrependReactor("patch", "inferenceservices", func(_ k8stesting.Action) (bool, k8sruntime.Object, error) {
			return true, nil, apierrors.NewInternalError(errors.New(testDeploymentModeFailure))
		})

		target := newTestTarget(dynamicClient, "2.25.0", false)

		a := &modelserving.ModelMeshToRawAction{}
		actionResult, err := a.Run().Execute(ctx, target)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(actionResult.HasFailedSteps()).To(BeTrue())
		g.Expect(hasStepMessageContaining(
			actionResult.Status.Steps, result.StepFailed, "Processed 0 of 1 InferenceService(s)",
		)).To(BeTrue())
	})
}
