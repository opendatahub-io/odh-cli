package notebook

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/validate"
)

// hasDashboardAnnotation returns true if the notebook has any annotation indicating
// it was created or configured through the Dashboard UI.
func hasDashboardAnnotation(annotations map[string]string) bool {
	if annotations == nil {
		return false
	}

	return annotations[validate.AnnotationAcceleratorName] != "" ||
		annotations[annotationLastSizeSelection] != ""
}

const (
	// annotationLastSizeSelection is set by the Dashboard when a user selects a container size for a workbench.
	annotationLastSizeSelection = "notebooks.opendatahub.io/last-size-selection"

	msgNoNotebooksMismatch = "No Notebooks found with container name mismatch"
	msgNotebooksMismatch   = "Found %d Notebook(s) where the primary container name does not match the Notebook CR name"
)

// hasDashboardAnnotationAndNameMismatch returns true if the notebook has a Dashboard-managed
// annotation (accelerator profile or size selection) and its primary (non-infrastructure)
// container name does not match the notebook CR name.
func hasDashboardAnnotationAndNameMismatch(nb *unstructured.Unstructured) (bool, error) {
	// Only check notebooks created or configured via Dashboard, identified by known annotations.
	annotations := nb.GetAnnotations()
	if !hasDashboardAnnotation(annotations) {
		return false, nil
	}

	// Extract workload containers (infrastructure sidecars already filtered out).
	containers, err := ExtractWorkloadContainers(nb)
	if err != nil {
		return false, fmt.Errorf("extracting containers from notebook %s/%s: %w", nb.GetNamespace(), nb.GetName(), err)
	}

	if len(containers) == 0 {
		return false, nil
	}

	// The first workload container is the primary workbench container.
	return containers[0].Name != nb.GetName(), nil
}

func (c *ContainerNameCheck) newContainerNameCondition(
	_ context.Context,
	req *validate.WorkloadRequest[*unstructured.Unstructured],
) ([]result.Condition, error) {
	count := len(req.Items)

	if count == 0 {
		return []result.Condition{check.NewCondition(
			ConditionTypeContainerNameValid,
			metav1.ConditionTrue,
			check.WithReason(check.ReasonConfigurationValid),
			check.WithMessage(msgNoNotebooksMismatch),
		)}, nil
	}

	return []result.Condition{check.NewCondition(
		ConditionTypeContainerNameValid,
		metav1.ConditionFalse,
		check.WithReason(check.ReasonConfigurationInvalid),
		check.WithMessage(msgNotebooksMismatch, count),
		check.WithImpact(result.ImpactAdvisory),
		check.WithRemediation(c.CheckRemediation),
	)}, nil
}
