package kserve

import (
	"errors"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/lburgazzoli/odh-cli/pkg/lint/check"
	"github.com/lburgazzoli/odh-cli/pkg/lint/check/result"
	"github.com/lburgazzoli/odh-cli/pkg/resources"
	"github.com/lburgazzoli/odh-cli/pkg/util/jq"
	"github.com/lburgazzoli/odh-cli/pkg/util/kube"
)

// isImpactedISVC returns true for InferenceServices with Serverless or ModelMesh deployment mode.
func isImpactedISVC(obj *metav1.PartialObjectMetadata) (bool, error) {
	return kube.HasAnnotation(obj, annotationDeploymentMode, deploymentModeServerless) ||
		kube.HasAnnotation(obj, annotationDeploymentMode, deploymentModeModelMesh), nil
}

// newWorkloadCompatibilityCondition creates a compatibility condition based on workload count.
// When count > 0, returns a failure condition indicating impacted workloads.
// When count == 0, returns a success condition indicating readiness for upgrade.
func (c *ImpactedWorkloadsCheck) newWorkloadCompatibilityCondition(
	conditionType string,
	count int,
	workloadDescription string,
) result.Condition {
	if count > 0 {
		return check.NewCondition(
			conditionType,
			metav1.ConditionFalse,
			check.WithReason(check.ReasonVersionIncompatible),
			check.WithMessage("Found %d %s - will be impacted in RHOAI 3.x", count, workloadDescription),
			check.WithImpact(result.ImpactBlocking),
			check.WithRemediation(c.CheckRemediation),
		)
	}

	return check.NewCondition(
		conditionType,
		metav1.ConditionTrue,
		check.WithReason(check.ReasonVersionCompatible),
		check.WithMessage("No %s found - ready for RHOAI 3.x upgrade", workloadDescription),
	)
}

// appendServerlessISVCCondition filters Serverless InferenceServices and appends
// the condition and impacted objects to the result.
func (c *ImpactedWorkloadsCheck) appendServerlessISVCCondition(
	dr *result.DiagnosticResult,
	allISVCs []*metav1.PartialObjectMetadata,
) {
	c.appendISVCCondition(dr, allISVCs,
		ConditionTypeServerlessISVCCompatible,
		deploymentModeServerless,
		"Serverless InferenceService(s)",
	)
}

// appendModelMeshISVCCondition filters ModelMesh InferenceServices and appends
// the condition and impacted objects to the result.
func (c *ImpactedWorkloadsCheck) appendModelMeshISVCCondition(
	dr *result.DiagnosticResult,
	allISVCs []*metav1.PartialObjectMetadata,
) {
	c.appendISVCCondition(dr, allISVCs,
		ConditionTypeModelMeshISVCCompatible,
		deploymentModeModelMesh,
		"ModelMesh InferenceService(s)",
	)
}

// appendISVCCondition filters ISVCs by deployment mode and appends the condition
// and impacted objects to the result.
func (c *ImpactedWorkloadsCheck) appendISVCCondition(
	dr *result.DiagnosticResult,
	allISVCs []*metav1.PartialObjectMetadata,
	conditionType string,
	deploymentMode string,
	workloadDescription string,
) {
	var filtered []*metav1.PartialObjectMetadata

	for _, isvc := range allISVCs {
		if kube.HasAnnotation(isvc, annotationDeploymentMode, deploymentMode) {
			filtered = append(filtered, isvc)
		}
	}

	dr.Status.Conditions = append(dr.Status.Conditions,
		c.newWorkloadCompatibilityCondition(conditionType, len(filtered), workloadDescription),
	)

	for _, r := range filtered {
		dr.ImpactedObjects = append(dr.ImpactedObjects, metav1.PartialObjectMetadata{
			TypeMeta: resources.InferenceService.TypeMeta(),
			ObjectMeta: metav1.ObjectMeta{
				Namespace: r.GetNamespace(),
				Name:      r.GetName(),
				Annotations: map[string]string{
					annotationDeploymentMode: deploymentMode,
				},
			},
		})
	}
}

// appendModelMeshSRCondition appends the condition and impacted objects for
// multi-model ServingRuntimes to the result.
func (c *ImpactedWorkloadsCheck) appendModelMeshSRCondition(
	dr *result.DiagnosticResult,
	impactedSRs []*unstructured.Unstructured,
) {
	dr.Status.Conditions = append(dr.Status.Conditions,
		c.newWorkloadCompatibilityCondition(
			ConditionTypeModelMeshSRCompatible,
			len(impactedSRs),
			"ModelMesh ServingRuntime(s)",
		),
	)

	for _, r := range impactedSRs {
		dr.ImpactedObjects = append(dr.ImpactedObjects, metav1.PartialObjectMetadata{
			TypeMeta: resources.ServingRuntime.TypeMeta(),
			ObjectMeta: metav1.ObjectMeta{
				Namespace: r.GetNamespace(),
				Name:      r.GetName(),
			},
		})
	}
}

// isUsingRemovedRuntime returns true for InferenceServices referencing a removed ServingRuntime.
func isUsingRemovedRuntime(obj *unstructured.Unstructured) (bool, error) {
	runtime, err := jq.Query[string](obj, ".spec.predictor.model.runtime")

	switch {
	case errors.Is(err, jq.ErrNotFound):
		return false, nil
	case err != nil:
		return false, fmt.Errorf("querying runtime for %s/%s: %w", obj.GetNamespace(), obj.GetName(), err)
	case runtime == runtimeOVMS:
		return true, nil
	case runtime == runtimeCaikitStandalone:
		return true, nil
	case runtime == runtimeCaikitTGIS:
		return true, nil
	default:
		return false, nil
	}
}

// appendRemovedRuntimeISVCCondition appends the condition and impacted objects for
// InferenceServices using removed ServingRuntimes to the result.
func (c *ImpactedWorkloadsCheck) appendRemovedRuntimeISVCCondition(
	dr *result.DiagnosticResult,
	items []*unstructured.Unstructured,
) error {
	dr.Status.Conditions = append(dr.Status.Conditions,
		c.newWorkloadCompatibilityCondition(
			ConditionTypeRemovedSRCompatible,
			len(items),
			"InferenceService(s) using removed ServingRuntime(s)",
		),
	)

	for _, r := range items {
		runtime, err := jq.Query[string](r, ".spec.predictor.model.runtime")
		if err != nil {
			return fmt.Errorf("querying runtime for %s/%s: %w", r.GetNamespace(), r.GetName(), err)
		}

		dr.ImpactedObjects = append(dr.ImpactedObjects, metav1.PartialObjectMetadata{
			TypeMeta: resources.InferenceService.TypeMeta(),
			ObjectMeta: metav1.ObjectMeta{
				Namespace: r.GetNamespace(),
				Name:      r.GetName(),
				Annotations: map[string]string{
					"serving.kserve.io/runtime": runtime,
				},
			},
		})
	}

	return nil
}
