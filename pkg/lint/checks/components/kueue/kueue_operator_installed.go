package kueue

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/opendatahub-io/odh-cli/pkg/constants"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/validate"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
	"github.com/opendatahub-io/odh-cli/pkg/util/components"
	"github.com/opendatahub-io/odh-cli/pkg/util/kube/olm"
)

const (
	checkTypeOperatorInstalled = "operator-installed"
	subscriptionName           = "kueue-operator"
	annotationInstalledVersion = "operator.opendatahub.io/installed-version"
)

// OperatorInstalledCheck validates the Red Hat build of Kueue operator installation status against the Kueue
// component management state:
//   - Managed + operator present: blocking — the two cannot coexist
//   - Unmanaged + operator absent: blocking — Unmanaged requires the Red Hat build of Kueue operator
type OperatorInstalledCheck struct {
	check.BaseCheck
}

func NewOperatorInstalledCheck() *OperatorInstalledCheck {
	return &OperatorInstalledCheck{
		BaseCheck: check.BaseCheck{
			CheckGroup:       check.GroupComponent,
			Kind:             kind,
			Type:             checkTypeOperatorInstalled,
			CheckID:          "components.kueue.operator-installed",
			CheckName:        "Components :: Kueue :: Operator Installed",
			CheckDescription: "Validates Red Hat build of Kueue operator installation is consistent with Kueue management state",
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
		constants.ManagementStateManaged, constants.ManagementStateUnmanaged,
	), nil
}

func (c *OperatorInstalledCheck) Validate(ctx context.Context, target check.Target) (*result.DiagnosticResult, error) {
	return validate.Component(c, target).
		Run(ctx, func(ctx context.Context, req *validate.ComponentRequest) error {
			info, err := olm.FindOperator(ctx, req.Client, func(sub *olm.SubscriptionInfo) bool {
				return sub.Name == subscriptionName
			})
			if err != nil {
				return fmt.Errorf("checking Red Hat build of Kueue operator presence: %w", err)
			}

			if info.GetVersion() != "" {
				req.Result.Annotations[annotationInstalledVersion] = info.GetVersion()
			}

			switch req.ManagementState {
			case constants.ManagementStateManaged:
				c.validateManaged(req, info)
			case constants.ManagementStateUnmanaged:
				c.validateUnmanaged(req, info)
			}

			return nil
		})
}

// validateManaged checks that the Red Hat build of Kueue operator is NOT installed when Kueue is Managed.
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
			check.WithMessage("Red Hat build of Kueue operator (%s) is installed but Kueue managementState is Managed — the two cannot coexist", info.GetVersion()),
			check.WithImpact(result.ImpactBlocking),
		))
	default:
		req.Result.SetCondition(check.NewCondition(
			check.ConditionTypeCompatible,
			metav1.ConditionTrue,
			check.WithReason(check.ReasonVersionCompatible),
			check.WithMessage("Red Hat build of Kueue operator is not installed — consistent with Managed state"),
		))
	}
}

// validateUnmanaged checks that the Red Hat build of Kueue operator IS installed when Kueue is Unmanaged.
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
			check.WithMessage("Red Hat build of Kueue operator is not installed but Kueue managementState is Unmanaged — Red Hat build of Kueue operator is required"),
			check.WithImpact(result.ImpactBlocking),
		))
	default:
		req.Result.SetCondition(check.NewCondition(
			check.ConditionTypeCompatible,
			metav1.ConditionTrue,
			check.WithReason(check.ReasonVersionCompatible),
			check.WithMessage("Red Hat build of Kueue operator installed: %s", info.GetVersion()),
		))
	}
}
