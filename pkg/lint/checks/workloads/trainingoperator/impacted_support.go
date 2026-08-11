package trainingoperator

import (
	"errors"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/util/jq"
)

func (c *ImpactedWorkloadsCheck) newPyTorchJobCondition(
	activeCount int,
	completedCount int,
	isRemoval bool,
) result.Condition {
	totalCount := activeCount + completedCount

	impact := result.ImpactAdvisory
	verb := "deprecated"
	if isRemoval {
		impact = result.ImpactBlocking
		verb = "removed in 3.6"
	}

	if totalCount == 0 {
		return check.NewCondition(
			ConditionTypePyTorchJobsCompatible,
			metav1.ConditionTrue,
			check.WithReason(check.ReasonVersionCompatible),
			check.WithMessage("No PyTorchJob(s) found - no workloads impacted by TrainingOperator removal"),
		)
	}

	if activeCount > 0 && completedCount > 0 {
		return check.NewCondition(
			ConditionTypePyTorchJobsCompatible,
			metav1.ConditionFalse,
			check.WithReason(check.ReasonWorkloadsImpacted),
			check.WithMessage("Found %d PyTorchJob(s) (%d active, %d completed) - TrainingOperator (Kubeflow v1) is %s, migrate to Trainer v2", totalCount, activeCount, completedCount, verb),
			check.WithImpact(impact),
			check.WithRemediation(c.CheckRemediation),
		)
	}

	if activeCount > 0 {
		return check.NewCondition(
			ConditionTypePyTorchJobsCompatible,
			metav1.ConditionFalse,
			check.WithReason(check.ReasonWorkloadsImpacted),
			check.WithMessage("Found %d active PyTorchJob(s) - TrainingOperator (Kubeflow v1) is %s, drain and migrate to Trainer v2", activeCount, verb),
			check.WithImpact(impact),
			check.WithRemediation(c.CheckRemediation),
		)
	}

	return check.NewCondition(
		ConditionTypePyTorchJobsCompatible,
		metav1.ConditionTrue,
		check.WithReason(check.ReasonVersionCompatible),
		check.WithMessage("Found %d completed PyTorchJob(s) - workloads previously used TrainingOperator (Kubeflow v1)", completedCount),
	)
}

func isJobCompleted(job *unstructured.Unstructured) (bool, error) {
	conditions, err := jq.Query[[]any](job, ".status.conditions")
	if errors.Is(err, jq.ErrNotFound) {
		return false, nil
	}

	if err != nil {
		return false, fmt.Errorf("querying job conditions: %w", err)
	}

	if len(conditions) == 0 {
		return false, nil
	}

	for _, conditionAny := range conditions {
		condition, ok := conditionAny.(map[string]any)
		if !ok {
			continue
		}

		condType, _ := condition["type"].(string)
		status, _ := condition["status"].(string)

		if (condType == "Succeeded" || condType == "Failed") && status == "True" {
			return true, nil
		}
	}

	return false, nil
}
