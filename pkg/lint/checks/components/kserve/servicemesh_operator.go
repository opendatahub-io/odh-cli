package kserve

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/opendatahub-io/odh-cli/pkg/constants"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/validate"
	"github.com/opendatahub-io/odh-cli/pkg/util/version"
)

// ServiceMeshOperatorCheck validates that Service Mesh Operator v2 is not installed when upgrading to 3.x,
// as it is no longer required by RHOAI 3.x (OpenShift 4.19+ handles service mesh internally).
type ServiceMeshOperatorCheck struct {
	check.BaseCheck
}

func NewServiceMeshOperatorCheck() *ServiceMeshOperatorCheck {
	return &ServiceMeshOperatorCheck{
		BaseCheck: check.BaseCheck{
			CheckGroup:       check.GroupComponent,
			Kind:             constants.ComponentKServe,
			Type:             "servicemesh-operator-upgrade",
			CheckID:          "components.kserve.servicemesh-operator-upgrade",
			CheckName:        "Components :: KServe :: ServiceMesh Operator Upgrade (3.x)",
			CheckDescription: "Validates that Service Mesh Operator v2 is not installed when upgrading to RHOAI 3.x (no longer required, OpenShift 4.19+ handles service mesh internally)",
		},
	}
}

func (c *ServiceMeshOperatorCheck) CanApply(_ context.Context, target check.Target) (bool, error) {
	return version.IsUpgradeFrom2xTo3x(target.CurrentVersion, target.TargetVersion), nil
}

func (c *ServiceMeshOperatorCheck) Validate(ctx context.Context, target check.Target) (*result.DiagnosticResult, error) {
	tv := version.MajorMinorLabel(target.TargetVersion)

	return validate.Operator(c, target).
		WithNames("servicemeshoperator").
		WithChannels("stable", "v2.x").
		WithConditionBuilder(func(found bool, operatorVersion string) result.Condition {
			if !found {
				return check.NewCondition(
					check.ConditionTypeCompatible,
					metav1.ConditionTrue,
					check.WithReason(check.ReasonVersionCompatible),
					check.WithMessage("Service Mesh Operator v2 is not installed - ready for RHOAI %s upgrade", tv),
				)
			}

			return check.NewCondition(
				check.ConditionTypeCompatible,
				metav1.ConditionFalse,
				check.WithReason(check.ReasonVersionIncompatible),
				check.WithMessage("Service Mesh Operator v2 (%s) is installed but no longer required by RHOAI %s and should be removed. OpenShift 4.19+ handles service mesh internally", operatorVersion, tv),
			)
		}).
		Run(ctx)
}
