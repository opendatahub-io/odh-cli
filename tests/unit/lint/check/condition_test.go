package check_test

import (
	"errors"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/lburgazzoli/odh-cli/pkg/lint/check"
	"github.com/lburgazzoli/odh-cli/pkg/lint/check/result"

	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

// T021: metav1.Condition usage tests

func TestCondition_ValidConditionCreation(t *testing.T) {
	g := NewWithT(t)

	condition := metav1.Condition{
		Type:               check.ConditionTypeValidated,
		Status:             metav1.ConditionTrue,
		Reason:             check.ReasonRequirementsMet,
		Message:            "All requirements validated successfully",
		LastTransitionTime: metav1.Now(),
	}

	g.Expect(condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeValidated),
		"Status":  Equal(metav1.ConditionTrue),
		"Reason":  Equal(check.ReasonRequirementsMet),
		"Message": Equal("All requirements validated successfully"),
	}))
}

func TestCondition_AllConditionTypes(t *testing.T) {
	g := NewWithT(t)

	conditionTypes := []string{
		check.ConditionTypeValidated,
		check.ConditionTypeAvailable,
		check.ConditionTypeReady,
		check.ConditionTypeCompatible,
		check.ConditionTypeConfigured,
		check.ConditionTypeAuthorized,
	}

	for _, condType := range conditionTypes {
		condition := metav1.Condition{
			Type:               condType,
			Status:             metav1.ConditionTrue,
			Reason:             check.ReasonRequirementsMet,
			Message:            "Test condition",
			LastTransitionTime: metav1.Now(),
		}

		g.Expect(condition).To(MatchFields(IgnoreExtras, Fields{
			"Type": Equal(condType),
		}))
	}
}

func TestCondition_SuccessReasons(t *testing.T) {
	g := NewWithT(t)

	successReasons := []string{
		check.ReasonRequirementsMet,
		check.ReasonResourceFound,
		check.ReasonResourceAvailable,
		check.ReasonConfigurationValid,
		check.ReasonVersionCompatible,
		check.ReasonPermissionGranted,
	}

	for _, reason := range successReasons {
		condition := metav1.Condition{
			Type:               check.ConditionTypeValidated,
			Status:             metav1.ConditionTrue,
			Reason:             reason,
			Message:            "Success",
			LastTransitionTime: metav1.Now(),
		}

		g.Expect(condition).To(MatchFields(IgnoreExtras, Fields{
			"Reason": Equal(reason),
			"Status": Equal(metav1.ConditionTrue),
		}))
	}
}

func TestCondition_FailureReasons(t *testing.T) {
	g := NewWithT(t)

	failureReasons := []string{
		check.ReasonResourceNotFound,
		check.ReasonResourceUnavailable,
		check.ReasonConfigurationInvalid,
		check.ReasonVersionIncompatible,
		check.ReasonPermissionDenied,
		check.ReasonQuotaExceeded,
		check.ReasonDependencyUnavailable,
	}

	for _, reason := range failureReasons {
		condition := metav1.Condition{
			Type:               check.ConditionTypeValidated,
			Status:             metav1.ConditionFalse,
			Reason:             reason,
			Message:            "Failure",
			LastTransitionTime: metav1.Now(),
		}

		g.Expect(condition).To(MatchFields(IgnoreExtras, Fields{
			"Reason": Equal(reason),
			"Status": Equal(metav1.ConditionFalse),
		}))
	}
}

func TestCondition_UnknownReasons(t *testing.T) {
	g := NewWithT(t)

	unknownReasons := []string{
		check.ReasonCheckExecutionFailed,
		check.ReasonCheckSkipped,
		check.ReasonAPIAccessDenied,
		check.ReasonInsufficientData,
	}

	for _, reason := range unknownReasons {
		condition := metav1.Condition{
			Type:               check.ConditionTypeValidated,
			Status:             metav1.ConditionUnknown,
			Reason:             reason,
			Message:            "Unknown",
			LastTransitionTime: metav1.Now(),
		}

		g.Expect(condition).To(MatchFields(IgnoreExtras, Fields{
			"Reason": Equal(reason),
			"Status": Equal(metav1.ConditionUnknown),
		}))
	}
}

// T022: Condition Status semantics tests

func TestConditionStatus_TrueMeansPassing(t *testing.T) {
	g := NewWithT(t)

	condition := metav1.Condition{
		Type:               check.ConditionTypeValidated,
		Status:             metav1.ConditionTrue,
		Reason:             check.ReasonRequirementsMet,
		Message:            "Check passed",
		LastTransitionTime: metav1.Now(),
	}

	g.Expect(condition).To(MatchFields(IgnoreExtras, Fields{
		"Status": Equal(metav1.ConditionTrue),
	}))
	g.Expect(string(condition.Status)).To(Equal("True"))
}

