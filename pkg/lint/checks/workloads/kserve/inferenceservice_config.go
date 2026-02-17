package kserve

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/opendatahub-io/odh-cli/pkg/constants"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/validate"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
	"github.com/opendatahub-io/odh-cli/pkg/util/components"
	"github.com/opendatahub-io/odh-cli/pkg/util/jq"
	"github.com/opendatahub-io/odh-cli/pkg/util/kube"
	"github.com/opendatahub-io/odh-cli/pkg/util/version"
)

// inferenceServiceConfigName is the name of the KServe configuration ConfigMap.
const inferenceServiceConfigName = "inferenceservice-config"

// inferenceServiceDataKey is the ConfigMap data key containing InferenceService configuration.
const inferenceServiceDataKey = "inferenceService"

const (
	msgConfigMapNotFound            = "inferenceservice-config ConfigMap not found in namespace %s - no migration needed"
	msgManagedAnnotationMissing     = "inferenceservice-config ConfigMap must have %s=false and include hardware-profile annotations in serviceAnnotationDisallowedList, otherwise models may get restarted during upgrade to RHOAI 3.x"
	msgDisallowedAnnotationsMissing = "inferenceservice-config ConfigMap must include the following annotations in serviceAnnotationDisallowedList to prevent models from being restarted during upgrade to RHOAI 3.x: %s"
	msgConfigMapReady               = "inferenceservice-config ConfigMap has %s=false and serviceAnnotationDisallowedList includes required hardware-profile annotations - ready for RHOAI 3.x upgrade"
)

// requiredDisallowedAnnotations lists annotations that must be present in the
// inferenceService serviceAnnotationDisallowedList to prevent reconciliation
// loops after hardware-profile migration.
//
//nolint:gochecknoglobals // Constant-like list used across check methods.
var requiredDisallowedAnnotations = []string{
	"opendatahub.io/hardware-profile-name",
	"opendatahub.io/hardware-profile-namespace",
}

// inferenceServiceConfig represents the JSON structure of the inferenceService data key.
type inferenceServiceConfig struct {
	ServiceAnnotationDisallowedList []string `json:"serviceAnnotationDisallowedList"`
}

// InferenceServiceConfigCheck validates that the inferenceservice-config ConfigMap
// has opendatahub.io/managed=false and includes hardware-profile annotations in the
// serviceAnnotationDisallowedList before upgrading to 3.x.
type InferenceServiceConfigCheck struct {
	check.BaseCheck
}

func NewInferenceServiceConfigCheck() *InferenceServiceConfigCheck {
	return &InferenceServiceConfigCheck{
		BaseCheck: check.BaseCheck{
			CheckGroup:       check.GroupWorkload,
			Kind:             constants.ComponentKServe,
			Type:             check.CheckTypeImpactedWorkloads,
			CheckID:          "workloads.kserve.inferenceservice-config",
			CheckName:        "Workloads :: KServe :: InferenceService Config Migration",
			CheckDescription: "Validates that inferenceservice-config ConfigMap has opendatahub.io/managed=false and includes hardware-profile annotations in serviceAnnotationDisallowedList before upgrading to RHOAI 3.x",
			CheckRemediation: "Set the annotation opendatahub.io/managed=false on the inferenceservice-config ConfigMap, and add opendatahub.io/hardware-profile-name and opendatahub.io/hardware-profile-namespace to the serviceAnnotationDisallowedList in the inferenceService data key",
		},
	}
}

// CanApply returns whether this check should run for the given target.
// This check only applies when upgrading FROM 2.x TO 3.x and KServe is Managed.
func (c *InferenceServiceConfigCheck) CanApply(ctx context.Context, target check.Target) (bool, error) {
	if !version.IsUpgradeFrom2xTo3x(target.CurrentVersion, target.TargetVersion) {
		return false, nil
	}

	dsc, err := client.GetDataScienceCluster(ctx, target.Client)
	if err != nil {
		return false, fmt.Errorf("getting DataScienceCluster: %w", err)
	}

	return components.HasManagementState(dsc, constants.ComponentKServe, constants.ManagementStateManaged), nil
}

func (c *InferenceServiceConfigCheck) Validate(ctx context.Context, target check.Target) (*result.DiagnosticResult, error) {
	return validate.Component(c, target).
		WithApplicationsNamespace().
		Run(ctx, func(ctx context.Context, req *validate.ComponentRequest) error {
			res, err := req.Client.GetResource(
				ctx,
				resources.ConfigMap,
				inferenceServiceConfigName,
				client.InNamespace(req.ApplicationsNamespace),
			)

			switch {
			case apierrors.IsNotFound(err):
				req.Result.SetCondition(check.NewCondition(
					check.ConditionTypeCompatible,
					metav1.ConditionTrue,
					check.WithReason(check.ReasonVersionCompatible),
					check.WithMessage(msgConfigMapNotFound, req.ApplicationsNamespace),
				))

				return nil
			case err != nil:
				return fmt.Errorf("getting inferenceservice-config ConfigMap: %w", err)
			// The managed annotation must be explicitly set to false so the operator
			// does not overwrite user customizations during upgrade.
			case kube.IsManaged(res):
				req.Result.SetCondition(check.NewCondition(
					check.ConditionTypeConfigured,
					metav1.ConditionFalse,
					check.WithReason(check.ReasonConfigurationUnmanaged),
					check.WithMessage(msgManagedAnnotationMissing, kube.AnnotationManaged),
					check.WithImpact(result.ImpactAdvisory),
					check.WithRemediation(c.CheckRemediation),
				))

				return nil
			}

			// Check that the hardware-profile annotations are in the disallowed list
			// to prevent reconciliation loops after migration.
			missing, err := findMissingDisallowedAnnotations(res, requiredDisallowedAnnotations)
			if err != nil {
				return fmt.Errorf("checking serviceAnnotationDisallowedList: %w", err)
			}

			if len(missing) > 0 {
				req.Result.SetCondition(check.NewCondition(
					check.ConditionTypeConfigured,
					metav1.ConditionFalse,
					check.WithReason(check.ReasonConfigurationInvalid),
					check.WithMessage(msgDisallowedAnnotationsMissing, strings.Join(missing, ", ")),
					check.WithImpact(result.ImpactAdvisory),
					check.WithRemediation(c.CheckRemediation),
				))

				return nil
			}

			req.Result.SetCondition(check.NewCondition(
				check.ConditionTypeCompatible,
				metav1.ConditionTrue,
				check.WithReason(check.ReasonVersionCompatible),
				check.WithMessage(msgConfigMapReady, kube.AnnotationManaged),
			))

			return nil
		})
}

// findMissingDisallowedAnnotations parses the inferenceService data key and returns
// which of the required annotations are missing from serviceAnnotationDisallowedList.
func findMissingDisallowedAnnotations(
	configMap *unstructured.Unstructured,
	required []string,
) ([]string, error) {
	dataJSON, err := jq.Query[string](configMap, ".data."+inferenceServiceDataKey)
	if err != nil {
		return required, nil //nolint:nilerr // Missing data key means all annotations are missing.
	}

	var cfg inferenceServiceConfig
	if err := json.Unmarshal([]byte(dataJSON), &cfg); err != nil {
		return nil, fmt.Errorf("parsing %s JSON: %w", inferenceServiceDataKey, err)
	}

	var missing []string
	for _, annotation := range required {
		if !slices.Contains(cfg.ServiceAnnotationDisallowedList, annotation) {
			missing = append(missing, annotation)
		}
	}

	return missing, nil
}
