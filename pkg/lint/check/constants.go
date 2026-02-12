package check

// CheckType represents the type of check being performed.
type CheckType string

// Check type names used across multiple packages.
const (
	CheckTypeRemoval                     CheckType = "removal"
	CheckTypeInstalled                   CheckType = "installed"
	CheckTypeImpactedWorkloads           CheckType = "impacted-workloads"
	CheckTypeConfigMigration             CheckType = "config-migration"
	CheckTypeAcceleratorProfileMigration CheckType = "acceleratorprofile-migration"
)

// Annotation keys used across multiple packages.
const (
	// AnnotationComponentManagementState is the management state for components.
	AnnotationComponentManagementState = "component.opendatahub.io/management-state"

	// AnnotationCheckTargetVersion is the target version for upgrade checks.
	AnnotationCheckTargetVersion = "check.opendatahub.io/target-version"

	// AnnotationImpactedWorkloadCount is the count of impacted workloads.
	AnnotationImpactedWorkloadCount = "workload.opendatahub.io/impacted-count"
)
