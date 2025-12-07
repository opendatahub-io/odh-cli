package check

import (
	"context"

	"github.com/blang/semver/v4"
)

// CheckCategory represents the category of a check.
type CheckCategory string

const (
	CategoryComponent  CheckCategory = "component"
	CategoryService    CheckCategory = "service"
	CategoryWorkload   CheckCategory = "workload"
	CategoryDependency CheckCategory = "dependency"
)

// Check represents a diagnostic test that validates a specific aspect of cluster configuration.
type Check interface {
	// ID returns the unique identifier for this check
	ID() string

	// Name returns the human-readable check name
	Name() string

	// Description returns what this check validates
	Description() string

	// Category returns the check category (component, service, workload)
	Category() CheckCategory

	// CanApply returns whether this check should run given the current and target versions.
	// currentVersion: the current cluster version (source for upgrades, nil for lint)
	// targetVersion: the target version being checked (for upgrades) or current version (for lint)
	// Both arguments are parsed semver versions. Either can be nil.
	// Default behavior: returns true if applicable to targetVersion.
	// Upgrade checks can override this to check both currentVersion and targetVersion.
	CanApply(currentVersion *semver.Version, targetVersion *semver.Version) bool

	// Validate executes the check against the provided target
	// Returns DiagnosticResult with status, severity, and remediation guidance
	Validate(ctx context.Context, target *CheckTarget) (*DiagnosticResult, error)
}
