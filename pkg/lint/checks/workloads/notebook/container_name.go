package notebook

import (
	"context"

	"github.com/opendatahub-io/odh-cli/pkg/constants"
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
	check.EnhancedVerboseFormatter
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
// Applies regardless of version; component state is checked via ForComponent in Validate.
func (c *ContainerNameCheck) CanApply(_ context.Context, _ check.Target) (bool, error) {
	return true, nil
}

// Validate executes the check against the provided target.
func (c *ContainerNameCheck) Validate(
	ctx context.Context,
	target check.Target,
) (*result.DiagnosticResult, error) {
	return validate.Workloads(c, target, resources.Notebook).
		ForComponent(constants.ComponentWorkbenches).
		Filter(hasDashboardAnnotationAndNameMismatch).
		Complete(ctx, c.newContainerNameCondition)
}
