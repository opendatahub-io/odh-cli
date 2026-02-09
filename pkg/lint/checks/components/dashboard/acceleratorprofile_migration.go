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

const (
	acceleratorProfileCheckType = "acceleratorprofile-migration"

	// minMigrationMajorVersion is the minimum major version for this check to apply.
	minMigrationMajorVersion = 3
)

// AcceleratorProfileMigrationCheck detects AcceleratorProfiles that will be auto-migrated to HardwareProfiles
// during upgrade to RHOAI 3.x.
type AcceleratorProfileMigrationCheck struct {
	base.BaseCheck
}

// NewAcceleratorProfileMigrationCheck creates a new AcceleratorProfileMigrationCheck instance.
func NewAcceleratorProfileMigrationCheck() *AcceleratorProfileMigrationCheck {
	return &AcceleratorProfileMigrationCheck{
		BaseCheck: base.BaseCheck{
			CheckGroup:       check.GroupComponent,
			Kind:             check.ComponentDashboard,
			Type:             acceleratorProfileCheckType,
			CheckID:          "components.dashboard.acceleratorprofile-migration",
			CheckName:        "Components :: Dashboard :: AcceleratorProfile Migration (3.x)",
			CheckDescription: "Lists AcceleratorProfiles that will be auto-migrated to HardwareProfiles during upgrade",
			CheckRemediation: "AcceleratorProfiles will be automatically migrated to HardwareProfiles during upgrade - no manual action required",
		},
	}
}

// CanApply returns whether this check should run for the given target.
// Only applies when upgrading to 3.x or later.
func (c *AcceleratorProfileMigrationCheck) CanApply(_ context.Context, target check.Target) bool {
	return version.IsVersionAtLeast(target.TargetVersion, minMigrationMajorVersion, 0)
}

// Validate executes the check against the provided target.
func (c *AcceleratorProfileMigrationCheck) Validate(
	ctx context.Context,
	target check.Target,
) (*result.DiagnosticResult, error) {
	dr := c.NewResult()

	err := migration.ValidateResources(ctx, target, dr, migration.Config{
		ResourceType:            resources.AcceleratorProfile,
		ResourceLabel:           "AcceleratorProfile",
		NoMigrationMessage:      "No AcceleratorProfiles found - no migration required",
		MigrationPendingMessage: "Found %d AcceleratorProfile(s) that will be automatically migrated to HardwareProfiles during upgrade",
		Remediation:             c.CheckRemediation,
	})
	if err != nil {
		return nil, fmt.Errorf("validating AcceleratorProfile migration: %w", err)
	}

	return dr, nil
}
