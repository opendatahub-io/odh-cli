package kueue

import (
	"context"
	"fmt"

	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/validate"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
)

const (
	remediationLabeledInNonKueueNs = "Add the kueue-managed or kueue.openshift.io/managed label to the affected namespaces, " +
		"or remove the kueue.x-k8s.io/queue-name label from the workloads if kueue integration is not intended"

	remediationMissingLabelInKueueNs = "Add the kueue.x-k8s.io/queue-name label to the workloads in kueue-enabled namespaces, " +
		"or remove the kueue-managed or kueue.openshift.io/managed label from the namespaces if kueue integration is not intended for those workloads"
)

// CheckConfig holds the per-resource parameters that differentiate each kueue label check.
type CheckConfig struct {
	Kind                      string
	Resource                  resources.ResourceType
	ConditionType             string
	MissingLabelConditionType string
	KindLabel                 string
	CheckID                   string
	CheckName                 string
}

// KueueLabelCheck verifies that workloads with the kueue queue label are in
// kueue-enabled namespaces, and that workloads in kueue-enabled namespaces
// have the kueue queue label.
type KueueLabelCheck struct {
	check.BaseCheck

	resource                  resources.ResourceType
	conditionType             string
	missingLabelConditionType string
	kindLabel                 string
}

func NewCheck(cfg CheckConfig) *KueueLabelCheck {
	return &KueueLabelCheck{
		BaseCheck: check.BaseCheck{
			CheckGroup:       check.GroupWorkload,
			Kind:             cfg.Kind,
			Type:             check.CheckTypeDataIntegrity,
			CheckID:          cfg.CheckID,
			CheckName:        cfg.CheckName,
			CheckDescription: fmt.Sprintf("Verifies that %ss with the kueue queue label are in kueue-enabled namespaces", cfg.KindLabel),
			CheckRemediation: remediationLabeledInNonKueueNs,
		},
		resource:                  cfg.Resource,
		conditionType:             cfg.ConditionType,
		missingLabelConditionType: cfg.MissingLabelConditionType,
		kindLabel:                 cfg.KindLabel,
	}
}

func (c *KueueLabelCheck) CanApply(ctx context.Context, target check.Target) (bool, error) {
	ok, err := IsKueueActive(ctx, target)
	if err != nil {
		return false, fmt.Errorf("checking kueue state: %w", err)
	}

	return ok, nil
}

func (c *KueueLabelCheck) Validate(
	ctx context.Context,
	target check.Target,
) (*result.DiagnosticResult, error) {
	return validate.WorkloadsMetadata(c, target, c.resource).
		Run(ctx, ValidateFn(
			c.resource,
			c.conditionType,
			c.missingLabelConditionType,
			c.kindLabel,
		))
}
