package check

import (
	"context"

	"github.com/blang/semver/v4"

	"github.com/lburgazzoli/odh-cli/pkg/lint/check/result"
)

// CheckGroup classifies checks into logical groups (component, service, workload, dependency).
type CheckGroup string

const (
	GroupComponent  CheckGroup = "component"
	GroupService    CheckGroup = "service"
	GroupWorkload   CheckGroup = "workload"
	GroupDependency CheckGroup = "dependency"
)

// Check represents a diagnostic test that validates a specific aspect of cluster configuration.
//
// # Diagnostic Results (Kubernetes CR pattern)
//
// The diagnostic framework supports Kubernetes Custom Resource (CR) conventions through
// result.DiagnosticResult. This structure provides:
//   - Metadata: Group, Kind, Name, and Annotations for resource-like identification
//   - Spec: Description of what the check validates
//   - Status: Array of metav1.Condition for multi-condition reporting
//
// Example using the CR pattern:
//
//	import "github.com/lburgazzoli/odh-cli/pkg/lint/check/result"
//
//	diagnostic := result.New(
//	    "components",           // Group: diagnostic category
//	    "kserve",              // Kind: specific component
//	    "version-compatibility", // Name: check identifier
//	    "Validates KServe version compatibility for upgrade readiness",
//	)
//
//	// Add version annotations
//	diagnostic.Metadata.Annotations["check.opendatahub.io/source-version"] = "2.15"
//	diagnostic.Metadata.Annotations["check.opendatahub.io/target-version"] = "3.0"
//
//	// Add conditions (one per validation requirement)
//	diagnostic.Status.Conditions = append(diagnostic.Status.Conditions, metav1.Condition{
//	    Type:               check.ConditionTypeValidated,
//	    Status:             metav1.ConditionTrue,  // True=pass, False=fail, Unknown=error
//	    Reason:             check.ReasonRequirementsMet,
//	    Message:            "KServe v0.11 is compatible with OpenShift AI 3.0",
//	    LastTransitionTime: metav1.Now(),
//	})
//
// Multi-condition example:
//
//	diagnostic.Status.Conditions = []metav1.Condition{
//	    {
//	        Type:   check.ConditionTypeAvailable,
//	        Status: metav1.ConditionTrue,
//	        Reason: check.ReasonResourceFound,
//	        Message: "KServe deployment found",
//	        LastTransitionTime: metav1.Now(),
//	    },
//	    {
//	        Type:   check.ConditionTypeReady,
//	        Status: metav1.ConditionTrue,
//	        Reason: "PodsReady",
//	        Message: "All KServe pods are ready (3/3)",
//	        LastTransitionTime: metav1.Now(),
//	    },
//	}
type Check interface {
	// ID returns the unique identifier for this check
	ID() string

	// Name returns the human-readable check name
	Name() string

	// Description returns what this check validates
	Description() string

	// Group returns the check group (component, service, workload, dependency)
	Group() CheckGroup

	// CanApply returns whether this check should run given the current and target versions.
	// currentVersion: the current cluster version (source for upgrades, nil for lint)
	// targetVersion: the target version being checked (for upgrades) or current version (for lint)
	// Both arguments are parsed semver versions. Either can be nil.
	// Default behavior: returns true if applicable to targetVersion.
	// Upgrade checks can override this to check both currentVersion and targetVersion.
	CanApply(currentVersion *semver.Version, targetVersion *semver.Version) bool

	// Validate executes the check against the provided target
	// Returns DiagnosticResult following Kubernetes CR pattern with conditions
	Validate(ctx context.Context, target *CheckTarget) (*result.DiagnosticResult, error)
}
