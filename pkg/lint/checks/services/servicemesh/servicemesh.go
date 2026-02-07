package servicemesh

import (
	"context"
	"errors"
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/lburgazzoli/odh-cli/pkg/lint/check"
	"github.com/lburgazzoli/odh-cli/pkg/lint/check/result"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/shared/base"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/shared/results"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/shared/validate"
	"github.com/lburgazzoli/odh-cli/pkg/util/jq"
	"github.com/lburgazzoli/odh-cli/pkg/util/version"
)

// RemovalCheck validates that ServiceMesh is disabled before upgrading to 3.x.
type RemovalCheck struct {
	base.BaseCheck
}

// NewRemovalCheck creates a new ServiceMesh removal check.
func NewRemovalCheck() *RemovalCheck {
	return &RemovalCheck{
		BaseCheck: base.BaseCheck{
			CheckGroup:       check.GroupService,
			Kind:             "servicemesh",
			Type:             check.CheckTypeRemoval,
			CheckID:          "services.servicemesh.removal",
			CheckName:        "Services :: ServiceMesh :: Removal (3.x)",
			CheckDescription: "Validates that ServiceMesh is disabled before upgrading from RHOAI 2.x to 3.x (service mesh will be removed)",
		},
	}
}

// CanApply returns whether this check should run for the given target.
// This check only applies when upgrading FROM 2.x TO 3.x.
func (c *RemovalCheck) CanApply(_ context.Context, target check.Target) bool {
	return version.IsUpgradeFrom2xTo3x(target.CurrentVersion, target.TargetVersion)
}

// Validate executes the check against the provided target.
func (c *RemovalCheck) Validate(ctx context.Context, target check.Target) (*result.DiagnosticResult, error) {
	return validate.DSCI(c).Run(ctx, target, func(dr *result.DiagnosticResult, dsci *unstructured.Unstructured) error {
		managementState, err := jq.Query[string](dsci, ".spec.serviceMesh.managementState")

		switch {
		case errors.Is(err, jq.ErrNotFound):
			results.SetServiceNotConfigured(dr, "ServiceMesh")
		case err != nil:
			return fmt.Errorf("querying servicemesh managementState: %w", err)
		case managementState == check.ManagementStateManaged || managementState == check.ManagementStateUnmanaged:
			results.SetCompatibilityFailuref(dr, "ServiceMesh is enabled (state: %s) but will be removed in RHOAI 3.x", managementState)
		default:
			results.SetCompatibilitySuccessf(dr, "ServiceMesh is disabled (state: %s) - ready for RHOAI 3.x upgrade", managementState)
		}

		return nil
	})
}
