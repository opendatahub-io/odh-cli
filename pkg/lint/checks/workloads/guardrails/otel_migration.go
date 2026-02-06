package guardrails

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"k8s.io/apimachinery/pkg/types"

	"github.com/lburgazzoli/odh-cli/pkg/lint/check"
	"github.com/lburgazzoli/odh-cli/pkg/lint/check/result"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/shared/base"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/shared/results"
	"github.com/lburgazzoli/odh-cli/pkg/resources"
	"github.com/lburgazzoli/odh-cli/pkg/util/client"
	"github.com/lburgazzoli/odh-cli/pkg/util/jq"
	"github.com/lburgazzoli/odh-cli/pkg/util/version"
)

const (
	ConditionTypeOtelConfigCompatible = "OtelConfigCompatible"

	// minTargetMajorVersion is the minimum major version for this check to apply.
	minTargetMajorVersion = 3
)

// OtelMigrationCheck detects GuardrailsOrchestrator CRs using deprecated otelExporter configuration fields.
type OtelMigrationCheck struct {
	base.BaseCheck
}

func NewOtelMigrationCheck() *OtelMigrationCheck {
	return &OtelMigrationCheck{
		BaseCheck: base.BaseCheck{
			CheckGroup:       check.GroupWorkload,
			Kind:             check.ComponentGuardrails,
			CheckType:        check.CheckTypeConfigMigration,
			CheckID:          "workloads.guardrails.otel-config-migration",
			CheckName:        "Workloads :: Guardrails :: OTEL Config Migration (3.x)",
			CheckDescription: "Detects GuardrailsOrchestrator CRs using deprecated otelExporter configuration fields that need migration",
		},
	}
}

// CanApply returns whether this check should run for the given target.
// Only applies when upgrading to 3.x or later.
func (c *OtelMigrationCheck) CanApply(target check.Target) bool {
	return version.IsVersionAtLeast(target.TargetVersion, minTargetMajorVersion, 0)
}

// Validate executes the check against the provided target.
func (c *OtelMigrationCheck) Validate(
	ctx context.Context,
	target check.Target,
) (*result.DiagnosticResult, error) {
	dr := c.NewResult()

	if target.TargetVersion != nil {
		dr.Annotations[check.AnnotationCheckTargetVersion] = target.TargetVersion.String()
	}

	// Find orchestrators with deprecated OTEL configuration
	impacted, err := c.findOrchestratorWithDeprecatedConfig(ctx, target)
	if err != nil {
		return nil, err
	}

	totalImpacted := len(impacted)
	dr.Annotations[check.AnnotationImpactedWorkloadCount] = strconv.Itoa(totalImpacted)

	// Add condition
	dr.Status.Conditions = append(dr.Status.Conditions,
		newOtelMigrationCondition(totalImpacted),
	)

	// Populate ImpactedObjects if any orchestrators found
	if totalImpacted > 0 {
		results.PopulateImpactedObjects(dr, resources.GuardrailsOrchestrator, impacted)
	}

	return dr, nil
}

func (c *OtelMigrationCheck) findOrchestratorWithDeprecatedConfig(
	ctx context.Context,
	target check.Target,
) ([]types.NamespacedName, error) {
	orchestrators, err := target.Client.ListResources(ctx, resources.GuardrailsOrchestrator.GVR())
	if err != nil {
		if client.IsResourceTypeNotFound(err) {
			return nil, nil
		}

		return nil, fmt.Errorf("listing GuardrailsOrchestrators: %w", err)
	}

	impacted := make([]types.NamespacedName, 0)

	for _, orch := range orchestrators {
		hasDeprecated, err := hasDeprecatedOtelFields(orch.Object)
		if err != nil {
			return nil, fmt.Errorf("checking deprecated fields for %s/%s: %w",
				orch.GetNamespace(), orch.GetName(), err)
		}

		if hasDeprecated {
			impacted = append(impacted, types.NamespacedName{
				Namespace: orch.GetNamespace(),
				Name:      orch.GetName(),
			})
		}
	}

	return impacted, nil
}

// hasDeprecatedOtelFields checks if the object contains any deprecated otelExporter fields.
func hasDeprecatedOtelFields(obj map[string]any) (bool, error) {
	for _, field := range deprecatedOtelFields {
		query := ".spec.otelExporter." + field

		_, err := jq.Query[any](obj, query)
		if err != nil {
			if errors.Is(err, jq.ErrNotFound) {
				continue
			}

			return false, fmt.Errorf("querying field %s: %w", field, err)
		}

		// Field exists (no error and not ErrNotFound)
		return true, nil
	}

	return false, nil
}
