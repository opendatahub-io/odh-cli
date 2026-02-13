package trainingoperator

import (
	"context"
	"fmt"

	"github.com/opendatahub-io/odh-cli/pkg/constants"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/validate"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
	"github.com/opendatahub-io/odh-cli/pkg/util/components"
	"github.com/opendatahub-io/odh-cli/pkg/util/version"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
)

const (
	ConditionTypePyTorchJobsCompatible = "PyTorchJobsCompatible"
)

type ImpactedWorkloadsCheck struct {
	check.BaseCheck
}

func NewImpactedWorkloadsCheck() *ImpactedWorkloadsCheck {
	return &ImpactedWorkloadsCheck{
		BaseCheck: check.BaseCheck{
			CheckGroup:       check.GroupWorkload,
			Kind:             constants.ComponentTrainingOperator,
			Type:             check.CheckTypeImpactedWorkloads,
			CheckID:          "workloads.trainingoperator.impacted-workloads",
			CheckName:        "Workloads :: TrainingOperator :: Impacted Workloads (3.3+)",
			CheckDescription: "Lists PyTorchJobs using deprecated TrainingOperator (Kubeflow v1) that will be impacted by transition to Trainer v2",
			CheckRemediation: "Complete or delete active PyTorchJobs before upgrading; plan migration to Trainer v2 API",
		},
	}
}

// CanApply returns whether this check should run for the given target.
// Only applies when target version >= 3.3 and TrainingOperator is Managed.
func (c *ImpactedWorkloadsCheck) CanApply(ctx context.Context, target check.Target) (bool, error) {
	//nolint:mnd // Version numbers 3.3
	if !version.IsVersionAtLeast(target.TargetVersion, 3, 3) {
		return false, nil
	}

	dsc, err := client.GetDataScienceCluster(ctx, target.Client)
	if err != nil {
		return false, fmt.Errorf("getting DataScienceCluster: %w", err)
	}

	return components.HasManagementState(dsc, constants.ComponentTrainingOperator, constants.ManagementStateManaged), nil
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

			req.Result.SetCondition(c.newPyTorchJobCondition(len(active), len(completed)))

			return nil
		})
}
