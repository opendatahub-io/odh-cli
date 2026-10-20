// Package modelregistry provides lint checks for the Model Registry component.
package modelregistry

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/opendatahub-io/odh-cli/pkg/constants"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/validate"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
	"github.com/opendatahub-io/odh-cli/pkg/util/components"
	"github.com/opendatahub-io/odh-cli/pkg/util/version"
)

const (
	kind              = "modelregistry"
	checkTypeRenaming = "renaming"

	msgRenaming = "Model Registry component (state: %s) is renamed to AI Hub in DSC v3, available from RHOAI 3.6"

	remediationRenaming = "No action required - DSC v2 compatibility is retained. " +
		"From RHOAI 3.6 onward, use DSC v3 '.spec.components.aiHub' instead of " +
		"'.spec.components.modelregistry' in new or updated configuration"
)

// RenamingCheck informs users that the Model Registry component is renamed to
// AI Hub in DSC v3 (RHOAI 3.6+). DSC v2 compatibility is retained, so the
// reported condition is informational only and requires no migration action.
type RenamingCheck struct {
	check.BaseCheck
}

func NewRenamingCheck() *RenamingCheck {
	return &RenamingCheck{
		BaseCheck: check.BaseCheck{
			CheckGroup:       check.GroupComponent,
			Kind:             kind,
			Type:             checkTypeRenaming,
			CheckID:          "components.modelregistry.renaming",
			CheckName:        "Components :: Model Registry :: AI Hub Renaming (DSC v3)",
			CheckDescription: "Informs about Model Registry component renaming to AI Hub in DSC v3 (RHOAI 3.6+)",
			CheckRemediation: remediationRenaming,
		},
	}
}

// CanApply returns whether this check should run for the given target.
// This check applies when upgrading to a RHOAI 3.6+ target within major 3 from
// a different major/minor version, and the Model Registry component is Managed
// under the v2 component key. Any other state skips the check entirely, so no
// result row is produced for unconfigured components.
func (c *RenamingCheck) CanApply(ctx context.Context, target check.Target) (bool, error) {
	if target.CurrentVersion == nil || target.TargetVersion == nil {
		return false, nil
	}
	if !target.CurrentVersion.LT(*target.TargetVersion) {
		return false, nil
	}
	if version.SameMajorMinor(target.CurrentVersion, target.TargetVersion) {
		return false, nil
	}
	if !version.IsVersion3x(target.TargetVersion) {
		return false, nil
	}
	if !version.IsVersionAtLeast(target.TargetVersion, 3, 6) { //nolint:mnd // RHOAI 3.6 introduces DSC v3
		return false, nil
	}

	dsc, err := client.GetDataScienceCluster(ctx, target.Client)
	if err != nil {
		return false, fmt.Errorf("getting DataScienceCluster: %w", err)
	}

	return components.HasManagementState(dsc, kind, constants.ManagementStateManaged), nil
}

// Validate executes the check against the provided target.
func (c *RenamingCheck) Validate(ctx context.Context, target check.Target) (*result.DiagnosticResult, error) {
	return validate.Component(c, target).
		InState(constants.ManagementStateManaged).
		Complete(ctx, c.newRenamingCondition)
}

// newRenamingCondition reports the rename informationally: DSC v2 compatibility
// is retained, so the condition passes with no impact.
func (c *RenamingCheck) newRenamingCondition(_ context.Context, req *validate.ComponentRequest) ([]result.Condition, error) {
	return []result.Condition{
		check.NewCondition(
			check.ConditionTypeCompatible,
			metav1.ConditionTrue,
			check.WithReason(check.ReasonComponentRenamed),
			check.WithMessage(msgRenaming, req.ManagementState),
			check.WithRemediation(c.CheckRemediation),
		),
	}, nil
}
