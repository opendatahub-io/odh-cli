package trainer

import (
	"context"
	"fmt"

	"github.com/opendatahub-io/odh-cli/pkg/constants"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/validate"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
	"github.com/opendatahub-io/odh-cli/pkg/util/components"
	"github.com/opendatahub-io/odh-cli/pkg/util/version"
)

const (
	ConditionTypePodTemplateOverridesCompatible = "PodTemplateOverridesCompatible"
	kbArticleURL                                = "https://access.redhat.com/articles/7146204"
)

// PodTemplateOverridesCheck warns about TrainJobs that still use podTemplateOverrides
// when upgrading to OpenShift AI 3.6+ (Trainer v2.3 replaces that field with runtimePatches).
type PodTemplateOverridesCheck struct {
	check.BaseCheck
}

func NewPodTemplateOverridesCheck() *PodTemplateOverridesCheck {
	return &PodTemplateOverridesCheck{
		BaseCheck: check.BaseCheck{
			CheckGroup: check.GroupWorkload,
			Kind:       constants.ComponentTrainer,
			Type:       check.CheckTypeImpactedWorkloads,
			CheckID:    "workloads.trainer.podtemplateoverrides",
			CheckName:  "Workloads :: Trainer :: PodTemplateOverrides (3.6+)",
			CheckDescription: "Detects TrainJobs using podTemplateOverrides that may fail pause/resume " +
				"after upgrading to OpenShift AI 3.6",
			CheckRemediation: "Wait for these jobs to complete before upgrading. After upgrade, recreate " +
				"TrainJobs using runtimePatches. See " + kbArticleURL,
		},
	}
}

// CanApply returns whether this check should run for the given target.
// Applies when upgrading from <=3.5.x to >=3.6 and Trainer is Managed.
func (c *PodTemplateOverridesCheck) CanApply(ctx context.Context, target check.Target) (bool, error) {
	//nolint:mnd // Version numbers 3.6
	if !version.IsVersionAtLeast(target.TargetVersion, 3, 6) {
		return false, nil
	}

	//nolint:mnd // Version numbers 3.6
	if version.IsVersionAtLeast(target.CurrentVersion, 3, 6) {
		return false, nil
	}

	dsc, err := client.GetDataScienceCluster(ctx, target.Client)
	if err != nil {
		return false, fmt.Errorf("getting DataScienceCluster: %w", err)
	}

	return components.HasManagementState(dsc, constants.ComponentTrainer, constants.ManagementStateManaged), nil
}

// Validate lists TrainJobs with non-empty podTemplateOverrides and returns an advisory warning.
func (c *PodTemplateOverridesCheck) Validate(
	ctx context.Context,
	target check.Target,
) (*result.DiagnosticResult, error) {
	return validate.Workloads(c, target, resources.TrainJob).
		ForComponent(constants.ComponentTrainer).
		Filter(hasPodTemplateOverrides).
		Complete(ctx, c.newPodTemplateOverridesCondition)
}
