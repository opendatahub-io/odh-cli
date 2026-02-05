package notebook

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/lburgazzoli/odh-cli/pkg/lint/check"
	"github.com/lburgazzoli/odh-cli/pkg/lint/check/result"
	"github.com/lburgazzoli/odh-cli/pkg/resources"
)

func newAcceleratorMigrationCondition(totalImpacted int, totalMissing int) result.Condition {
	if totalImpacted == 0 {
		return check.NewCondition(
			ConditionTypeAcceleratorProfileCompatible,
			metav1.ConditionTrue,
			check.ReasonVersionCompatible,
			"No Notebooks found using AcceleratorProfiles - no migration needed",
		)
	}

	// If there are missing profiles, this is a blocking issue
	if totalMissing > 0 {
		return check.NewCondition(
			ConditionTypeAcceleratorProfileCompatible,
			metav1.ConditionFalse,
			check.ReasonResourceNotFound,
			"Found %d Notebook(s) referencing AcceleratorProfiles (%d missing) - ensure AcceleratorProfiles exist and migrate to HardwareProfiles",
			totalImpacted,
			totalMissing,
			check.WithImpact(result.ImpactBlocking),
		)
	}

	// All referenced profiles exist - advisory only
	return check.NewCondition(
		ConditionTypeAcceleratorProfileCompatible,
		metav1.ConditionFalse,
		check.ReasonConfigurationInvalid,
		"Found %d Notebook(s) using AcceleratorProfiles - migrate to HardwareProfiles before upgrading",
		totalImpacted,
		check.WithImpact(result.ImpactAdvisory),
	)
}

func populateAcceleratorImpactedObjects(
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
