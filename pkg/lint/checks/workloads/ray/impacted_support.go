package ray

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/validate"
	"github.com/opendatahub-io/odh-cli/pkg/util/version"
)

// newWorkloadCompatibilityCondition creates a compatibility condition based on workload count.
// When count > 0, returns a failure condition indicating impacted workloads.
// When count == 0, returns a success condition indicating readiness for upgrade.
func (c *ImpactedWorkloadsCheck) newWorkloadCompatibilityCondition(
	conditionType string,
	count int,
	workloadDescription string,
	targetVersionLabel string,
) result.Condition {
	if count > 0 {
		return check.NewCondition(
			conditionType,
			metav1.ConditionFalse,
			check.WithReason(check.ReasonVersionIncompatible),
			check.WithMessage("Found %d %s - will be impacted in RHOAI %s (CodeFlare not available)", count, workloadDescription, targetVersionLabel),
			check.WithImpact(result.ImpactAdvisory),
			check.WithRemediation(c.CheckRemediation),
		)
	}

	return check.NewCondition(
		conditionType,
		metav1.ConditionTrue,
		check.WithReason(check.ReasonVersionCompatible),
		check.WithMessage("No %s found - ready for RHOAI %s upgrade", workloadDescription, targetVersionLabel),
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
		version.MajorMinorLabel(req.TargetVersion),
	)}, nil
}
