package constants

// Management state values for components and services.
const (
	ManagementStateManaged   = "Managed"
	ManagementStateUnmanaged = "Unmanaged"
	ManagementStateRemoved   = "Removed"
)

// Component names used across multiple package groups.
const (
	ComponentDashboard        = "dashboard"
	ComponentKServe           = "kserve"
	ComponentTrainingOperator = "trainingoperator"
)

// Workload annotations used across multiple check packages.
const (
	// AnnotationLegacyHardwareProfile is the annotation key for legacy hardware profile references
	// on workload CRs (Notebooks, InferenceServices).
	AnnotationLegacyHardwareProfile = "opendatahub.io/legacy-hardware-profile-name"
)
