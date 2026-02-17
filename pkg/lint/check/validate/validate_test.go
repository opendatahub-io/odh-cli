package validate_test

import (
	"context"
	"errors"
	"testing"

	"github.com/blang/semver/v4"
	operatorsv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	operatorfake "github.com/operator-framework/operator-lifecycle-manager/pkg/api/client/clientset/versioned/fake"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	"github.com/opendatahub-io/odh-cli/pkg/constants"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/validate"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"

	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
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
	check.BaseCheck
}

func (c *testCheck) CanApply(_ context.Context, _ check.Target) (bool, error) {
	return true, nil
}

func (c *testCheck) Validate(_ context.Context, _ check.Target) (*result.DiagnosticResult, error) {
	return c.NewResult(), nil
}

func newTestCheck() *testCheck {
	return &testCheck{
		BaseCheck: check.BaseCheck{
			CheckGroup:       check.GroupComponent,
			Kind:             "codeflare",
			Type:             check.CheckTypeRemoval,
			CheckID:          "test.check",
			CheckName:        "Test Check",
			CheckDescription: "Test description",
		},
	}
}

func TestComponentBuilder(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

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

	t.Run("should pass when component state not in required states", func(t *testing.T) {
		dsc := createDSCWithComponent("codeflare", constants.ManagementStateRemoved)
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
			InState(constants.ManagementStateManaged, constants.ManagementStateUnmanaged).
			Run(ctx, func(_ context.Context, _ *validate.ComponentRequest) error {
				t.Fatal("validation function should not be called when state not in required states")

				return nil
			})

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(dr).ToNot(BeNil())
		g.Expect(dr.Status.Conditions).To(HaveLen(1))
		g.Expect(dr.Status.Conditions[0].Type).To(Equal(check.ConditionTypeConfigured))
		g.Expect(dr.Status.Conditions[0].Status).To(Equal(metav1.ConditionTrue))
		g.Expect(dr.Status.Conditions[0].Reason).To(Equal(check.ReasonRequirementsMet))
	})

	t.Run("should call validation function when component state matches", func(t *testing.T) {
		dsc := createDSCWithComponent("codeflare", constants.ManagementStateManaged)
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
			InState(constants.ManagementStateManaged, constants.ManagementStateUnmanaged).
			Run(ctx, func(_ context.Context, req *validate.ComponentRequest) error {
				validationCalled = true
				g.Expect(req.ManagementState).To(Equal(constants.ManagementStateManaged))
				g.Expect(req.DSC).ToNot(BeNil())
				g.Expect(req.Client).ToNot(BeNil())
				req.Result.SetCondition(check.NewCondition(
					check.ConditionTypeCompatible,
					metav1.ConditionTrue,
					check.WithReason(check.ReasonVersionCompatible),
					check.WithMessage("Test passed"),
				))

				return nil
			})

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(dr).ToNot(BeNil())
		g.Expect(validationCalled).To(BeTrue())

		// Verify annotations are auto-populated
		g.Expect(dr.Annotations[check.AnnotationComponentManagementState]).To(Equal(constants.ManagementStateManaged))
		g.Expect(dr.Annotations[check.AnnotationCheckTargetVersion]).To(Equal("3.0.0"))

		// Verify condition from validation function
		g.Expect(dr.Status.Conditions).To(HaveLen(1))
		g.Expect(dr.Status.Conditions[0].Type).To(Equal(check.ConditionTypeCompatible))
	})

	t.Run("should run validation without InState filter", func(t *testing.T) {
		dsc := createDSCWithComponent("codeflare", constants.ManagementStateRemoved)
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
				g.Expect(req.ManagementState).To(Equal(constants.ManagementStateRemoved))
				req.Result.SetCondition(check.NewCondition(
					check.ConditionTypeCompatible,
					metav1.ConditionTrue,
					check.WithReason(check.ReasonVersionCompatible),
					check.WithMessage("Test passed"),
				))

				return nil
			})

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(dr).ToNot(BeNil())
		g.Expect(validationCalled).To(BeTrue())
	})

	t.Run("should propagate error from validation function", func(t *testing.T) {
		dsc := createDSCWithComponent("codeflare", constants.ManagementStateManaged)
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
			InState(constants.ManagementStateManaged).
			Run(ctx, func(_ context.Context, _ *validate.ComponentRequest) error {
				return expectedErr
			})

		g.Expect(err).To(MatchError(expectedErr))
	})
}

