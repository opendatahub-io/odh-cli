package results

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/lburgazzoli/odh-cli/pkg/lint/check"
	"github.com/lburgazzoli/odh-cli/pkg/lint/check/result"
)

// DataScienceClusterNotFound returns a standard passing result when DataScienceCluster is not found.
// This is used by component checks that require DSC to exist.
func DataScienceClusterNotFound(group string, kind string, name string, description string) *result.DiagnosticResult {
	dr := result.New(group, kind, name, description)
	dr.Status.Conditions = []metav1.Condition{
		check.NewCondition(
			check.ConditionTypeAvailable,
			metav1.ConditionFalse,
			check.ReasonResourceNotFound,
			"No DataScienceCluster found",
		),
	}

	return dr
}

// DSCInitializationNotFound returns a standard passing result when DSCInitialization is not found.
// This is used by service checks that require DSCInitialization to exist.
func DSCInitializationNotFound(group string, kind string, name string, description string) *result.DiagnosticResult {
	dr := result.New(group, kind, name, description)
	dr.Status.Conditions = []metav1.Condition{
		check.NewCondition(
			check.ConditionTypeAvailable,
			metav1.ConditionFalse,
			check.ReasonResourceNotFound,
			"No DSCInitialization found",
		),
	}

	return dr
}

// SetCondition updates or adds a condition to the diagnostic result.
// If a condition with the same type already exists, it updates it.
// If no condition with that type exists, it adds a new one.
// This allows checks to potentially have multiple conditions in the future.
func SetCondition(
	dr *result.DiagnosticResult,
	conditionType string,
	status metav1.ConditionStatus,
	reason string,
	message string,
) {
	newCondition := check.NewCondition(conditionType, status, reason, message)

	// Find and update existing condition of this type
	for i := range dr.Status.Conditions {
		if dr.Status.Conditions[i].Type == conditionType {
			dr.Status.Conditions[i] = newCondition

			return
		}
	}

	// No existing condition found, append new one
	dr.Status.Conditions = append(dr.Status.Conditions, newCondition)
}

// SetCompatibilitySuccessf sets a success condition for version compatibility checks.
// Supports printf-style formatting for cleaner message construction.
//
// Example:
//
//	SetCompatibilitySuccessf(dr, "State: %s is compatible with RHOAI %s", state, version)
func SetCompatibilitySuccessf(dr *result.DiagnosticResult, format string, args ...any) {
	SetCondition(dr,
		check.ConditionTypeCompatible,
		metav1.ConditionTrue,
		check.ReasonVersionCompatible,
		fmt.Sprintf(format, args...))
}

// SetCompatibilityFailuref sets a failure condition for version compatibility checks.
// Supports printf-style formatting for cleaner message construction.
//
// Example:
//
//	SetCompatibilityFailuref(dr, "State: %s is incompatible with RHOAI %s", state, version)
func SetCompatibilityFailuref(dr *result.DiagnosticResult, format string, args ...any) {
	SetCondition(dr,
		check.ConditionTypeCompatible,
		metav1.ConditionFalse,
		check.ReasonVersionIncompatible,
		fmt.Sprintf(format, args...))
}

// SetAvailabilitySuccessf sets a success condition for resource availability checks.
// Supports printf-style formatting for cleaner message construction.
//
// Example:
//
//	SetAvailabilitySuccessf(dr, "Found %d resources", count)
func SetAvailabilitySuccessf(dr *result.DiagnosticResult, format string, args ...any) {
	SetCondition(dr,
		check.ConditionTypeAvailable,
		metav1.ConditionTrue,
		check.ReasonResourceFound,
		fmt.Sprintf(format, args...))
}

// SetAvailabilityFailuref sets a failure condition for resource availability checks.
// Supports printf-style formatting for cleaner message construction.
//
// Example:
//
//	SetAvailabilityFailuref(dr, "Resource %s not found", name)
func SetAvailabilityFailuref(dr *result.DiagnosticResult, format string, args ...any) {
	SetCondition(dr,
		check.ConditionTypeAvailable,
		metav1.ConditionFalse,
		check.ReasonResourceNotFound,
		fmt.Sprintf(format, args...))
}

// SetComponentNotConfigured sets a condition indicating a component is not configured in DataScienceCluster.
func SetComponentNotConfigured(dr *result.DiagnosticResult, componentName string) {
	SetCondition(dr,
		check.ConditionTypeConfigured,
		metav1.ConditionFalse,
		check.ReasonResourceNotFound,
		componentName+" component is not configured in DataScienceCluster")
}

// SetServiceNotConfigured sets a condition indicating a service is not configured in DSCInitialization.
func SetServiceNotConfigured(dr *result.DiagnosticResult, serviceName string) {
	SetCondition(dr,
		check.ConditionTypeConfigured,
		metav1.ConditionFalse,
		check.ReasonResourceNotFound,
		serviceName+" is not configured in DSCInitialization")
}

// SetComponentNotManaged sets a condition indicating a component exists but is not managed.
func SetComponentNotManaged(dr *result.DiagnosticResult, componentName string, state string) {
	SetCondition(dr,
		check.ConditionTypeConfigured,
		metav1.ConditionFalse,
		"ComponentNotManaged",
		fmt.Sprintf("%s component is not managed (state: %s)", componentName, state))
}
