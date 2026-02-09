package ray

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/lburgazzoli/odh-cli/pkg/lint/check"
	"github.com/lburgazzoli/odh-cli/pkg/lint/check/result"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/shared/validate"
)

// newWorkloadCompatibilityCondition creates a compatibility condition based on workload count.
// When count > 0, returns a failure condition indicating impacted workloads.
// When count == 0, returns a success condition indicating readiness for upgrade.
func (c *ImpactedWorkloadsCheck) newWorkloadCompatibilityCondition(
	conditionType string,
	count int,
	workloadDescription string,
) result.Condition {
	if count > 0 {
		return check.NewCondition(
			conditionType,
			metav1.ConditionFalse,
			check.ReasonVersionIncompatible,
			"Found %d %s - will be impacted in RHOAI 3.x (CodeFlare not available)",
			count,
			workloadDescription,
			check.WithImpact(result.ImpactAdvisory),
		)
	}

	return check.NewCondition(
		conditionType,
		metav1.ConditionTrue,
		check.ReasonVersionCompatible,
		"No %s found - ready for RHOAI 3.x upgrade",
		workloadDescription,
	)
}

func (c *ImpactedWorkloadsCheck) newCodeFlareRayClusterCondition(
	_ context.Context,
	req *validate.WorkloadRequest[*metav1.PartialObjectMetadata],
) ([]result.Condition, error) {
	return []result.Condition{c.newWorkloadCompatibilityCondition(
		ConditionTypeCodeFlareRayClusterCompatible,
		len(req.Items),
		"CodeFlare-managed RayCluster(s)",
	)}, nil
}
