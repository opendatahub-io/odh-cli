package kueueoperator

import (
	"context"
	"fmt"

	"github.com/lburgazzoli/odh-cli/pkg/lint/check"
	"github.com/lburgazzoli/odh-cli/pkg/lint/check/result"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/shared/base"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/shared/components"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/shared/validate"
	"github.com/lburgazzoli/odh-cli/pkg/util/client"
)

const kind = "kueueoperator"

// Check validates kueue-operator installation.
type Check struct {
	base.BaseCheck
}

// NewCheck creates a new kueue-operator installation check.
func NewCheck() *Check {
	return &Check{
		BaseCheck: base.BaseCheck{
			CheckGroup:       check.GroupDependency,
			Kind:             kind,
			Type:             check.CheckTypeInstalled,
			CheckID:          "dependencies.kueueoperator.installed",
			CheckName:        "Dependencies :: KueueOperator :: Installed",
			CheckDescription: "Reports the kueue-operator installation status and version",
		},
	}
}

// CanApply returns whether this check should run for the given target.
// This check only applies when the kueue component is enabled in DataScienceCluster.
func (c *Check) CanApply(ctx context.Context, target check.Target) (bool, error) {
	if target.Client == nil {
		return false, nil
	}

	dsc, err := client.GetDataScienceCluster(ctx, target.Client)
	if err != nil {
		return false, fmt.Errorf("getting DataScienceCluster: %w", err)
	}

	return components.HasManagementState(dsc, "kueue", check.ManagementStateManaged, check.ManagementStateUnmanaged), nil
}

func (c *Check) Validate(ctx context.Context, target check.Target) (*result.DiagnosticResult, error) {
	return validate.Operator(c, target).
		WithNames("kueue-operator").
		Run(ctx)
}
