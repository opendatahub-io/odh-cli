package aisafety

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
	"github.com/opendatahub-io/odh-cli/pkg/util/version"
)

const kind = "aisafety"

const (
	guardrailsCRDName = "guardrailsorchestrators.trustyai.opendatahub.io"

	msgCRDNotFound       = "FMS Guardrails Orchestrator CRD not present - ready for upgrade"
	msgCRDFoundNoCRs     = "FMS Guardrails Orchestrator CRD %q must be deleted before upgrading to RHOAI %s"
	msgCRDFoundWithCRs   = "Found %d GuardrailsOrchestrator CR(s) that must be deleted along with CRD %q before upgrading to RHOAI %s"
	msgCRDFoundListError = "FMS Guardrails Orchestrator CRD %q exists but unable to list CR instances: %v. Delete all GuardrailsOrchestrator CRs and the CRD before upgrading to RHOAI %s"
)

const (
	remediationDeleteCRD       = "Delete the CRD before upgrading: oc delete crd " + guardrailsCRDName
	remediationDeleteCRsAndCRD = "Delete all GuardrailsOrchestrator CRs first, then delete the CRD: oc delete guardrailsorchestrators.trustyai.opendatahub.io --all -A && oc delete crd " + guardrailsCRDName
)

// FMSGuardrailsCRDRemovalCheck detects the FMS Guardrails Orchestrator CRD that must
// be removed before upgrading past RHOAI 3.5.
type FMSGuardrailsCRDRemovalCheck struct {
	check.BaseCheck
}

// NewFMSGuardrailsCRDRemovalCheck creates a new FMSGuardrailsCRDRemovalCheck.
func NewFMSGuardrailsCRDRemovalCheck() *FMSGuardrailsCRDRemovalCheck {
	return &FMSGuardrailsCRDRemovalCheck{
		BaseCheck: check.BaseCheck{
			CheckGroup:       check.GroupComponent,
			Kind:             kind,
			Type:             check.CheckTypeRemoval,
			CheckID:          "components.aisafety.fms-guardrails-crd-removal",
			CheckName:        "Components :: AI Safety :: FMS Guardrails CRD Removal (3.6)",
			CheckDescription: "Detects FMS Guardrails Orchestrator CRD that must be deleted before upgrading past RHOAI 3.5",
			CheckRemediation: remediationDeleteCRD,
		},
	}
}

// CanApply returns true when upgrading from <=3.5 to >=3.6.
func (c *FMSGuardrailsCRDRemovalCheck) CanApply(_ context.Context, target check.Target) (bool, error) {
	if target.CurrentVersion == nil || target.TargetVersion == nil {
		return false, nil
	}

	return !version.IsVersionAtLeast(target.CurrentVersion, 3, 6) && //nolint:mnd
		version.IsVersionAtLeast(target.TargetVersion, 3, 6), nil //nolint:mnd
}

// Validate checks whether the FMS Guardrails Orchestrator CRD exists on the cluster.
func (c *FMSGuardrailsCRDRemovalCheck) Validate(
	ctx context.Context,
	target check.Target,
) (*result.DiagnosticResult, error) {
	dr := c.NewResult()
	tv := version.MajorMinorLabel(target.TargetVersion)

	if target.TargetVersion != nil {
		dr.Annotations[check.AnnotationCheckTargetVersion] = target.TargetVersion.String()
	}

	crd, err := target.Client.GetResource(ctx, resources.CustomResourceDefinition, guardrailsCRDName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			dr.SetCondition(check.NewCondition(
				check.ConditionTypeCompatible,
				metav1.ConditionTrue,
				check.WithReason(check.ReasonRequirementsMet),
				check.WithMessage(msgCRDNotFound),
			))

			return dr, nil
		}

		return nil, fmt.Errorf("getting CRD %s: %w", guardrailsCRDName, err)
	}

	if crd == nil {
		dr.SetCondition(check.NewCondition(
			check.ConditionTypeCompatible,
			metav1.ConditionUnknown,
			check.WithReason(check.ReasonAPIAccessDenied),
			check.WithMessage("Unable to access CRD %s - insufficient permissions", guardrailsCRDName),
		))

		return dr, nil
	}

	orchestrators, err := client.List[*unstructured.Unstructured](
		ctx, target.Client, resources.GuardrailsOrchestrator, nil,
	)
	if err != nil {
		dr.SetCondition(check.NewCondition(
			check.ConditionTypeCompatible,
			metav1.ConditionFalse,
			check.WithReason(check.ReasonFeatureRemoved),
			check.WithMessage(msgCRDFoundListError, guardrailsCRDName, err, tv),
			check.WithImpact(result.ImpactBlocking),
			check.WithRemediation(remediationDeleteCRsAndCRD),
		))

		return dr, nil
	}

	if len(orchestrators) == 0 {
		dr.SetCondition(check.NewCondition(
			check.ConditionTypeCompatible,
			metav1.ConditionFalse,
			check.WithReason(check.ReasonFeatureRemoved),
			check.WithMessage(msgCRDFoundNoCRs, guardrailsCRDName, tv),
			check.WithImpact(result.ImpactBlocking),
			check.WithRemediation(remediationDeleteCRD),
		))

		return dr, nil
	}

	for _, orch := range orchestrators {
		dr.ImpactedObjects = append(dr.ImpactedObjects, metav1.PartialObjectMetadata{
			TypeMeta: resources.GuardrailsOrchestrator.TypeMeta(),
			ObjectMeta: metav1.ObjectMeta{
				Namespace: orch.GetNamespace(),
				Name:      orch.GetName(),
			},
		})
	}

	dr.SetCondition(check.NewCondition(
		check.ConditionTypeCompatible,
		metav1.ConditionFalse,
		check.WithReason(check.ReasonFeatureRemoved),
		check.WithMessage(msgCRDFoundWithCRs, len(orchestrators), guardrailsCRDName, tv),
		check.WithImpact(result.ImpactBlocking),
		check.WithRemediation(remediationDeleteCRsAndCRD),
	))

	return dr, nil
}
