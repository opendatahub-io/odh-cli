package kserve

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/lburgazzoli/odh-cli/pkg/lint/check"
	"github.com/lburgazzoli/odh-cli/pkg/lint/check/result"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/shared/base"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/shared/results"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/shared/validate"
	"github.com/lburgazzoli/odh-cli/pkg/resources"
	"github.com/lburgazzoli/odh-cli/pkg/util/client"
	"github.com/lburgazzoli/odh-cli/pkg/util/kube"
	"github.com/lburgazzoli/odh-cli/pkg/util/version"
)

// inferenceServiceConfigName is the name of the KServe configuration ConfigMap.
const inferenceServiceConfigName = "inferenceservice-config"

// InferenceServiceConfigCheck validates that the inferenceservice-config ConfigMap
// is managed by the operator before upgrading to 3.x.
type InferenceServiceConfigCheck struct {
	base.BaseCheck
}

func NewInferenceServiceConfigCheck() *InferenceServiceConfigCheck {
	return &InferenceServiceConfigCheck{
		BaseCheck: base.BaseCheck{
			CheckGroup:       check.GroupComponent,
			Kind:             check.ComponentKServe,
			Type:             check.CheckTypeConfigMigration,
			CheckID:          "components.kserve.inferenceservice-config",
			CheckName:        "Components :: KServe :: InferenceService Config Migration",
			CheckDescription: "Validates that inferenceservice-config ConfigMap is managed by the operator before upgrading to RHOAI 3.x",
			CheckRemediation: "Remove the annotation opendatahub.io/managed=false from the inferenceservice-config ConfigMap, or back up your custom configuration for manual re-application after upgrade",
		},
	}
}

// CanApply returns whether this check should run for the given target.
// This check only applies when upgrading FROM 2.x TO 3.x.
func (c *InferenceServiceConfigCheck) CanApply(_ context.Context, target check.Target) (bool, error) {
	return version.IsUpgradeFrom2xTo3x(target.CurrentVersion, target.TargetVersion), nil
}

func (c *InferenceServiceConfigCheck) Validate(ctx context.Context, target check.Target) (*result.DiagnosticResult, error) {
	return validate.Component(c, target).
		InState(check.ManagementStateManaged).
		WithApplicationsNamespace().
		Run(ctx, func(ctx context.Context, req *validate.ComponentRequest) error {
			res, err := req.Client.GetResourceMetadata(
				ctx,
				resources.ConfigMap,
				inferenceServiceConfigName,
				client.InNamespace(req.ApplicationsNamespace),
			)

			switch {
			case apierrors.IsNotFound(err):
				results.SetCompatibilitySuccessf(req.Result,
					"inferenceservice-config ConfigMap not found in namespace %s - no migration needed",
					req.ApplicationsNamespace,
				)
			case err != nil:
				return fmt.Errorf("getting inferenceservice-config ConfigMap: %w", err)
			case kube.IsManaged(res):
				results.SetCompatibilitySuccessf(req.Result,
					"inferenceservice-config ConfigMap is managed by operator - ready for RHOAI 3.x upgrade",
				)
			default:
				results.SetCondition(req.Result, check.NewCondition(
					check.ConditionTypeConfigured,
					metav1.ConditionFalse,
					check.ReasonConfigurationInvalid,
					"inferenceservice-config ConfigMap has %s=false - migration will not update it and configuration may become out of sync after upgrade to RHOAI 3.x",
					kube.AnnotationManaged,
					check.WithImpact(result.ImpactAdvisory),
					check.WithRemediation(c.CheckRemediation),
				))
			}

			return nil
		})
}
