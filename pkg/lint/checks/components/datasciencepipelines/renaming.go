package datasciencepipelines

import (
	"context"
	"fmt"

	"github.com/opendatahub-io/odh-cli/pkg/constants"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/validate"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
	"github.com/opendatahub-io/odh-cli/pkg/util/components"
	"github.com/opendatahub-io/odh-cli/pkg/util/version"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	kind              = "datasciencepipelines"
	checkTypeRenaming = "renaming"
)

type RenamingCheck struct {
	check.BaseCheck
}

func NewRenamingCheck() *RenamingCheck {
	return &RenamingCheck{
		BaseCheck: check.BaseCheck{
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
// This check only applies when upgrading FROM 2.x TO 3.x and DataSciencePipelines is Managed.
func (c *RenamingCheck) CanApply(ctx context.Context, target check.Target) (bool, error) {
	if !version.IsUpgradeFrom2xTo3x(target.CurrentVersion, target.TargetVersion) {
		return false, nil
	}

	dsc, err := client.GetDataScienceCluster(ctx, target.Client)
	if err != nil {
		return false, fmt.Errorf("getting DataScienceCluster: %w", err)
	}

	return components.HasManagementState(dsc, kind, constants.ManagementStateManaged), nil
}

func (c *RenamingCheck) Validate(ctx context.Context, target check.Target) (*result.DiagnosticResult, error) {
	return validate.Component(c, target).
		Complete(ctx, newRenamingCondition)
}

func newRenamingCondition(_ context.Context, req *validate.ComponentRequest) ([]result.Condition, error) {
	return []result.Condition{
		check.NewCondition(
			check.ConditionTypeCompatible,
			metav1.ConditionFalse,
			check.WithReason(check.ReasonComponentRenamed),
			check.WithMessage("DataSciencePipelines component (state: %s) will be renamed to AIPipelines in DSC v2 (RHOAI 3.x). The field path changes from '.spec.components.datasciencepipelines' to '.spec.components.aipipelines'", req.ManagementState),
			check.WithImpact(result.ImpactAdvisory),
			check.WithRemediation("No action required - the component will be automatically renamed. Update any automation referencing '.spec.components.datasciencepipelines' to use '.spec.components.aipipelines' after upgrade"),
		),
	}, nil
}
