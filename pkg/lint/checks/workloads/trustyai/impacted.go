package trustyai

import (
	"context"
	"fmt"

	"github.com/opendatahub-io/odh-cli/pkg/constants"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/validate"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
	"github.com/opendatahub-io/odh-cli/pkg/util/components"
	"github.com/opendatahub-io/odh-cli/pkg/util/version"
)

// ImpactedWorkloadsCheck detects TrustyAIService CRs in user namespaces.
// The user must back up TrustyAI data before upgrading, since the operator
// module adoption process may alter the pod/volume configuration.
type ImpactedWorkloadsCheck struct {
	check.BaseCheck
}

func NewImpactedWorkloadsCheck() *ImpactedWorkloadsCheck {
	return &ImpactedWorkloadsCheck{
		BaseCheck: check.BaseCheck{
			CheckGroup:       check.GroupWorkload,
			Kind:             kind,
			Type:             check.CheckTypeImpactedWorkloads,
			CheckID:          "workloads.trustyai.impacted-workloads",
			CheckName:        "Workloads :: TrustyAI :: Impacted Workloads (3.x)",
			CheckDescription: "Detects TrustyAIService CRs that require a data backup before upgrading to RHOAI 3.x",
			CheckRemediation: "Run 'odh migrate run trustyai.migrate-data' to back up TrustyAI data before upgrading",
		},
	}
}

// CanApply returns true when upgrading from 2.x to 3.x and TrustyAI is Managed.
func (c *ImpactedWorkloadsCheck) CanApply(ctx context.Context, target check.Target) (bool, error) {
	if !version.IsUpgradeFrom2xTo3x(target.CurrentVersion, target.TargetVersion) {
		return false, nil
	}

	dsc, err := client.GetDataScienceCluster(ctx, target.Client)
	if err != nil {
		return false, fmt.Errorf("getting DataScienceCluster: %w", err)
	}

	return components.HasManagementState(dsc, "trustyai", constants.ManagementStateManaged), nil
}

// Validate lists all TrustyAIService CRs and warns if any are found.
func (c *ImpactedWorkloadsCheck) Validate(
	ctx context.Context,
	target check.Target,
) (*result.DiagnosticResult, error) {
	return validate.Workloads(c, target, resources.TrustyAIService).
		Complete(ctx, c.newImpactedCondition)
}
