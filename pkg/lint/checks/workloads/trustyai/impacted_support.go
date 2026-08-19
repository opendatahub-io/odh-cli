package trustyai

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/validate"
)

func (c *ImpactedWorkloadsCheck) newImpactedCondition(
	_ context.Context,
	req *validate.WorkloadRequest[*unstructured.Unstructured],
) ([]result.Condition, error) {
	count := len(req.Items)

	if count == 0 {
		return []result.Condition{check.NewCondition(
			check.ConditionTypeCompatible,
			metav1.ConditionTrue,
			check.WithReason(check.ReasonResourceNotFound),
			check.WithMessage("No TrustyAIService instances found - no data backup required"),
		)}, nil
	}

	return []result.Condition{check.NewCondition(
		check.ConditionTypeCompatible,
		metav1.ConditionFalse,
		check.WithReason(check.ReasonWorkloadsImpacted),
		check.WithMessage("Found %d TrustyAIService instance(s) - back up TrustyAI data before upgrading to prevent data loss", count),
		check.WithImpact(result.ImpactAdvisory),
		check.WithRemediation(c.CheckRemediation),
	)}, nil
}
