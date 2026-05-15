package llamastack

import (
	"context"
	"fmt"

	"github.com/opendatahub-io/odh-cli/pkg/constants"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/validate"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
	"github.com/opendatahub-io/odh-cli/pkg/util/components"
	"github.com/opendatahub-io/odh-cli/pkg/util/version"
)

const kind = "llamastackoperator"

// RemovalCheck validates that LlamaStack Operator is disabled before upgrading from 3.4 to 3.5.
type RemovalCheck struct {
	check.BaseCheck
}

func NewRemovalCheck() *RemovalCheck {
	return &RemovalCheck{
		BaseCheck: check.BaseCheck{
			CheckGroup:       check.GroupComponent,
			Kind:             kind,
			Type:             check.CheckTypeRemoval,
			CheckID:          "components.llamastackoperator.removal",
			CheckName:        "Components :: LlamaStack Operator :: Removal (3.5)",
			CheckDescription: "Validates that LlamaStack Operator is disabled before upgrading from RHOAI 3.4 to 3.5 (component is replaced by ogx)",
			CheckRemediation: "Disable LlamaStack Operator by setting managementState to 'Removed' in DataScienceCluster before upgrading",
		},
	}
}

// CanApply returns whether this check should run for the given target.
// This check only applies when upgrading FROM 3.4.x TO 3.5.x and LlamaStack Operator is Managed.
func (c *RemovalCheck) CanApply(ctx context.Context, target check.Target) (bool, error) {
	if !version.IsUpgradeFrom34To35(target.CurrentVersion, target.TargetVersion) {
		return false, nil
	}

	dsc, err := client.GetDataScienceCluster(ctx, target.Client)
	if err != nil {
		return false, fmt.Errorf("getting DataScienceCluster: %w", err)
	}

	return components.HasManagementState(dsc, kind, constants.ManagementStateManaged), nil
}

func (c *RemovalCheck) Validate(ctx context.Context, target check.Target) (*result.DiagnosticResult, error) {
	return validate.Component(c, target).
		Run(ctx, validate.Removal("LlamaStack Operator is enabled (state: %s) but is replaced by ogx in RHOAI %s",
			check.WithImpact(result.ImpactBlocking),
			check.WithRemediation(c.CheckRemediation)))
}
