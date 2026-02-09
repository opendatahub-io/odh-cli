package kserve

import (
	"context"
	"strconv"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/lburgazzoli/odh-cli/pkg/lint/check"
	"github.com/lburgazzoli/odh-cli/pkg/lint/check/result"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/shared/base"
	"github.com/lburgazzoli/odh-cli/pkg/resources"
	"github.com/lburgazzoli/odh-cli/pkg/util/client"
	"github.com/lburgazzoli/odh-cli/pkg/util/jq"
	"github.com/lburgazzoli/odh-cli/pkg/util/version"
)

const (
	annotationDeploymentMode = "serving.kserve.io/deploymentMode"
	deploymentModeModelMesh  = "ModelMesh"
	deploymentModeServerless = "Serverless"
)

const (
	ConditionTypeServerlessISVCCompatible = "ServerlessInferenceServicesCompatible"
	ConditionTypeModelMeshISVCCompatible  = "ModelMeshInferenceServicesCompatible"
	ConditionTypeModelMeshSRCompatible    = "ModelMeshServingRuntimesCompatible"
	ConditionTypeRemovedSRCompatible      = "RemovedServingRuntimesCompatible"
)

const (
	runtimeOVMS             = "ovms"
	runtimeCaikitStandalone = "caikit-standalone-serving-template"
	runtimeCaikitTGIS       = "caikit-tgis-serving-template"
)

// ImpactedWorkloadsCheck lists InferenceServices and ServingRuntimes using deprecated deployment modes.
type ImpactedWorkloadsCheck struct {
	base.BaseCheck
}

func NewImpactedWorkloadsCheck() *ImpactedWorkloadsCheck {
	return &ImpactedWorkloadsCheck{
		BaseCheck: base.BaseCheck{
			CheckGroup:       check.GroupWorkload,
			Kind:             check.ComponentKServe,
			Type:             check.CheckTypeImpactedWorkloads,
			CheckID:          "workloads.kserve.impacted-workloads",
			CheckName:        "Workloads :: KServe :: Impacted Workloads (3.x)",
			CheckDescription: "Lists InferenceServices and ServingRuntimes using deprecated deployment modes (ModelMesh, Serverless) or removed ServingRuntimes that will be impacted in RHOAI 3.x",
			CheckRemediation: "Migrate InferenceServices from Serverless/ModelMesh to RawDeployment mode, and update ServingRuntimes to supported versions before upgrading",
		},
	}
}

// CanApply returns whether this check should run for the given target.
// Only applies when upgrading FROM 2.x TO 3.x since KServe workloads may be impacted.
func (c *ImpactedWorkloadsCheck) CanApply(_ context.Context, target check.Target) (bool, error) {
	return version.IsUpgradeFrom2xTo3x(target.CurrentVersion, target.TargetVersion), nil
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

	// Each function appends its condition and impacted objects to the result
	c.appendServerlessISVCCondition(dr, allISVCs)
	c.appendModelMeshISVCCondition(dr, allISVCs)
	c.appendModelMeshSRCondition(dr, impactedSRs)

	if err := c.appendRemovedRuntimeISVCCondition(dr, removedRuntimeISVCs); err != nil {
		return nil, err
	}

	dr.Annotations[check.AnnotationImpactedWorkloadCount] = strconv.Itoa(len(dr.ImpactedObjects))

	return dr, nil
}
