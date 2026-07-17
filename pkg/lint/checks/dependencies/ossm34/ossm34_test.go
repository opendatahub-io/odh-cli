package ossm34_test

import (
	"errors"
	"testing"

	"github.com/blang/semver/v4"
	operatorsv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	operatorfake "github.com/operator-framework/operator-lifecycle-manager/pkg/api/client/clientset/versioned/fake"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8stesting "k8s.io/client-go/testing"

	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	resultpkg "github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/testutil"
	"github.com/opendatahub-io/odh-cli/pkg/lint/checks/dependencies/ossm34"
	"github.com/opendatahub-io/odh-cli/pkg/resources"

	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

func newSubscription(startingCSV, installedCSV string) *operatorsv1alpha1.Subscription {
	return &operatorsv1alpha1.Subscription{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "servicemeshoperator3",
			Namespace: "openshift-operators",
		},
		Spec: &operatorsv1alpha1.SubscriptionSpec{
			StartingCSV: startingCSV,
			Channel:     "stable",
		},
		Status: operatorsv1alpha1.SubscriptionStatus{
			InstalledCSV: installedCSV,
		},
	}
}

func TestOSSM34Check_NoOLM(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	target := testutil.NewTarget(t, testutil.TargetConfig{
		TargetVersion: "3.0.0",
	})

	chk := ossm34.NewCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(check.ConditionTypeCompatible),
		"Status": Equal(metav1.ConditionTrue),
		"Reason": Equal(check.ReasonCheckSkipped),
	}))
}

func TestOSSM34Check_NoSubscription(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	target := testutil.NewTarget(t, testutil.TargetConfig{
		OLM:           operatorfake.NewSimpleClientset(), //nolint:staticcheck // NewClientset requires generated apply configs not available in OLM
		TargetVersion: "3.0.0",
	})

	chk := ossm34.NewCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(check.ConditionTypeCompatible),
		"Status": Equal(metav1.ConditionTrue),
		"Reason": Equal(check.ReasonRequirementsMet),
	}))
	g.Expect(result.Status.Conditions[0].Message).To(ContainSubstring("not found"))
}

func TestOSSM34Check_APIError_Propagated(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	olmClient := operatorfake.NewSimpleClientset() //nolint:staticcheck // NewClientset requires generated apply configs not available in OLM
	olmClient.PrependReactor("list", "subscriptions", func(_ k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("connection refused")
	})

	target := testutil.NewTarget(t, testutil.TargetConfig{
		OLM:           olmClient,
		TargetVersion: "3.0.0",
	})

	chk := ossm34.NewCheck()
	_, err := chk.Validate(ctx, target)

	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("listing subscriptions"))
	g.Expect(err.Error()).To(ContainSubstring("connection refused"))
}

func TestOSSM34Check_NoDrift(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	sub := newSubscription("servicemeshoperator3.v3.1.0", "servicemeshoperator3.v3.1.0")

	target := testutil.NewTarget(t, testutil.TargetConfig{
		OLM:           operatorfake.NewSimpleClientset(sub), //nolint:staticcheck // NewClientset requires generated apply configs not available in OLM
		TargetVersion: "3.0.0",
	})

	chk := ossm34.NewCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(check.ConditionTypeCompatible),
		"Status": Equal(metav1.ConditionTrue),
		"Reason": Equal(check.ReasonVersionCompatible),
	}))
	g.Expect(result.Status.Conditions[0].Message).To(ContainSubstring("not affected"))
}

func TestOSSM34Check_V33NotAffected(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	sub := newSubscription("servicemeshoperator3.v3.1.0", "servicemeshoperator3.v3.3.5")

	target := testutil.NewTarget(t, testutil.TargetConfig{
		OLM:           operatorfake.NewSimpleClientset(sub), //nolint:staticcheck // NewClientset requires generated apply configs not available in OLM
		TargetVersion: "3.0.0",
	})

	chk := ossm34.NewCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(check.ConditionTypeCompatible),
		"Status": Equal(metav1.ConditionTrue),
		"Reason": Equal(check.ReasonVersionCompatible),
	}))
}

func TestOSSM34Check_DriftToV34_Blocking(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	sub := newSubscription("servicemeshoperator3.v3.1.0", "servicemeshoperator3.v3.4.0")

	target := testutil.NewTarget(t, testutil.TargetConfig{
		OLM:           operatorfake.NewSimpleClientset(sub), //nolint:staticcheck // NewClientset requires generated apply configs not available in OLM
		TargetVersion: "3.0.0",
	})

	chk := ossm34.NewCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(check.ConditionTypeCompatible),
		"Status": Equal(metav1.ConditionFalse),
		"Reason": Equal(check.ReasonVersionIncompatible),
	}))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactBlocking))
	g.Expect(result.Status.Conditions[0].Message).To(ContainSubstring("3.4.0"))
	g.Expect(result.Status.Conditions[0].Message).To(ContainSubstring("GatewayConfig"))
	g.Expect(result.Status.Conditions[0].Message).To(ContainSubstring("drifted from pinned version"))
}

