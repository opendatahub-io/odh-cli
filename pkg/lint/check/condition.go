package check

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/lburgazzoli/odh-cli/pkg/lint/check/result"
)

// ConditionOption is a functional option for customizing condition creation.
type ConditionOption func(*result.Condition)

// WithImpact sets the impact explicitly, overriding auto-derivation.
// Use this when the default impact (derived from Status) is not appropriate.
//
// Example:
//
//	// Status=False normally derives Impact=Blocking
//	// Override to Advisory for deprecation warnings
//	check.NewCondition(
//	    check.ConditionTypeCompatible,
//	    metav1.ConditionFalse,
//	    check.ReasonDeprecated,
//	    "TrainingOperator deprecated in RHOAI 3.3",
//	    check.WithImpact(result.ImpactAdvisory),
//	)
func WithImpact(impact result.Impact) ConditionOption {
	return func(c *result.Condition) {
		c.Impact = impact
	}
}

// deriveImpact derives the default impact from condition status.
func deriveImpact(status metav1.ConditionStatus) result.Impact {
	switch status {
	case metav1.ConditionTrue:
		return result.ImpactNone
	case metav1.ConditionFalse:
		return result.ImpactBlocking
	case metav1.ConditionUnknown:
		return result.ImpactAdvisory
	}
	// Unreachable - all ConditionStatus values handled above
	return result.ImpactNone
}

// NewCondition creates a new Condition with automatic Impact derivation.
// Impact is derived from Status unless explicitly overridden via WithImpact option:
//   - Status=True  → Impact=None      (requirement met, no issues)
//   - Status=False → Impact=Blocking  (requirement not met, blocks upgrade)
//   - Status=Unknown → Impact=Advisory (unable to determine, proceed with caution)
//
// The message parameter supports printf-style formatting when args are provided.
//
// Examples:
//
//	// Default behavior: Impact auto-derived as Blocking
//	condition := check.NewCondition(
//	    check.ConditionTypeAvailable,
//	    metav1.ConditionFalse,
//	    check.ReasonResourceNotFound,
//	    "DataScienceCluster not found",
//	)
//
//	// Override impact: Deprecation is non-blocking
//	condition := check.NewCondition(
//	    check.ConditionTypeCompatible,
//	    metav1.ConditionFalse,
//	    check.ReasonDeprecated,
//	    "TrainingOperator deprecated in RHOAI 3.3",
//	    check.WithImpact(result.ImpactAdvisory),
//	)
//
//	// Formatted message
//	condition := check.NewCondition(
//	    check.ConditionTypeCompatible,
//	    metav1.ConditionFalse,
//	    check.ReasonVersionIncompatible,
//	    "Found %d resources using deprecated API version %s",
//	    count, apiVersion,
//	)
func NewCondition(
	conditionType string,
	status metav1.ConditionStatus,
	reason string,
	message string,
	argsAndOptions ...any,
) result.Condition {
	// Separate printf args from functional options.
	var options []ConditionOption
	var messageArgs []any

	for _, arg := range argsAndOptions {
		if opt, ok := arg.(ConditionOption); ok {
			options = append(options, opt)
		} else {
			messageArgs = append(messageArgs, arg)
		}
	}

	// Format message if args provided.
	if len(messageArgs) > 0 {
		message = fmt.Sprintf(message, messageArgs...)
	}

	// Create condition with auto-derived impact.
	c := result.Condition{
		Condition: metav1.Condition{
			Type:               conditionType,
			Status:             status,
			Reason:             reason,
			Message:            message,
			LastTransitionTime: metav1.Now(),
		},
		Impact: deriveImpact(status), // Auto-derive by default
	}

	// Apply functional options (can override Impact).
	for _, opt := range options {
		opt(&c)
	}

	// Validate the final condition.
	if err := c.Validate(); err != nil {
		panic(fmt.Sprintf("invalid condition: %v", err))
	}

	return c
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

	// ReasonComponentRenamed indicates a component was renamed in a new version.
	ReasonComponentRenamed = "ComponentRenamed"
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

	// ReasonDeprecated indicates a component or feature is deprecated.
	ReasonDeprecated = "Deprecated"

	// ReasonWorkloadsImpacted indicates workloads will be affected by changes.
	ReasonWorkloadsImpacted = "WorkloadsImpacted"

	// ReasonFeatureRemoved indicates a feature was removed in a new version.
	ReasonFeatureRemoved = "FeatureRemoved"
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
