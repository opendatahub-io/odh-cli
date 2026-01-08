package kserve

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/blang/semver/v4"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/lburgazzoli/odh-cli/pkg/lint/check"
	"github.com/lburgazzoli/odh-cli/pkg/lint/check/result"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/shared/base"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/shared/results"
	"github.com/lburgazzoli/odh-cli/pkg/resources"
)

const (
	annotationDeploymentMode = "serving.kserve.io/deploymentMode"
	deploymentModeModelMesh  = "ModelMesh"
	deploymentModeServerless = "Serverless"
)

type impactedResource struct {
	namespace      string
	name           string
	deploymentMode string
}

// ImpactedWorkloadsCheck lists InferenceServices and ServingRuntimes using deprecated deployment modes.
type ImpactedWorkloadsCheck struct {
	base.BaseCheck
}

func NewImpactedWorkloadsCheck() *ImpactedWorkloadsCheck {
	return &ImpactedWorkloadsCheck{
		BaseCheck: base.BaseCheck{
			CheckGroup:       check.GroupWorkload,
			Kind:             check.ComponentKServe,
			CheckType:        check.CheckTypeImpactedWorkloads,
			CheckID:          "workloads.kserve.impacted-workloads",
			CheckName:        "Workloads :: KServe :: Impacted Workloads (3.x)",
			CheckDescription: "Lists InferenceServices and ServingRuntimes using deprecated deployment modes (ModelMesh, Serverless) that will be impacted in RHOAI 3.x",
		},
	}
}

// CanApply returns whether this check should run for the given versions.
// This check only applies when upgrading FROM 2.x TO 3.x.
func (c *ImpactedWorkloadsCheck) CanApply(
	currentVersion *semver.Version,
	targetVersion *semver.Version,
) bool {
	if currentVersion == nil || targetVersion == nil {
		return false
	}

	return currentVersion.Major == 2 && targetVersion.Major >= 3
}

// Validate executes the check against the provided target.
func (c *ImpactedWorkloadsCheck) Validate(
	ctx context.Context,
	target *check.CheckTarget,
) (*result.DiagnosticResult, error) {
	dr := c.NewResult()

	if target.Version != nil {
		dr.Annotations[check.AnnotationCheckTargetVersion] = target.Version.Version
	}

	// Find impacted InferenceServices
	impactedISVCs, err := c.findImpactedInferenceServices(ctx, target)
	if err != nil {
		return nil, err
	}

	// Find impacted ServingRuntimes
	impactedSRs, err := c.findImpactedServingRuntimes(ctx, target)
	if err != nil {
		return nil, err
	}

	totalImpacted := len(impactedISVCs) + len(impactedSRs)
	dr.Annotations[check.AnnotationImpactedWorkloadCount] = strconv.Itoa(totalImpacted)

	if totalImpacted == 0 {
		results.SetCompatibilitySuccessf(dr, "No InferenceServices or ServingRuntimes using deprecated deployment modes found - ready for RHOAI 3.x upgrade")

		return dr, nil
	}

	// Populate ImpactedObjects with PartialObjectMetadata
	c.populateImpactedObjects(dr, impactedISVCs, impactedSRs)

	message := c.buildImpactMessage(impactedISVCs, impactedSRs)
	results.SetCompatibilityFailuref(dr, "%s", message)

	return dr, nil
}

func (c *ImpactedWorkloadsCheck) findImpactedInferenceServices(
	ctx context.Context,
	target *check.CheckTarget,
) ([]impactedResource, error) {
	inferenceServices, err := target.Client.ListMetadata(ctx, resources.InferenceService)
	if err != nil {
		return nil, fmt.Errorf("listing InferenceServices: %w", err)
	}

	var impacted []impactedResource

	for _, isvc := range inferenceServices {
		annotations := isvc.GetAnnotations()

		mode := annotations[annotationDeploymentMode]
		if mode == deploymentModeModelMesh || mode == deploymentModeServerless {
			impacted = append(impacted, impactedResource{
				namespace:      isvc.GetNamespace(),
				name:           isvc.GetName(),
				deploymentMode: mode,
			})
		}
	}

	return impacted, nil
}

