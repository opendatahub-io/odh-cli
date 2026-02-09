package notebook

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/lburgazzoli/odh-cli/pkg/lint/check"
	"github.com/lburgazzoli/odh-cli/pkg/lint/check/result"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/shared/base"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/shared/validate"
	"github.com/lburgazzoli/odh-cli/pkg/resources"
	"github.com/lburgazzoli/odh-cli/pkg/util/version"
)

const (
	kind = "notebook"

	ConditionTypeNotebooksCompatible = "NotebooksCompatible"
)

// ImpactedWorkloadsCheck lists Notebook (workbench) instances that will be impacted by deprecation.
type ImpactedWorkloadsCheck struct {
	base.BaseCheck
}

func NewImpactedWorkloadsCheck() *ImpactedWorkloadsCheck {
	return &ImpactedWorkloadsCheck{
		BaseCheck: base.BaseCheck{
			CheckGroup:       check.GroupWorkload,
			Kind:             kind,
			Type:             check.CheckTypeImpactedWorkloads,
			CheckID:          "workloads.notebook.impacted-workloads",
			CheckName:        "Workloads :: Notebook :: Impacted Workloads (3.x)",
			CheckDescription: "Lists Notebook (workbench) instances that will be impacted in RHOAI 3.x",
			CheckRemediation: "Stop all running Notebook workbenches and back up any unsaved work before upgrading",
		},
	}
}

// CanApply returns whether this check should run for the given target.
// Only applies when upgrading FROM 2.x TO 3.x since Notebook workloads may be impacted.
func (c *ImpactedWorkloadsCheck) CanApply(_ context.Context, target check.Target) (bool, error) {
	return version.IsUpgradeFrom2xTo3x(target.CurrentVersion, target.TargetVersion), nil
}

// Validate executes the check against the provided target.
func (c *ImpactedWorkloadsCheck) Validate(
	ctx context.Context,
	target check.Target,
) (*result.DiagnosticResult, error) {
	return validate.WorkloadsMetadata(c, target, resources.Notebook).
		Complete(ctx, c.newNotebookCondition)
}

func (c *ImpactedWorkloadsCheck) newNotebookCondition(
	_ context.Context,
	req *validate.WorkloadRequest[*metav1.PartialObjectMetadata],
) ([]result.Condition, error) {
	count := len(req.Items)

	if count == 0 {
		return []result.Condition{check.NewCondition(
			ConditionTypeNotebooksCompatible,
			metav1.ConditionTrue,
			check.ReasonVersionCompatible,
			"No Notebooks found - no workloads impacted by deprecation",
		)}, nil
	}

	return []result.Condition{check.NewCondition(
		ConditionTypeNotebooksCompatible,
		metav1.ConditionFalse,
		check.ReasonWorkloadsImpacted,
		"Found %d Notebook(s) - workloads will be impacted in RHOAI 3.x",
		count,
		check.WithImpact(result.ImpactAdvisory),
		check.WithRemediation(c.CheckRemediation),
	)}, nil
}
