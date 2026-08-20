package check

// CheckType represents the type of check being performed.
type CheckType string

// Check type names used across multiple packages.
const (
	CheckTypeReadiness                   CheckType = "readiness"
	CheckTypeRemoval                     CheckType = "removal"
	CheckTypeInstalled                   CheckType = "installed"
	CheckTypeImpactedWorkloads           CheckType = "impacted-workloads"
	CheckTypeConfigMigration             CheckType = "config-migration"
	CheckTypeDataIntegrity               CheckType = "data-integrity"
	CheckTypeWorkloadState               CheckType = "workload-state"
	CheckTypeAcceleratorProfileMigration CheckType = "acceleratorprofile-migration"
	CheckTypeWebhookAvailability         CheckType = "webhook-availability"
	CheckTypeStoredVersions              CheckType = "stored-versions"
	CheckTypeFinalizerOrphan             CheckType = "finalizer-orphan"
	CheckTypeAuthModelMigration          CheckType = "auth-model-migration"
	CheckTypeRouteMigration              CheckType = "route-migration"
	CheckTypeResourceCapacity            CheckType = "resource-capacity"
	CheckTypeConfigCompatibility         CheckType = "config-compatibility"
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
