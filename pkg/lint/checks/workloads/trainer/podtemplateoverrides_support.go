package trainer

import (
	"context"
	"errors"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/validate"
	"github.com/opendatahub-io/odh-cli/pkg/util/jq"
)

// hasPodTemplateOverrides returns true when TrainJob.spec.podTemplateOverrides is present
// and non-empty.
func hasPodTemplateOverrides(obj *unstructured.Unstructured) (bool, error) {
	val, err := jq.Query[[]any](obj, ".spec.podTemplateOverrides")

	switch {
	case errors.Is(err, jq.ErrNotFound):
		return false, nil
	case err != nil:
		return false, err
	default:
		return len(val) > 0, nil
	}
}

func (c *PodTemplateOverridesCheck) newPodTemplateOverridesCondition(
	_ context.Context,
	req *validate.WorkloadRequest[*unstructured.Unstructured],
) ([]result.Condition, error) {
	count := len(req.Items)

	if count == 0 {
		return []result.Condition{check.NewCondition(
			ConditionTypePodTemplateOverridesCompatible,
			metav1.ConditionTrue,
			check.WithReason(check.ReasonVersionCompatible),
			check.WithMessage("No TrainJob(s) with podTemplateOverrides found - ready for OpenShift AI 3.6 Trainer upgrade"),
		)}, nil
	}

	return []result.Condition{check.NewCondition(
		ConditionTypePodTemplateOverridesCompatible,
		metav1.ConditionFalse,
		check.WithReason(check.ReasonWorkloadsImpacted),
		check.WithMessage(
			"Found %d TrainJob(s) that use podTemplateOverrides. In OpenShift AI 3.6, Trainer v2.3 removes "+
				"podTemplateOverrides and replaces it with runtimePatches. Pause and resume of these "+
				"pre-upgrade TrainJobs may fail after upgrade, including when managed by Kueue. "+
				"We recommend waiting for these jobs to complete before upgrading when possible. "+
				"After upgrade, recreate TrainJobs using runtimePatches. See %s",
			count,
			kbArticleURL,
		),
		check.WithImpact(result.ImpactAdvisory),
		check.WithRemediation(c.CheckRemediation),
	)}, nil
}
