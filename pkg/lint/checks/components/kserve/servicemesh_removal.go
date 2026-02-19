package kserve

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

// ServiceMeshRemovalCheck validates that ServiceMesh is disabled before upgrading to 3.x.
type ServiceMeshRemovalCheck struct {
	check.BaseCheck
}

func NewServiceMeshRemovalCheck() *ServiceMeshRemovalCheck {
	return &ServiceMeshRemovalCheck{
		BaseCheck: check.BaseCheck{
			CheckGroup:       check.GroupComponent,
			Kind:             constants.ComponentKServe,
			Type:             "servicemesh-removal",
			CheckID:          "components.kserve.servicemesh-removal",
			CheckName:        "Components :: KServe :: ServiceMesh Removal (3.x)",
			CheckDescription: "Validates that ServiceMesh is disabled before upgrading from RHOAI 2.x to 3.x (no longer required, OpenShift 4.19+ handles service mesh internally)",
			CheckRemediation: "Disable ServiceMesh by setting managementState to 'Removed' in DSCInitialization before upgrading",
		},
	}
}

func (c *ServiceMeshRemovalCheck) CanApply(_ context.Context, target check.Target) (bool, error) {
	return version.IsUpgradeFrom2xTo3x(target.CurrentVersion, target.TargetVersion), nil
}

func (c *ServiceMeshRemovalCheck) Validate(ctx context.Context, target check.Target) (*result.DiagnosticResult, error) {
	tv := version.MajorMinorLabel(target.TargetVersion)

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
				check.WithMessage("ServiceMesh is enabled (state: %s) but is no longer required by RHOAI %s. OpenShift 4.19+ handles service mesh internally", managementState, tv),
				check.WithImpact(result.ImpactBlocking),
				check.WithRemediation(c.CheckRemediation),
			))
		default:
			dr.SetCondition(check.NewCondition(
				check.ConditionTypeCompatible,
				metav1.ConditionTrue,
				check.WithReason(check.ReasonVersionCompatible),
				check.WithMessage("ServiceMesh is disabled (state: %s) - ready for RHOAI %s upgrade", managementState, tv),
			))
		}

		return nil
	})
}
