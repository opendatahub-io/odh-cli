package notebook

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/opendatahub-io/odh-cli/pkg/constants"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/validate"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/version"
)

// RunningWorkloadsCheck detects Notebook CRs that are not in a stopped state.
// A Notebook is considered running when it does not have the kubeflow-resource-stopped annotation.
type RunningWorkloadsCheck struct {
	check.BaseCheck
	check.EnhancedVerboseFormatter
}

func NewRunningWorkloadsCheck() *RunningWorkloadsCheck {
	return &RunningWorkloadsCheck{
		BaseCheck: check.BaseCheck{
			CheckGroup:       check.GroupWorkload,
			Kind:             kind,
			Type:             check.CheckTypeWorkloadState,
			CheckID:          "workloads.notebook.running-workloads",
			CheckName:        "Workloads :: Notebook :: Running Workloads",
			CheckDescription: "Detects Notebook CRs that are currently running (not stopped) on the cluster",
			CheckRemediation: "Save all pending work in running Notebooks, then stop them before upgrading",
		},
	}
}

// CanApply returns whether this check should run for the given target.
// Only applies when upgrading from 2.x to 3.x; component state is checked via ForComponent in Validate.
func (c *RunningWorkloadsCheck) CanApply(_ context.Context, target check.Target) (bool, error) {
	return version.IsUpgradeFrom2xTo3x(target.CurrentVersion, target.TargetVersion), nil
}

// Validate lists all Notebooks and reports an advisory for any that are not stopped.
func (c *RunningWorkloadsCheck) Validate(
	ctx context.Context,
	target check.Target,
) (*result.DiagnosticResult, error) {
	return validate.WorkloadsMetadata(c, target, resources.Notebook).
		ForComponent(constants.ComponentWorkbenches).
		Filter(isRunning).
		Complete(ctx, c.newRunningWorkloadsCondition)
}

// isRunning returns true when the Notebook does not have the kubeflow-resource-stopped annotation.
func isRunning(nb *metav1.PartialObjectMetadata) (bool, error) {
	annotations := nb.GetAnnotations()
	if annotations == nil {
		return true, nil
	}

	_, stopped := annotations[AnnotationKubeflowResourceStopped]

	return !stopped, nil
}

func (c *RunningWorkloadsCheck) newRunningWorkloadsCondition(
	_ context.Context,
	req *validate.WorkloadRequest[*metav1.PartialObjectMetadata],
) ([]result.Condition, error) {
	count := len(req.Items)

	if count == 0 {
		return []result.Condition{
			check.NewCondition(
				ConditionTypeRunningWorkloads,
				metav1.ConditionTrue,
				check.WithReason(check.ReasonRequirementsMet),
				check.WithMessage(MsgAllNotebooksStopped),
			),
		}, nil
	}

	return []result.Condition{
		check.NewCondition(
			ConditionTypeRunningWorkloads,
			metav1.ConditionFalse,
			check.WithReason(check.ReasonWorkloadsImpacted),
			check.WithMessage(MsgRunningNotebooksFound, count),
			check.WithImpact(result.ImpactAdvisory),
			check.WithRemediation(c.CheckRemediation),
		),
	}, nil
}