func TestOSSM34Check_DriftToHigherVersion_Blocking(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	sub := newSubscription("servicemeshoperator3.v3.1.0", "servicemeshoperator3.v3.5.0")

	target := testutil.NewTarget(t, testutil.TargetConfig{
		OLM:           operatorfake.NewSimpleClientset(sub), //nolint:staticcheck // NewClientset requires generated apply configs not available in OLM
		TargetVersion: "3.0.0",
	})

	chk := ossm34.NewCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(check.ConditionTypeCompatible),
		"Status": Equal(metav1.ConditionFalse),
		"Reason": Equal(check.ReasonVersionIncompatible),
	}))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactBlocking))
	g.Expect(result.Status.Conditions[0].Message).To(ContainSubstring("3.5.0"))
}

func TestOSSM34Check_NoStartingCSV_StillFlags(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	sub := newSubscription("", "servicemeshoperator3.v3.4.0")

	target := testutil.NewTarget(t, testutil.TargetConfig{
		OLM:           operatorfake.NewSimpleClientset(sub), //nolint:staticcheck // NewClientset requires generated apply configs not available in OLM
		TargetVersion: "3.0.0",
	})

	chk := ossm34.NewCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(check.ConditionTypeCompatible),
		"Status": Equal(metav1.ConditionFalse),
		"Reason": Equal(check.ReasonVersionIncompatible),
	}))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactBlocking))
	g.Expect(result.Status.Conditions[0].Message).ToNot(ContainSubstring("drifted from pinned version"))
}

func TestOSSM34Check_UnparsableCSV(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	sub := newSubscription("", "servicemeshoperator3.vNOTASEMVER")

	target := testutil.NewTarget(t, testutil.TargetConfig{
		OLM:           operatorfake.NewSimpleClientset(sub), //nolint:staticcheck // NewClientset requires generated apply configs not available in OLM
		TargetVersion: "3.0.0",
	})

	chk := ossm34.NewCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(check.ConditionTypeCompatible),
		"Status": Equal(metav1.ConditionUnknown),
		"Reason": Equal(check.ReasonInsufficientData),
	}))
}

func createClusterVersion(ver string) *unstructured.Unstructured {
	cv := &unstructured.Unstructured{}
	cv.SetAPIVersion("config.openshift.io/v1")
	cv.SetKind("ClusterVersion")
	cv.SetName("version")

	_ = unstructured.SetNestedField(cv.Object, ver, "status", "desired", "version")

	return cv
}

func TestOSSM34Check_DriftToV34_OCPPatched(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	sub := newSubscription("servicemeshoperator3.v3.1.0", "servicemeshoperator3.v3.4.0")
	cv := createClusterVersion("4.21.22")

	target := testutil.NewTarget(t, testutil.TargetConfig{
		OLM: operatorfake.NewSimpleClientset(sub), //nolint:staticcheck // NewClientset requires generated apply configs not available in OLM
		ListKinds: map[schema.GroupVersionResource]string{
			resources.ClusterVersion.GVR(): "ClusterVersionList",
		},
		Objects:       []*unstructured.Unstructured{cv},
		TargetVersion: "3.0.0",
	})

	chk := ossm34.NewCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(check.ConditionTypeCompatible),
		"Status": Equal(metav1.ConditionTrue),
		"Reason": Equal(check.ReasonVersionCompatible),
	}))
	g.Expect(result.Status.Conditions[0].Message).To(ContainSubstring("includes the fix"))
}

func TestOSSM34Check_DriftToV34_OCPHigherPatched(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	sub := newSubscription("servicemeshoperator3.v3.1.0", "servicemeshoperator3.v3.4.0")
	cv := createClusterVersion("4.22.0")

	target := testutil.NewTarget(t, testutil.TargetConfig{
		OLM: operatorfake.NewSimpleClientset(sub), //nolint:staticcheck // NewClientset requires generated apply configs not available in OLM
		ListKinds: map[schema.GroupVersionResource]string{
			resources.ClusterVersion.GVR(): "ClusterVersionList",
		},
		Objects:       []*unstructured.Unstructured{cv},
		TargetVersion: "3.0.0",
	})

	chk := ossm34.NewCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(check.ConditionTypeCompatible),
		"Status": Equal(metav1.ConditionTrue),
		"Reason": Equal(check.ReasonVersionCompatible),
	}))
}

func TestOSSM34Check_DriftToV34_OCPBelowFix(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	sub := newSubscription("servicemeshoperator3.v3.1.0", "servicemeshoperator3.v3.4.0")
	cv := createClusterVersion("4.21.21")

	target := testutil.NewTarget(t, testutil.TargetConfig{
		OLM: operatorfake.NewSimpleClientset(sub), //nolint:staticcheck // NewClientset requires generated apply configs not available in OLM
		ListKinds: map[schema.GroupVersionResource]string{
			resources.ClusterVersion.GVR(): "ClusterVersionList",
		},
		Objects:       []*unstructured.Unstructured{cv},
		TargetVersion: "3.0.0",
	})

	chk := ossm34.NewCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(check.ConditionTypeCompatible),
		"Status": Equal(metav1.ConditionFalse),
		"Reason": Equal(check.ReasonVersionIncompatible),
	}))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactBlocking))
}

