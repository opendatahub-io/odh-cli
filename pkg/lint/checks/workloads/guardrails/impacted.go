package guardrails

import (
	"context"
	"fmt"
	"strconv"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/opendatahub-io/odh-cli/pkg/constants"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
	"github.com/opendatahub-io/odh-cli/pkg/util/components"
	"github.com/opendatahub-io/odh-cli/pkg/util/version"
)

const (
	ConditionTypeConfigurationValid = "ConfigurationValid"
)

const (
	annotationOrchestratorConfig = "guardrails.opendatahub.io/orchestrator-config"
	annotationGatewayConfig      = "guardrails.opendatahub.io/gateway-config"
	annotationBuiltinDetectors   = "guardrails.opendatahub.io/builtin-detectors"
	annotationOrchestratorCM     = "guardrails.opendatahub.io/orchestrator-configmap"
	annotationGatewayCM          = "guardrails.opendatahub.io/gateway-configmap"
)

// ImpactedWorkloadsCheck detects GuardrailsOrchestrator CRs with configuration
// that will be impacted in a RHOAI 2.x to 3.x upgrade.
type ImpactedWorkloadsCheck struct {
	check.BaseCheck
}

func NewImpactedWorkloadsCheck() *ImpactedWorkloadsCheck {
	return &ImpactedWorkloadsCheck{
		BaseCheck: check.BaseCheck{
			CheckGroup:       check.GroupWorkload,
			Kind:             kind,
			Type:             check.CheckTypeImpactedWorkloads,
			CheckID:          "workloads.guardrails.impacted-workloads",
			CheckName:        "Workloads :: Guardrails :: Impacted Workloads (3.x)",
			CheckDescription: "Detects GuardrailsOrchestrator CRs with configuration that will be impacted in RHOAI 3.x upgrade",
			CheckRemediation: "Review and fix GuardrailsOrchestrator configuration before upgrading to ensure correct operation in RHOAI 3.x",
		},
	}
}

// CanApply returns whether this check should run for the given target.
// Only applies when upgrading from 2.x to 3.x and TrustyAI is Managed.
func (c *ImpactedWorkloadsCheck) CanApply(ctx context.Context, target check.Target) (bool, error) {
	if !version.IsUpgradeFrom2xTo3x(target.CurrentVersion, target.TargetVersion) {
		return false, nil
	}

	dsc, err := client.GetDataScienceCluster(ctx, target.Client)
	if err != nil {
		return false, fmt.Errorf("getting DataScienceCluster: %w", err)
	}

	return components.HasManagementState(dsc, "trustyai", constants.ManagementStateManaged), nil
}

// Validate executes the check against the provided target.
func (c *ImpactedWorkloadsCheck) Validate(
	ctx context.Context,
	target check.Target,
) (*result.DiagnosticResult, error) {
	dr := c.NewResult()

	if target.TargetVersion != nil {
		dr.Annotations[check.AnnotationCheckTargetVersion] = target.TargetVersion.String()
	}

	// List all GuardrailsOrchestrator CRs across all namespaces.
	orchestrators, err := client.List[*unstructured.Unstructured](
		ctx, target.Client, resources.GuardrailsOrchestrator, nil,
	)
	if err != nil {
		return nil, fmt.Errorf("listing GuardrailsOrchestrators: %w", err)
	}

	total := len(orchestrators)

	var impactedCRs int

	for _, orch := range orchestrators {
		cr := c.validateCR(ctx, target.Client, orch)

		if len(cr.annotations) > 0 {
			impactedCRs++
			c.appendImpactedObject(dr, orch, cr.annotations)
		}

		// Add misconfigured ConfigMaps as impacted objects.
		c.appendImpactedConfigMaps(dr, orch, cr)
	}

	dr.Status.Conditions = append(dr.Status.Conditions,
		c.newConfigurationCondition(total, impactedCRs),
	)

	dr.Annotations[check.AnnotationImpactedWorkloadCount] = strconv.Itoa(len(dr.ImpactedObjects))

	return dr, nil
}
