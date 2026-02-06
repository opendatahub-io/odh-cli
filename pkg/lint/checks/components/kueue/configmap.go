package kueue

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

const configMapName = "kueue-manager-config"

// ConfigMapManagedCheck validates that kueue-manager-config ConfigMap is managed by the operator.
// If the ConfigMap has the annotation opendatahub.io/managed=false, the migration to 3.x will
// not update it, which may result in configuration drift.
type ConfigMapManagedCheck struct {
	base.BaseCheck
}

// NewConfigMapManagedCheck creates a new ConfigMapManagedCheck.
func NewConfigMapManagedCheck() *ConfigMapManagedCheck {
	return &ConfigMapManagedCheck{
		BaseCheck: base.BaseCheck{
			CheckGroup:       check.GroupComponent,
			Kind:             check.ComponentKueue,
			Type:             check.CheckTypeConfigMigration,
			CheckID:          "components.kueue.configmap-managed",
			CheckName:        "Components :: Kueue :: ConfigMap Managed Check (3.x)",
			CheckDescription: "Validates that kueue-manager-config ConfigMap is managed by the operator before upgrading from RHOAI 2.x to 3.x",
		},
	}
}

// CanApply returns whether this check should run for the given target.
// This check only applies when upgrading FROM 2.x TO 3.x.
func (c *ConfigMapManagedCheck) CanApply(_ context.Context, target check.Target) bool {
	return version.IsUpgradeFrom2xTo3x(target.CurrentVersion, target.TargetVersion)
}

func (c *ConfigMapManagedCheck) Validate(ctx context.Context, target check.Target) (*result.DiagnosticResult, error) {
	return validate.Component(c, target).
		InState(check.ManagementStateManaged).
		WithApplicationsNamespace().
		Run(ctx, func(ctx context.Context, req *validate.ComponentRequest) error {
			res, err := req.Client.GetResourceMetadata(
				ctx,
				resources.ConfigMap,
				configMapName,
				client.InNamespace(req.ApplicationsNamespace),
			)

			switch {
			case apierrors.IsNotFound(err):
				results.SetCompatibilitySuccessf(req.Result,
					"ConfigMap %s/%s not found - no action required", req.ApplicationsNamespace, configMapName)
			case err != nil:
				return fmt.Errorf("getting ConfigMap %s/%s: %w", req.ApplicationsNamespace, configMapName, err)
			case kube.IsManaged(res):
				results.SetCompatibilitySuccessf(req.Result,
					"ConfigMap %s/%s is managed by operator (annotation %s not set to false)",
					req.ApplicationsNamespace, configMapName, kube.AnnotationManaged)
			default:
				results.SetCondition(req.Result, check.NewCondition(
					check.ConditionTypeConfigured,
					metav1.ConditionFalse,
					check.ReasonConfigurationInvalid,
					"ConfigMap %s/%s has annotation %s=false - migration will not update this ConfigMap and it may become out of sync with operator defaults",
					req.ApplicationsNamespace, configMapName, kube.AnnotationManaged,
					check.WithImpact(result.ImpactAdvisory),
				))
			}

			return nil
		})
}
