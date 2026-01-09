package ray

import (
	"fmt"

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
			fmt.Sprintf("Found %d %s - will be impacted in RHOAI 3.x (CodeFlare not available)", count, workloadDescription),
		)
	}

	return check.NewCondition(
		conditionType,
		metav1.ConditionTrue,
		check.ReasonVersionCompatible,
		fmt.Sprintf("No %s found - ready for RHOAI 3.x upgrade", workloadDescription),
	)
}

func newCodeFlareRayClusterCondition(count int) metav1.Condition {
	return newWorkloadCompatibilityCondition(
		ConditionTypeCodeFlareRayClusterCompatible,
		count,
		"CodeFlare-managed RayCluster(s)",
	)
}

func populateImpactedObjects(
	dr *result.DiagnosticResult,
	impactedClusters []types.NamespacedName,
) {
	dr.ImpactedObjects = make([]metav1.PartialObjectMetadata, 0, len(impactedClusters))

	for _, r := range impactedClusters {
		obj := metav1.PartialObjectMetadata{
			TypeMeta: resources.RayCluster.TypeMeta(),
			ObjectMeta: metav1.ObjectMeta{
				Namespace: r.Namespace,
				Name:      r.Name,
				Annotations: map[string]string{
					"managed-by": "CodeFlare",
				},
			},
		}
		dr.ImpactedObjects = append(dr.ImpactedObjects, obj)
	}
}
