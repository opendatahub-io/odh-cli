package modelserving_test

import (
	"errors"
	"testing"

	"github.com/blang/semver/v4"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8stesting "k8s.io/client-go/testing"

	"github.com/opendatahub-io/odh-cli/pkg/migrate/action"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/action/result"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/actions/modelserving"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"

	. "github.com/onsi/gomega"
)

func TestServerlessToRawAction_ID(t *testing.T) {
	g := NewWithT(t)

	a := &modelserving.ServerlessToRawAction{}
	g.Expect(a.ID()).To(Equal("modelserving.serverless-to-raw"))
}

func TestServerlessToRawAction_CanApply(t *testing.T) {
	t.Run("should return true for version 2.25", func(t *testing.T) {
		g := NewWithT(t)

		a := &modelserving.ServerlessToRawAction{}
		v := semver.MustParse("2.25.0")
		target := action.Target{CurrentVersion: &v}

		g.Expect(a.CanApply(target)).To(BeTrue())
	})

	t.Run("should return false for version 3.x", func(t *testing.T) {
		g := NewWithT(t)

		a := &modelserving.ServerlessToRawAction{}
		v := semver.MustParse("3.0.0")
		target := action.Target{CurrentVersion: &v}

		g.Expect(a.CanApply(target)).To(BeFalse())
	})
}

func TestServerlessToRawAction_RunValidate(t *testing.T) {
	t.Run("should report Serverless ISVCs found", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		isvc := newISVC(testISVCNamespace, "serverless-model", "Serverless")

		scheme := runtime.NewScheme()

		listKinds := map[schema.GroupVersionResource]string{
			resources.InferenceService.GVR(): resources.InferenceService.ListKind(),
		}

		dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
			scheme, listKinds, isvc,
		)

		target := newTestTarget(dynamicClient, "2.25.0", false)

		a := &modelserving.ServerlessToRawAction{}
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

	t.Run("should report no Serverless ISVCs when none exist", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		scheme := runtime.NewScheme()

		listKinds := map[schema.GroupVersionResource]string{
			resources.InferenceService.GVR(): resources.InferenceService.ListKind(),
		}

		dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, listKinds)

		target := newTestTarget(dynamicClient, "2.25.0", false)

		a := &modelserving.ServerlessToRawAction{}
		actionResult, err := a.Run().Validate(ctx, target)

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
}

func serverlessListKinds() map[schema.GroupVersionResource]string {
	return map[schema.GroupVersionResource]string{
		resources.InferenceService.GVR(): resources.InferenceService.ListKind(),
		resources.ServiceAccount.GVR():   resources.ServiceAccount.ListKind(),
		resources.Role.GVR():             resources.Role.ListKind(),
		resources.RoleBinding.GVR():      resources.RoleBinding.ListKind(),
		resources.VirtualService.GVR():   resources.VirtualService.ListKind(),
	}
}