func TestOSSM34Check_DriftToV34_OCP420Affected(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	sub := newSubscription("servicemeshoperator3.v3.1.0", "servicemeshoperator3.v3.4.0")
	cv := createClusterVersion("4.20.15")

	target := testutil.NewTarget(t, testutil.TargetConfig{
		OLM: operatorfake.NewSimpleClientset(sub), //nolint:staticcheck // NewClientset requires generated apply configs not available in OLM
		ListKinds: map[schema.GroupVersionResource]string{
			resources.ClusterVersion.GVR(): "ClusterVersionList",
		},
		Objects:       []*unstructured.Unstructured{cv},
		TargetVersion: "3.0.0",
	})

	chk := ossm34.NewCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(check.ConditionTypeCompatible),
		"Status": Equal(metav1.ConditionFalse),
		"Reason": Equal(check.ReasonVersionIncompatible),
	}))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactBlocking))
}

func TestOSSM34Check_DriftToV34_OCP419Affected(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	sub := newSubscription("servicemeshoperator3.v3.1.0", "servicemeshoperator3.v3.4.0")
	cv := createClusterVersion("4.19.30")

	target := testutil.NewTarget(t, testutil.TargetConfig{
		OLM: operatorfake.NewSimpleClientset(sub), //nolint:staticcheck // NewClientset requires generated apply configs not available in OLM
		ListKinds: map[schema.GroupVersionResource]string{
			resources.ClusterVersion.GVR(): "ClusterVersionList",
		},
		Objects:       []*unstructured.Unstructured{cv},
		TargetVersion: "3.0.0",
	})

	chk := ossm34.NewCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(check.ConditionTypeCompatible),
		"Status": Equal(metav1.ConditionFalse),
		"Reason": Equal(check.ReasonVersionIncompatible),
	}))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactBlocking))
}

func TestOSSM34Check_DriftToV34_OCPUndetectable(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	sub := newSubscription("servicemeshoperator3.v3.1.0", "servicemeshoperator3.v3.4.0")

	target := testutil.NewTarget(t, testutil.TargetConfig{
		OLM:           operatorfake.NewSimpleClientset(sub), //nolint:staticcheck // NewClientset requires generated apply configs not available in OLM
		TargetVersion: "3.0.0",
	})

	chk := ossm34.NewCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(check.ConditionTypeCompatible),
		"Status": Equal(metav1.ConditionFalse),
		"Reason": Equal(check.ReasonVersionIncompatible),
	}))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactBlocking))
}

func TestOSSM34Check_EmptyInstalledCSV(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	sub := newSubscription("servicemeshoperator3.v3.1.0", "")

	target := testutil.NewTarget(t, testutil.TargetConfig{
		OLM:           operatorfake.NewSimpleClientset(sub), //nolint:staticcheck // NewClientset requires generated apply configs not available in OLM
		TargetVersion: "3.0.0",
	})

	chk := ossm34.NewCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(check.ConditionTypeCompatible),
		"Status": Equal(metav1.ConditionTrue),
		"Reason": Equal(check.ReasonRequirementsMet),
	}))
	g.Expect(result.Status.Conditions[0].Message).To(ContainSubstring("pending installation"))
}

func TestOSSM34Check_CanApply_3xTarget(t *testing.T) {
	g := NewWithT(t)

	chk := ossm34.NewCheck()

	targetVer := semver.MustParse("3.0.0")
	target := check.Target{
		TargetVersion: &targetVer,
	}

	canApply, err := chk.CanApply(t.Context(), target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeTrue())
}

func TestOSSM34Check_CanApply_2xTarget(t *testing.T) {
	g := NewWithT(t)

	chk := ossm34.NewCheck()

	targetVer := semver.MustParse("2.17.0")
	target := check.Target{
		TargetVersion: &targetVer,
	}

	canApply, err := chk.CanApply(t.Context(), target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeFalse())
}

func TestOSSM34Check_CanApply_NilVersion(t *testing.T) {
	g := NewWithT(t)

	chk := ossm34.NewCheck()

	canApply, err := chk.CanApply(t.Context(), check.Target{})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeFalse())
}

func TestOSSM34Check_Metadata(t *testing.T) {
	g := NewWithT(t)

	chk := ossm34.NewCheck()

	g.Expect(chk.ID()).To(Equal("dependencies.ossm-v3-compatibility.compatibility"))
	g.Expect(chk.Name()).To(Equal("Dependencies :: OSSM v3 Compatibility :: Version Compatibility"))
	g.Expect(chk.Group()).To(Equal(check.GroupDependency))
	g.Expect(chk.Description()).ToNot(BeEmpty())
}
