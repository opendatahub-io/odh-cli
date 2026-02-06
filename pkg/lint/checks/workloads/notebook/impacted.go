package notebook

import (
	"context"
	"fmt"
	"strconv"

	"k8s.io/apimachinery/pkg/types"

	"github.com/lburgazzoli/odh-cli/pkg/lint/check"
	"github.com/lburgazzoli/odh-cli/pkg/lint/check/result"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/shared/base"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/shared/results"
	"github.com/lburgazzoli/odh-cli/pkg/resources"
	"github.com/lburgazzoli/odh-cli/pkg/util/client"
	"github.com/lburgazzoli/odh-cli/pkg/util/version"
)

const (
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
			Kind:             check.ComponentNotebook,
			CheckType:        check.CheckTypeImpactedWorkloads,
			CheckID:          "workloads.notebook.impacted-workloads",
			CheckName:        "Workloads :: Notebook :: Impacted Workloads (3.x)",
			CheckDescription: "Lists Notebook (workbench) instances that will be impacted in RHOAI 3.x",
		},
	}
}

// CanApply returns whether this check should run for the given target.
// Only applies when upgrading FROM 2.x TO 3.x since Notebook workloads may be impacted.
func (c *ImpactedWorkloadsCheck) CanApply(target check.Target) bool {
	return version.IsUpgradeFrom2xTo3x(target.CurrentVersion, target.TargetVersion)
}

// Validate executes the check against the provided target.
func (c *ImpactedWorkloadsCheck) Validate(
	ctx context.Context,
	target check.Target,
) (*result.DiagnosticResult, error) {
	dr := c.NewResult()

	if target.TargetVersion != nil {
		dr.Annotations[check.AnnotationCheckTargetVersion] = target.TargetVersion.String()
	}

	// Find all notebooks
	notebooks, err := c.findImpactedNotebooks(ctx, target)
	if err != nil {
		return nil, err
	}

	totalImpacted := len(notebooks)
	dr.Annotations[check.AnnotationImpactedWorkloadCount] = strconv.Itoa(totalImpacted)

	// Add condition
	dr.Status.Conditions = append(dr.Status.Conditions,
		newNotebookCondition(totalImpacted),
	)

	// Populate ImpactedObjects if any notebooks found
	if totalImpacted > 0 {
		results.PopulateImpactedObjects(dr, resources.Notebook, notebooks)
	}

	return dr, nil
}

func (c *ImpactedWorkloadsCheck) findImpactedNotebooks(
	ctx context.Context,
	target check.Target,
) ([]types.NamespacedName, error) {
	notebooks, err := target.Client.ListMetadata(ctx, resources.Notebook)
	if err != nil {
		if client.IsResourceTypeNotFound(err) {
			return nil, nil
		}

		return nil, fmt.Errorf("listing Notebooks: %w", err)
	}

	impacted := make([]types.NamespacedName, 0, len(notebooks))

	for _, nb := range notebooks {
		impacted = append(impacted, types.NamespacedName{
			Namespace: nb.GetNamespace(),
			Name:      nb.GetName(),
		})
	}

	return impacted, nil
}
