package trainingoperator

import (
	"errors"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/lburgazzoli/odh-cli/pkg/lint/check"
	"github.com/lburgazzoli/odh-cli/pkg/lint/check/result"
	"github.com/lburgazzoli/odh-cli/pkg/util/jq"
)

func (c *ImpactedWorkloadsCheck) newPyTorchJobCondition(
	activeCount int,
	completedCount int,
) result.Condition {
	totalCount := activeCount + completedCount

	if totalCount == 0 {
		return check.NewCondition(
			ConditionTypePyTorchJobsCompatible,
			metav1.ConditionTrue,
			check.ReasonVersionCompatible,
			"No PyTorchJob(s) found - no workloads impacted by TrainingOperator deprecation",
		)
	}

	if activeCount > 0 && completedCount > 0 {
		return check.NewCondition(
			ConditionTypePyTorchJobsCompatible,
			metav1.ConditionFalse,
			check.ReasonWorkloadsImpacted,
			"Found %d PyTorchJob(s) (%d active, %d completed) - workloads use deprecated TrainingOperator (Kubeflow v1) which will be replaced by Trainer v2",
			totalCount,
			activeCount,
			completedCount,
			check.WithImpact(result.ImpactAdvisory),
		)
	}

	if activeCount > 0 {
		return check.NewCondition(
			ConditionTypePyTorchJobsCompatible,
			metav1.ConditionFalse,
			check.ReasonWorkloadsImpacted,
			"Found %d active PyTorchJob(s) - workloads use deprecated TrainingOperator (Kubeflow v1) which will be replaced by Trainer v2",
			activeCount,
			check.WithImpact(result.ImpactAdvisory),
		)
	}

	return check.NewCondition(
		ConditionTypePyTorchJobsCompatible,
		metav1.ConditionTrue,
		check.ReasonVersionCompatible,
		"Found %d completed PyTorchJob(s) - workloads previously used deprecated TrainingOperator (Kubeflow v1)",
		completedCount,
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