func TestConditionStatus_FalseMeansFailing(t *testing.T) {
	g := NewWithT(t)

	condition := metav1.Condition{
		Type:               check.ConditionTypeValidated,
		Status:             metav1.ConditionFalse,
		Reason:             check.ReasonResourceNotFound,
		Message:            "Check failed",
		LastTransitionTime: metav1.Now(),
	}

	g.Expect(condition).To(MatchFields(IgnoreExtras, Fields{
		"Status": Equal(metav1.ConditionFalse),
	}))
	g.Expect(string(condition.Status)).To(Equal("False"))
}

func TestConditionStatus_UnknownMeansUnableToDetermine(t *testing.T) {
	g := NewWithT(t)

	condition := metav1.Condition{
		Type:               check.ConditionTypeValidated,
		Status:             metav1.ConditionUnknown,
		Reason:             check.ReasonCheckExecutionFailed,
		Message:            "Unable to determine status",
		LastTransitionTime: metav1.Now(),
	}

	g.Expect(condition).To(MatchFields(IgnoreExtras, Fields{
		"Status": Equal(metav1.ConditionUnknown),
	}))
	g.Expect(string(condition.Status)).To(Equal("Unknown"))
}

func TestConditionStatus_AllValidStatuses(t *testing.T) {
	g := NewWithT(t)

	validStatuses := []metav1.ConditionStatus{
		metav1.ConditionTrue,
		metav1.ConditionFalse,
		metav1.ConditionUnknown,
	}

	for _, status := range validStatuses {
		condition := metav1.Condition{
			Type:               check.ConditionTypeValidated,
			Status:             status,
			Reason:             check.ReasonRequirementsMet,
			Message:            "Test",
			LastTransitionTime: metav1.Now(),
		}

		g.Expect(condition).To(MatchFields(IgnoreExtras, Fields{
			"Status": Equal(status),
		}))
	}
}

func TestConditionStatus_PassingScenario(t *testing.T) {
	g := NewWithT(t)

	condition := metav1.Condition{
		Type:               check.ConditionTypeReady,
		Status:             metav1.ConditionTrue,
		Reason:             "PodsReady",
		Message:            "All pods are running and ready",
		LastTransitionTime: metav1.Now(),
	}

	// True status indicates condition is met (passing)
	g.Expect(condition).To(MatchFields(IgnoreExtras, Fields{
		"Status": Equal(metav1.ConditionTrue),
	}))
}

func TestConditionStatus_FailingScenario(t *testing.T) {
	g := NewWithT(t)

	condition := metav1.Condition{
		Type:               check.ConditionTypeReady,
		Status:             metav1.ConditionFalse,
		Reason:             "PodsNotReady",
		Message:            "2 of 3 pods are not ready",
		LastTransitionTime: metav1.Now(),
	}

	// False status indicates condition is not met (failing)
	g.Expect(condition).To(MatchFields(IgnoreExtras, Fields{
		"Status": Equal(metav1.ConditionFalse),
	}))
}

func TestConditionStatus_ErrorScenario(t *testing.T) {
	g := NewWithT(t)

	condition := metav1.Condition{
		Type:               check.ConditionTypeReady,
		Status:             metav1.ConditionUnknown,
		Reason:             check.ReasonCheckExecutionFailed,
		Message:            "Failed to query pod status: connection timeout",
		LastTransitionTime: metav1.Now(),
	}

	// Unknown status indicates unable to determine (error/skipped)
	g.Expect(condition).To(MatchFields(IgnoreExtras, Fields{
		"Status": Equal(metav1.ConditionUnknown),
	}))
}

// T025: Condition builder helper function tests

func TestNewCondition_CreatesValidCondition(t *testing.T) {
	g := NewWithT(t)

	condition := check.NewCondition(
		check.ConditionTypeValidated,
		metav1.ConditionTrue,
		check.ReasonRequirementsMet,
		"All requirements validated successfully",
	)

	g.Expect(condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeValidated),
		"Status":  Equal(metav1.ConditionTrue),
		"Reason":  Equal(check.ReasonRequirementsMet),
		"Message": Equal("All requirements validated successfully"),
	}))
	g.Expect(condition.LastTransitionTime.Time).To(BeTemporally("~", time.Now(), time.Second))
}

func TestNewCondition_AutomaticallySetsDuration(t *testing.T) {
	g := NewWithT(t)

	beforeTime := time.Now()
	condition := check.NewCondition(
		check.ConditionTypeReady,
		metav1.ConditionTrue,
		"PodsReady",
		"All pods ready",
	)
	afterTime := time.Now()

	g.Expect(condition.LastTransitionTime.Time).To(BeTemporally(">=", beforeTime))
	g.Expect(condition.LastTransitionTime.Time).To(BeTemporally("<=", afterTime))
}

func TestNewCondition_FailureCondition(t *testing.T) {
	g := NewWithT(t)

	condition := check.NewCondition(
		check.ConditionTypeAvailable,
		metav1.ConditionFalse,
		check.ReasonResourceNotFound,
		"Resource not found in cluster",
	)

	g.Expect(condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeAvailable),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonResourceNotFound),
		"Message": Equal("Resource not found in cluster"),
	}))
}

