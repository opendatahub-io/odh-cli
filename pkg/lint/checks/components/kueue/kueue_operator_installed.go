package kueue

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/lburgazzoli/odh-cli/pkg/lint/check"
	"github.com/lburgazzoli/odh-cli/pkg/lint/check/result"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/shared/base"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/shared/components"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/shared/validate"
	"github.com/lburgazzoli/odh-cli/pkg/util/client"
	"github.com/lburgazzoli/odh-cli/pkg/util/kube/olm"
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
			info, err := olm.FindOperator(ctx, req.Client, func(sub *olm.SubscriptionInfo) bool {
				return sub.Name == subscriptionName
			})
			if err != nil {
				return fmt.Errorf("checking kueue-operator presence: %w", err)
			}

			if info.GetVersion() != "" {
				req.Result.Annotations[annotationInstalledVersion] = info.GetVersion()
			}

			switch req.ManagementState {
			case check.ManagementStateManaged:
				c.validateManaged(req, info)
			case check.ManagementStateUnmanaged:
				c.validateUnmanaged(req, info)
			}

			return nil
		})
}

// validateManaged checks that the kueue-operator is NOT installed when Kueue is Managed.
func (c *OperatorInstalledCheck) validateManaged(
	req *validate.ComponentRequest,
	info *olm.SubscriptionInfo,
) {
	switch {
	case info.Found():
		req.Result.SetCondition(check.NewCondition(
			check.ConditionTypeCompatible,
			metav1.ConditionFalse,
			check.WithReason(check.ReasonVersionIncompatible),
			check.WithMessage("kueue-operator (%s) is installed but Kueue managementState is Managed — the two cannot coexist", info.GetVersion()),
			check.WithImpact(result.ImpactBlocking),
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
	info *olm.SubscriptionInfo,
) {
	switch {
	case !info.Found():
		req.Result.SetCondition(check.NewCondition(
			check.ConditionTypeCompatible,
			metav1.ConditionFalse,
			check.WithReason(check.ReasonVersionIncompatible),
			check.WithMessage("kueue-operator is not installed but Kueue managementState is Unmanaged — the standalone operator is required"),
			check.WithImpact(result.ImpactBlocking),
		))
	default:
		req.Result.SetCondition(check.NewCondition(
			check.ConditionTypeCompatible,
			metav1.ConditionTrue,
			check.WithReason(check.ReasonVersionCompatible),
			check.WithMessage("kueue-operator installed: %s", info.GetVersion()),
		))
	}
}
