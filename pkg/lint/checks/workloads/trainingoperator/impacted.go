package trainingoperator

import (
	"context"
	"fmt"
	"strconv"

	"k8s.io/apimachinery/pkg/types"

	"github.com/lburgazzoli/odh-cli/pkg/lint/check"
	"github.com/lburgazzoli/odh-cli/pkg/lint/check/result"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/shared/base"
	"github.com/lburgazzoli/odh-cli/pkg/resources"
	"github.com/lburgazzoli/odh-cli/pkg/util/client"
	"github.com/lburgazzoli/odh-cli/pkg/util/version"
)

const (
	ConditionTypePyTorchJobsCompatible = "PyTorchJobsCompatible"
)

type impactedPyTorchJobs struct {
	Active    []types.NamespacedName
	Completed []types.NamespacedName
}

type ImpactedWorkloadsCheck struct {
	base.BaseCheck
}

func NewImpactedWorkloadsCheck() *ImpactedWorkloadsCheck {
	return &ImpactedWorkloadsCheck{
		BaseCheck: base.BaseCheck{
			CheckGroup:       check.GroupWorkload,
			Kind:             check.ComponentTrainingOperator,
			CheckType:        check.CheckTypeImpactedWorkloads,
			CheckID:          "workloads.trainingoperator.impacted-workloads",
			CheckName:        "Workloads :: TrainingOperator :: Impacted Workloads (3.3+)",
			CheckDescription: "Lists PyTorchJobs using deprecated TrainingOperator (Kubeflow v1) that will be impacted by transition to Trainer v2",
		},
	}
}

func (c *ImpactedWorkloadsCheck) CanApply(target check.Target) bool {
	//nolint:mnd // Version numbers 3.3
	return version.IsVersionAtLeast(target.TargetVersion, 3, 3)
}

func (c *ImpactedWorkloadsCheck) Validate(
	ctx context.Context,
	target check.Target,
) (*result.DiagnosticResult, error) {
	dr := c.NewResult()

	if target.TargetVersion != nil {
		dr.Annotations[check.AnnotationCheckTargetVersion] = target.TargetVersion.String()
	}

	jobs, err := c.findImpactedPyTorchJobs(ctx, target)
	if err != nil {
		return nil, err
	}

	totalActive := len(jobs.Active)
	totalCompleted := len(jobs.Completed)
	totalImpacted := totalActive + totalCompleted

	dr.Annotations[check.AnnotationImpactedWorkloadCount] = strconv.Itoa(totalImpacted)

	dr.Status.Conditions = append(dr.Status.Conditions,
		newPyTorchJobCondition(totalActive, totalCompleted),
	)

	if totalImpacted > 0 {
		populateImpactedObjects(dr, jobs.Active, jobs.Completed)
	}

	return dr, nil
}

func (c *ImpactedWorkloadsCheck) findImpactedPyTorchJobs(
	ctx context.Context,
	target check.Target,
) (impactedPyTorchJobs, error) {
	pytorchJobs, err := target.Client.List(ctx, resources.PyTorchJob)
	if err != nil {
		if client.IsResourceTypeNotFound(err) {
			return impactedPyTorchJobs{}, nil
		}

		return impactedPyTorchJobs{}, fmt.Errorf("listing PyTorchJobs: %w", err)
	}

	impacted := impactedPyTorchJobs{
		Active:    make([]types.NamespacedName, 0),
		Completed: make([]types.NamespacedName, 0),
	}

	for _, job := range pytorchJobs {
		nsName := types.NamespacedName{
			Namespace: job.GetNamespace(),
			Name:      job.GetName(),
		}

		if isJobCompleted(&job) {
			impacted.Completed = append(impacted.Completed, nsName)
		} else {
			impacted.Active = append(impacted.Active, nsName)
		}
	}

	return impacted, nil
}