func TestNewCondition_UnknownCondition(t *testing.T) {
	g := NewWithT(t)

	condition := check.NewCondition(
		check.ConditionTypeValidated,
		metav1.ConditionUnknown,
		check.ReasonCheckExecutionFailed,
		"Check execution failed: timeout",
	)

	g.Expect(condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeValidated),
		"Status":  Equal(metav1.ConditionUnknown),
		"Reason":  Equal(check.ReasonCheckExecutionFailed),
		"Message": Equal("Check execution failed: timeout"),
	}))
}

func TestNewCondition_MultipleConditionsHaveDifferentTimestamps(t *testing.T) {
	g := NewWithT(t)

	condition1 := check.NewCondition(
		check.ConditionTypeAvailable,
		metav1.ConditionTrue,
		check.ReasonResourceFound,
		"First condition",
	)

	time.Sleep(10 * time.Millisecond)

	condition2 := check.NewCondition(
		check.ConditionTypeReady,
		metav1.ConditionTrue,
		"PodsReady",
		"Second condition",
	)

	// Timestamps should be different (second one should be later)
	g.Expect(condition2.LastTransitionTime.Time).To(BeTemporally(">", condition1.LastTransitionTime.Time))
}

func TestNewCondition_UsedInDiagnosticResult(t *testing.T) {
	g := NewWithT(t)

	dr := result.New(
		"components",
		"kserve",
		"readiness",
		"Validates KServe readiness",
	)

	// Use NewCondition helper to add conditions
	dr.Status.Conditions = append(dr.Status.Conditions,
		check.NewCondition(
			check.ConditionTypeAvailable,
			metav1.ConditionTrue,
			check.ReasonResourceFound,
			"KServe deployment found",
		),
	)

	dr.Status.Conditions = append(dr.Status.Conditions,
		check.NewCondition(
			check.ConditionTypeReady,
			metav1.ConditionTrue,
			"PodsReady",
			"All KServe pods ready",
		),
	)

	g.Expect(dr.Status.Conditions).To(HaveLen(2))
	g.Expect(dr.Status.Conditions[0]).To(MatchFields(IgnoreExtras, Fields{
		"Type": Equal(check.ConditionTypeAvailable),
	}))
	g.Expect(dr.Status.Conditions[1]).To(MatchFields(IgnoreExtras, Fields{
		"Type": Equal(check.ConditionTypeReady),
	}))

	err := dr.Validate()
	g.Expect(err).ToNot(HaveOccurred())
}

// T026: NewCondition printf-style formatting tests

func TestNewCondition_WithSingleFormatArg(t *testing.T) {
	g := NewWithT(t)

	condition := check.NewCondition(
		check.ConditionTypeCompatible,
		metav1.ConditionFalse,
		check.ReasonVersionIncompatible,
		"Found %d incompatible resources",
		5,
	)

	g.Expect(condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeCompatible),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonVersionIncompatible),
		"Message": Equal("Found 5 incompatible resources"),
	}))
}

func TestNewCondition_WithMultipleFormatArgs(t *testing.T) {
	g := NewWithT(t)

	condition := check.NewCondition(
		check.ConditionTypeCompatible,
		metav1.ConditionFalse,
		check.ReasonVersionIncompatible,
		"Found %d %s - will be impacted in RHOAI %s",
		3,
		"InferenceServices",
		"3.x",
	)

	g.Expect(condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeCompatible),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonVersionIncompatible),
		"Message": Equal("Found 3 InferenceServices - will be impacted in RHOAI 3.x"),
	}))
}

func TestNewCondition_WithNoFormatArgs(t *testing.T) {
	g := NewWithT(t)

	condition := check.NewCondition(
		check.ConditionTypeValidated,
		metav1.ConditionTrue,
		check.ReasonRequirementsMet,
		"All requirements validated successfully",
	)

	g.Expect(condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeValidated),
		"Status":  Equal(metav1.ConditionTrue),
		"Reason":  Equal(check.ReasonRequirementsMet),
		"Message": Equal("All requirements validated successfully"),
	}))
}

func TestNewCondition_WithStringFormatArg(t *testing.T) {
	g := NewWithT(t)

	condition := check.NewCondition(
		check.ConditionTypeAvailable,
		metav1.ConditionFalse,
		check.ReasonResourceNotFound,
		"Resource %s not found",
		"my-deployment",
	)

	g.Expect(condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeAvailable),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonResourceNotFound),
		"Message": Equal("Resource my-deployment not found"),
	}))
}

func TestNewCondition_WithErrorFormatArg(t *testing.T) {
	g := NewWithT(t)

	testErr := errors.New("connection timeout")

	condition := check.NewCondition(
		check.ConditionTypeValidated,
		metav1.ConditionUnknown,
		check.ReasonCheckExecutionFailed,
		"Check execution failed: %v",
		testErr,
	)

	g.Expect(condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeValidated),
		"Status":  Equal(metav1.ConditionUnknown),
		"Reason":  Equal(check.ReasonCheckExecutionFailed),
		"Message": Equal("Check execution failed: connection timeout"),
	}))
}
