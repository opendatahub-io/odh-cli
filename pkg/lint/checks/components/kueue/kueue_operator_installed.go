package kueue

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/lburgazzoli/odh-cli/pkg/lint/check"
	"github.com/lburgazzoli/odh-cli/pkg/lint/check/result"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/shared/base"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/shared/components"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/shared/operators"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/shared/validate"
	"github.com/lburgazzoli/odh-cli/pkg/util/client"
)

const (
	checkTypeOperatorInstalled = "operator-installed"
	subscriptionName           = "kueue-operator"
	annotationInstalledVersion = "operator.opendatahub.io/installed-version"
)

// OperatorInstalledCheck validates the kueue-operator installation status against the Kueue
// component management state:
//   - Managed + operator present: blocking — the two cannot coexist
//   - Unmanaged + operator absent: blocking — Unmanaged requires the standalone operator
type OperatorInstalledCheck struct {
	base.BaseCheck
}

func NewOperatorInstalledCheck() *OperatorInstalledCheck {
	return &OperatorInstalledCheck{
		BaseCheck: base.BaseCheck{
			CheckGroup:       check.GroupComponent,
			Kind:             kind,
			Type:             checkTypeOperatorInstalled,
			CheckID:          "components.kueue.operator-installed",
			CheckName:        "Components :: Kueue :: Operator Installed",
			CheckDescription: "Validates kueue-operator installation is consistent with Kueue management state",
		},
	}
}

func (c *OperatorInstalledCheck) CanApply(ctx context.Context, target check.Target) (bool, error) {
	dsc, err := client.GetDataScienceCluster(ctx, target.Client)
	if err != nil {
		return false, fmt.Errorf("getting DataScienceCluster: %w", err)
	}

	return components.HasManagementState(
		dsc, "kueue",
		check.ManagementStateManaged, check.ManagementStateUnmanaged,
	), nil
}

func (c *OperatorInstalledCheck) Validate(ctx context.Context, target check.Target) (*result.DiagnosticResult, error) {
	return validate.Component(c, target).
		Run(ctx, func(ctx context.Context, req *validate.ComponentRequest) error {
			found, err := operators.FindOperator(ctx, req.Client, func(sub *operators.SubscriptionInfo) bool {
				return sub.Name == subscriptionName
			})
			if err != nil {
				return fmt.Errorf("checking kueue-operator presence: %w", err)
			}

			if found.Found && found.Version != "" {
				req.Result.Annotations[annotationInstalledVersion] = found.Version
			}

			switch req.ManagementState {
			case check.ManagementStateManaged:
				c.validateManaged(req, found)
			case check.ManagementStateUnmanaged:
				c.validateUnmanaged(req, found)
			}

			return nil
		})
}

// validateManaged checks that the kueue-operator is NOT installed when Kueue is Managed.
func (c *OperatorInstalledCheck) validateManaged(
	req *validate.ComponentRequest,
	found *operators.FindResult,
) {
	switch {
	case found.Found:
		req.Result.SetCondition(check.NewCondition(
			check.ConditionTypeCompatible,
			metav1.ConditionFalse,
			check.WithReason(check.ReasonVersionIncompatible),
			check.WithMessage("kueue-operator (%s) is installed but Kueue managementState is Managed — the two cannot coexist", found.Version),
		))
	default:
		req.Result.SetCondition(check.NewCondition(
			check.ConditionTypeCompatible,
			metav1.ConditionTrue,
			check.WithReason(check.ReasonVersionCompatible),
			check.WithMessage("kueue-operator is not installed — consistent with Managed state"),
		))
	}
}

// validateUnmanaged checks that the kueue-operator IS installed when Kueue is Unmanaged.
func (c *OperatorInstalledCheck) validateUnmanaged(
	req *validate.ComponentRequest,
	found *operators.FindResult,
) {
	switch {
	case !found.Found:
		req.Result.SetCondition(check.NewCondition(
			check.ConditionTypeCompatible,
			metav1.ConditionFalse,
			check.WithReason(check.ReasonVersionIncompatible),
			check.WithMessage("kueue-operator is not installed but Kueue managementState is Unmanaged — the standalone operator is required"),
		))
	default:
		req.Result.SetCondition(check.NewCondition(
			check.ConditionTypeCompatible,
			metav1.ConditionTrue,
			check.WithReason(check.ReasonVersionCompatible),
			check.WithMessage("kueue-operator installed: %s", found.Version),
		))
	}
}
