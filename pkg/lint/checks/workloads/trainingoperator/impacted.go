package trainingoperator

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"

	"github.com/lburgazzoli/odh-cli/pkg/lint/check"
	"github.com/lburgazzoli/odh-cli/pkg/lint/check/result"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/shared/base"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/shared/results"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/shared/validate"
	"github.com/lburgazzoli/odh-cli/pkg/resources"
	"github.com/lburgazzoli/odh-cli/pkg/util/version"
)

const (
	ConditionTypePyTorchJobsCompatible = "PyTorchJobsCompatible"
)

type ImpactedWorkloadsCheck struct {
	base.BaseCheck
}

func NewImpactedWorkloadsCheck() *ImpactedWorkloadsCheck {
	return &ImpactedWorkloadsCheck{
		BaseCheck: base.BaseCheck{
			CheckGroup:       check.GroupWorkload,
			Kind:             check.ComponentTrainingOperator,
			Type:             check.CheckTypeImpactedWorkloads,
			CheckID:          "workloads.trainingoperator.impacted-workloads",
			CheckName:        "Workloads :: TrainingOperator :: Impacted Workloads (3.3+)",
			CheckDescription: "Lists PyTorchJobs using deprecated TrainingOperator (Kubeflow v1) that will be impacted by transition to Trainer v2",
		},
	}
}

func (c *ImpactedWorkloadsCheck) CanApply(_ context.Context, target check.Target) bool {
	//nolint:mnd // Version numbers 3.3
	return version.IsVersionAtLeast(target.TargetVersion, 3, 3)
}

func (c *ImpactedWorkloadsCheck) Validate(
	ctx context.Context,
	target check.Target,
) (*result.DiagnosticResult, error) {
	return validate.Workloads(c, target, resources.PyTorchJob).
		Run(ctx, func(_ context.Context, req *validate.WorkloadRequest[*unstructured.Unstructured]) error {
			var active, completed []types.NamespacedName

			for _, job := range req.Items {
				nsName := types.NamespacedName{
					Namespace: job.GetNamespace(),
					Name:      job.GetName(),
				}

				done, err := isJobCompleted(job)
				if err != nil {
					return fmt.Errorf("checking job %s/%s completion: %w", nsName.Namespace, nsName.Name, err)
				}

				if done {
					completed = append(completed, nsName)
				} else {
					active = append(active, nsName)
				}
			}

			results.SetCondition(req.Result, c.newPyTorchJobCondition(len(active), len(completed)))

			return nil
		})
}
