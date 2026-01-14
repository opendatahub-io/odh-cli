package check

// Management state values for components and services.
const (
	ManagementStateManaged   = "Managed"
	ManagementStateUnmanaged = "Unmanaged"
	ManagementStateRemoved   = "Removed"
)

// Component names for diagnostic results.
const (
	ComponentCodeFlare            = "codeflare"
	ComponentDataSciencePipelines = "datasciencepipelines"
	ComponentModelMesh            = "modelmesh"
	ComponentKServe               = "kserve"
	ComponentKueue                = "kueue"
	ComponentRay                  = "ray"
	ComponentTrainingOperator     = "trainingoperator"
)

// Service names for diagnostic results.
const (
	ServiceServiceMesh = "servicemesh"
)

// Dependency names for diagnostic results.
const (
	DependencyCertManager           = "certmanager"
	DependencyKueueOperator         = "kueueoperator"
	DependencyServiceMeshOperatorV2 = "servicemesh-operator-v2"
	DependencyOpenShiftPlatform     = "openshift-platform"
)

// Check type names (third parameter to result.New).
const (
	CheckTypeRemoval            = "removal"
	CheckTypeInstalled          = "installed"
	CheckTypeInstructLabRemoval = "instructlab-removal"
	CheckTypeManagedRemoval     = "managed-removal"
	CheckTypeRenaming           = "renaming"
	CheckTypeServerlessRemoval  = "serverless-removal"
	CheckTypeImpactedWorkloads  = "impacted-workloads"
	CheckTypeDeprecation        = "deprecation"
	CheckTypeVersionRequirement = "version-requirement"
)

// Annotation keys for diagnostic results.
const (
	// AnnotationComponentManagementState is the management state for components.
	AnnotationComponentManagementState = "component.opendatahub.io/management-state"
	// AnnotationComponentKServeState is the KServe component management state.
	AnnotationComponentKServeState = "component.opendatahub.io/kserve-management-state"
	// AnnotationComponentServingState is the serving (serverless) management state.
	AnnotationComponentServingState = "component.opendatahub.io/serving-management-state"

	// AnnotationServiceManagementState is the management state for services.
	AnnotationServiceManagementState = "service.opendatahub.io/management-state"

	// AnnotationCheckTargetVersion is the target version for upgrade checks.
	AnnotationCheckTargetVersion = "check.opendatahub.io/target-version"

	// AnnotationOperatorInstalledVersion is the installed operator version.
	AnnotationOperatorInstalledVersion = "operator.opendatahub.io/installed-version"

	// AnnotationImpactedWorkloadCount is the count of impacted workloads.
	AnnotationImpactedWorkloadCount = "workload.opendatahub.io/impacted-count"
)
