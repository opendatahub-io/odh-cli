package kserve

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/lburgazzoli/odh-cli/pkg/lint/check"
	"github.com/lburgazzoli/odh-cli/pkg/lint/check/result"
	"github.com/lburgazzoli/odh-cli/pkg/resources"
)

func newISVCAcceleratorMigrationCondition(totalImpacted int, totalMissing int, remediation string) result.Condition {
	if totalImpacted == 0 {
		return check.NewCondition(
			ConditionTypeISVCAcceleratorProfileCompatible,
			metav1.ConditionTrue,
			check.ReasonVersionCompatible,
			"No InferenceServices found using AcceleratorProfiles - no migration needed",
		)
	}

	// If there are missing profiles, this is a blocking issue
	if totalMissing > 0 {
		return check.NewCondition(
			ConditionTypeISVCAcceleratorProfileCompatible,
			metav1.ConditionFalse,
			check.ReasonResourceNotFound,
			"Found %d InferenceService(s) referencing AcceleratorProfiles (%d missing) - ensure AcceleratorProfiles exist and migrate to HardwareProfiles",
			totalImpacted,
			totalMissing,
			check.WithImpact(result.ImpactBlocking),
			check.WithRemediation(remediation),
		)
	}

	// All referenced profiles exist - advisory only
	return check.NewCondition(
		ConditionTypeISVCAcceleratorProfileCompatible,
		metav1.ConditionFalse,
		check.ReasonConfigurationInvalid,
		"Found %d InferenceService(s) using AcceleratorProfiles - migrate to HardwareProfiles before upgrading",
		totalImpacted,
		check.WithImpact(result.ImpactAdvisory),
		check.WithRemediation(remediation),
	)
}

func populateISVCAcceleratorImpactedObjects(
	dr *result.DiagnosticResult,
	inferenceServices []types.NamespacedName,
) {
	dr.ImpactedObjects = make([]metav1.PartialObjectMetadata, 0, len(inferenceServices))

	for _, isvc := range inferenceServices {
		obj := metav1.PartialObjectMetadata{
			TypeMeta: resources.InferenceService.TypeMeta(),
			ObjectMeta: metav1.ObjectMeta{
				Namespace: isvc.Namespace,
				Name:      isvc.Name,
			},
		}
		dr.ImpactedObjects = append(dr.ImpactedObjects, obj)
	}
}
