package dashboard

import (
	"context"
	"fmt"

	"github.com/lburgazzoli/odh-cli/pkg/lint/check"
	"github.com/lburgazzoli/odh-cli/pkg/lint/check/result"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/shared/base"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/shared/migration"
	"github.com/lburgazzoli/odh-cli/pkg/resources"
	"github.com/lburgazzoli/odh-cli/pkg/util/version"
)

const hardwareProfileCheckType = "hardwareprofile-migration"

// HardwareProfileMigrationCheck detects HardwareProfiles in the opendatahub.io API group that will be
// auto-migrated to infrastructure.opendatahub.io during upgrade to RHOAI 3.x.
type HardwareProfileMigrationCheck struct {
	base.BaseCheck
}

// NewHardwareProfileMigrationCheck creates a new HardwareProfileMigrationCheck instance.
func NewHardwareProfileMigrationCheck() *HardwareProfileMigrationCheck {
	return &HardwareProfileMigrationCheck{
		BaseCheck: base.BaseCheck{
			CheckGroup:       check.GroupComponent,
			Kind:             check.ComponentDashboard,
			Type:             hardwareProfileCheckType,
			CheckID:          "components.dashboard.hardwareprofile-migration",
			CheckName:        "Components :: Dashboard :: HardwareProfile Migration (3.x)",
			CheckDescription: "Lists HardwareProfiles that will be auto-migrated from opendatahub.io to infrastructure.opendatahub.io during upgrade",
			CheckRemediation: "HardwareProfiles will be automatically migrated during upgrade - no manual action required",
		},
	}
}

// CanApply returns whether this check should run for the given target.
// Only applies when upgrading to 3.x or later.
func (c *HardwareProfileMigrationCheck) CanApply(_ context.Context, target check.Target) bool {
	return version.IsVersionAtLeast(target.TargetVersion, minMigrationMajorVersion, 0)
}

// Validate executes the check against the provided target.
func (c *HardwareProfileMigrationCheck) Validate(
	ctx context.Context,
	target check.Target,
) (*result.DiagnosticResult, error) {
	dr := c.NewResult()

	err := migration.ValidateResources(ctx, target, dr, migration.Config{
		ResourceType:            resources.HardwareProfile,
		ResourceLabel:           "HardwareProfile",
		NoMigrationMessage:      "No HardwareProfiles found in opendatahub.io API group - no migration required",
		MigrationPendingMessage: "Found %d HardwareProfile(s) in opendatahub.io that will be automatically migrated to infrastructure.opendatahub.io during upgrade",
		Remediation:             c.CheckRemediation,
	})
	if err != nil {
		return nil, fmt.Errorf("validating HardwareProfile migration: %w", err)
	}

	return dr, nil
}
