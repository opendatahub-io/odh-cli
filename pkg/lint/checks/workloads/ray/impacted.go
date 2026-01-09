package ray

import (
	"context"
	"fmt"
	"slices"
	"strconv"

	"github.com/blang/semver/v4"

	"k8s.io/apimachinery/pkg/types"

	"github.com/lburgazzoli/odh-cli/pkg/lint/check"
	"github.com/lburgazzoli/odh-cli/pkg/lint/check/result"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/shared/base"
	"github.com/lburgazzoli/odh-cli/pkg/resources"
)

const (
	finalizerCodeFlareOAuth = "ray.openshift.ai/oauth-finalizer"
)

const (
	ConditionTypeCodeFlareRayClusterCompatible = "CodeFlareRayClustersCompatible"
)

// ImpactedWorkloadsCheck lists RayClusters managed by CodeFlare.
type ImpactedWorkloadsCheck struct {
	base.BaseCheck
}

func NewImpactedWorkloadsCheck() *ImpactedWorkloadsCheck {
	return &ImpactedWorkloadsCheck{
		BaseCheck: base.BaseCheck{
			CheckGroup:       check.GroupWorkload,
			Kind:             check.ComponentRay,
			CheckType:        check.CheckTypeImpactedWorkloads,
			CheckID:          "workloads.ray.impacted-workloads",
			CheckName:        "Workloads :: Ray :: Impacted Workloads (3.x)",
			CheckDescription: "Lists RayClusters managed by CodeFlare that will be impacted in RHOAI 3.x (CodeFlare not available)",
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

	// Find impacted RayClusters
	impactedClusters, err := c.findImpactedRayClusters(ctx, target)
	if err != nil {
		return nil, err
	}

	totalImpacted := len(impactedClusters)
	dr.Annotations[check.AnnotationImpactedWorkloadCount] = strconv.Itoa(totalImpacted)

	// Add condition for CodeFlare RayClusters
	dr.Status.Conditions = append(dr.Status.Conditions,
		newCodeFlareRayClusterCondition(totalImpacted),
	)

	// Populate ImpactedObjects if any workloads found
	if totalImpacted > 0 {
		populateImpactedObjects(dr, impactedClusters)
	}

	return dr, nil
}

func (c *ImpactedWorkloadsCheck) findImpactedRayClusters(
	ctx context.Context,
	target *check.CheckTarget,
) ([]types.NamespacedName, error) {
	rayClusters, err := target.Client.ListMetadata(ctx, resources.RayCluster)
	if err != nil {
		return nil, fmt.Errorf("listing RayClusters: %w", err)
	}

	var impacted []types.NamespacedName

	for _, cluster := range rayClusters {
		finalizers := cluster.GetFinalizers()

		if slices.Contains(finalizers, finalizerCodeFlareOAuth) {
			impacted = append(impacted, types.NamespacedName{
				Namespace: cluster.GetNamespace(),
				Name:      cluster.GetName(),
			})
		}
	}

	return impacted, nil
}

// Register the check in the global registry.
//
//nolint:gochecknoinits // Required for auto-registration pattern
func init() {
	check.MustRegisterCheck(NewImpactedWorkloadsCheck())
}
