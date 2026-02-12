package dashboard

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/lburgazzoli/odh-cli/pkg/constants"
	"github.com/lburgazzoli/odh-cli/pkg/lint/check"
	"github.com/lburgazzoli/odh-cli/pkg/lint/check/result"
	"github.com/lburgazzoli/odh-cli/pkg/lint/check/validate"
	"github.com/lburgazzoli/odh-cli/pkg/resources"
	"github.com/lburgazzoli/odh-cli/pkg/util/version"
)

// AcceleratorProfileMigrationCheck detects legacy AcceleratorProfiles that will be auto-migrated to
// HardwareProfiles (infrastructure.opendatahub.io) during upgrade to RHOAI 3.x.
type AcceleratorProfileMigrationCheck struct {
	check.BaseCheck
}

// NewAcceleratorProfileMigrationCheck creates a new AcceleratorProfileMigrationCheck instance.
func NewAcceleratorProfileMigrationCheck() *AcceleratorProfileMigrationCheck {
	return &AcceleratorProfileMigrationCheck{
		BaseCheck: check.BaseCheck{
			CheckGroup:       check.GroupComponent,
			Kind:             constants.ComponentDashboard,
			Type:             check.CheckTypeAcceleratorProfileMigration,
			CheckID:          "components.dashboard.acceleratorprofile-migration",
			CheckName:        "Components :: Dashboard :: AcceleratorProfile Migration (3.x)",
			CheckDescription: "Lists legacy AcceleratorProfiles that will be auto-migrated to HardwareProfiles (infrastructure.opendatahub.io) during upgrade",
			CheckRemediation: "Legacy AcceleratorProfiles will be automatically migrated to HardwareProfiles (infrastructure.opendatahub.io) during upgrade - no manual action required",
		},
	}
}

// CanApply returns whether this check should run for the given target.
// Only applies when upgrading from 2.x to 3.x.
func (c *AcceleratorProfileMigrationCheck) CanApply(_ context.Context, target check.Target) (bool, error) {
	return version.IsUpgradeFrom2xTo3x(target.CurrentVersion, target.TargetVersion), nil
}

// Validate executes the check against the provided target.
func (c *AcceleratorProfileMigrationCheck) Validate(
	ctx context.Context,
	target check.Target,
) (*result.DiagnosticResult, error) {
	return validate.WorkloadsMetadata(c, target, resources.AcceleratorProfile).
		Complete(ctx, c.newMigrationCondition)
}

func (c *AcceleratorProfileMigrationCheck) newMigrationCondition(
	_ context.Context,
	req *validate.WorkloadRequest[*metav1.PartialObjectMetadata],
) ([]result.Condition, error) {
	switch {
	case len(req.Items) == 0:
		return []result.Condition{check.NewCondition(
			check.ConditionTypeMigrationRequired,
			metav1.ConditionTrue,
			check.WithReason(check.ReasonNoMigrationRequired),
			check.WithMessage("No legacy AcceleratorProfiles found - no migration required"),
		)}, nil
	default:
		return []result.Condition{check.NewCondition(
			check.ConditionTypeMigrationRequired,
			metav1.ConditionFalse,
			check.WithReason(check.ReasonMigrationPending),
			check.WithMessage("Found %d legacy AcceleratorProfile(s) that will be automatically migrated to HardwareProfiles (infrastructure.opendatahub.io) during upgrade", len(req.Items)),
			check.WithImpact(result.ImpactAdvisory),
			check.WithRemediation(c.CheckRemediation),
		)}, nil
	}
}
