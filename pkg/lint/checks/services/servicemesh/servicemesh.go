package servicemesh

import (
	"context"
	"errors"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/opendatahub-io/odh-cli/pkg/constants"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/validate"
	"github.com/opendatahub-io/odh-cli/pkg/util/jq"
	"github.com/opendatahub-io/odh-cli/pkg/util/version"
)

// RemovalCheck validates that ServiceMesh is disabled before upgrading to 3.x.
type RemovalCheck struct {
	check.BaseCheck
}

// NewRemovalCheck creates a new ServiceMesh removal check.
func NewRemovalCheck() *RemovalCheck {
	return &RemovalCheck{
		BaseCheck: check.BaseCheck{
			CheckGroup:       check.GroupService,
			Kind:             "servicemesh",
			Type:             check.CheckTypeRemoval,
			CheckID:          "services.servicemesh.removal",
			CheckName:        "Services :: ServiceMesh :: Removal (3.x)",
			CheckDescription: "Validates that ServiceMesh is disabled before upgrading from RHOAI 2.x to 3.x (no longer required, OpenShift 4.19+ handles service mesh internally)",
			CheckRemediation: "Disable ServiceMesh by setting managementState to 'Removed' in DSCInitialization before upgrading",
		},
	}
}

// CanApply returns whether this check should run for the given target.
// This check only applies when upgrading FROM 2.x TO 3.x.
func (c *RemovalCheck) CanApply(_ context.Context, target check.Target) (bool, error) {
	return version.IsUpgradeFrom2xTo3x(target.CurrentVersion, target.TargetVersion), nil
}

// Validate executes the check against the provided target.
func (c *RemovalCheck) Validate(ctx context.Context, target check.Target) (*result.DiagnosticResult, error) {
	return validate.DSCI(c, target).Run(ctx, func(dr *result.DiagnosticResult, dsci *unstructured.Unstructured) error {
		managementState, err := jq.Query[string](dsci, ".spec.serviceMesh.managementState")

		switch {
		case errors.Is(err, jq.ErrNotFound):
			dr.SetCondition(check.NewCondition(
				check.ConditionTypeConfigured,
				metav1.ConditionFalse,
				check.WithReason(check.ReasonResourceNotFound),
				check.WithMessage("ServiceMesh is not configured in DSCInitialization"),
			))
		case err != nil:
			return fmt.Errorf("querying servicemesh managementState: %w", err)
		case managementState == constants.ManagementStateManaged || managementState == constants.ManagementStateUnmanaged:
			dr.SetCondition(check.NewCondition(
				check.ConditionTypeCompatible,
				metav1.ConditionFalse,
				check.WithReason(check.ReasonVersionIncompatible),
				check.WithMessage("ServiceMesh is enabled (state: %s) but is no longer required by RHOAI 3.x. OpenShift 4.19+ handles service mesh internally", managementState),
				check.WithImpact(result.ImpactBlocking),
				check.WithRemediation(c.CheckRemediation),
			))
		default:
			dr.SetCondition(check.NewCondition(
				check.ConditionTypeCompatible,
				metav1.ConditionTrue,
				check.WithReason(check.ReasonVersionCompatible),
				check.WithMessage("ServiceMesh is disabled (state: %s) - ready for RHOAI 3.x upgrade", managementState),
			))
		}

		return nil
	})
}
