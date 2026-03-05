package dscinitialization

import (
	"errors"
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/util/jq"
)

// dsciStatusCondition is a minimal representation of a DSCI status condition used for
// collecting unhappy condition messages regardless of phase.
type dsciStatusCondition struct {
	Type    string `json:"type"`
	Status  string `json:"status"`
	Reason  string `json:"reason"`
	Message string `json:"message"`
}

// collectUnhappyDSCIConditionMessages returns messages from DSCI status conditions that
// deviate from their expected state. Progressing and Degraded conditions are expected to
// be False when healthy; all other condition types are expected to be True.
// CapabilityServiceMesh and CapabilityServiceMeshAuthorization are exceptions: when False
// with reason Removed they represent a healthy disabled state, not a problem.
// Returns nil if no conditions field exists or no conditions are unhappy.
func collectUnhappyDSCIConditionMessages(obj *unstructured.Unstructured) ([]string, error) {
	conditions, err := jq.Query[[]dsciStatusCondition](obj, ".status.conditions")
	if errors.Is(err, jq.ErrNotFound) {
		return nil, nil
	}

	if err != nil {
		return nil, fmt.Errorf("querying DSCI conditions: %w", err)
	}

	var messages []string

	for _, cond := range conditions {
		// Progressing and Degraded follow negative semantics: only False is healthy.
		// All other conditions expect True. Any value other than the expected one is unhappy.
		expectFalse := cond.Type == "Progressing" || cond.Type == "Degraded"
		unhappy := (expectFalse && cond.Status != string(metav1.ConditionFalse)) ||
			(!expectFalse && cond.Status != string(metav1.ConditionTrue))

		// CapabilityServiceMesh and CapabilityServiceMeshAuthorization with reason Removed
		// indicate the capability is intentionally disabled, which is a valid healthy state.
		if isRemovedCapabilityServiceMesh(cond) {
			unhappy = false
		}

		if unhappy && cond.Message != "" {
			messages = append(messages, fmt.Sprintf("%s: %s", cond.Type, cond.Message))
		}
	}

	return messages, nil
}

// isRemovedCapabilityServiceMesh returns true for capability conditions that are False because the
// capability was explicitly removed, which is an expected healthy state.
func isRemovedCapabilityServiceMesh(cond dsciStatusCondition) bool {
	isCapabilityServiceMesh := cond.Type == "CapabilityServiceMesh" || cond.Type == "CapabilityServiceMeshAuthorization"

	return isCapabilityServiceMesh && cond.Status == string(metav1.ConditionFalse) && cond.Reason == "Removed"
}

// validatePhaseReady checks that the status.phase field is "Ready" on a resource and there are no unexpected conditions.
func validatePhaseReady(
	dr *result.DiagnosticResult,
	obj *unstructured.Unstructured,
	resourceName string,
) error {
	phaseStatus, err := jq.Query[string](obj, `.status.phase`)

	switch {
	case errors.Is(err, jq.ErrNotFound):
		dr.SetCondition(check.NewCondition(
			check.ConditionTypeReady,
			metav1.ConditionFalse,
			check.WithReason(check.ReasonInsufficientData),
			check.WithMessage("%s resource found but phase field is missing", resourceName),
			check.WithImpact(result.ImpactBlocking),
		))
	case err != nil:
		return fmt.Errorf("querying %s phase: %w", resourceName, err)
	case phaseStatus == "":
		dr.SetCondition(check.NewCondition(
			check.ConditionTypeReady,
			metav1.ConditionFalse,
			check.WithReason(check.ReasonInsufficientData),
			check.WithMessage("%s resource found but phase is empty", resourceName),
			check.WithImpact(result.ImpactBlocking),
		))
	case phaseStatus != "Ready":
		condMessages, msgErr := collectUnhappyDSCIConditionMessages(obj)
		if msgErr != nil {
			return fmt.Errorf("collecting conditions from %s: %w", resourceName, msgErr)
		}

		msg := fmt.Sprintf("%s is not ready (phase: %s). %s must be ready before upgrading", resourceName, phaseStatus, resourceName)
		if len(condMessages) > 0 {
			msg += ". Conditions: " + strings.Join(condMessages, "; ")
		}

		dr.SetCondition(check.NewCondition(
			check.ConditionTypeReady,
			metav1.ConditionFalse,
			check.WithReason(check.ReasonResourceUnavailable),
			check.WithMessage("%s", msg),
			check.WithImpact(result.ImpactBlocking),
		))
	default:
		// Phase is Ready, but conditions may still signal problems.
		condMessages, msgErr := collectUnhappyDSCIConditionMessages(obj)
		if msgErr != nil {
			return fmt.Errorf("collecting conditions from %s: %w", resourceName, msgErr)
		}

		if len(condMessages) > 0 {
			dr.SetCondition(check.NewCondition(
				check.ConditionTypeReady,
				metav1.ConditionFalse,
				check.WithReason(check.ReasonResourceUnavailable),
				check.WithMessage("%s phase is Ready but some conditions are not expected. Conditions: %s", resourceName, strings.Join(condMessages, "; ")),
				check.WithImpact(result.ImpactBlocking),
			))
		} else {
			dr.SetCondition(check.NewCondition(
				check.ConditionTypeReady,
				metav1.ConditionTrue,
				check.WithReason(check.ReasonResourceAvailable),
				check.WithMessage("%s is ready", resourceName),
			))
		}
	}

	return nil
}
