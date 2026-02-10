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
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/shared/components"
	"github.com/lburgazzoli/odh-cli/pkg/resources"
	"github.com/lburgazzoli/odh-cli/pkg/util/client"
	"github.com/lburgazzoli/odh-cli/pkg/util/version"
)

const ConditionTypeISVCAcceleratorProfileCompatible = "AcceleratorProfileCompatible"

// AcceleratorMigrationCheck detects InferenceService CRs referencing legacy AcceleratorProfiles
// that will be auto-migrated to HardwareProfiles (infrastructure.opendatahub.io) during RHOAI 3.x upgrade.
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
			CheckDescription: "Detects InferenceService CRs referencing legacy AcceleratorProfiles that will be auto-migrated to HardwareProfiles (infrastructure.opendatahub.io) during upgrade",
			CheckRemediation: "Legacy AcceleratorProfiles will be automatically migrated to HardwareProfiles (infrastructure.opendatahub.io) during upgrade - no manual action required",
		},
	}
}

// CanApply returns whether this check should run for the given target.
// Only applies when upgrading from 2.x to 3.x and KServe or ModelMesh is Managed.
func (c *AcceleratorMigrationCheck) CanApply(ctx context.Context, target check.Target) (bool, error) {
	if !version.IsUpgradeFrom2xTo3x(target.CurrentVersion, target.TargetVersion) {
		return false, nil
	}

	dsc, err := client.GetDataScienceCluster(ctx, target.Client)
	if err != nil {
		return false, fmt.Errorf("getting DataScienceCluster: %w", err)
	}

	return components.HasManagementState(dsc, check.ComponentKServe, check.ManagementStateManaged) ||
		components.HasManagementState(dsc, "modelmeshserving", check.ManagementStateManaged), nil
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
		dr.SetImpactedObjects(resources.InferenceService, impacted)
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
			check.WithReason(check.ReasonVersionCompatible),
			check.WithMessage("No InferenceServices found using legacy AcceleratorProfiles - no migration needed"),
		)
	}

	// If there are missing profiles, this is a blocking issue
	if totalMissing > 0 {
		return check.NewCondition(
			ConditionTypeISVCAcceleratorProfileCompatible,
			metav1.ConditionFalse,
			check.WithReason(check.ReasonResourceNotFound),
			check.WithMessage("Found %d InferenceService(s) referencing legacy AcceleratorProfiles (%d missing): AcceleratorProfiles and InferenceService references are automatically migrated to HardwareProfiles (infrastructure.opendatahub.io) during upgrade", totalImpacted, totalMissing),
			check.WithImpact(result.ImpactAdvisory),
			check.WithRemediation(c.CheckRemediation),
		)
	}

	// All referenced profiles exist - advisory only
	return check.NewCondition(
		ConditionTypeISVCAcceleratorProfileCompatible,
		metav1.ConditionFalse,
		check.WithReason(check.ReasonConfigurationInvalid),
		check.WithMessage("Found %d InferenceService(s) using legacy AcceleratorProfiles: AcceleratorProfiles and InferenceService references are automatically migrated to HardwareProfiles (infrastructure.opendatahub.io) during upgrade", totalImpacted),
		check.WithImpact(result.ImpactAdvisory),
		check.WithRemediation(c.CheckRemediation),
	)
}