func TestComponentBuilder_Complete_SetsConditions(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	dsc := createDSCWithComponent("codeflare", constants.ManagementStateManaged)
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

	chk := newTestCheck()
	dr, err := validate.Component(chk, target).
		InState(constants.ManagementStateManaged).
		Complete(ctx, func(_ context.Context, req *validate.ComponentRequest) ([]result.Condition, error) {
			return []result.Condition{
				check.NewCondition(
					check.ConditionTypeCompatible,
					metav1.ConditionTrue,
					check.WithReason(check.ReasonVersionCompatible),
					check.WithMessage("Component %s is valid", req.ManagementState),
				),
			}, nil
		})

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(dr).ToNot(BeNil())
	g.Expect(dr.Status.Conditions).To(HaveLen(1))
	g.Expect(dr.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(check.ConditionTypeCompatible),
		"Status": Equal(metav1.ConditionTrue),
	}))
	g.Expect(dr.Status.Conditions[0].Message).To(Equal("Component Managed is valid"))
	g.Expect(dr.Annotations[check.AnnotationComponentManagementState]).To(Equal(constants.ManagementStateManaged))
	g.Expect(dr.Annotations[check.AnnotationCheckTargetVersion]).To(Equal("3.0.0"))
}

func TestComponentBuilder_Complete_ErrorPropagated(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	dsc := createDSCWithComponent("codeflare", constants.ManagementStateManaged)
	scheme := runtime.NewScheme()
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, dscListKinds, dsc)
	c := client.NewForTesting(client.TestClientConfig{
		Dynamic: dynamicClient,
	})

	target := check.Target{
		Client: c,
	}

	expectedErr := errors.New("condition fn failed")
	chk := newTestCheck()

	_, err := validate.Component(chk, target).
		InState(constants.ManagementStateManaged).
		Complete(ctx, func(_ context.Context, _ *validate.ComponentRequest) ([]result.Condition, error) {
			return nil, expectedErr
		})

	g.Expect(err).To(MatchError(expectedErr))
}

