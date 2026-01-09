package kserve

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/lburgazzoli/odh-cli/pkg/lint/check"
	"github.com/lburgazzoli/odh-cli/pkg/lint/check/result"
	"github.com/lburgazzoli/odh-cli/pkg/resources"
)

// newWorkloadCompatibilityCondition creates a compatibility condition based on workload count.
// When count > 0, returns a failure condition indicating impacted workloads.
// When count == 0, returns a success condition indicating readiness for upgrade.
func newWorkloadCompatibilityCondition(
	conditionType string,
	count int,
	workloadDescription string,
) metav1.Condition {
	if count > 0 {
		return check.NewCondition(
			conditionType,
			metav1.ConditionFalse,
			check.ReasonVersionIncompatible,
			"Found %d %s - will be impacted in RHOAI 3.x",
			count,
			workloadDescription,
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

func newServerlessISVCCondition(count int) metav1.Condition {
	return newWorkloadCompatibilityCondition(
		ConditionTypeServerlessISVCCompatible,
		count,
		"Serverless InferenceService(s)",
	)
}

func newModelMeshISVCCondition(count int) metav1.Condition {
	return newWorkloadCompatibilityCondition(
		ConditionTypeModelMeshISVCCompatible,
		count,
		"ModelMesh InferenceService(s)",
	)
}

func newModelMeshSRCondition(count int) metav1.Condition {
	return newWorkloadCompatibilityCondition(
		ConditionTypeModelMeshSRCompatible,
		count,
		"ModelMesh ServingRuntime(s)",
	)
}

func populateImpactedObjects(
	dr *result.DiagnosticResult,
	isvcsByMode impactedInferenceServices,
	impactedSRs []types.NamespacedName,
) {
	totalCount := len(isvcsByMode.serverless) + len(isvcsByMode.modelMesh) + len(impactedSRs)
	dr.ImpactedObjects = make([]metav1.PartialObjectMetadata, 0, totalCount)

	// Add Serverless InferenceServices
	for _, r := range isvcsByMode.serverless {
		obj := metav1.PartialObjectMetadata{
			TypeMeta: resources.InferenceService.TypeMeta(),
			ObjectMeta: metav1.ObjectMeta{
				Namespace: r.Namespace,
				Name:      r.Name,
				Annotations: map[string]string{
					annotationDeploymentMode: deploymentModeServerless,
				},
			},
		}
		dr.ImpactedObjects = append(dr.ImpactedObjects, obj)
	}

	// Add ModelMesh InferenceServices
	for _, r := range isvcsByMode.modelMesh {
		obj := metav1.PartialObjectMetadata{
			TypeMeta: resources.InferenceService.TypeMeta(),
			ObjectMeta: metav1.ObjectMeta{
				Namespace: r.Namespace,
				Name:      r.Name,
				Annotations: map[string]string{
					annotationDeploymentMode: deploymentModeModelMesh,
				},
			},
		}
		dr.ImpactedObjects = append(dr.ImpactedObjects, obj)
	}

	// Add ServingRuntimes (no annotations - they use .spec.multiModel)
	for _, r := range impactedSRs {
		obj := metav1.PartialObjectMetadata{
			TypeMeta: resources.ServingRuntime.TypeMeta(),
			ObjectMeta: metav1.ObjectMeta{
				Namespace: r.Namespace,
				Name:      r.Name,
			},
		}
		dr.ImpactedObjects = append(dr.ImpactedObjects, obj)
	}
}