func TestServerlessToRawAction_RunExecute(t *testing.T) {
	t.Run("should delete and recreate ISVC with RawDeployment mode", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		isvc := newISVC(testISVCNamespace, "serverless-model", "Serverless")

		scheme := runtime.NewScheme()
		dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
			scheme, serverlessListKinds(), isvc,
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

		a := &modelserving.ServerlessToRawAction{}
		actionResult, err := a.Run().Execute(ctx, target)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(actionResult).ToNot(BeNil())

		updated, err := dynamicClient.Resource(resources.InferenceService.GVR()).
			Namespace(testISVCNamespace).
			Get(ctx, "serverless-model", metav1.GetOptions{})

		g.Expect(err).ToNot(HaveOccurred())

		annotations := updated.GetAnnotations()
		g.Expect(annotations).To(HaveKeyWithValue("serving.kserve.io/deploymentMode", "RawDeployment"))

		labels := updated.GetLabels()
		g.Expect(labels).To(HaveKeyWithValue("networking.kserve.io/visibility", "exposed"))
	})

	t.Run("should strip istio and knative annotations and labels", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		isvc := newISVCWithIstioAnnotations(testISVCNamespace, "serverless-model", "Serverless")

		scheme := runtime.NewScheme()
		dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
			scheme, serverlessListKinds(), isvc,
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

		a := &modelserving.ServerlessToRawAction{}
		actionResult, err := a.Run().Execute(ctx, target)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(actionResult).ToNot(BeNil())

		updated, err := dynamicClient.Resource(resources.InferenceService.GVR()).
			Namespace(testISVCNamespace).
			Get(ctx, "serverless-model", metav1.GetOptions{})

		g.Expect(err).ToNot(HaveOccurred())

		annotations := updated.GetAnnotations()
		g.Expect(annotations).NotTo(HaveKey("istio.io/rev"))
		g.Expect(annotations).NotTo(HaveKey("sidecar.istio.io/inject"))
		g.Expect(annotations).NotTo(HaveKey("serving.knative.dev/creator"))
		g.Expect(annotations).To(HaveKeyWithValue("serving.kserve.io/deploymentMode", "RawDeployment"))

		labels := updated.GetLabels()
		g.Expect(labels).NotTo(HaveKey("networking.istio.io/gateway"))
		g.Expect(labels).NotTo(HaveKey("networking.knative.dev/visibility"))
		g.Expect(labels).To(HaveKeyWithValue("networking.kserve.io/visibility", "exposed"))
	})

	t.Run("should create auth resources when auth is enabled", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		isvc := newISVCWithAuth(testISVCNamespace, "auth-model", "Serverless")

		scheme := runtime.NewScheme()
		dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
			scheme, serverlessListKinds(), isvc,
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

		a := &modelserving.ServerlessToRawAction{}
		actionResult, err := a.Run().Execute(ctx, target)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(actionResult).ToNot(BeNil())

		_, err = dynamicClient.Resource(resources.ServiceAccount.GVR()).
			Namespace(testISVCNamespace).
			Get(ctx, "auth-model-sa", metav1.GetOptions{})
		g.Expect(err).ToNot(HaveOccurred())

		_, err = dynamicClient.Resource(resources.Role.GVR()).
			Namespace(testISVCNamespace).
			Get(ctx, "auth-model-view-role", metav1.GetOptions{})
		g.Expect(err).ToNot(HaveOccurred())

		_, err = dynamicClient.Resource(resources.RoleBinding.GVR()).
			Namespace(testISVCNamespace).
			Get(ctx, "auth-model-view", metav1.GetOptions{})
		g.Expect(err).ToNot(HaveOccurred())
	})

	t.Run("should skip auth resources when auth is not enabled", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		isvc := newISVC(testISVCNamespace, "no-auth-model", "Serverless")

		scheme := runtime.NewScheme()
		dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
			scheme, serverlessListKinds(), isvc,
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

		a := &modelserving.ServerlessToRawAction{}
		actionResult, err := a.Run().Execute(ctx, target)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(actionResult).ToNot(BeNil())

		_, err = dynamicClient.Resource(resources.ServiceAccount.GVR()).
			Namespace(testISVCNamespace).
			Get(ctx, "no-auth-model-sa", metav1.GetOptions{})
		g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
	})

	t.Run("should delete Istio VirtualService during conversion", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		isvc := newISVC(testISVCNamespace, "serverless-model", "Serverless")
		vs := newVirtualService("istio-system", "serverless-model-"+testISVCNamespace)

		scheme := runtime.NewScheme()
		dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
			scheme, serverlessListKinds(), isvc, vs,
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

		a := &modelserving.ServerlessToRawAction{}
		actionResult, err := a.Run().Execute(ctx, target)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(actionResult).ToNot(BeNil())

		_, err = dynamicClient.Resource(resources.VirtualService.GVR()).
			Namespace("istio-system").
			Get(ctx, "serverless-model-"+testISVCNamespace, metav1.GetOptions{})
		g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
	})

	t.Run("should skip when no Serverless ISVCs exist", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		scheme := runtime.NewScheme()

		listKinds := map[schema.GroupVersionResource]string{
			resources.InferenceService.GVR(): resources.InferenceService.ListKind(),
		}

		dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, listKinds)

		target := newTestTarget(dynamicClient, "2.25.0", false)

		a := &modelserving.ServerlessToRawAction{}
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

	t.Run("should rollback original ISVC when recreation fails", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		isvc := newISVC(testISVCNamespace, "serverless-model", "Serverless")

		scheme := runtime.NewScheme()
		dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
			scheme, serverlessListKinds(), isvc,
		)

		createCount := 0
		dynamicClient.PrependReactor("create", "inferenceservices", func(_ k8stesting.Action) (bool, runtime.Object, error) {
			createCount++
			if createCount == 1 {
				return true, nil, errors.New("simulated webhook rejection")
			}

			return false, nil, nil
		})

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

		a := &modelserving.ServerlessToRawAction{}
		actionResult, err := a.Run().Execute(ctx, target)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(actionResult).ToNot(BeNil())

		restored, err := dynamicClient.Resource(resources.InferenceService.GVR()).
			Namespace(testISVCNamespace).
			Get(ctx, "serverless-model", metav1.GetOptions{})

		g.Expect(err).ToNot(HaveOccurred())

		annotations := restored.GetAnnotations()
		g.Expect(annotations).To(HaveKeyWithValue("serving.kserve.io/deploymentMode", "Serverless"))

		hasFailed := false
		for _, step := range actionResult.Status.Steps {
			if step.Status == result.StepFailed {
				hasFailed = true
			}
		}

		g.Expect(hasFailed).To(BeTrue())
	})

	t.Run("should not mutate in dry-run mode", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		isvc := newISVC(testISVCNamespace, "serverless-model", "Serverless")

		scheme := runtime.NewScheme()
		dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
			scheme, serverlessListKinds(), isvc,
		)

		target := newTestTarget(dynamicClient, "2.25.0", true)

		a := &modelserving.ServerlessToRawAction{}
		actionResult, err := a.Run().Execute(ctx, target)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(actionResult).ToNot(BeNil())

		original, err := dynamicClient.Resource(resources.InferenceService.GVR()).
			Namespace(testISVCNamespace).
			Get(ctx, "serverless-model", metav1.GetOptions{})

		g.Expect(err).ToNot(HaveOccurred())

		annotations := original.GetAnnotations()
		g.Expect(annotations).To(HaveKeyWithValue("serving.kserve.io/deploymentMode", "Serverless"))
	})
}