func (c *ImpactedWorkloadsCheck) findImpactedServingRuntimes(
	ctx context.Context,
	target *check.CheckTarget,
) ([]impactedResource, error) {
	servingRuntimes, err := target.Client.ListMetadata(ctx, resources.ServingRuntime)
	if err != nil {
		return nil, fmt.Errorf("listing ServingRuntimes: %w", err)
	}

	var impacted []impactedResource

	for _, sr := range servingRuntimes {
		annotations := sr.GetAnnotations()

		mode := annotations[annotationDeploymentMode]
		// Only check for ModelMesh on ServingRuntimes (not Serverless)
		if mode == deploymentModeModelMesh {
			impacted = append(impacted, impactedResource{
				namespace:      sr.GetNamespace(),
				name:           sr.GetName(),
				deploymentMode: mode,
			})
		}
	}

	return impacted, nil
}

func (c *ImpactedWorkloadsCheck) populateImpactedObjects(
	dr *result.DiagnosticResult,
	impactedISVCs []impactedResource,
	impactedSRs []impactedResource,
) {
	totalCount := len(impactedISVCs) + len(impactedSRs)
	dr.ImpactedObjects = make([]metav1.PartialObjectMetadata, 0, totalCount)

	// Add InferenceServices
	for _, r := range impactedISVCs {
		obj := metav1.PartialObjectMetadata{
			TypeMeta: metav1.TypeMeta{
				APIVersion: resources.InferenceService.APIVersion(),
				Kind:       resources.InferenceService.Kind,
			},
			ObjectMeta: metav1.ObjectMeta{
				Namespace: r.namespace,
				Name:      r.name,
				Annotations: map[string]string{
					annotationDeploymentMode: r.deploymentMode,
				},
			},
		}
		dr.ImpactedObjects = append(dr.ImpactedObjects, obj)
	}

	// Add ServingRuntimes
	for _, r := range impactedSRs {
		obj := metav1.PartialObjectMetadata{
			TypeMeta: metav1.TypeMeta{
				APIVersion: resources.ServingRuntime.APIVersion(),
				Kind:       resources.ServingRuntime.Kind,
			},
			ObjectMeta: metav1.ObjectMeta{
				Namespace: r.namespace,
				Name:      r.name,
				Annotations: map[string]string{
					annotationDeploymentMode: r.deploymentMode,
				},
			},
		}
		dr.ImpactedObjects = append(dr.ImpactedObjects, obj)
	}
}

func (c *ImpactedWorkloadsCheck) buildImpactMessage(
	impactedISVCs []impactedResource,
	impactedSRs []impactedResource,
) string {
	totalCount := len(impactedISVCs) + len(impactedSRs)

	// Count deployment modes for InferenceServices
	serverlessCount := 0
	modelMeshCount := 0
	for _, r := range impactedISVCs {
		switch r.deploymentMode {
		case deploymentModeServerless:
			serverlessCount++
		case deploymentModeModelMesh:
			modelMeshCount++
		}
	}

	// Add ModelMesh ServingRuntimes to ModelMesh count
	modelMeshCount += len(impactedSRs)

	// Build summary message with counts
	msg := fmt.Sprintf("Found %d deprecated KServe workload(s)", totalCount)

	// Add breakdown by deployment mode
	if len(impactedISVCs) > 0 && len(impactedSRs) > 0 {
		msg += fmt.Sprintf(" (%d InferenceService(s), %d ServingRuntime(s))", len(impactedISVCs), len(impactedSRs))
	}

	if serverlessCount > 0 || modelMeshCount > 0 {
		var modeParts []string
		if serverlessCount > 0 {
			modeParts = append(modeParts, fmt.Sprintf("%d Serverless", serverlessCount))
		}
		if modelMeshCount > 0 {
			modeParts = append(modeParts, fmt.Sprintf("%d ModelMesh", modelMeshCount))
		}
		msg += " [" + strings.Join(modeParts, ", ") + "]"
	}

	return msg
}

// Register the check in the global registry.
//
//nolint:gochecknoinits // Required for auto-registration pattern
func init() {
	check.MustRegisterCheck(NewImpactedWorkloadsCheck())
}
