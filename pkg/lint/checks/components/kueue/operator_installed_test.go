package kueue_test

import (
	"testing"

	operatorsv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	operatorfake "github.com/operator-framework/operator-lifecycle-manager/pkg/api/client/clientset/versioned/fake"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/lburgazzoli/odh-cli/pkg/lint/check"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/components/kueue"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/shared/testutil"

	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

func newKueueOperatorSubscription() *operatorsv1alpha1.Subscription {
	return &operatorsv1alpha1.Subscription{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kueue-operator",
			Namespace: "kueue-system",
		},
		Status: operatorsv1alpha1.SubscriptionStatus{
			InstalledCSV: "kueue-operator.v0.6.0",
		},
	}
}

// Unmanaged + operator installed = pass.
func TestOperatorInstalledCheck_UnmanagedInstalled(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:     listKinds,
		Objects:       []*unstructured.Unstructured{testutil.NewDSC(map[string]string{"kueue": "Unmanaged"})},
		OLM:           operatorfake.NewSimpleClientset(newKueueOperatorSubscription()), //nolint:staticcheck // NewClientset requires generated apply configs not available in OLM
		TargetVersion: "2.17.0",
	})

	chk := kueue.NewOperatorInstalledCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeCompatible),
		"Status":  Equal(metav1.ConditionTrue),
		"Reason":  Equal(check.ReasonVersionCompatible),
		"Message": ContainSubstring("kueue-operator.v0.6.0"),
	}))
	g.Expect(result.Annotations).To(HaveKeyWithValue("operator.opendatahub.io/installed-version", "kueue-operator.v0.6.0"))
}

// Unmanaged + operator NOT installed = blocking.
func TestOperatorInstalledCheck_UnmanagedNotInstalled(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:     listKinds,
		Objects:       []*unstructured.Unstructured{testutil.NewDSC(map[string]string{"kueue": "Unmanaged"})},
		OLM:           operatorfake.NewSimpleClientset(), //nolint:staticcheck // NewClientset requires generated apply configs not available in OLM
		TargetVersion: "2.17.0",
	})

	chk := kueue.NewOperatorInstalledCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeCompatible),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonVersionIncompatible),
		"Message": And(ContainSubstring("not installed"), ContainSubstring("Unmanaged")),
	}))
}

// Managed + operator NOT installed = pass.
func TestOperatorInstalledCheck_ManagedNotInstalled(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:     listKinds,
		Objects:       []*unstructured.Unstructured{testutil.NewDSC(map[string]string{"kueue": "Managed"})},
		OLM:           operatorfake.NewSimpleClientset(), //nolint:staticcheck // NewClientset requires generated apply configs not available in OLM
		TargetVersion: "2.17.0",
	})

	chk := kueue.NewOperatorInstalledCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeCompatible),
		"Status":  Equal(metav1.ConditionTrue),
		"Reason":  Equal(check.ReasonVersionCompatible),
		"Message": ContainSubstring("Managed"),
	}))
}

// Managed + operator installed = blocking.
func TestOperatorInstalledCheck_ManagedInstalled(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:     listKinds,
		Objects:       []*unstructured.Unstructured{testutil.NewDSC(map[string]string{"kueue": "Managed"})},
		OLM:           operatorfake.NewSimpleClientset(newKueueOperatorSubscription()), //nolint:staticcheck // NewClientset requires generated apply configs not available in OLM
		TargetVersion: "2.17.0",
	})

	chk := kueue.NewOperatorInstalledCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeCompatible),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonVersionIncompatible),
		"Message": And(ContainSubstring("kueue-operator"), ContainSubstring("cannot coexist")),
	}))
	g.Expect(result.Annotations).To(HaveKeyWithValue("operator.opendatahub.io/installed-version", "kueue-operator.v0.6.0"))
}

func TestOperatorInstalledCheck_CanApply_ManagementState(t *testing.T) {
	g := NewWithT(t)

	chk := kueue.NewOperatorInstalledCheck()

	testCases := []struct {
		name     string
		state    string
		expected bool
	}{
		{name: "Managed", state: "Managed", expected: true},
		{name: "Unmanaged", state: "Unmanaged", expected: true},
		{name: "Removed", state: "Removed", expected: false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			dsc := testutil.NewDSC(map[string]string{"kueue": tc.state})
			target := testutil.NewTarget(t, testutil.TargetConfig{
				ListKinds:     listKinds,
				Objects:       []*unstructured.Unstructured{dsc},
				TargetVersion: "2.17.0",
			})

			canApply, err := chk.CanApply(t.Context(), target)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(canApply).To(Equal(tc.expected))
		})
	}
}

func TestOperatorInstalledCheck_Metadata(t *testing.T) {
	g := NewWithT(t)

	chk := kueue.NewOperatorInstalledCheck()

	g.Expect(chk.ID()).To(Equal("components.kueue.operator-installed"))
	g.Expect(chk.Name()).To(Equal("Components :: Kueue :: Operator Installed"))
	g.Expect(chk.Group()).To(Equal(check.GroupComponent))
	g.Expect(chk.Description()).ToNot(BeEmpty())
}
