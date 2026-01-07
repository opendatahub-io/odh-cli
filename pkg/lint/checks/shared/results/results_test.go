package results_test

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/lburgazzoli/odh-cli/pkg/lint/check"
	"github.com/lburgazzoli/odh-cli/pkg/lint/check/result"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/shared/results"

	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

func TestSetCondition_AddNew(t *testing.T) {
	g := NewWithT(t)

	dr := result.New("component", "test", "check", "description")
	results.SetCondition(dr, check.ConditionTypeCompatible, metav1.ConditionTrue, "TestReason", "test message")

	g.Expect(dr.Status.Conditions).To(HaveLen(1))
	g.Expect(dr.Status.Conditions[0]).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeCompatible),
		"Status":  Equal(metav1.ConditionTrue),
		"Reason":  Equal("TestReason"),
		"Message": Equal("test message"),
	}))
}

func TestSetCondition_UpdateExisting(t *testing.T) {
	g := NewWithT(t)

	dr := result.New("component", "test", "check", "description")

	// First call adds new condition
	results.SetCondition(dr, check.ConditionTypeCompatible, metav1.ConditionTrue, "reason1", "message1")
	g.Expect(dr.Status.Conditions).To(HaveLen(1))

	// Second call with same type updates existing
	results.SetCondition(dr, check.ConditionTypeCompatible, metav1.ConditionFalse, "reason2", "message2")
	g.Expect(dr.Status.Conditions).To(HaveLen(1))
	g.Expect(dr.Status.Conditions[0]).To(MatchFields(IgnoreExtras, Fields{
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal("reason2"),
		"Message": Equal("message2"),
	}))
}

func TestSetCondition_MultipleConditionTypes(t *testing.T) {
	g := NewWithT(t)

	dr := result.New("component", "test", "check", "description")

	// Add first condition type
	results.SetCondition(dr, check.ConditionTypeCompatible, metav1.ConditionTrue, "reason1", "message1")
	g.Expect(dr.Status.Conditions).To(HaveLen(1))

	// Add different condition type
	results.SetCondition(dr, check.ConditionTypeAvailable, metav1.ConditionTrue, "reason2", "message2")
	g.Expect(dr.Status.Conditions).To(HaveLen(2))

	// Update first condition type
	results.SetCondition(dr, check.ConditionTypeCompatible, metav1.ConditionFalse, "reason3", "message3")
	g.Expect(dr.Status.Conditions).To(HaveLen(2))

	// Verify both conditions exist with correct values
	g.Expect(dr.Status.Conditions[0]).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(check.ConditionTypeCompatible),
		"Status": Equal(metav1.ConditionFalse),
	}))
	g.Expect(dr.Status.Conditions[1]).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(check.ConditionTypeAvailable),
		"Status": Equal(metav1.ConditionTrue),
	}))
}

func TestSetCompatibilitySuccess_Simple(t *testing.T) {
	g := NewWithT(t)

	dr := result.New("component", "test", "check", "description")
	results.SetCompatibilitySuccessf(dr, "simple success message")

	g.Expect(dr.Status.Conditions).To(HaveLen(1))
	g.Expect(dr.Status.Conditions[0]).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeCompatible),
		"Status":  Equal(metav1.ConditionTrue),
		"Reason":  Equal(check.ReasonVersionCompatible),
		"Message": Equal("simple success message"),
	}))
}

func TestSetCompatibilitySuccess_WithFormatting(t *testing.T) {
	g := NewWithT(t)

	dr := result.New("component", "test", "check", "description")
	results.SetCompatibilitySuccessf(dr, "State: %s is compatible with version %s", "Managed", "3.0")

	g.Expect(dr.Status.Conditions).To(HaveLen(1))
	g.Expect(dr.Status.Conditions[0]).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeCompatible),
		"Status":  Equal(metav1.ConditionTrue),
		"Reason":  Equal(check.ReasonVersionCompatible),
		"Message": Equal("State: Managed is compatible with version 3.0"),
	}))
}

func TestSetCompatibilityFailure_Simple(t *testing.T) {
	g := NewWithT(t)

	dr := result.New("component", "test", "check", "description")
	results.SetCompatibilityFailuref(dr, "simple failure message")

	g.Expect(dr.Status.Conditions).To(HaveLen(1))
	g.Expect(dr.Status.Conditions[0]).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeCompatible),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonVersionIncompatible),
		"Message": Equal("simple failure message"),
	}))
}

