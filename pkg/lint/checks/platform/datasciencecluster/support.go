package datasciencecluster

import (
	"errors"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/util/jq"
)

// validateReadyCondition checks that the Ready condition is True on a resource.
func validateReadyCondition(
	dr *result.DiagnosticResult,
	obj *unstructured.Unstructured,
	resourceName string,
	expectedStatus metav1.ConditionStatus,
) error {
	// Use // [] so that a missing conditions field yields an empty array rather than
	// a gojq "cannot iterate over: null" error.
	readyCondition, err := jq.Query[metav1.Condition](obj, `.status.conditions // [] | .[] | select(.type == "Ready")`)

	switch {
	case err != nil && !errors.Is(err, jq.ErrNotFound):
		return fmt.Errorf("querying %s ready condition: %w", resourceName, err)
	case errors.Is(err, jq.ErrNotFound), readyCondition.Type == "":
		// Either the query returned null or select found no matching condition.
		dr.SetCondition(check.NewCondition(
			check.ConditionTypeReady,
			metav1.ConditionFalse,
			check.WithReason(check.ReasonInsufficientData),
			check.WithMessage("%s resource found but Ready condition is missing", resourceName),
			check.WithImpact(result.ImpactBlocking),
		))
	case readyCondition.Status != expectedStatus:
		dr.SetCondition(check.NewCondition(
			check.ConditionTypeReady,
			metav1.ConditionFalse,
			check.WithReason(check.ReasonResourceUnavailable),
			check.WithMessage("%s is not ready (status: %s) due to '%s'. %s must be ready before upgrading", resourceName, readyCondition.Status, readyCondition.Message, resourceName),
			check.WithImpact(result.ImpactBlocking),
		))
	default:
		dr.SetCondition(check.NewCondition(
			check.ConditionTypeReady,
			metav1.ConditionTrue,
			check.WithReason(check.ReasonResourceAvailable),
			check.WithMessage("%s is ready", resourceName),
		))
	}

	return nil
}
