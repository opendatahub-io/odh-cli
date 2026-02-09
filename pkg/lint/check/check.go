package check

import (
	"context"

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

// CanonicalGroupOrder defines the execution order for check groups.
// Dependencies run first to validate platform prerequisites, followed by
// services, components, and finally workloads.
//
//nolint:gochecknoglobals // Canonical ordering must be accessible across packages
var CanonicalGroupOrder = []CheckGroup{
	GroupDependency,
	GroupService,
	GroupComponent,
	GroupWorkload,
}

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

	// CheckKind returns the kind of resource being checked (e.g., "kserve", "codeflare").
	// Used by validation builders to construct diagnostic results.
	// For component checks, this should match the DSC spec component path key
	// (the key used in .spec.components.<key>.managementState).
	CheckKind() string

	// CheckType returns the type of check (e.g., "removal", "deprecation").
	// Used by validation builders to construct diagnostic results.
	CheckType() string

	// CanApply returns whether this check should run given the check target context.
	// The target provides access to:
	// - CurrentVersion: the current cluster version (source for upgrades, nil for lint mode)
	// - TargetVersion: the target version being checked (for upgrades) or current version (for lint mode)
	// - Client: Kubernetes client for querying cluster state (enables component-conditional checks)
	// Default behavior: returns true if applicable to target.TargetVersion.
	// Upgrade checks can check both CurrentVersion and TargetVersion.
	// Component-conditional checks can query DataScienceCluster via target.Client.
	CanApply(ctx context.Context, target Target) (bool, error)

	// Validate executes the check against the provided target
	// Returns DiagnosticResult following Kubernetes CR pattern with conditions
	Validate(ctx context.Context, target Target) (*result.DiagnosticResult, error)
}
