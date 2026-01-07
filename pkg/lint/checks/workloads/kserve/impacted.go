package kserve

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/blang/semver/v4"

	"github.com/lburgazzoli/odh-cli/pkg/lint/check"
	"github.com/lburgazzoli/odh-cli/pkg/lint/check/result"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/shared/results"
	"github.com/lburgazzoli/odh-cli/pkg/resources"
)

const (
	checkID          = "workloads.kserve.impacted-workloads"
	checkName        = "Workloads :: KServe :: Impacted Workloads (3.x)"
	checkDescription = "Lists InferenceServices and ServingRuntimes using deprecated deployment modes (ModelMesh, Serverless) that will be impacted in RHOAI 3.x"

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
type ImpactedWorkloadsCheck struct{}

// ID returns the unique identifier for this check.
func (c *ImpactedWorkloadsCheck) ID() string {
	return checkID
}

// Name returns the human-readable check name.
func (c *ImpactedWorkloadsCheck) Name() string {
	return checkName
}

// Description returns what this check validates.
func (c *ImpactedWorkloadsCheck) Description() string {
	return checkDescription
}

// Group returns the check group.
func (c *ImpactedWorkloadsCheck) Group() check.CheckGroup {
	return check.GroupWorkload
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
	dr := result.New(
		string(check.GroupWorkload),
		check.ComponentKServe,
		check.CheckTypeImpactedWorkloads,
		checkDescription,
	)

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

	message := c.buildImpactMessage(impactedISVCs, impactedSRs)
	results.SetCompatibilityFailuref(dr, "%s", message)

	return dr, nil
}

func (c *ImpactedWorkloadsCheck) findImpactedInferenceServices(
	ctx context.Context,
	target *check.CheckTarget,
) ([]impactedResource, error) {
	inferenceServices, err := target.Client.List(ctx, resources.InferenceService)
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
	servingRuntimes, err := target.Client.List(ctx, resources.ServingRuntime)
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

func (c *ImpactedWorkloadsCheck) buildImpactMessage(
	impactedISVCs []impactedResource,
	impactedSRs []impactedResource,
) string {
	var parts []string

	if len(impactedISVCs) > 0 {
		resourceStrs := make([]string, len(impactedISVCs))
		for i, r := range impactedISVCs {
			resourceStrs[i] = fmt.Sprintf("%s/%s (%s)", r.namespace, r.name, r.deploymentMode)
		}
		parts = append(parts, fmt.Sprintf(
			"%d InferenceService(s): %s",
			len(impactedISVCs),
			strings.Join(resourceStrs, ", "),
		))
	}

	if len(impactedSRs) > 0 {
		resourceStrs := make([]string, len(impactedSRs))
		for i, r := range impactedSRs {
			resourceStrs[i] = fmt.Sprintf("%s/%s (%s)", r.namespace, r.name, r.deploymentMode)
		}
		parts = append(parts, fmt.Sprintf(
			"%d ServingRuntime(s): %s",
			len(impactedSRs),
			strings.Join(resourceStrs, ", "),
		))
	}

	return "Found deprecated KServe workloads that will be impacted: " + strings.Join(parts, "; ")
}

// Register the check in the global registry.
//
//nolint:gochecknoinits // Required for auto-registration pattern
func init() {
	check.MustRegisterCheck(&ImpactedWorkloadsCheck{})
}
