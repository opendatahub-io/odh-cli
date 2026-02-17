package kserve

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/opendatahub-io/odh-cli/pkg/constants"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/validate"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
	"github.com/opendatahub-io/odh-cli/pkg/util/components"
	"github.com/opendatahub-io/odh-cli/pkg/util/kube"
)

// ConditionTypeISVCHardwareProfileCompatible indicates whether InferenceServices reference legacy hardware profiles.
const ConditionTypeISVCHardwareProfileCompatible = "HardwareProfileCompatible"

// HardwareProfileMigrationCheck detects InferenceService CRs carrying the legacy
// opendatahub.io/legacy-hardware-profile-name annotation that may need attention.
type HardwareProfileMigrationCheck struct {
	check.BaseCheck
}

// NewHardwareProfileMigrationCheck creates a new HardwareProfileMigrationCheck.
func NewHardwareProfileMigrationCheck() *HardwareProfileMigrationCheck {
	return &HardwareProfileMigrationCheck{
		BaseCheck: check.BaseCheck{
			CheckGroup:       check.GroupWorkload,
			Kind:             constants.ComponentKServe,
			Type:             check.CheckTypeConfigMigration,
			CheckID:          "workloads.kserve.hardwareprofile-migration",
			CheckName:        "Workloads :: KServe :: Legacy HardwareProfile Migration",
			CheckDescription: "Detects InferenceService CRs carrying the legacy opendatahub.io/legacy-hardware-profile-name annotation that may need attention",
			CheckRemediation: "Update InferenceServices to use current HardwareProfiles and remove the legacy-hardware-profile-name annotation",
		},
	}
}

// CanApply returns whether this check should run for the given target.
// Only applies when KServe is in a Managed state.
func (c *HardwareProfileMigrationCheck) CanApply(ctx context.Context, target check.Target) (bool, error) {
	dsc, err := client.GetDataScienceCluster(ctx, target.Client)
	if err != nil {
		return false, fmt.Errorf("getting DataScienceCluster: %w", err)
	}

	return components.HasManagementState(dsc, constants.ComponentKServe, constants.ManagementStateManaged), nil
}

// Validate executes the check against the provided target.
func (c *HardwareProfileMigrationCheck) Validate(
	ctx context.Context,
	target check.Target,
) (*result.DiagnosticResult, error) {
	return validate.WorkloadsMetadata(c, target, resources.InferenceService).
		Filter(hasISVCLegacyHardwareProfileAnnotation).
		Complete(ctx, c.newCondition)
}

// hasISVCLegacyHardwareProfileAnnotation returns true when the object has a non-empty
// opendatahub.io/legacy-hardware-profile-name annotation.
func hasISVCLegacyHardwareProfileAnnotation(obj *metav1.PartialObjectMetadata) (bool, error) {
	return kube.GetAnnotation(obj, constants.AnnotationLegacyHardwareProfile) != "", nil
}

func (c *HardwareProfileMigrationCheck) newCondition(
	_ context.Context,
	req *validate.WorkloadRequest[*metav1.PartialObjectMetadata],
) ([]result.Condition, error) {
	count := len(req.Items)

	if count == 0 {
		return []result.Condition{check.NewCondition(
			ConditionTypeISVCHardwareProfileCompatible,
			metav1.ConditionTrue,
			check.WithReason(check.ReasonNoMigrationRequired),
			check.WithMessage("No InferenceServices found with legacy hardware profile annotation - no migration needed"),
		)}, nil
	}

	return []result.Condition{check.NewCondition(
		ConditionTypeISVCHardwareProfileCompatible,
		metav1.ConditionFalse,
		check.WithReason(check.ReasonMigrationPending),
		check.WithMessage("Found %d InferenceService(s) with legacy hardware profile annotation that may need attention", count),
		check.WithImpact(result.ImpactAdvisory),
		check.WithRemediation(c.CheckRemediation),
	)}, nil
}
