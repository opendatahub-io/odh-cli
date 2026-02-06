package validate_test

import (
	"context"
	"errors"
	"testing"

	"github.com/blang/semver/v4"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	"github.com/lburgazzoli/odh-cli/pkg/lint/check"
	"github.com/lburgazzoli/odh-cli/pkg/lint/check/result"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/shared/base"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/shared/results"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/shared/validate"
	"github.com/lburgazzoli/odh-cli/pkg/resources"
	"github.com/lburgazzoli/odh-cli/pkg/util/client"

	. "github.com/onsi/gomega"
)

//nolint:gochecknoglobals // Test fixture - shared across test functions
var dscListKinds = map[schema.GroupVersionResource]string{
	resources.DataScienceCluster.GVR(): resources.DataScienceCluster.ListKind(),
}

//nolint:gochecknoglobals // Test fixture - shared across test functions
var dsciListKinds = map[schema.GroupVersionResource]string{
	resources.DSCInitialization.GVR(): resources.DSCInitialization.ListKind(),
}

// testCheck implements check.Check for testing.
type testCheck struct {
	base.BaseCheck
}

func (c *testCheck) CanApply(_ context.Context, _ check.Target) bool {
	return true
}

func (c *testCheck) Validate(_ context.Context, _ check.Target) (*result.DiagnosticResult, error) {
	return c.NewResult(), nil
}

func newTestCheck() *testCheck {
	return &testCheck{
		BaseCheck: base.BaseCheck{
			CheckGroup:       check.GroupComponent,
			Kind:             check.ComponentCodeFlare,
			Type:             check.CheckTypeRemoval,
			CheckID:          "test.check",
			CheckName:        "Test Check",
			CheckDescription: "Test description",
		},
	}
}

func TestComponentBuilder(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	t.Run("should return not found when DSC does not exist", func(t *testing.T) {
		scheme := runtime.NewScheme()
		dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, dscListKinds)
		c := client.NewForTesting(client.TestClientConfig{
			Dynamic: dynamicClient,
		})

		target := check.Target{
			Client: c,
		}

		chk := newTestCheck()
		dr, err := validate.Component(chk, target).
			Run(ctx, func(_ context.Context, _ *validate.ComponentRequest) error {
				t.Fatal("validation function should not be called when DSC not found")

				return nil
			})

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(dr).ToNot(BeNil())
		g.Expect(dr.Status.Conditions).To(HaveLen(1))
		g.Expect(dr.Status.Conditions[0].Type).To(Equal(check.ConditionTypeAvailable))
		g.Expect(dr.Status.Conditions[0].Status).To(Equal(metav1.ConditionFalse))
		g.Expect(dr.Status.Conditions[0].Reason).To(Equal(check.ReasonResourceNotFound))
	})

	t.Run("should return not configured when component state not in required states", func(t *testing.T) {
		dsc := createDSCWithComponent("codeflare", check.ManagementStateRemoved)
		scheme := runtime.NewScheme()
		dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, dscListKinds, dsc)
		c := client.NewForTesting(client.TestClientConfig{
			Dynamic: dynamicClient,
		})

		target := check.Target{
			Client: c,
		}

		chk := newTestCheck()
		dr, err := validate.Component(chk, target).
			InState(check.ManagementStateManaged, check.ManagementStateUnmanaged).
			Run(ctx, func(_ context.Context, _ *validate.ComponentRequest) error {
				t.Fatal("validation function should not be called when state not in required states")

				return nil
			})

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(dr).ToNot(BeNil())
		g.Expect(dr.Status.Conditions).To(HaveLen(1))
		g.Expect(dr.Status.Conditions[0].Type).To(Equal(check.ConditionTypeConfigured))
		g.Expect(dr.Status.Conditions[0].Status).To(Equal(metav1.ConditionFalse))
	})

	t.Run("should call validation function when component state matches", func(t *testing.T) {
		dsc := createDSCWithComponent("codeflare", check.ManagementStateManaged)
		scheme := runtime.NewScheme()
		dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, dscListKinds, dsc)
		c := client.NewForTesting(client.TestClientConfig{
			Dynamic: dynamicClient,
		})

		targetVersion := semver.MustParse("3.0.0")
		target := check.Target{
			Client:        c,
			TargetVersion: &targetVersion,
		}

		validationCalled := false
		chk := newTestCheck()
		dr, err := validate.Component(chk, target).
			InState(check.ManagementStateManaged, check.ManagementStateUnmanaged).
			Run(ctx, func(_ context.Context, req *validate.ComponentRequest) error {
				validationCalled = true
				g.Expect(req.ManagementState).To(Equal(check.ManagementStateManaged))
				g.Expect(req.DSC).ToNot(BeNil())
				g.Expect(req.Client).ToNot(BeNil())
				results.SetCompatibilitySuccessf(req.Result, "Test passed")

				return nil
			})

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(dr).ToNot(BeNil())
		g.Expect(validationCalled).To(BeTrue())

		// Verify annotations are auto-populated
		g.Expect(dr.Annotations[check.AnnotationComponentManagementState]).To(Equal(check.ManagementStateManaged))
		g.Expect(dr.Annotations[check.AnnotationCheckTargetVersion]).To(Equal("3.0.0"))

		// Verify condition from validation function
		g.Expect(dr.Status.Conditions).To(HaveLen(1))
		g.Expect(dr.Status.Conditions[0].Type).To(Equal(check.ConditionTypeCompatible))
	})

	t.Run("should run validation without InState filter", func(t *testing.T) {
		dsc := createDSCWithComponent("codeflare", check.ManagementStateRemoved)
		scheme := runtime.NewScheme()
		dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, dscListKinds, dsc)
		c := client.NewForTesting(client.TestClientConfig{
			Dynamic: dynamicClient,
		})

		target := check.Target{
			Client: c,
		}

		validationCalled := false
		chk := newTestCheck()
		dr, err := validate.Component(chk, target).
			Run(ctx, func(_ context.Context, req *validate.ComponentRequest) error {
				validationCalled = true
				g.Expect(req.ManagementState).To(Equal(check.ManagementStateRemoved))
				results.SetCompatibilitySuccessf(req.Result, "Test passed")

				return nil
			})

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(dr).ToNot(BeNil())
		g.Expect(validationCalled).To(BeTrue())
	})

	t.Run("should propagate error from validation function", func(t *testing.T) {
		dsc := createDSCWithComponent("codeflare", check.ManagementStateManaged)
		scheme := runtime.NewScheme()
		dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, dscListKinds, dsc)
		c := client.NewForTesting(client.TestClientConfig{
			Dynamic: dynamicClient,
		})

		target := check.Target{
			Client: c,
		}

		expectedErr := errors.New("validation error")
		chk := newTestCheck()
		_, err := validate.Component(chk, target).
			InState(check.ManagementStateManaged).
			Run(ctx, func(_ context.Context, _ *validate.ComponentRequest) error {
				return expectedErr
			})

		g.Expect(err).To(MatchError(expectedErr))
	})
}

