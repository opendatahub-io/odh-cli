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
	"github.com/lburgazzoli/odh-cli/pkg/resources"
	"github.com/lburgazzoli/odh-cli/pkg/util/client"
	"github.com/lburgazzoli/odh-cli/pkg/util/version"
)

const (
	configMapName          = "kueue-manager-config"
	annotationManagedKey   = "opendatahub.io/managed"
	annotationManagedFalse = "false"
)

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
			CheckType:        check.CheckTypeConfigMigration,
			CheckID:          "components.kueue.configmap-managed",
			CheckName:        "Components :: Kueue :: ConfigMap Managed Check (3.x)",
			CheckDescription: "Validates that kueue-manager-config ConfigMap is managed by the operator before upgrading from RHOAI 2.x to 3.x",
		},
	}
}

// CanApply returns whether this check should run for the given target.
// This check only applies when upgrading FROM 2.x TO 3.x.
func (c *ConfigMapManagedCheck) CanApply(target check.Target) bool {
	return version.IsUpgradeFrom2xTo3x(target.CurrentVersion, target.TargetVersion)
}

// Validate executes the check against the provided target.
func (c *ConfigMapManagedCheck) Validate(
	ctx context.Context,
	target check.Target,
) (*result.DiagnosticResult, error) {
	dr := c.NewResult()

	// Get the applications namespace from DSCI
	applicationsNamespace, err := target.Client.GetApplicationsNamespace(ctx)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return results.DSCInitializationNotFound(
				string(c.Group()), c.Kind, c.CheckType, c.Description(),
			), nil
		}

		return nil, fmt.Errorf("getting applications namespace: %w", err)
	}

	// Check if ConfigMap exists in that namespace
	configMap, err := target.Client.GetResource(
		ctx, resources.ConfigMap, configMapName, client.InNamespace(applicationsNamespace),
	)
	if err != nil {
		// NotFound is okay - ConfigMap doesn't exist, no warning needed
		if apierrors.IsNotFound(err) {
			results.SetCompatibilitySuccessf(dr,
				"ConfigMap %s/%s not found - no action required", applicationsNamespace, configMapName)

			return dr, nil
		}

		return nil, fmt.Errorf("getting ConfigMap %s/%s: %w", applicationsNamespace, configMapName, err)
	}

	// ConfigMap nil (permission error) - treated as not found, no warning needed
	if configMap == nil {
		results.SetCompatibilitySuccessf(dr,
			"ConfigMap %s/%s not accessible - no action required", applicationsNamespace, configMapName)

		return dr, nil
	}

	// Check annotation on the ConfigMap
	annotations := configMap.GetAnnotations()
	managedValue := annotations[annotationManagedKey]

	if target.TargetVersion != nil {
		dr.Annotations[check.AnnotationCheckTargetVersion] = target.TargetVersion.String()
	}

	if managedValue == annotationManagedFalse {
		// ConfigMap has managed=false - advisory warning (non-blocking)
		results.SetCondition(dr, check.NewCondition(
			check.ConditionTypeConfigured,
			metav1.ConditionFalse,
			check.ReasonConfigurationInvalid,
			"ConfigMap %s/%s has annotation %s=%s - migration will not update this ConfigMap and it may become out of sync with operator defaults",
			applicationsNamespace, configMapName, annotationManagedKey, annotationManagedFalse,
			check.WithImpact(result.ImpactAdvisory),
		))

		return dr, nil
	}

	// ConfigMap exists without managed=false annotation - check passes
	results.SetCompatibilitySuccessf(dr,
		"ConfigMap %s/%s is managed by operator (annotation %s not set to false)",
		applicationsNamespace, configMapName, annotationManagedKey)

	return dr, nil
}
