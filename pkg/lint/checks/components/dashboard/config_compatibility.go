package dashboard

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/opendatahub-io/odh-cli/pkg/constants"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/validate"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
	"github.com/opendatahub-io/odh-cli/pkg/util/version"
)

const (
	odhDashboardConfigName = "odh-dashboard-config"

	conditionTypeConfigCompatibility = "ConfigCompatibility"

	msgConfigFound    = "OdhDashboardConfig %q found (version: %s)"
	msgConfigNotFound = "OdhDashboardConfig not found in applications namespace - configuration may need to be recreated after upgrade"
)

// ConfigCompatibilityCheck verifies OdhDashboardConfig exists in the applications namespace.
type ConfigCompatibilityCheck struct {
	check.BaseCheck
}

func NewConfigCompatibilityCheck() *ConfigCompatibilityCheck {
	return &ConfigCompatibilityCheck{
		BaseCheck: check.BaseCheck{
			CheckGroup:       check.GroupComponent,
			Kind:             constants.ComponentDashboard,
			Type:             check.CheckTypeConfigCompatibility,
			CheckID:          "components.dashboard.config-compatibility",
			CheckName:        "Components :: Dashboard :: Config Compatibility (3.x)",
			CheckDescription: "Verifies OdhDashboardConfig exists and reports its version annotation",
			CheckRemediation: "If OdhDashboardConfig is missing after upgrade, recreate it with the appropriate settings",
		},
	}
}

func (c *ConfigCompatibilityCheck) CanApply(_ context.Context, target check.Target) (bool, error) {
	return version.IsUpgradeFrom2xTo3x(target.CurrentVersion, target.TargetVersion), nil
}

func (c *ConfigCompatibilityCheck) Validate(
	ctx context.Context,
	target check.Target,
) (*result.DiagnosticResult, error) {
	return validate.Component(c, target).
		WithApplicationsNamespace().
		Run(ctx, c.checkConfig)
}

func (c *ConfigCompatibilityCheck) checkConfig(
	ctx context.Context,
	req *validate.ComponentRequest,
) error {
	config, err := req.Client.GetResource(ctx, resources.OdhDashboardConfig,
		odhDashboardConfigName, client.InNamespace(req.ApplicationsNamespace))
	if err != nil {
		if apierrors.IsNotFound(err) {
			req.Result.SetCondition(check.NewCondition(
				conditionTypeConfigCompatibility,
				metav1.ConditionFalse,
				check.WithReason(check.ReasonResourceNotFound),
				check.WithMessage(msgConfigNotFound),
				check.WithImpact(result.ImpactAdvisory),
				check.WithRemediation(c.CheckRemediation),
			))

			return nil
		}

		return fmt.Errorf("getting OdhDashboardConfig: %w", err)
	}

	if config == nil {
		req.Result.SetCondition(check.NewCondition(
			conditionTypeConfigCompatibility,
			metav1.ConditionFalse,
			check.WithReason(check.ReasonInsufficientData),
			check.WithMessage("OdhDashboardConfig access restricted - unable to verify"),
			check.WithImpact(result.ImpactAdvisory),
			check.WithRemediation(c.CheckRemediation),
		))

		return nil
	}

	annotations := config.GetAnnotations()
	versionAnnotation := "unknown"

	if v, ok := annotations["platform.opendatahub.io/version"]; ok && v != "" {
		versionAnnotation = v
	}

	req.Result.SetCondition(check.NewCondition(
		conditionTypeConfigCompatibility,
		metav1.ConditionTrue,
		check.WithReason(check.ReasonResourceFound),
		check.WithMessage(msgConfigFound, odhDashboardConfigName, versionAnnotation),
	))

	return nil
}
