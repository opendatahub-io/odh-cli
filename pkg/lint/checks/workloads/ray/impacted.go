package ray

import (
	"context"
	"fmt"
	"slices"

	"github.com/opendatahub-io/odh-cli/pkg/constants"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/validate"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
	"github.com/opendatahub-io/odh-cli/pkg/util/components"
	"github.com/opendatahub-io/odh-cli/pkg/util/version"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	kind                    = "ray"
	finalizerCodeFlareOAuth = "ray.openshift.ai/oauth-finalizer"
)

const (
	ConditionTypeCodeFlareRayClusterCompatible = "CodeFlareRayClustersCompatible"
)

// ImpactedWorkloadsCheck lists RayClusters managed by CodeFlare.
type ImpactedWorkloadsCheck struct {
	check.BaseCheck
}

func NewImpactedWorkloadsCheck() *ImpactedWorkloadsCheck {
	return &ImpactedWorkloadsCheck{
		BaseCheck: check.BaseCheck{
			CheckGroup:       check.GroupWorkload,
			Kind:             kind,
			Type:             check.CheckTypeImpactedWorkloads,
			CheckID:          "workloads.ray.impacted-workloads",
			CheckName:        "Workloads :: Ray :: Impacted Workloads (3.x)",
			CheckDescription: "Lists RayClusters managed by CodeFlare that will be impacted in RHOAI 3.x (CodeFlare not available)",
			CheckRemediation: "Delete or back up CodeFlare-managed RayClusters before upgrading, as CodeFlare will not be available in RHOAI 3.x",
		},
	}
}

// CanApply returns whether this check should run for the given target.
// Only applies when upgrading FROM 2.x TO 3.x and Ray is Managed.
func (c *ImpactedWorkloadsCheck) CanApply(ctx context.Context, target check.Target) (bool, error) {
	if !version.IsUpgradeFrom2xTo3x(target.CurrentVersion, target.TargetVersion) {
		return false, nil
	}

	dsc, err := client.GetDataScienceCluster(ctx, target.Client)
	if err != nil {
		return false, fmt.Errorf("getting DataScienceCluster: %w", err)
	}

	return components.HasManagementState(dsc, kind, constants.ManagementStateManaged), nil
}

// Validate executes the check against the provided target.
func (c *ImpactedWorkloadsCheck) Validate(
	ctx context.Context,
	target check.Target,
) (*result.DiagnosticResult, error) {
	return validate.WorkloadsMetadata(c, target, resources.RayCluster).
		Filter(func(cluster *metav1.PartialObjectMetadata) (bool, error) {
			return slices.Contains(cluster.GetFinalizers(), finalizerCodeFlareOAuth), nil
		}).
		Complete(ctx, c.newCodeFlareRayClusterCondition)
}
