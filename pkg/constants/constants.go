package constants

// Management state values for components and services.
const (
	ManagementStateManaged   = "Managed"
	ManagementStateUnmanaged = "Unmanaged"
	ManagementStateRemoved   = "Removed"
)

// Platform names for DSC and DSCI check kind identifiers.
const (
	PlatformDSCI = "dsci"
	PlatformDSC  = "dsc"
)

// Component names used across multiple package groups.
const (
	ComponentDashboard        = "dashboard"
	ComponentKServe           = "kserve"
	ComponentRay              = "ray"
	ComponentTrainingOperator = "trainingoperator"
	ComponentWorkbenches      = "workbenches"
)

// Component names for Kueue integration.
const (
	ComponentKueue = "kueue"
)

// Kueue-specific label keys used across workload check packages.
const (
	LabelKueueManaged          = "kueue-managed"
	LabelKueueOpenshiftManaged = "kueue.openshift.io/managed"
	LabelKueueQueueName        = "kueue.x-k8s.io/queue-name"
)

// Workload annotations used across multiple check packages.
const (
	// AnnotationLegacyHardwareProfile is the annotation key for legacy hardware profile references
	// on workload CRs (Notebooks, InferenceServices).
	AnnotationLegacyHardwareProfile = "opendatahub.io/legacy-hardware-profile-name"
)
