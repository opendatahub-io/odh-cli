package kueueoperator

import (
	"context"
	"fmt"

	operatorsv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"

	"github.com/lburgazzoli/odh-cli/pkg/lint/check"
	"github.com/lburgazzoli/odh-cli/pkg/lint/check/result"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/shared/components"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/shared/operators"
)

const (
	checkID          = "dependencies.kueueoperator.installed"
	checkName        = "Dependencies :: KueueOperator :: Installed"
	checkDescription = "Reports the kueue-operator installation status and version"
)

type Check struct{}

// NewCheck creates a new kueue-operator installation check.
func NewCheck() *Check {
	return &Check{}
}

func (c *Check) ID() string {
	return checkID
}

func (c *Check) Name() string {
	return checkName
}

func (c *Check) Description() string {
	return checkDescription
}

func (c *Check) Group() check.CheckGroup {
	return check.GroupDependency
}

// CanApply returns whether this check should run for the given target.
// This check only applies when the kueue component is enabled in DataScienceCluster.
func (c *Check) CanApply(target check.Target) bool {
	if target.Client == nil {
		return false
	}

	dsc, err := target.Client.GetDataScienceCluster(context.Background())
	if err != nil {
		return false
	}

	return components.HasManagementState(dsc, "kueue", check.ManagementStateManaged, check.ManagementStateUnmanaged)
}

func (c *Check) Validate(ctx context.Context, target check.Target) (*result.DiagnosticResult, error) {
	// kueue-operator check uses all defaults from operators.CheckOperatorPresence
	// since the subscription name matches the operator kind ("kueue-operator")
	res, err := operators.CheckOperatorPresence(
		ctx,
		target.Client,
		"kueue-operator",
		operators.WithDescription(checkDescription),
		operators.WithMatcher(func(subscription *operatorsv1alpha1.Subscription) bool {
			op := operators.GetOperator(subscription)

			return op.Name == "kueue-operator"
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("checking kueue-operator presence: %w", err)
	}

	return res, nil
}
