package kserve

import (
	"context"
	"fmt"
	"strconv"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/lburgazzoli/odh-cli/pkg/lint/check"
	"github.com/lburgazzoli/odh-cli/pkg/lint/check/result"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/shared/accelerator"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/shared/base"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/shared/results"
	"github.com/lburgazzoli/odh-cli/pkg/resources"
	"github.com/lburgazzoli/odh-cli/pkg/util/version"
)

const ConditionTypeISVCAcceleratorProfileCompatible = "AcceleratorProfileCompatible"

// AcceleratorMigrationCheck detects InferenceService CRs referencing AcceleratorProfiles
// that need to be migrated to HardwareProfiles in RHOAI 3.x.
type AcceleratorMigrationCheck struct {
	base.BaseCheck
}

func NewAcceleratorMigrationCheck() *AcceleratorMigrationCheck {
	return &AcceleratorMigrationCheck{
		BaseCheck: base.BaseCheck{
			CheckGroup:       check.GroupWorkload,
			Kind:             check.ComponentKServe,
			Type:             check.CheckTypeImpactedWorkloads,
			CheckID:          "workloads.kserve.accelerator-migration",
			CheckName:        "Workloads :: KServe :: AcceleratorProfile Migration (3.x)",
			CheckDescription: "Detects InferenceService CRs referencing AcceleratorProfiles that need migration to HardwareProfiles",
			CheckRemediation: "Migrate AcceleratorProfiles to HardwareProfiles before upgrading to RHOAI 3.x",
		},
	}
}

// CanApply returns whether this check should run for the given target.
// Only applies when upgrading from 2.x to 3.x.
func (c *AcceleratorMigrationCheck) CanApply(_ context.Context, target check.Target) bool {
	return version.IsUpgradeFrom2xTo3x(target.CurrentVersion, target.TargetVersion)
}

// Validate executes the check against the provided target.
func (c *AcceleratorMigrationCheck) Validate(
	ctx context.Context,
	target check.Target,
) (*result.DiagnosticResult, error) {
	dr := c.NewResult()

	if target.TargetVersion != nil {
		dr.Annotations[check.AnnotationCheckTargetVersion] = target.TargetVersion.String()
	}

	impacted, missingCount, err := accelerator.FindWorkloadsWithAcceleratorRefs(ctx, target, resources.InferenceService)
	if err != nil {
		return nil, fmt.Errorf("finding InferenceServices with AcceleratorProfiles: %w", err)
	}

	totalImpacted := len(impacted)
	dr.Annotations[check.AnnotationImpactedWorkloadCount] = strconv.Itoa(totalImpacted)

	dr.Status.Conditions = append(
		dr.Status.Conditions,
		c.newISVCAcceleratorMigrationCondition(totalImpacted, missingCount),
	)

	if totalImpacted > 0 {
		results.PopulateImpactedObjects(dr, resources.InferenceService, impacted)
	}

	return dr, nil
}

func (c *AcceleratorMigrationCheck) newISVCAcceleratorMigrationCondition(
	totalImpacted int,
	totalMissing int,
) result.Condition {
	if totalImpacted == 0 {
		return check.NewCondition(
			ConditionTypeISVCAcceleratorProfileCompatible,
			metav1.ConditionTrue,
			check.ReasonVersionCompatible,
			"No InferenceServices found using AcceleratorProfiles - no migration needed",
		)
	}

	// If there are missing profiles, this is a blocking issue
	if totalMissing > 0 {
		return check.NewCondition(
			ConditionTypeISVCAcceleratorProfileCompatible,
			metav1.ConditionFalse,
			check.ReasonResourceNotFound,
			"Found %d InferenceService(s) referencing AcceleratorProfiles (%d missing) - ensure AcceleratorProfiles exist and migrate to HardwareProfiles",
			totalImpacted,
			totalMissing,
			check.WithImpact(result.ImpactAdvisory),
			check.WithRemediation(c.CheckRemediation),
		)
	}

	// All referenced profiles exist - advisory only
	return check.NewCondition(
		ConditionTypeISVCAcceleratorProfileCompatible,
		metav1.ConditionFalse,
		check.ReasonConfigurationInvalid,
		"Found %d InferenceService(s) using AcceleratorProfiles - migrate to HardwareProfiles before upgrading",
		totalImpacted,
		check.WithImpact(result.ImpactAdvisory),
		check.WithRemediation(c.CheckRemediation),
	)
}
