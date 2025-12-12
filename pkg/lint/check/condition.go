package check

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NewCondition creates a new metav1.Condition with the current timestamp.
//
// This helper ensures LastTransitionTime is automatically set to the current time,
// providing consistent condition creation across all checks.
//
// Example usage:
//
//	condition := check.NewCondition(
//	    check.ConditionTypeValidated,
//	    metav1.ConditionTrue,
//	    check.ReasonRequirementsMet,
//	    "All version requirements validated successfully",
//	)
func NewCondition(
	conditionType string,
	status metav1.ConditionStatus,
	reason string,
	message string,
) metav1.Condition {
	return metav1.Condition{
		Type:               conditionType,
		Status:             status,
		Reason:             reason,
		Message:            message,
		LastTransitionTime: metav1.Now(),
	}
}

// Standard Condition Types.
const (
	// ConditionTypeValidated indicates the resource has been validated.
	ConditionTypeValidated = "Validated"

	// ConditionTypeAvailable indicates the resource is available.
	ConditionTypeAvailable = "Available"

	// ConditionTypeReady indicates the resource is ready.
	ConditionTypeReady = "Ready"

	// ConditionTypeCompatible indicates version/configuration compatibility.
	ConditionTypeCompatible = "Compatible"

	// ConditionTypeConfigured indicates configuration is valid.
	ConditionTypeConfigured = "Configured"

	// ConditionTypeAuthorized indicates permissions/access are granted.
	ConditionTypeAuthorized = "Authorized"
)

// Standard Reason Values - Success.
const (
	// ReasonRequirementsMet indicates all requirements are satisfied.
	ReasonRequirementsMet = "RequirementsMet"

	// ReasonResourceFound indicates the resource was found.
	ReasonResourceFound = "ResourceFound"

	// ReasonResourceAvailable indicates the resource is available.
	ReasonResourceAvailable = "ResourceAvailable"

	// ReasonConfigurationValid indicates configuration is valid.
	ReasonConfigurationValid = "ConfigurationValid"

	// ReasonVersionCompatible indicates version compatibility is confirmed.
	ReasonVersionCompatible = "VersionCompatible"

	// ReasonPermissionGranted indicates required permissions are granted.
	ReasonPermissionGranted = "PermissionGranted"
)

// Standard Reason Values - Failure.
const (
	// ReasonResourceNotFound indicates the resource was not found.
	ReasonResourceNotFound = "ResourceNotFound"

	// ReasonResourceUnavailable indicates the resource is unavailable.
	ReasonResourceUnavailable = "ResourceUnavailable"

	// ReasonConfigurationInvalid indicates configuration is invalid.
	ReasonConfigurationInvalid = "ConfigurationInvalid"

	// ReasonVersionIncompatible indicates version incompatibility.
	ReasonVersionIncompatible = "VersionIncompatible"

	// ReasonPermissionDenied indicates required permissions are denied.
	ReasonPermissionDenied = "PermissionDenied"

	// ReasonQuotaExceeded indicates resource quota has been exceeded.
	ReasonQuotaExceeded = "QuotaExceeded"

	// ReasonDependencyUnavailable indicates a dependency is unavailable.
	ReasonDependencyUnavailable = "DependencyUnavailable"
)

// Standard Reason Values - Unknown/Error.
const (
	// ReasonCheckExecutionFailed indicates the check execution failed.
	ReasonCheckExecutionFailed = "CheckExecutionFailed"

	// ReasonCheckSkipped indicates the check was skipped.
	ReasonCheckSkipped = "CheckSkipped"

	// ReasonAPIAccessDenied indicates API access was denied.
	ReasonAPIAccessDenied = "APIAccessDenied"

	// ReasonInsufficientData indicates insufficient data to determine status.
	ReasonInsufficientData = "InsufficientData"
)
