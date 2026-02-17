package notebook

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/opendatahub-io/odh-cli/pkg/constants"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/validate"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/kube"
)

// ConditionTypeHardwareProfileCompatible indicates whether notebooks reference legacy hardware profiles.
const ConditionTypeHardwareProfileCompatible = "HardwareProfileCompatible"

// HardwareProfileMigrationCheck detects Notebook CRs carrying the legacy
// opendatahub.io/legacy-hardware-profile-name annotation that may need attention.
type HardwareProfileMigrationCheck struct {
	check.BaseCheck
}

// NewHardwareProfileMigrationCheck creates a new HardwareProfileMigrationCheck.
func NewHardwareProfileMigrationCheck() *HardwareProfileMigrationCheck {
	return &HardwareProfileMigrationCheck{
		BaseCheck: check.BaseCheck{
			CheckGroup:       check.GroupWorkload,
			Kind:             kind,
			Type:             check.CheckTypeConfigMigration,
			CheckID:          "workloads.notebook.hardwareprofile-migration",
			CheckName:        "Workloads :: Notebook :: Legacy HardwareProfile Migration",
			CheckDescription: "Detects Notebook CRs carrying the legacy opendatahub.io/legacy-hardware-profile-name annotation that may need attention",
			CheckRemediation: "Update Notebooks to use current HardwareProfiles and remove the legacy-hardware-profile-name annotation",
		},
	}
}

// CanApply returns whether this check should run for the given target.
// Applies in all modes (lint and upgrade).
func (c *HardwareProfileMigrationCheck) CanApply(_ context.Context, _ check.Target) (bool, error) {
	return true, nil
}

// Validate executes the check against the provided target.
func (c *HardwareProfileMigrationCheck) Validate(
	ctx context.Context,
	target check.Target,
) (*result.DiagnosticResult, error) {
	return validate.WorkloadsMetadata(c, target, resources.Notebook).
		Filter(hasLegacyHardwareProfileAnnotation).
		Complete(ctx, c.newCondition)
}

// hasLegacyHardwareProfileAnnotation returns true when the object has a non-empty
// opendatahub.io/legacy-hardware-profile-name annotation.
func hasLegacyHardwareProfileAnnotation(obj *metav1.PartialObjectMetadata) (bool, error) {
	return kube.GetAnnotation(obj, constants.AnnotationLegacyHardwareProfile) != "", nil
}

func (c *HardwareProfileMigrationCheck) newCondition(
	_ context.Context,
	req *validate.WorkloadRequest[*metav1.PartialObjectMetadata],
) ([]result.Condition, error) {
	count := len(req.Items)

	if count == 0 {
		return []result.Condition{check.NewCondition(
			ConditionTypeHardwareProfileCompatible,
			metav1.ConditionTrue,
			check.WithReason(check.ReasonNoMigrationRequired),
			check.WithMessage("No Notebooks found with legacy hardware profile annotation - no migration needed"),
		)}, nil
	}

	return []result.Condition{check.NewCondition(
		ConditionTypeHardwareProfileCompatible,
		metav1.ConditionFalse,
		check.WithReason(check.ReasonMigrationPending),
		check.WithMessage("Found %d Notebook(s) with legacy hardware profile annotation that may need attention", count),
		check.WithImpact(result.ImpactAdvisory),
		check.WithRemediation(c.CheckRemediation),
	)}, nil
}
