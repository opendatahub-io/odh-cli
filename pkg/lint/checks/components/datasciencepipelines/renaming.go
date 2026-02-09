package datasciencepipelines

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/lburgazzoli/odh-cli/pkg/lint/check"
	"github.com/lburgazzoli/odh-cli/pkg/lint/check/result"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/shared/base"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/shared/validate"
	"github.com/lburgazzoli/odh-cli/pkg/util/version"
)

const (
	kind              = "datasciencepipelines"
	checkTypeRenaming = "renaming"
)

type RenamingCheck struct {
	base.BaseCheck
}

func NewRenamingCheck() *RenamingCheck {
	return &RenamingCheck{
		BaseCheck: base.BaseCheck{
			CheckGroup:       check.GroupComponent,
			Kind:             kind,
			Type:             checkTypeRenaming,
			CheckID:          "components.datasciencepipelines.renaming",
			CheckName:        "Components :: DataSciencePipelines :: Component Renaming (3.x)",
			CheckDescription: "Informs about DataSciencePipelines component renaming to AIPipelines in DSC v2 (RHOAI 3.x)",
			CheckRemediation: "No action required - the component will be automatically renamed. Update any automation referencing '.spec.components.datasciencepipelines' to use '.spec.components.aipipelines' after upgrade",
		},
	}
}

// CanApply returns whether this check should run for the given target.
// This check only applies when upgrading FROM 2.x TO 3.x.
func (c *RenamingCheck) CanApply(_ context.Context, target check.Target) (bool, error) {
	return version.IsUpgradeFrom2xTo3x(target.CurrentVersion, target.TargetVersion), nil
}

func (c *RenamingCheck) Validate(ctx context.Context, target check.Target) (*result.DiagnosticResult, error) {
	return validate.Component(c, target).
		InState(check.ManagementStateManaged).
		Complete(ctx, newRenamingCondition)
}

func newRenamingCondition(_ context.Context, req *validate.ComponentRequest) ([]result.Condition, error) {
	return []result.Condition{
		check.NewCondition(
			check.ConditionTypeCompatible,
			metav1.ConditionFalse,
			check.ReasonComponentRenamed,
			"DataSciencePipelines component (state: %s) will be renamed to AIPipelines in DSC v2 (RHOAI 3.x). The field path changes from '.spec.components.datasciencepipelines' to '.spec.components.aipipelines'",
			req.ManagementState,
			check.WithImpact(result.ImpactAdvisory),
			check.WithRemediation("No action required - the component will be automatically renamed. Update any automation referencing '.spec.components.datasciencepipelines' to use '.spec.components.aipipelines' after upgrade"),
		),
	}, nil
}
