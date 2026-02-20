package notebook

import (
	"context"

	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/validate"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
)

// ContainerNameCheck detects Notebook (workbench) CRs where the primary container name
// does not match the Notebook CR name. This mismatch can cause issues with accelerator
// injection, size selection, and workload identification. Only notebooks with
// Dashboard-managed annotations (accelerator profile or size selection) are checked.
type ContainerNameCheck struct {
	check.BaseCheck
}

// NewContainerNameCheck creates a new ContainerNameCheck.
func NewContainerNameCheck() *ContainerNameCheck {
	return &ContainerNameCheck{
		BaseCheck: check.BaseCheck{
			CheckGroup:       check.GroupWorkload,
			Kind:             kind,
			Type:             check.CheckTypeConfigMigration,
			CheckID:          "workloads.notebook.container-name-mismatch",
			CheckName:        "Workloads :: Notebook :: Container Name Mismatch",
			CheckDescription: "Detects Dashboard-managed Notebook (workbench) CRs where the primary container name does not match the Notebook CR name",
			CheckRemediation: "Rename the primary container in the Notebook spec to match the Notebook CR name",
		},
	}
}

// CanApply returns whether this check should run for the given target.
// Applies in all modes when Workbenches is Managed.
func (c *ContainerNameCheck) CanApply(ctx context.Context, target check.Target) (bool, error) {
	return isWorkbenchesManaged(ctx, target)
}

// Validate executes the check against the provided target.
func (c *ContainerNameCheck) Validate(
	ctx context.Context,
	target check.Target,
) (*result.DiagnosticResult, error) {
	return validate.Workloads(c, target, resources.Notebook).
		Filter(hasDashboardAnnotationAndNameMismatch).
		Complete(ctx, c.newContainerNameCondition)
}
