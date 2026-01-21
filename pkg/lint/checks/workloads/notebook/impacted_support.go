package notebook

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/lburgazzoli/odh-cli/pkg/lint/check"
	"github.com/lburgazzoli/odh-cli/pkg/lint/check/result"
	"github.com/lburgazzoli/odh-cli/pkg/resources"
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

func populateImpactedObjects(
	dr *result.DiagnosticResult,
	notebooks []types.NamespacedName,
) {
	dr.ImpactedObjects = make([]metav1.PartialObjectMetadata, 0, len(notebooks))

	for _, nb := range notebooks {
		obj := metav1.PartialObjectMetadata{
			TypeMeta: resources.Notebook.TypeMeta(),
			ObjectMeta: metav1.ObjectMeta{
				Namespace: nb.Namespace,
				Name:      nb.Name,
			},
		}
		dr.ImpactedObjects = append(dr.ImpactedObjects, obj)
	}
}
