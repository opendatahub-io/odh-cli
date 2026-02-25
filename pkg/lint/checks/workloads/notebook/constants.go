package notebook

const (
	// kind is the check kind for all notebook checks.
	kind = "notebook"

	// componentWorkbenches is the DSC component name used to check management state.
	componentWorkbenches = "workbenches"
)

// Condition types reported by notebook checks.
const (
	ConditionTypeAcceleratorProfileCompatible = "AcceleratorProfileCompatible"
	ConditionTypeConnectionIntegrity          = "ConnectionIntegrity"
	ConditionTypeContainerNameValid           = "ContainerNameValid"
	ConditionTypeHardwareProfileCompatible    = "HardwareProfileCompatible"
	ConditionTypeHardwareProfileIntegrity     = "HardwareProfileIntegrity"
	ConditionTypeNotebooksCompatible          = "NotebooksCompatible"
	ConditionTypeKueueLabels                  = "KueueLabels"
	ConditionTypeRunningWorkloads             = "RunningWorkloads"
)

// Annotation keys used to detect notebook state and referenced resources.
const (
	// AnnotationKubeflowResourceStopped is present on a Notebook when it has been stopped.
	// Its value is an RFC3339 timestamp, but only the presence or absence of the key matters.
	AnnotationKubeflowResourceStopped = "kubeflow-resource-stopped"

	// AnnotationHardwareProfileName references an infrastructure HardwareProfile by name.
	AnnotationHardwareProfileName = "opendatahub.io/hardware-profile-name"

	// AnnotationHardwareProfileNamespace is the namespace of the referenced HardwareProfile.
	AnnotationHardwareProfileNamespace = "opendatahub.io/hardware-profile-namespace"

	// AnnotationConnections is a comma-separated list of namespace/name pairs
	// referencing Secrets that contain connection information.
	AnnotationConnections = "opendatahub.io/connections"
)

// Annotation keys set on ImpactedObjects by the ImpactedWorkloads check.
const (
	AnnotationCheckImageStatus = "check.opendatahub.io/image-status"
	AnnotationCheckImageRef    = "check.opendatahub.io/image-ref"
	AnnotationCheckReason      = "check.opendatahub.io/reason"
)

// Messages for ImpactedWorkloads check.
const (
	MsgNoNotebookInstances    = "No Notebook (workbench) instances found"
	MsgAllNotebooksCompatible = "All %d Notebook(s) use compatible OOTB images"
	MsgNotebookImageSummary   = "Found %d Notebook(s) using %d unique images:"
	MsgCompatibleCount        = "  - %d compatible (%d images, OOTB ready for %s)"
	MsgCustomCount            = "  - %d custom (%d images, user verification needed)"
	MsgIncompatibleCount      = "  - %d incompatible (%d images, must update before upgrade)"
	MsgUnverifiedCount        = "  - %d unverified (%d images, could not determine status)"
	MsgVerifyCustomImages     = "Verify custom images are compatible with RHOAI %s before upgrading"
)

// Messages for AcceleratorMigration check.
const (
	MsgNoAcceleratorProfiles        = "No Notebooks found using deprecated AcceleratorProfiles - no migration needed"
	MsgAcceleratorProfilesMissing   = "Found %d Notebook(s) referencing deprecated AcceleratorProfiles (%d missing): AcceleratorProfiles and Notebook references are automatically migrated to HardwareProfiles (infrastructure.opendatahub.io) during upgrade"
	MsgAcceleratorProfilesMigrating = "Found %d Notebook(s) using deprecated AcceleratorProfiles: AcceleratorProfiles and Notebook references are automatically migrated to HardwareProfiles (infrastructure.opendatahub.io) during upgrade"
)

// Messages for RunningWorkloads check.
const (
	MsgAllNotebooksStopped   = "No running Notebooks found - all Notebooks are stopped"
	MsgRunningNotebooksFound = "Found %d running Notebook(s) that should be stopped before upgrading"
)

// Messages for HardwareProfileIntegrity check.
const (
	MsgAllHardwareProfilesValid = "All Notebooks reference existing HardwareProfiles"
	MsgHardwareProfilesMissing  = "Found %d Notebook(s) referencing HardwareProfiles that do not exist on the cluster"
)

// Messages for ConnectionIntegrity check.
const (
	MsgAllConnectionsValid = "All Notebook connections reference existing Secrets"
	MsgConnectionsMissing  = "Found %d Notebook(s) referencing connection Secrets that do not exist on the cluster"
)

// Messages for KueueLabels check.
const (
	MsgAllKueueLabelsValid = "All Notebooks in kueue-enabled namespaces have the required queue label"
	MsgKueueLabelsMissing  = "Found %d Notebook(s) in kueue-enabled namespaces missing the kueue.x-k8s.io/queue-name label"
)

// Messages for ContainerName check.
const (
	MsgNoContainerNameMismatch = "No Notebooks found with container name mismatch"
	MsgContainerNameMismatch   = "Found %d Notebook(s) where the primary container name does not match the Notebook CR name"
)

// Messages for HardwareProfileMigration check.
const (
	MsgNoLegacyHardwareProfiles = "No Notebooks found with legacy hardware profile annotation - no migration needed"
	MsgLegacyHardwareProfiles   = "Found %d Notebook(s) with legacy hardware profile annotation that may need attention"
)
