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

// ScheduledMetricsBackupCheck warns when TrustyAIService CRs are present.
// Scheduled metrics are held in memory and are not persisted in the CRD; they
// are lost on pod restart and cannot be recovered after an upgrade.
type ScheduledMetricsBackupCheck struct {
	check.BaseCheck
}

func NewScheduledMetricsBackupCheck() *ScheduledMetricsBackupCheck {
	return &ScheduledMetricsBackupCheck{
		BaseCheck: check.BaseCheck{
			CheckGroup:       check.GroupWorkload,
			Kind:             kind,
			Type:             check.CheckTypeWorkloadState,
			CheckID:          "workloads.trustyai.scheduled-metrics-backup",
			CheckName:        "Workloads :: TrustyAI :: Scheduled Metrics Backup (3.x)",
			CheckDescription: "Warns when TrustyAIService instances are found because scheduled metrics are in-memory only and will be lost on pod restart during the upgrade",
			CheckRemediation: "Record all scheduled metrics via the TrustyAI REST API before upgrading, then re-register them after the upgrade completes",
		},
	}
}

// CanApply returns true when upgrading from 2.x to 3.x and TrustyAI is Managed.
func (c *ScheduledMetricsBackupCheck) CanApply(ctx context.Context, target check.Target) (bool, error) {
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
func (c *ScheduledMetricsBackupCheck) Validate(
	ctx context.Context,
	target check.Target,
) (*result.DiagnosticResult, error) {
	return validate.Workloads(c, target, resources.TrustyAIService).
		Complete(ctx, c.newScheduledMetricsCondition)
}