func TestDSCIBuilder(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

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
		dr, err := validate.DSCI(chk, target).
			Run(ctx, func(dr *result.DiagnosticResult, dsci *unstructured.Unstructured) error {
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
		dr, err := validate.DSCI(chk, target).
			Run(ctx, func(dr *result.DiagnosticResult, dsci *unstructured.Unstructured) error {
				validationCalled = true
				dr.SetCondition(check.NewCondition(
					check.ConditionTypeCompatible,
					metav1.ConditionTrue,
					check.WithReason(check.ReasonVersionCompatible),
					check.WithMessage("Test passed"),
				))

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
		_, err := validate.DSCI(chk, target).
			Run(ctx, func(dr *result.DiagnosticResult, dsci *unstructured.Unstructured) error {
				return expectedErr
			})

		g.Expect(err).To(MatchError(expectedErr))
	})
}

func newTestOperatorCheck() *testCheck {
	return &testCheck{
		BaseCheck: check.BaseCheck{
			CheckGroup:       check.GroupDependency,
			Kind:             "certmanager",
			Type:             check.CheckTypeInstalled,
			CheckID:          "test.operator.check",
			CheckName:        "Test Operator Check",
			CheckDescription: "Test operator description",
		},
	}
}

func TestOperatorBuilder(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	t.Run("should return OLM unavailable when no OLM client", func(t *testing.T) {
		scheme := runtime.NewScheme()
		dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, nil)

		// No OLM client configured - OLM().Available() returns false.
		c := client.NewForTesting(client.TestClientConfig{
			Dynamic: dynamicClient,
		})

		ver := semver.MustParse("2.17.0")
		target := check.Target{
			Client:        c,
			TargetVersion: &ver,
		}

		chk := newTestOperatorCheck()
		dr, err := validate.Operator(chk, target).Run(ctx)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(dr).ToNot(BeNil())
		g.Expect(dr.Status.Conditions).To(HaveLen(1))
		g.Expect(dr.Status.Conditions[0].Message).To(Equal("OLM client not available"))
		g.Expect(dr.Annotations[check.AnnotationCheckTargetVersion]).To(Equal("2.17.0"))
	})

	t.Run("should return not found when operator not installed", func(t *testing.T) {
		scheme := runtime.NewScheme()
		dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, nil)
		olmClient := operatorfake.NewSimpleClientset() //nolint:staticcheck // NewClientset requires generated apply configs not available in OLM

		c := client.NewForTesting(client.TestClientConfig{
			Dynamic: dynamicClient,
			OLM:     olmClient,
		})

		ver := semver.MustParse("2.17.0")
		target := check.Target{
			Client:        c,
			TargetVersion: &ver,
		}

		chk := newTestOperatorCheck()
		dr, err := validate.Operator(chk, target).Run(ctx)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(dr).ToNot(BeNil())
		g.Expect(dr.Group).To(Equal("dependency"))
		g.Expect(dr.Kind).To(Equal("certmanager"))
		g.Expect(dr.Name).To(Equal(string(check.CheckTypeInstalled)))
		g.Expect(dr.Status.Conditions).To(HaveLen(1))
		g.Expect(dr.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
			"Type":    Equal(check.ConditionTypeAvailable),
			"Status":  Equal(metav1.ConditionFalse),
			"Reason":  Equal(check.ReasonResourceNotFound),
			"Message": ContainSubstring("not installed"),
		}))
	})

	t.Run("should return found when operator is installed", func(t *testing.T) {
		sub := &operatorsv1alpha1.Subscription{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "certmanager",
				Namespace: "cert-manager",
			},
			Status: operatorsv1alpha1.SubscriptionStatus{
				InstalledCSV: "cert-manager.v1.13.0",
			},
		}

		scheme := runtime.NewScheme()
		dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, nil)
		olmClient := operatorfake.NewSimpleClientset(sub) //nolint:staticcheck // NewClientset requires generated apply configs not available in OLM

		c := client.NewForTesting(client.TestClientConfig{
			Dynamic: dynamicClient,
			OLM:     olmClient,
		})

		ver := semver.MustParse("2.17.0")
		target := check.Target{
			Client:        c,
			TargetVersion: &ver,
		}

		chk := newTestOperatorCheck()
		dr, err := validate.Operator(chk, target).Run(ctx)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(dr).ToNot(BeNil())
		g.Expect(dr.Status.Conditions).To(HaveLen(1))
		g.Expect(dr.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
			"Type":    Equal(check.ConditionTypeAvailable),
			"Status":  Equal(metav1.ConditionTrue),
			"Reason":  Equal(check.ReasonResourceFound),
			"Message": ContainSubstring("cert-manager.v1.13.0"),
		}))
		g.Expect(dr.Annotations).To(HaveKeyWithValue("operator.opendatahub.io/installed-version", "cert-manager.v1.13.0"))
		g.Expect(dr.Annotations).To(HaveKeyWithValue(check.AnnotationCheckTargetVersion, "2.17.0"))
	})

	t.Run("should match with WithNames", func(t *testing.T) {
		sub := &operatorsv1alpha1.Subscription{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "openshift-cert-manager-operator",
				Namespace: "cert-manager-operator",
			},
			Status: operatorsv1alpha1.SubscriptionStatus{
				InstalledCSV: "cert-manager-operator.v1.12.0",
			},
		}

		scheme := runtime.NewScheme()
		dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, nil)
		olmClient := operatorfake.NewSimpleClientset(sub) //nolint:staticcheck // NewClientset requires generated apply configs not available in OLM

		c := client.NewForTesting(client.TestClientConfig{
			Dynamic: dynamicClient,
			OLM:     olmClient,
		})

		ver := semver.MustParse("2.17.0")
		target := check.Target{
			Client:        c,
			TargetVersion: &ver,
		}

		chk := newTestOperatorCheck()
		dr, err := validate.Operator(chk, target).
			WithNames("cert-manager", "openshift-cert-manager-operator").
			Run(ctx)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(dr).ToNot(BeNil())
		g.Expect(dr.Status.Conditions).To(HaveLen(1))
		g.Expect(dr.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
			"Type":   Equal(check.ConditionTypeAvailable),
			"Status": Equal(metav1.ConditionTrue),
		}))
		g.Expect(dr.Annotations).To(HaveKeyWithValue("operator.opendatahub.io/installed-version", "cert-manager-operator.v1.12.0"))
	})

	t.Run("should match with WithNames and WithChannels", func(t *testing.T) {
		sub := &operatorsv1alpha1.Subscription{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "servicemeshoperator",
				Namespace: "openshift-operators",
			},
			Spec: &operatorsv1alpha1.SubscriptionSpec{
				Channel: "stable",
			},
			Status: operatorsv1alpha1.SubscriptionStatus{
				InstalledCSV: "servicemeshoperator.v2.6.0",
			},
		}

		scheme := runtime.NewScheme()
		dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, nil)
		olmClient := operatorfake.NewSimpleClientset(sub) //nolint:staticcheck // NewClientset requires generated apply configs not available in OLM

		c := client.NewForTesting(client.TestClientConfig{
			Dynamic: dynamicClient,
			OLM:     olmClient,
		})

		ver := semver.MustParse("3.0.0")
		target := check.Target{
			Client:        c,
			TargetVersion: &ver,
		}

		chk := newTestOperatorCheck()
		dr, err := validate.Operator(chk, target).
			WithNames("servicemeshoperator").
			WithChannels("stable", "v2.x").
			Run(ctx)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(dr).ToNot(BeNil())
		g.Expect(dr.Status.Conditions).To(HaveLen(1))
		g.Expect(dr.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
			"Type":   Equal(check.ConditionTypeAvailable),
			"Status": Equal(metav1.ConditionTrue),
		}))
		g.Expect(dr.Annotations).To(HaveKeyWithValue("operator.opendatahub.io/installed-version", "servicemeshoperator.v2.6.0"))
	})

	t.Run("should not match when channel does not match", func(t *testing.T) {
		sub := &operatorsv1alpha1.Subscription{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "servicemeshoperator",
				Namespace: "openshift-operators",
			},
			Spec: &operatorsv1alpha1.SubscriptionSpec{
				Channel: "v3.x",
			},
			Status: operatorsv1alpha1.SubscriptionStatus{
				InstalledCSV: "servicemeshoperator.v3.0.0",
			},
		}

		scheme := runtime.NewScheme()
		dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, nil)
		olmClient := operatorfake.NewSimpleClientset(sub) //nolint:staticcheck // NewClientset requires generated apply configs not available in OLM

		c := client.NewForTesting(client.TestClientConfig{
			Dynamic: dynamicClient,
			OLM:     olmClient,
		})

		ver := semver.MustParse("3.0.0")
		target := check.Target{
			Client:        c,
			TargetVersion: &ver,
		}

		chk := newTestOperatorCheck()
		dr, err := validate.Operator(chk, target).
			WithNames("servicemeshoperator").
			WithChannels("stable", "v2.x").
			Run(ctx)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(dr).ToNot(BeNil())
		g.Expect(dr.Status.Conditions).To(HaveLen(1))
		g.Expect(dr.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
			"Type":   Equal(check.ConditionTypeAvailable),
			"Status": Equal(metav1.ConditionFalse),
		}))
	})

	t.Run("should use custom condition builder", func(t *testing.T) {
		scheme := runtime.NewScheme()
		dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, nil)
		olmClient := operatorfake.NewSimpleClientset() //nolint:staticcheck // NewClientset requires generated apply configs not available in OLM

		c := client.NewForTesting(client.TestClientConfig{
			Dynamic: dynamicClient,
			OLM:     olmClient,
		})

		ver := semver.MustParse("3.0.0")
		target := check.Target{
			Client:        c,
			TargetVersion: &ver,
		}

		// Inverted logic: NOT finding operator is success.
		chk := newTestOperatorCheck()
		dr, err := validate.Operator(chk, target).
			WithConditionBuilder(func(found bool, version string) result.Condition {
				if !found {
					return check.NewCondition(
						check.ConditionTypeCompatible,
						metav1.ConditionTrue,
						check.WithReason(check.ReasonVersionCompatible),
						check.WithMessage("Operator not installed - good"),
					)
				}

				return check.NewCondition(
					check.ConditionTypeCompatible,
					metav1.ConditionFalse,
					check.WithReason(check.ReasonVersionIncompatible),
					check.WithMessage("Operator (%s) should not be installed", version),
				)
			}).
			Run(ctx)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(dr).ToNot(BeNil())
		g.Expect(dr.Status.Conditions).To(HaveLen(1))
		g.Expect(dr.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
			"Type":    Equal(check.ConditionTypeCompatible),
			"Status":  Equal(metav1.ConditionTrue),
			"Reason":  Equal(check.ReasonVersionCompatible),
			"Message": ContainSubstring("not installed"),
		}))
	})

	t.Run("should not set version annotation when operator not found", func(t *testing.T) {
		scheme := runtime.NewScheme()
		dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, nil)
		olmClient := operatorfake.NewSimpleClientset() //nolint:staticcheck // NewClientset requires generated apply configs not available in OLM

		c := client.NewForTesting(client.TestClientConfig{
			Dynamic: dynamicClient,
			OLM:     olmClient,
		})

		target := check.Target{
			Client: c,
		}

		chk := newTestOperatorCheck()
		dr, err := validate.Operator(chk, target).Run(ctx)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(dr).ToNot(BeNil())
		g.Expect(dr.Annotations).ToNot(HaveKey("operator.opendatahub.io/installed-version"))
		// No target version set, so annotation should also be absent.
		g.Expect(dr.Annotations).ToNot(HaveKey(check.AnnotationCheckTargetVersion))
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
