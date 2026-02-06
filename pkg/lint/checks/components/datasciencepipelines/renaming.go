package datasciencepipelines

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/lburgazzoli/odh-cli/pkg/lint/check"
	"github.com/lburgazzoli/odh-cli/pkg/lint/check/result"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/shared/base"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/shared/results"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/shared/validate"
	"github.com/lburgazzoli/odh-cli/pkg/util/version"
)

type RenamingCheck struct {
	base.BaseCheck
}

func NewRenamingCheck() *RenamingCheck {
	return &RenamingCheck{
		BaseCheck: base.BaseCheck{
			CheckGroup:       check.GroupComponent,
			Kind:             check.ComponentDataSciencePipelines,
			Type:             check.CheckTypeRenaming,
			CheckID:          "components.datasciencepipelines.renaming",
			CheckName:        "Components :: DataSciencePipelines :: Component Renaming (3.x)",
			CheckDescription: "Informs about DataSciencePipelines component renaming to AIPipelines in DSC v2 (RHOAI 3.x)",
		},
	}
}

// CanApply returns whether this check should run for the given target.
// This check only applies when upgrading FROM 2.x TO 3.x.
func (c *RenamingCheck) CanApply(_ context.Context, target check.Target) bool {
	return version.IsUpgradeFrom2xTo3x(target.CurrentVersion, target.TargetVersion)
}

func (c *RenamingCheck) Validate(ctx context.Context, target check.Target) (*result.DiagnosticResult, error) {
	return validate.Component(c, target).
		InState(check.ManagementStateManaged).
		Run(ctx, func(_ context.Context, req *validate.ComponentRequest) error {
			results.SetCondition(req.Result, check.NewCondition(
				check.ConditionTypeCompatible,
				metav1.ConditionFalse,
				check.ReasonComponentRenamed,
				"DataSciencePipelines component (state: %s) will be renamed to AIPipelines in DSC v2 (RHOAI 3.x). The field path changes from '.spec.components.datasciencepipelines' to '.spec.components.aipipelines'",
				req.ManagementState,
				check.WithImpact(result.ImpactAdvisory),
			))

			return nil
		})
}
