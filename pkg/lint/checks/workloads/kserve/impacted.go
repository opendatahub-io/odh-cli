package kserve

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strconv"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/opendatahub-io/odh-cli/pkg/constants"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/printer/table"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
	"github.com/opendatahub-io/odh-cli/pkg/util/components"
	"github.com/opendatahub-io/odh-cli/pkg/util/jq"
	"github.com/opendatahub-io/odh-cli/pkg/util/kube"
	"github.com/opendatahub-io/odh-cli/pkg/util/version"
)

const (
	annotationDeploymentMode = "serving.kserve.io/deploymentMode"
	deploymentModeModelMesh  = "ModelMesh"
	deploymentModeServerless = "Serverless"
)

const (
	ConditionTypeServerlessISVCCompatible        = "ServerlessInferenceServicesCompatible"
	ConditionTypeModelMeshISVCCompatible         = "ModelMeshInferenceServicesCompatible"
	ConditionTypeModelMeshSRCompatible           = "ModelMeshServingRuntimesCompatible"
	ConditionTypeRemovedSRCompatible             = "RemovedServingRuntimesCompatible"
	ConditionTypeAcceleratorOnlySRCompatible     = "AcceleratorOnlyServingRuntimesCompatible"
	ConditionTypeAcceleratorAndHWProfileSRCompat = "AcceleratorAndHWProfileServingRuntimesCompatible"
	ConditionTypeAcceleratorSRISVCCompatible     = "AcceleratorServingRuntimeISVCsCompatible"
)

const (
	annotationHardwareProfileName = "opendatahub.io/hardware-profile-name"
)

const (
	runtimeOVMS             = "ovms"
	runtimeCaikitStandalone = "caikit-standalone-serving-template"
	runtimeCaikitTGIS       = "caikit-tgis-serving-template"
)

// ImpactedWorkloadsCheck lists InferenceServices and ServingRuntimes using deprecated deployment modes.
type ImpactedWorkloadsCheck struct {
	check.BaseCheck

	// deploymentModeFilter filters InferenceServices by deployment mode in verbose output.
	// Valid values: "all" (default), "serverless", "modelmesh".
	deploymentModeFilter string
}

func NewImpactedWorkloadsCheck() *ImpactedWorkloadsCheck {
	return &ImpactedWorkloadsCheck{
		BaseCheck: check.BaseCheck{
			CheckGroup:       check.GroupWorkload,
			Kind:             constants.ComponentKServe,
			Type:             check.CheckTypeImpactedWorkloads,
			CheckID:          "workloads.kserve.impacted-workloads",
			CheckName:        "Workloads :: KServe :: Impacted Workloads (3.x)",
			CheckDescription: "Lists InferenceServices and ServingRuntimes using deprecated deployment modes (ModelMesh, Serverless), removed ServingRuntimes, or ServingRuntimes referencing deprecated AcceleratorProfiles that will be impacted in RHOAI 3.x",
			CheckRemediation: "Migrate InferenceServices from Serverless/ModelMesh to RawDeployment mode, update ServingRuntimes to supported versions, and review AcceleratorProfile references before upgrading",
		},
		deploymentModeFilter: "all", // Default to showing all deployment modes
	}
}

// SetDeploymentModeFilter sets the filter for InferenceService display by deployment mode.
// Valid values: "all", "serverless", "modelmesh".
func (c *ImpactedWorkloadsCheck) SetDeploymentModeFilter(filter string) {
	c.deploymentModeFilter = filter
}

// CanApply returns whether this check should run for the given target.
// Only applies when upgrading FROM 2.x TO 3.x and KServe or ModelMesh is Managed.
func (c *ImpactedWorkloadsCheck) CanApply(ctx context.Context, target check.Target) (bool, error) {
	if !version.IsUpgradeFrom2xTo3x(target.CurrentVersion, target.TargetVersion) {
		return false, nil
	}

	dsc, err := client.GetDataScienceCluster(ctx, target.Client)
	if err != nil {
		return false, fmt.Errorf("getting DataScienceCluster: %w", err)
	}

	return components.HasManagementState(dsc, constants.ComponentKServe, constants.ManagementStateManaged) ||
		components.HasManagementState(dsc, "modelmeshserving", constants.ManagementStateManaged), nil
}

