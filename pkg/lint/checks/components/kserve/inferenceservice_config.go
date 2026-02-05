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
			CheckType:        check.CheckTypeConfigMigration,
			CheckID:          "components.kserve.inferenceservice-config",
			CheckName:        "Components :: KServe :: InferenceService Config Migration",
			CheckDescription: "Validates that inferenceservice-config ConfigMap is managed by the operator before upgrading to RHOAI 3.x",
		},
	}
}

// CanApply returns whether this check should run for the given target.
// This check only applies when upgrading FROM 2.x TO 3.x.
func (c *InferenceServiceConfigCheck) CanApply(target check.Target) bool {
	return version.IsUpgradeFrom2xTo3x(target.CurrentVersion, target.TargetVersion)
}

// Validate executes the check against the provided target.
func (c *InferenceServiceConfigCheck) Validate(
	ctx context.Context,
	target check.Target,
) (*result.DiagnosticResult, error) {
	dr := c.NewResult()

	// Get the applications namespace from DSCI
	applicationsNamespace, err := target.Client.GetApplicationsNamespace(ctx)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return results.DSCInitializationNotFound(string(c.Group()), c.Kind, c.CheckType, c.Description()), nil
		}

		return nil, fmt.Errorf("getting applications namespace: %w", err)
	}

	// Get the inferenceservice-config ConfigMap from the applications namespace
	configMap, err := target.Client.GetResource(
		ctx,
		resources.ConfigMap,
		inferenceServiceConfigName,
		client.InNamespace(applicationsNamespace),
	)
	if err != nil {
		// Handle not found case - ConfigMap doesn't exist, nothing to migrate
		if apierrors.IsNotFound(err) {
			results.SetCompatibilitySuccessf(dr,
				"inferenceservice-config ConfigMap not found in namespace %s - no migration needed",
				applicationsNamespace,
			)

			return dr, nil
		}

		return nil, fmt.Errorf("getting inferenceservice-config ConfigMap: %w", err)
	}

	// Handle case where GetResource returns nil (permission issues return nil, nil)
	if configMap == nil {
		results.SetCompatibilitySuccessf(dr,
			"inferenceservice-config ConfigMap not found in namespace %s - no migration needed",
			applicationsNamespace,
		)

		return dr, nil
	}

	if target.TargetVersion != nil {
		dr.Annotations[check.AnnotationCheckTargetVersion] = target.TargetVersion.String()
	}

	// Check if ConfigMap is managed using the kube helper
	if !kube.IsManaged(configMap) {
		// ConfigMap is not managed - advisory warning (non-blocking)
		results.SetCondition(dr, check.NewCondition(
			check.ConditionTypeConfigured,
			metav1.ConditionFalse,
			check.ReasonConfigurationInvalid,
			"inferenceservice-config ConfigMap has %s=false - migration will not update it and configuration may become out of sync after upgrade to RHOAI 3.x",
			kube.AnnotationManaged,
			check.WithImpact(result.ImpactAdvisory),
		))

		return dr, nil
	}

	// ConfigMap exists and is managed (or no annotation) - ready for upgrade
	results.SetCompatibilitySuccessf(dr,
		"inferenceservice-config ConfigMap is managed by operator - ready for RHOAI 3.x upgrade",
	)

	return dr, nil
}
