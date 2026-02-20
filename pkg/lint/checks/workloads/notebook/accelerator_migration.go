package notebook

import (
	"context"
	"strconv"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/validate"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/version"
)

// AcceleratorMigrationCheck detects Notebook (workbench) CRs referencing deprecated AcceleratorProfiles
// that will be auto-migrated to HardwareProfiles (infrastructure.opendatahub.io) during RHOAI 3.x upgrade.
type AcceleratorMigrationCheck struct {
	check.BaseCheck
}

func NewAcceleratorMigrationCheck() *AcceleratorMigrationCheck {
	return &AcceleratorMigrationCheck{
		BaseCheck: check.BaseCheck{
			CheckGroup:       check.GroupWorkload,
			Kind:             kind,
			Type:             check.CheckTypeAcceleratorProfileMigration,
			CheckID:          "workloads.notebook.accelerator-migration",
			CheckName:        "Workloads :: Notebook :: AcceleratorProfile Migration (3.x)",
			CheckDescription: "Detects Notebook (workbench) CRs referencing deprecated AcceleratorProfiles that will be auto-migrated to HardwareProfiles (infrastructure.opendatahub.io) during upgrade",
			CheckRemediation: "Deprecated AcceleratorProfiles will be automatically migrated to HardwareProfiles (infrastructure.opendatahub.io) during upgrade - no manual action required",
		},
	}
}

// CanApply returns whether this check should run for the given target.
// Only applies when upgrading from 2.x to 3.x and Workbenches is Managed.
func (c *AcceleratorMigrationCheck) CanApply(ctx context.Context, target check.Target) (bool, error) {
	if !version.IsUpgradeFrom2xTo3x(target.CurrentVersion, target.TargetVersion) {
		return false, nil
	}

	return isWorkbenchesManaged(ctx, target)
}

// Validate executes the check against the provided target.
func (c *AcceleratorMigrationCheck) Validate(
	ctx context.Context,
	target check.Target,
) (*result.DiagnosticResult, error) {
	return validate.WorkloadsMetadata(c, target, resources.Notebook).
		Run(ctx, c.checkAcceleratorRefs)
}

// checkAcceleratorRefs cross-references notebook accelerator annotations against existing AcceleratorProfiles.
func (c *AcceleratorMigrationCheck) checkAcceleratorRefs(
	ctx context.Context,
	req *validate.WorkloadRequest[*metav1.PartialObjectMetadata],
) error {
	dr := req.Result

	impacted, missingCount, err := validate.FilterWorkloadsWithAcceleratorRefs(ctx, req.Client, req.Items)
	if err != nil {
		return err
	}

	totalImpacted := len(impacted)
	dr.Annotations[check.AnnotationImpactedWorkloadCount] = strconv.Itoa(totalImpacted)

	dr.Status.Conditions = append(
		dr.Status.Conditions,
		c.newAcceleratorMigrationCondition(totalImpacted, missingCount),
	)
	dr.SetImpactedObjects(resources.Notebook, impacted)

	return nil
}

func (c *AcceleratorMigrationCheck) newAcceleratorMigrationCondition(
	totalImpacted int,
	totalMissing int,
) result.Condition {
	if totalImpacted == 0 {
		return check.NewCondition(
			ConditionTypeAcceleratorProfileCompatible,
			metav1.ConditionTrue,
			check.WithReason(check.ReasonVersionCompatible),
			check.WithMessage(MsgNoAcceleratorProfiles),
		)
	}

	// If there are missing profiles, this is a blocking issue
	if totalMissing > 0 {
		return check.NewCondition(
			ConditionTypeAcceleratorProfileCompatible,
			metav1.ConditionFalse,
			check.WithReason(check.ReasonResourceNotFound),
			check.WithMessage(MsgAcceleratorProfilesMissing, totalImpacted, totalMissing),
			check.WithImpact(result.ImpactAdvisory),
			check.WithRemediation(c.CheckRemediation),
		)
	}

	// All referenced profiles exist - advisory only
	return check.NewCondition(
		ConditionTypeAcceleratorProfileCompatible,
		metav1.ConditionFalse,
		check.WithReason(check.ReasonMigrationPending),
		check.WithMessage(MsgAcceleratorProfilesMigrating, totalImpacted),
		check.WithImpact(result.ImpactAdvisory),
		check.WithRemediation(c.CheckRemediation),
	)
}