func TestSetCompatibilityFailure_WithFormatting(t *testing.T) {
	g := NewWithT(t)

	dr := result.New("component", "test", "check", "description")
	results.SetCompatibilityFailuref(dr, "State: %s is incompatible with version %s", "Removed", "3.0")

	g.Expect(dr.Status.Conditions).To(HaveLen(1))
	g.Expect(dr.Status.Conditions[0]).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeCompatible),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonVersionIncompatible),
		"Message": Equal("State: Removed is incompatible with version 3.0"),
	}))
}

func TestSetAvailabilitySuccess_Simple(t *testing.T) {
	g := NewWithT(t)

	dr := result.New("component", "test", "check", "description")
	results.SetAvailabilitySuccessf(dr, "resource found")

	g.Expect(dr.Status.Conditions).To(HaveLen(1))
	g.Expect(dr.Status.Conditions[0]).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeAvailable),
		"Status":  Equal(metav1.ConditionTrue),
		"Reason":  Equal(check.ReasonResourceFound),
		"Message": Equal("resource found"),
	}))
}

func TestSetAvailabilitySuccess_WithFormatting(t *testing.T) {
	g := NewWithT(t)

	dr := result.New("component", "test", "check", "description")
	results.SetAvailabilitySuccessf(dr, "Found %d resources of type %s", 5, "Pod")

	g.Expect(dr.Status.Conditions).To(HaveLen(1))
	g.Expect(dr.Status.Conditions[0]).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeAvailable),
		"Status":  Equal(metav1.ConditionTrue),
		"Reason":  Equal(check.ReasonResourceFound),
		"Message": Equal("Found 5 resources of type Pod"),
	}))
}

func TestSetAvailabilityFailure_Simple(t *testing.T) {
	g := NewWithT(t)

	dr := result.New("component", "test", "check", "description")
	results.SetAvailabilityFailuref(dr, "resource not found")

	g.Expect(dr.Status.Conditions).To(HaveLen(1))
	g.Expect(dr.Status.Conditions[0]).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeAvailable),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonResourceNotFound),
		"Message": Equal("resource not found"),
	}))
}

func TestSetAvailabilityFailure_WithFormatting(t *testing.T) {
	g := NewWithT(t)

	dr := result.New("component", "test", "check", "description")
	results.SetAvailabilityFailuref(dr, "Resource %s not found in namespace %s", "my-deployment", "default")

	g.Expect(dr.Status.Conditions).To(HaveLen(1))
	g.Expect(dr.Status.Conditions[0]).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeAvailable),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonResourceNotFound),
		"Message": Equal("Resource my-deployment not found in namespace default"),
	}))
}

func TestSetComponentNotConfigured(t *testing.T) {
	g := NewWithT(t)

	dr := result.New("component", "test", "check", "description")
	results.SetComponentNotConfigured(dr, "kserve")

	g.Expect(dr.Status.Conditions).To(HaveLen(1))
	g.Expect(dr.Status.Conditions[0]).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeConfigured),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonResourceNotFound),
		"Message": Equal("kserve component is not configured in DataScienceCluster"),
	}))
}

func TestSetServiceNotConfigured(t *testing.T) {
	g := NewWithT(t)

	dr := result.New("service", "test", "check", "description")
	results.SetServiceNotConfigured(dr, "servicemesh")

	g.Expect(dr.Status.Conditions).To(HaveLen(1))
	g.Expect(dr.Status.Conditions[0]).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeConfigured),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonResourceNotFound),
		"Message": Equal("servicemesh is not configured in DSCInitialization"),
	}))
}

func TestSetComponentNotManaged(t *testing.T) {
	g := NewWithT(t)

	dr := result.New("component", "test", "check", "description")
	results.SetComponentNotManaged(dr, "kueue", "Removed")

	g.Expect(dr.Status.Conditions).To(HaveLen(1))
	g.Expect(dr.Status.Conditions[0]).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeConfigured),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal("ComponentNotManaged"),
		"Message": Equal("kueue component is not managed (state: Removed)"),
	}))
}

func TestDataScienceClusterNotFound(t *testing.T) {
	g := NewWithT(t)

	dr := results.DataScienceClusterNotFound("component", "test", "check", "description")

	g.Expect(dr.Status.Conditions).To(HaveLen(1))
	g.Expect(dr.Status.Conditions[0]).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeAvailable),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonResourceNotFound),
		"Message": Equal("No DataScienceCluster found"),
	}))
}

func TestDSCInitializationNotFound(t *testing.T) {
	g := NewWithT(t)

	dr := results.DSCInitializationNotFound("service", "test", "check", "description")

	g.Expect(dr.Status.Conditions).To(HaveLen(1))
	g.Expect(dr.Status.Conditions[0]).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeAvailable),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonResourceNotFound),
		"Message": Equal("No DSCInitialization found"),
	}))
}