// Validate executes the check against the provided target.
func (c *ImpactedWorkloadsCheck) Validate(
	ctx context.Context,
	target check.Target,
) (*result.DiagnosticResult, error) {
	dr := c.NewResult()

	if target.TargetVersion != nil {
		dr.Annotations[check.AnnotationCheckTargetVersion] = target.TargetVersion.String()
	}

	// Fetch InferenceServices with impacted deployment modes (Serverless or ModelMesh)
	allISVCs, err := client.List[*metav1.PartialObjectMetadata](
		ctx, target.Client, resources.InferenceService, isImpactedISVC,
	)
	if err != nil {
		return nil, err
	}

	// Fetch ServingRuntimes with multi-model enabled
	impactedSRs, err := client.List[*unstructured.Unstructured](
		ctx, target.Client, resources.ServingRuntime, jq.Predicate(".spec.multiModel == true"),
	)
	if err != nil {
		return nil, err
	}

	// Fetch InferenceServices referencing removed ServingRuntimes
	removedRuntimeISVCs, err := client.List[*unstructured.Unstructured](
		ctx, target.Client, resources.InferenceService, isUsingRemovedRuntime,
	)
	if err != nil {
		return nil, err
	}

	// Fetch ServingRuntimes with accelerator profile annotation
	acceleratorSRs, err := client.List[*metav1.PartialObjectMetadata](
		ctx, target.Client, resources.ServingRuntime, hasAcceleratorAnnotation,
	)
	if err != nil {
		return nil, err
	}

	// Split accelerator SRs into accelerator-only vs both annotations
	var acceleratorOnlySRs, acceleratorAndHWProfileSRs []*metav1.PartialObjectMetadata

	for _, sr := range acceleratorSRs {
		if kube.GetAnnotation(sr, annotationHardwareProfileName) != "" {
			acceleratorAndHWProfileSRs = append(acceleratorAndHWProfileSRs, sr)
		} else {
			acceleratorOnlySRs = append(acceleratorOnlySRs, sr)
		}
	}

	// Fetch InferenceServices referencing accelerator-linked ServingRuntimes
	allISVCsFull, err := client.List[*unstructured.Unstructured](
		ctx, target.Client, resources.InferenceService, nil,
	)
	if err != nil {
		return nil, err
	}

	tv := version.MajorMinorLabel(target.TargetVersion)

	// Each function appends its condition and impacted objects to the result
	c.appendServerlessISVCCondition(dr, allISVCs, tv)
	c.appendModelMeshISVCCondition(dr, allISVCs, tv)
	c.appendModelMeshSRCondition(dr, impactedSRs, tv)

	if err := c.appendRemovedRuntimeISVCCondition(dr, removedRuntimeISVCs, tv); err != nil {
		return nil, err
	}

	c.appendAcceleratorOnlySRCondition(dr, acceleratorOnlySRs)
	c.appendAcceleratorAndHWProfileSRCondition(dr, acceleratorAndHWProfileSRs)

	if err := c.appendAcceleratorSRISVCCondition(dr, acceleratorSRs, allISVCsFull); err != nil {
		return nil, err
	}

	dr.Annotations[check.AnnotationImpactedWorkloadCount] = strconv.Itoa(len(dr.ImpactedObjects))

	return dr, nil
}

// inferenceServiceRow represents a row in the InferenceService detail table.
type inferenceServiceRow struct {
	Name           string `mapstructure:"NAME"`
	Namespace      string `mapstructure:"NAMESPACE"`
	DeploymentMode string `mapstructure:"DEPLOYMENT MODE"`
}

// FormatVerboseOutput provides custom formatting for InferenceServices in verbose mode.
// Displays a detailed table showing Name, Namespace, and DeploymentMode for each InferenceService.
// Filters InferenceServices based on the deploymentModeFilter setting.
func (c *ImpactedWorkloadsCheck) FormatVerboseOutput(out io.Writer, dr *result.DiagnosticResult) {
	// Collect InferenceServices from impacted objects
	var isvcs []inferenceServiceRow

	for _, obj := range dr.ImpactedObjects {
		if obj.Kind != "InferenceService" {
			continue
		}

		deploymentMode := obj.Annotations[annotationDeploymentMode]
		if deploymentMode == "" {
			// Check for runtime annotation (for removed runtime ISVCs)
			if runtime := obj.Annotations["serving.kserve.io/runtime"]; runtime != "" {
				deploymentMode = "RawDeployment"
			} else {
				deploymentMode = "Unknown"
			}
		}

		// Apply deployment mode filter
		if c.deploymentModeFilter != "all" {
			filterMode := ""
			switch c.deploymentModeFilter {
			case "serverless":
				filterMode = deploymentModeServerless
			case "modelmesh":
				filterMode = deploymentModeModelMesh
			}

			if deploymentMode != filterMode {
				continue
			}
		}

		isvcs = append(isvcs, inferenceServiceRow{
			Name:           obj.Name,
			Namespace:      obj.Namespace,
			DeploymentMode: deploymentMode,
		})
	}

	if len(isvcs) == 0 {
		return
	}

	// Sort by namespace, then by name
	sort.Slice(isvcs, func(i, j int) bool {
		if isvcs[i].Namespace != isvcs[j].Namespace {
			return isvcs[i].Namespace < isvcs[j].Namespace
		}

		return isvcs[i].Name < isvcs[j].Name
	})

	// Render table with InferenceService details
	renderer := table.NewRenderer[inferenceServiceRow](
		table.WithWriter[inferenceServiceRow](out),
		table.WithHeaders[inferenceServiceRow]("NAME", "NAMESPACE", "DEPLOYMENT MODE"),
		table.WithTableOptions[inferenceServiceRow](table.DefaultTableOptions...),
	)

	for _, isvc := range isvcs {
		if err := renderer.Append(isvc); err != nil {
			_, _ = fmt.Fprintf(out, "    Error rendering InferenceService: %v\n", err)

			return
		}
	}

	if err := renderer.Render(); err != nil {
		_, _ = fmt.Fprintf(out, "    Error rendering table: %v\n", err)
	}
}