func TestDSCIBuilder(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	t.Run("should return not found when DSCI does not exist", func(t *testing.T) {
		scheme := runtime.NewScheme()
		dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, dsciListKinds)
		c := client.NewForTesting(client.TestClientConfig{
			Dynamic: dynamicClient,
		})

		target := check.Target{
			Client: c,
		}

		chk := newTestCheck()
		dr, err := validate.DSCI(chk).
			Run(ctx, target, func(dr *result.DiagnosticResult, dsci *unstructured.Unstructured) error {
				t.Fatal("validation function should not be called when DSCI not found")

				return nil
			})

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(dr).ToNot(BeNil())
		g.Expect(dr.Status.Conditions).To(HaveLen(1))
		g.Expect(dr.Status.Conditions[0].Type).To(Equal(check.ConditionTypeAvailable))
		g.Expect(dr.Status.Conditions[0].Status).To(Equal(metav1.ConditionFalse))
		g.Expect(dr.Status.Conditions[0].Reason).To(Equal(check.ReasonResourceNotFound))
	})

	t.Run("should call validation function when DSCI exists", func(t *testing.T) {
		dsci := createDSCI()
		scheme := runtime.NewScheme()
		dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, dsciListKinds, dsci)
		c := client.NewForTesting(client.TestClientConfig{
			Dynamic: dynamicClient,
		})

		targetVersion := semver.MustParse("3.0.0")
		target := check.Target{
			Client:        c,
			TargetVersion: &targetVersion,
		}

		validationCalled := false
		chk := newTestCheck()
		dr, err := validate.DSCI(chk).
			Run(ctx, target, func(dr *result.DiagnosticResult, dsci *unstructured.Unstructured) error {
				validationCalled = true
				results.SetCompatibilitySuccessf(dr, "Test passed")

				return nil
			})

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(dr).ToNot(BeNil())
		g.Expect(validationCalled).To(BeTrue())

		// Verify annotations are auto-populated
		g.Expect(dr.Annotations[check.AnnotationCheckTargetVersion]).To(Equal("3.0.0"))

		// Verify condition from validation function
		g.Expect(dr.Status.Conditions).To(HaveLen(1))
		g.Expect(dr.Status.Conditions[0].Type).To(Equal(check.ConditionTypeCompatible))
	})

	t.Run("should propagate error from validation function", func(t *testing.T) {
		dsci := createDSCI()
		scheme := runtime.NewScheme()
		dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, dsciListKinds, dsci)
		c := client.NewForTesting(client.TestClientConfig{
			Dynamic: dynamicClient,
		})

		target := check.Target{
			Client: c,
		}

		expectedErr := errors.New("validation error")
		chk := newTestCheck()
		_, err := validate.DSCI(chk).
			Run(ctx, target, func(dr *result.DiagnosticResult, dsci *unstructured.Unstructured) error {
				return expectedErr
			})

		g.Expect(err).To(MatchError(expectedErr))
	})
}

// Helper functions for creating test resources.

func createDSCWithComponent(componentName string, state string) *unstructured.Unstructured {
	dsc := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.DataScienceCluster.APIVersion(),
			"kind":       resources.DataScienceCluster.Kind,
			"metadata": map[string]any{
				"name": "default-dsc",
			},
			"spec": map[string]any{
				"components": map[string]any{
					componentName: map[string]any{
						"managementState": state,
					},
				},
			},
		},
	}

	return dsc
}

func createDSCI() *unstructured.Unstructured {
	dsci := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.DSCInitialization.APIVersion(),
			"kind":       resources.DSCInitialization.Kind,
			"metadata": map[string]any{
				"name": "default-dsci",
			},
			"spec": map[string]any{
				"applicationsNamespace": "opendatahub",
			},
		},
	}

	return dsci
}
