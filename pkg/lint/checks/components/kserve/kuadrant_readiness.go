package kserve

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/opendatahub-io/odh-cli/pkg/constants"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
	"github.com/opendatahub-io/odh-cli/pkg/util/version"
)

// KuadrantReadinessCheck validates that the Kuadrant resource is present and ready.
// Only applies when llm-d workloads (LLMInferenceService) are detected.
type KuadrantReadinessCheck struct {
	check.BaseCheck
}

func NewKuadrantReadinessCheck() *KuadrantReadinessCheck {
	return &KuadrantReadinessCheck{
		BaseCheck: check.BaseCheck{
			CheckGroup:       check.GroupComponent,
			Kind:             constants.ComponentKServe,
			Type:             "kuadrant-readiness",
			CheckID:          "components.kserve.kuadrant-readiness",
			CheckName:        "Components :: KServe :: Kuadrant Readiness",
			CheckDescription: "Validates that the Kuadrant resource is present and ready (required for llm-d)",
		},
	}
}

func (c *KuadrantReadinessCheck) CanApply(ctx context.Context, target check.Target) (bool, error) {
	if !version.IsUpgradeFrom2xTo3x(target.CurrentVersion, target.TargetVersion) {
		return false, nil
	}

	return hasLLMInferenceServices(ctx, target)
}

func (c *KuadrantReadinessCheck) Validate(ctx context.Context, target check.Target) (*result.DiagnosticResult, error) {
	dr := c.NewResult()

	obj, err := target.Client.GetResource(ctx, resources.Kuadrant, kuadrantName, client.InNamespace(kuadrantNamespace))

	switch {
	case apierrors.IsNotFound(err):
		dr.SetCondition(check.NewCondition(
			check.ConditionTypeReady,
			metav1.ConditionFalse,
			check.WithReason(check.ReasonResourceNotFound),
			check.WithMessage("Kuadrant resource not found. Kuadrant must be installed for llm-d"),
			check.WithImpact(result.ImpactBlocking),
		))

		return dr, nil
	case err != nil:
		return nil, fmt.Errorf("getting Kuadrant resource: %w", err)
	case obj == nil:
		dr.SetCondition(check.NewCondition(
			check.ConditionTypeReady,
			metav1.ConditionFalse,
			check.WithReason(check.ReasonInsufficientData),
			check.WithMessage("Unable to read Kuadrant resource (insufficient permissions)"),
			check.WithImpact(result.ImpactBlocking),
		))

		return dr, nil
	}

	if err := validateReadyCondition(dr, obj, "Kuadrant"); err != nil {
		return nil, err
	}

	return dr, nil
}
