package guardrails

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/lburgazzoli/odh-cli/pkg/lint/check"
	"github.com/lburgazzoli/odh-cli/pkg/lint/check/result"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/shared/base"
	"github.com/lburgazzoli/odh-cli/pkg/resources"
	"github.com/lburgazzoli/odh-cli/pkg/util/client"
	"github.com/lburgazzoli/odh-cli/pkg/util/version"
)

const (
	ConditionTypeOrchestratorCRConfigured   = "OrchestratorCRConfigured"
	ConditionTypeOrchestratorConfigMapValid = "OrchestratorConfigMapValid"
	ConditionTypeGatewayConfigMapValid      = "GatewayConfigMapValid"
)

// ImpactedWorkloadsCheck detects GuardrailsOrchestrator CRs with configuration
// that will be impacted in a RHOAI 2.x to 3.x upgrade.
type ImpactedWorkloadsCheck struct {
	base.BaseCheck
}

func NewImpactedWorkloadsCheck() *ImpactedWorkloadsCheck {
	return &ImpactedWorkloadsCheck{
		BaseCheck: base.BaseCheck{
			CheckGroup:       check.GroupWorkload,
			Kind:             kind,
			Type:             check.CheckTypeImpactedWorkloads,
			CheckID:          "workloads.guardrails.impacted-workloads",
			CheckName:        "Workloads :: Guardrails :: Impacted Workloads (3.x)",
			CheckDescription: "Detects GuardrailsOrchestrator CRs with configuration that will be impacted in RHOAI 3.x upgrade",
		},
	}
}

// CanApply returns whether this check should run for the given target.
// Only applies when upgrading from 2.x to 3.x.
func (c *ImpactedWorkloadsCheck) CanApply(_ context.Context, target check.Target) bool {
	return version.IsUpgradeFrom2xTo3x(target.CurrentVersion, target.TargetVersion)
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

	// List all GuardrailsOrchestrator CRs across all namespaces
	orchestrators, err := client.List[*unstructured.Unstructured](
		ctx, target.Client, resources.GuardrailsOrchestrator, nil,
	)
	if err != nil {
		return nil, fmt.Errorf("listing GuardrailsOrchestrators: %w", err)
	}

	total := len(orchestrators)

	// When no CRs exist, return success conditions for all three checks
	if total == 0 {
		dr.Status.Conditions = append(dr.Status.Conditions,
			c.newCRConfigCondition(0, 0, ""),
			c.newOrchestratorCMCondition(0, 0, ""),
			c.newGatewayCMCondition(0, 0, ""),
		)

		dr.Annotations[check.AnnotationImpactedWorkloadCount] = "0"

		return dr, nil
	}

	var (
		crIssueCount        int
		orchCMIssueCount    int
		gatewayCMIssueCount int
	)

	// Collect unique issues across all CRs for descriptive condition messages.
	allCRIssues := sets.New[string]()
	allOrchCMIssues := sets.New[string]()
	allGatewayCMIssues := sets.New[string]()

	for _, orch := range orchestrators {
		impacted := false

		// Validate CR spec fields
		cfg, crIssues := validateCRSpec(orch)
		if len(crIssues) > 0 {
			crIssueCount++
			impacted = true
			allCRIssues.Insert(crIssues...)
		}

		// Validate orchestrator ConfigMap
		if cfg.orchestratorConfigName != "" {
			orchIssues := validateOrchestratorConfigMap(ctx, target.Client, orch.GetNamespace(), cfg.orchestratorConfigName)
			if len(orchIssues) > 0 {
				orchCMIssueCount++
				impacted = true
				allOrchCMIssues.Insert(orchIssues...)
			}
		}

		// Validate gateway ConfigMap
		if cfg.gatewayConfigName != "" {
			gatewayIssues := validateGatewayConfigMap(ctx, target.Client, orch.GetNamespace(), cfg.gatewayConfigName)
			if len(gatewayIssues) > 0 {
				gatewayCMIssueCount++
				impacted = true
				allGatewayCMIssues.Insert(gatewayIssues...)
			}
		}

		if impacted {
			c.appendImpactedObject(dr, orch)
		}
	}

	dr.Status.Conditions = append(dr.Status.Conditions,
		c.newCRConfigCondition(total, crIssueCount, strings.Join(sets.List(allCRIssues), "; ")),
		c.newOrchestratorCMCondition(total, orchCMIssueCount, strings.Join(sets.List(allOrchCMIssues), "; ")),
		c.newGatewayCMCondition(total, gatewayCMIssueCount, strings.Join(sets.List(allGatewayCMIssues), "; ")),
	)

	dr.Annotations[check.AnnotationImpactedWorkloadCount] = strconv.Itoa(len(dr.ImpactedObjects))

	return dr, nil
}
