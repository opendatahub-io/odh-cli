package check

import (
	"fmt"

	"github.com/opendatahub-io/odh-cli/pkg/lint/check/result"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ConditionOption is a functional option for customizing condition creation.
type ConditionOption func(*result.Condition)

// WithReason sets the condition reason.
func WithReason(reason string) ConditionOption {
	return func(c *result.Condition) {
		c.Reason = reason
	}
}

// WithMessage sets the condition message. Supports printf-style formatting:
// if args are provided, the message is formatted with fmt.Sprintf.
func WithMessage(format string, args ...any) ConditionOption {
	return func(c *result.Condition) {
		if len(args) > 0 {
			c.Message = fmt.Sprintf(format, args...)
		} else {
			c.Message = format
		}
	}
}

// WithImpact sets the impact explicitly, overriding auto-derivation.
// Use this when the default impact (derived from Status) is not appropriate.
func WithImpact(impact result.Impact) ConditionOption {
	return func(c *result.Condition) {
		c.Impact = impact
	}
}

// WithRemediation sets actionable guidance on how to resolve the condition.
func WithRemediation(remediation string) ConditionOption {
	return func(c *result.Condition) {
		c.Remediation = remediation
	}
}

// deriveImpact derives the default impact from condition status.
// Status=False and Status=Unknown both default to Advisory; checks that
// truly block upgrades must explicitly opt in via WithImpact(result.ImpactBlocking).
func deriveImpact(status metav1.ConditionStatus) result.Impact {
	if status == metav1.ConditionTrue {
		return result.ImpactNone
	}

	return result.ImpactAdvisory
}

// NewCondition creates a new Condition with automatic Impact derivation.
// Impact is derived from Status unless explicitly overridden via WithImpact:
//   - Status=True    → Impact=None     (requirement met, no issues)
//   - Status=False   → Impact=Advisory (requirement not met, warning)
//   - Status=Unknown → Impact=Advisory (unable to determine, proceed with caution)
//
// Use WithImpact(result.ImpactBlocking) for conditions that truly block upgrades.
//
// Examples:
//
//	condition := check.NewCondition(
//	    check.ConditionTypeAvailable,
//	    metav1.ConditionFalse,
//	    check.WithReason(check.ReasonResourceNotFound),
//	    check.WithMessage("DataScienceCluster not found"),
//	)
//
//	condition := check.NewCondition(
//	    check.ConditionTypeCompatible,
//	    metav1.ConditionFalse,
//	    check.WithReason(check.ReasonVersionIncompatible),
//	    check.WithMessage("Found %d resources using deprecated API version %s", count, apiVersion),
//	    check.WithImpact(result.ImpactAdvisory),
//	    check.WithRemediation("Migrate resources before upgrading"),
//	)
func NewCondition(
	conditionType string,
	status metav1.ConditionStatus,
	opts ...ConditionOption,
) result.Condition {
	c := result.Condition{
		Condition: metav1.Condition{
			Type:               conditionType,
			Status:             status,
			LastTransitionTime: metav1.Now(),
		},
		Impact: deriveImpact(status),
	}

	for _, opt := range opts {
		opt(&c)
	}

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

	// ConditionTypeMigrationRequired indicates resources require migration during upgrade.
	ConditionTypeMigrationRequired = "MigrationRequired"
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

	// ReasonMigrationPending indicates resources will be auto-migrated during upgrade.
	ReasonMigrationPending = "MigrationPending"

	// ReasonNoMigrationRequired indicates no resources require migration.
	ReasonNoMigrationRequired = "NoMigrationRequired"
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

	// ReasonConfigurationUnmanaged indicates a configuration is not managed by the operator.
	ReasonConfigurationUnmanaged = "ConfigurationUnmanaged"
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
