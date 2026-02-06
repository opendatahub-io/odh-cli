package notebook

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/lburgazzoli/odh-cli/pkg/lint/check"
	"github.com/lburgazzoli/odh-cli/pkg/lint/check/result"
)

func newNotebookCondition(count int) result.Condition {
	if count == 0 {
		return check.NewCondition(
			ConditionTypeNotebooksCompatible,
			metav1.ConditionTrue,
			check.ReasonVersionCompatible,
			"No Notebooks found - no workloads impacted by deprecation",
		)
	}

	return check.NewCondition(
		ConditionTypeNotebooksCompatible,
		metav1.ConditionFalse,
		check.ReasonWorkloadsImpacted,
		"Found %d Notebook(s) - workloads will be impacted in RHOAI 3.x",
		count,
		check.WithImpact(result.ImpactBlocking),
	)
}
