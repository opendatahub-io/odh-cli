package kserve

import (
	"context"
	"errors"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
	"github.com/opendatahub-io/odh-cli/pkg/util/jq"
)

const (
	kuadrantNamespace = "kuadrant-system"
	kuadrantName      = "kuadrant"
	authorinoName     = "authorino"
)

// hasLLMInferenceServices returns true when at least one LLMInferenceService
// resource exists in the cluster, indicating llm-d is in use.
func hasLLMInferenceServices(ctx context.Context, target check.Target) (bool, error) {
	items, err := target.Client.ListMetadata(ctx, resources.LLMInferenceService, client.WithLimit(1))
	if err != nil {
		if client.IsResourceTypeNotFound(err) {
			return false, nil
		}

		return false, fmt.Errorf("listing LLMInferenceService resources: %w", err)
	}

	return len(items) > 0, nil
}

// validateReadyCondition checks that the Ready condition is True on a resource.
func validateReadyCondition(
	dr *result.DiagnosticResult,
	obj *unstructured.Unstructured,
	resourceName string,
) error {
	readyStatus, err := jq.Query[string](obj, `.status.conditions[] | select(.type == "Ready") | .status`)

	switch {
	case errors.Is(err, jq.ErrNotFound):
		dr.SetCondition(check.NewCondition(
			check.ConditionTypeReady,
			metav1.ConditionFalse,
			check.WithReason(check.ReasonInsufficientData),
			check.WithMessage("%s resource found but Ready condition is missing", resourceName),
			check.WithImpact(result.ImpactBlocking),
		))
	case err != nil:
		return fmt.Errorf("querying %s Ready condition: %w", resourceName, err)
	case readyStatus == "":
		dr.SetCondition(check.NewCondition(
			check.ConditionTypeReady,
			metav1.ConditionFalse,
			check.WithReason(check.ReasonInsufficientData),
			check.WithMessage("%s resource found but Ready condition status is empty", resourceName),
			check.WithImpact(result.ImpactBlocking),
		))
	case readyStatus != "True":
		dr.SetCondition(check.NewCondition(
			check.ConditionTypeReady,
			metav1.ConditionFalse,
			check.WithReason(check.ReasonResourceUnavailable),
			check.WithMessage("%s is not ready (status: %s). %s must be ready for llm-d", resourceName, readyStatus, resourceName),
			check.WithImpact(result.ImpactBlocking),
		))
	default:
		dr.SetCondition(check.NewCondition(
			check.ConditionTypeReady,
			metav1.ConditionTrue,
			check.WithReason(check.ReasonResourceAvailable),
			check.WithMessage("%s is installed and ready", resourceName),
		))
	}

	return nil
}
