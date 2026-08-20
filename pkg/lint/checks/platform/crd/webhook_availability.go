package crd

import (
	"context"
	"fmt"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/opendatahub-io/odh-cli/pkg/constants"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
	"github.com/opendatahub-io/odh-cli/pkg/util/jq"
	"github.com/opendatahub-io/odh-cli/pkg/util/version"
)

//nolint:gochecknoglobals // Constant-like list of CRD names to check
var webhookCRDs = []string{
	"datascienceclusters.datasciencecluster.opendatahub.io",
	"dscinitializations.dscinitialization.opendatahub.io",
}

const (
	conditionTypeWebhookHealthy = "WebhookHealthy"

	msgWebhookHealthy        = "All CRD conversion webhooks are healthy"
	msgWebhookServiceMissing = "CRD %s has conversion webhook strategy but the referenced service %s/%s is missing"
	msgWebhookNotConfigured  = "No CRDs with conversion webhook strategy found"
)

// WebhookAvailabilityCheck verifies that CRD conversion webhook services are healthy.
// Post-CRD-update: the operator has already deployed the webhook, so this is a health
// check confirming the webhook service is reachable, not a pre-flight check.
type WebhookAvailabilityCheck struct {
	check.BaseCheck
}

func NewWebhookAvailabilityCheck() *WebhookAvailabilityCheck {
	return &WebhookAvailabilityCheck{
		BaseCheck: check.BaseCheck{
			CheckGroup:       check.GroupPlatform,
			Kind:             constants.PlatformCRD,
			Type:             check.CheckTypeWebhookAvailability,
			CheckID:          "platform.crd.webhook-availability",
			CheckName:        "Platform :: CRD :: Conversion Webhook Health (3.x)",
			CheckDescription: "Verifies that CRD conversion webhook services are healthy and reachable",
			CheckRemediation: "Ensure the operator webhook service is running. Check operator pod logs and restart if necessary",
		},
	}
}

func (c *WebhookAvailabilityCheck) CanApply(_ context.Context, target check.Target) (bool, error) {
	return version.IsUpgradeFrom2xTo3x(target.CurrentVersion, target.TargetVersion), nil
}

func (c *WebhookAvailabilityCheck) Validate(
	ctx context.Context,
	target check.Target,
) (*result.DiagnosticResult, error) {
	dr := c.NewResult()

	if target.TargetVersion != nil {
		dr.Annotations[check.AnnotationCheckTargetVersion] = target.TargetVersion.String()
	}

	var issues []string

	for _, crdName := range webhookCRDs {
		crd, err := target.Client.GetResource(ctx, resources.CustomResourceDefinition, crdName)
		if err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}

			return nil, fmt.Errorf("getting CRD %s: %w", crdName, err)
		}

		if crd == nil {
			continue
		}

		strategy, _ := jq.Query[string](crd, ".spec.conversion.strategy")
		if strategy != "Webhook" {
			continue
		}

		svcName, _ := jq.Query[string](crd, ".spec.conversion.webhook.clientConfig.service.name")
		svcNamespace, _ := jq.Query[string](crd, ".spec.conversion.webhook.clientConfig.service.namespace")

		if svcName == "" || svcNamespace == "" {
			issues = append(issues, fmt.Sprintf("CRD %s has webhook strategy but missing service reference", crdName))

			continue
		}

		_, err = target.Client.GetResource(ctx, resources.Service, svcName, client.InNamespace(svcNamespace))
		if err != nil {
			if apierrors.IsNotFound(err) {
				issues = append(issues, fmt.Sprintf(msgWebhookServiceMissing, crdName, svcNamespace, svcName))

				continue
			}

			return nil, fmt.Errorf("checking webhook service %s/%s: %w", svcNamespace, svcName, err)
		}
	}

	if len(issues) > 0 {
		dr.SetCondition(check.NewCondition(
			conditionTypeWebhookHealthy,
			metav1.ConditionFalse,
			check.WithReason(check.ReasonResourceUnavailable),
			check.WithMessage("%s", strings.Join(issues, "; ")),
			check.WithImpact(result.ImpactBlocking),
			check.WithRemediation(c.CheckRemediation),
		))

		return dr, nil
	}

	dr.SetCondition(check.NewCondition(
		conditionTypeWebhookHealthy,
		metav1.ConditionTrue,
		check.WithReason(check.ReasonRequirementsMet),
		check.WithMessage(msgWebhookHealthy),
	))

	return dr, nil
}
