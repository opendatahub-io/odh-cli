package kserve

import (
	"context"
	"errors"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/opendatahub-io/odh-cli/pkg/constants"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
	"github.com/opendatahub-io/odh-cli/pkg/util/jq"
	"github.com/opendatahub-io/odh-cli/pkg/util/version"
)

// AuthorinoTLSReadinessCheck validates that Authorino is configured with TLS and ready.
// Only applies when llm-d workloads (LLMInferenceService) are detected.
type AuthorinoTLSReadinessCheck struct {
	check.BaseCheck
}

func NewAuthorinoTLSReadinessCheck() *AuthorinoTLSReadinessCheck {
	return &AuthorinoTLSReadinessCheck{
		BaseCheck: check.BaseCheck{
			CheckGroup:       check.GroupComponent,
			Kind:             constants.ComponentKServe,
			Type:             "authorino-tls-readiness",
			CheckID:          "components.kserve.authorino-tls-readiness",
			CheckName:        "Components :: KServe :: Authorino TLS Readiness",
			CheckDescription: "Validates that Authorino is configured with TLS and ready (required for llm-d)",
		},
	}
}

func (c *AuthorinoTLSReadinessCheck) CanApply(ctx context.Context, target check.Target) (bool, error) {
	if !version.IsUpgradeFrom2xTo3x(target.CurrentVersion, target.TargetVersion) {
		return false, nil
	}

	return hasLLMInferenceServices(ctx, target)
}

func (c *AuthorinoTLSReadinessCheck) Validate(ctx context.Context, target check.Target) (*result.DiagnosticResult, error) {
	dr := c.NewResult()

	obj, err := target.Client.GetResource(ctx, resources.Authorino, authorinoName, client.InNamespace(kuadrantNamespace))

	switch {
	case apierrors.IsNotFound(err):
		dr.SetCondition(check.NewCondition(
			check.ConditionTypeReady,
			metav1.ConditionFalse,
			check.WithReason(check.ReasonResourceNotFound),
			check.WithMessage("Authorino resource not found. Authorino with TLS must be installed for llm-d"),
			check.WithImpact(result.ImpactBlocking),
		))

		return dr, nil
	case err != nil:
		return nil, fmt.Errorf("getting Authorino resource: %w", err)
	case obj == nil:
		dr.SetCondition(check.NewCondition(
			check.ConditionTypeReady,
			metav1.ConditionFalse,
			check.WithReason(check.ReasonInsufficientData),
			check.WithMessage("Unable to read Authorino resource (insufficient permissions)"),
			check.WithImpact(result.ImpactBlocking),
		))

		return dr, nil
	}

	if err := validateAuthorinoTLS(dr, obj); err != nil {
		return nil, err
	}

	if err := validateReadyCondition(dr, obj, "Authorino"); err != nil {
		return nil, err
	}

	return dr, nil
}

// validateAuthorinoTLS checks that TLS is enabled and a cert secret is configured.
func validateAuthorinoTLS(dr *result.DiagnosticResult, obj *unstructured.Unstructured) error {
	tlsEnabled, err := jq.Query[bool](obj, ".spec.listener.tls.enabled")

	switch {
	case errors.Is(err, jq.ErrNotFound):
		dr.SetCondition(check.NewCondition(
			check.ConditionTypeConfigured,
			metav1.ConditionFalse,
			check.WithReason(check.ReasonConfigurationInvalid),
			check.WithMessage("Authorino listener TLS configuration is missing. TLS must be configured for llm-d"),
			check.WithImpact(result.ImpactBlocking),
		))

		return nil
	case err != nil:
		return fmt.Errorf("querying Authorino TLS enabled: %w", err)
	case !tlsEnabled:
		dr.SetCondition(check.NewCondition(
			check.ConditionTypeConfigured,
			metav1.ConditionFalse,
			check.WithReason(check.ReasonConfigurationInvalid),
			check.WithMessage("Authorino listener TLS is not enabled. TLS must be enabled for llm-d"),
			check.WithImpact(result.ImpactBlocking),
		))

		return nil
	}

	certSecret, err := jq.Query[string](obj, ".spec.listener.tls.certSecretRef.name")

	switch {
	case errors.Is(err, jq.ErrNotFound):
		dr.SetCondition(check.NewCondition(
			check.ConditionTypeConfigured,
			metav1.ConditionFalse,
			check.WithReason(check.ReasonConfigurationInvalid),
			check.WithMessage("Authorino TLS certSecretRef is not configured. A TLS certificate secret is required for llm-d"),
			check.WithImpact(result.ImpactBlocking),
		))
	case err != nil:
		return fmt.Errorf("querying Authorino TLS certSecretRef: %w", err)
	case certSecret == "":
		dr.SetCondition(check.NewCondition(
			check.ConditionTypeConfigured,
			metav1.ConditionFalse,
			check.WithReason(check.ReasonConfigurationInvalid),
			check.WithMessage("Authorino TLS certSecretRef.name is empty. A TLS certificate secret is required for llm-d"),
			check.WithImpact(result.ImpactBlocking),
		))
	default:
		dr.SetCondition(check.NewCondition(
			check.ConditionTypeConfigured,
			metav1.ConditionTrue,
			check.WithReason(check.ReasonConfigurationValid),
			check.WithMessage("Authorino TLS is enabled with certificate secret %q", certSecret),
		))
	}

	return nil
}
