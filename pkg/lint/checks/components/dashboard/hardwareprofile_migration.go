package dashboard

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/opendatahub-io/odh-cli/pkg/constants"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/validate"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/version"
)

const hardwareProfileCheckType = "hardwareprofile-migration"

// HardwareProfileMigrationCheck detects legacy HardwareProfiles (opendatahub.io) that will be
// auto-migrated to HardwareProfiles (infrastructure.opendatahub.io) during upgrade to RHOAI 3.x.
type HardwareProfileMigrationCheck struct {
	check.BaseCheck
}

// NewHardwareProfileMigrationCheck creates a new HardwareProfileMigrationCheck instance.
func NewHardwareProfileMigrationCheck() *HardwareProfileMigrationCheck {
	return &HardwareProfileMigrationCheck{
		BaseCheck: check.BaseCheck{
			CheckGroup:       check.GroupComponent,
			Kind:             constants.ComponentDashboard,
			Type:             hardwareProfileCheckType,
			CheckID:          "components.dashboard.hardwareprofile-migration",
			CheckName:        "Components :: Dashboard :: HardwareProfile Migration (3.x)",
			CheckDescription: "Lists legacy HardwareProfiles (opendatahub.io) that will be auto-migrated to HardwareProfiles (infrastructure.opendatahub.io) during upgrade",
			CheckRemediation: "Legacy HardwareProfiles will be automatically migrated to HardwareProfiles (infrastructure.opendatahub.io) during upgrade - no manual action required",
		},
	}
}

// CanApply returns whether this check should run for the given target.
// Only applies when upgrading from 2.x to 3.x.
func (c *HardwareProfileMigrationCheck) CanApply(_ context.Context, target check.Target) (bool, error) {
	return version.IsUpgradeFrom2xTo3x(target.CurrentVersion, target.TargetVersion), nil
}

// Validate executes the check against the provided target.
func (c *HardwareProfileMigrationCheck) Validate(
	ctx context.Context,
	target check.Target,
) (*result.DiagnosticResult, error) {
	return validate.WorkloadsMetadata(c, target, resources.HardwareProfile).
		Complete(ctx, c.newMigrationCondition)
}

func (c *HardwareProfileMigrationCheck) newMigrationCondition(
	_ context.Context,
	req *validate.WorkloadRequest[*metav1.PartialObjectMetadata],
) ([]result.Condition, error) {
	switch {
	case len(req.Items) == 0:
		return []result.Condition{check.NewCondition(
			check.ConditionTypeMigrationRequired,
			metav1.ConditionTrue,
			check.WithReason(check.ReasonNoMigrationRequired),
			check.WithMessage("No legacy HardwareProfiles found in opendatahub.io API group - no migration required"),
		)}, nil
	default:
		return []result.Condition{check.NewCondition(
			check.ConditionTypeMigrationRequired,
			metav1.ConditionFalse,
			check.WithReason(check.ReasonMigrationPending),
			check.WithMessage("Found %d legacy HardwareProfile(s) (opendatahub.io) that will be automatically migrated to HardwareProfiles (infrastructure.opendatahub.io) during upgrade", len(req.Items)),
			check.WithImpact(result.ImpactAdvisory),
			check.WithRemediation(c.CheckRemediation),
		)}, nil
	}
}
