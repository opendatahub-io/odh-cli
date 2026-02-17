package kserve

import (
	"context"
	"fmt"
	"strconv"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/opendatahub-io/odh-cli/pkg/constants"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
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
}

func NewImpactedWorkloadsCheck() *ImpactedWorkloadsCheck {
	return &ImpactedWorkloadsCheck{
		BaseCheck: check.BaseCheck{
			CheckGroup:       check.GroupWorkload,
			Kind:             constants.ComponentKServe,
			Type:             check.CheckTypeImpactedWorkloads,
			CheckID:          "workloads.kserve.impacted-workloads",
			CheckName:        "Workloads :: KServe :: Impacted Workloads (3.x)",
			CheckDescription: "Lists InferenceServices and ServingRuntimes using deprecated deployment modes (ModelMesh, Serverless), removed ServingRuntimes, or ServingRuntimes referencing legacy AcceleratorProfiles that will be impacted in RHOAI 3.x",
			CheckRemediation: "Migrate InferenceServices from Serverless/ModelMesh to RawDeployment mode, update ServingRuntimes to supported versions, and review AcceleratorProfile references before upgrading",
		},
	}
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

	// Each function appends its condition and impacted objects to the result
	c.appendServerlessISVCCondition(dr, allISVCs)
	c.appendModelMeshISVCCondition(dr, allISVCs)
	c.appendModelMeshSRCondition(dr, impactedSRs)

	if err := c.appendRemovedRuntimeISVCCondition(dr, removedRuntimeISVCs); err != nil {
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
