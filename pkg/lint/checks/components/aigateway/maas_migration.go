package aigateway

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
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
	"github.com/opendatahub-io/odh-cli/pkg/util/jq"
	"github.com/opendatahub-io/odh-cli/pkg/util/version"
)

const (
	kind = constants.ComponentAIGateway

	// kserveMaaSField is the 3.4 DSC path for MaaS nested under kserve.
	kserveMaaSField = ".spec.components.kserve.modelsAsService.managementState"
)

// MaaSFieldMigrationCheck detects when Models as a Service is configured via the
// deprecated kserve.modelsAsService field (3.4) instead of aigateway.modelsAsAService (3.5+).
type MaaSFieldMigrationCheck struct {
	check.BaseCheck
}

// NewMaaSFieldMigrationCheck creates a new MaaSFieldMigrationCheck.
func NewMaaSFieldMigrationCheck() *MaaSFieldMigrationCheck {
	return &MaaSFieldMigrationCheck{
		BaseCheck: check.BaseCheck{
			CheckGroup:       check.GroupComponent,
			Kind:             kind,
			Type:             "maas-field-migration",
			CheckID:          "components.aigateway.maas-field-migration",
			CheckName:        "Components :: AIGateway :: MaaS Field Migration (3.4 to 3.5)",
			CheckDescription: "Detects when Models as a Service is configured via the deprecated kserve.modelsAsService field instead of the new aigateway.modelsAsAService location",
			CheckRemediation: "Set spec.components.aigateway.managementState: Managed and spec.components.aigateway.modelsAsAService.managementState: Managed in DataScienceCluster, then set spec.components.kserve.modelsAsService.managementState: Removed to clear the deprecated field",
		},
	}
}

// CanApply returns true when upgrading from 3.4 to 3.5 with MaaS enabled via the deprecated field.
func (c *MaaSFieldMigrationCheck) CanApply(ctx context.Context, target check.Target) (bool, error) {
	if !version.IsUpgradeFrom34To35(target.CurrentVersion, target.TargetVersion) {
		return false, nil
	}

	dsc, err := client.GetDataScienceCluster(ctx, target.Client)
	if err != nil {
		return false, fmt.Errorf("getting DataScienceCluster: %w", err)
	}

	state, err := jq.Query[string](dsc, kserveMaaSField)
	if errors.Is(err, jq.ErrNotFound) {
		return false, nil
	}

	if err != nil {
		return false, fmt.Errorf("querying kserve modelsAsService state: %w", err)
	}

	return state == constants.ManagementStateManaged, nil
}

// Validate executes the check against the provided target.
func (c *MaaSFieldMigrationCheck) Validate(ctx context.Context, target check.Target) (*result.DiagnosticResult, error) {
	tv := version.MajorMinorLabel(target.TargetVersion)

	return validate.DSC(c, target).Run(ctx, func(dr *result.DiagnosticResult, dsc *unstructured.Unstructured) error {
		state, err := jq.Query[string](dsc, kserveMaaSField)

		switch {
		case errors.Is(err, jq.ErrNotFound):
			dr.SetCondition(check.NewCondition(
				check.ConditionTypeConfigured,
				metav1.ConditionTrue,
				check.WithReason(check.ReasonRequirementsMet),
				check.WithMessage("Models as a Service is not configured via the deprecated kserve.modelsAsService field"),
			))
		case err != nil:
			return fmt.Errorf("querying kserve modelsAsService state: %w", err)
		case state == constants.ManagementStateManaged || state == constants.ManagementStateUnmanaged:
			dr.SetCondition(check.NewCondition(
				check.ConditionTypeCompatible,
				metav1.ConditionFalse,
				check.WithReason(check.ReasonVersionIncompatible),
				check.WithMessage(
					"Models as a Service is enabled via the deprecated kserve.modelsAsService field (state: %s). "+
						"In RHOAI %s, MaaS is configured under aigateway.modelsAsAService",
					state, tv,
				),
				check.WithImpact(result.ImpactAdvisory),
				check.WithRemediation(c.CheckRemediation),
			))
		default:
			dr.SetCondition(check.NewCondition(
				check.ConditionTypeConfigured,
				metav1.ConditionTrue,
				check.WithReason(check.ReasonRequirementsMet),
				check.WithMessage("Models as a Service is not enabled via the deprecated kserve.modelsAsService field"),
			))
		}

		return nil
	})
}
