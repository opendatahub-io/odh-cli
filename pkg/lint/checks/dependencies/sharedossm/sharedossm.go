package sharedossm

import (
	"context"
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/lint/checks/dependencies/shared"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
	"github.com/opendatahub-io/odh-cli/pkg/util/version"
)

const (
	checkKind = "shared-ossm"
	checkType = "shared-usage"
)

func managedNamespaces() []string {
	return []string{
		"istio-system",
		"openshift-operators",
	}
}

// Check detects OSSM resources (SMCP, SMMR, SMM) outside RHOAI-managed namespaces,
// indicating shared Service Mesh usage between AI and non-AI workloads.
type Check struct {
	check.BaseCheck
}

func NewCheck() *Check {
	return &Check{
		BaseCheck: check.BaseCheck{
			CheckGroup:       check.GroupDependency,
			Kind:             checkKind,
			Type:             checkType,
			CheckID:          "dependencies.shared-ossm.shared-usage",
			CheckName:        "Dependencies :: Shared OSSM :: Shared Usage Detection",
			CheckDescription: "Detects OpenShift Service Mesh resources shared between RHOAI and non-AI workloads",
			CheckRemediation: "Review the identified Service Mesh resources before migration. Non-AI workloads sharing OSSM may be impacted by the RHOAI 2.x to 3.x migration.",
		},
	}
}

func (c *Check) CanApply(_ context.Context, target check.Target) (bool, error) {
	return version.IsUpgradeFrom2xTo3x(target.CurrentVersion, target.TargetVersion), nil
}

func (c *Check) Validate(ctx context.Context, target check.Target) (*result.DiagnosticResult, error) {
	dr := c.NewResult()

	rhoaiNS := shared.RHOAIManagedNamespaces(ctx, target.Client, managedNamespaces())
	isNonRHOAI := shared.IsNonRHOAIFilter(rhoaiNS)

	smcps, err := client.List(ctx, target.Client, resources.ServiceMeshControlPlane, isNonRHOAI)
	if err != nil {
		return nil, fmt.Errorf("listing ServiceMeshControlPlanes: %w", err)
	}

	smmrs, err := client.List(ctx, target.Client, resources.ServiceMeshMemberRoll, isNonRHOAI)
	if err != nil {
		return nil, fmt.Errorf("listing ServiceMeshMemberRolls: %w", err)
	}

	smms, err := client.List(ctx, target.Client, resources.ServiceMeshMember, isNonRHOAI)
	if err != nil {
		return nil, fmt.Errorf("listing ServiceMeshMembers: %w", err)
	}

	totalCount := len(smcps) + len(smmrs) + len(smms)
	if totalCount == 0 {
		dr.SetCondition(check.NewCondition(
			check.ConditionTypeValidated,
			metav1.ConditionTrue,
			check.WithReason(check.ReasonRequirementsMet),
			check.WithMessage("No shared OSSM resources detected outside RHOAI-managed namespaces"),
		))

		return dr, nil
	}

	namespaces := shared.CollectNamespaces(smcps, smmrs, smms)

	dr.SetCondition(check.NewCondition(
		check.ConditionTypeValidated,
		metav1.ConditionFalse,
		check.WithReason(check.ReasonWorkloadsImpacted),
		check.WithMessage(
			"Found %d OSSM resource(s) outside RHOAI-managed namespaces in: %s. These may be impacted by the RHOAI migration",
			totalCount,
			strings.Join(namespaces, ", "),
		),
		check.WithRemediation(c.CheckRemediation),
	))

	shared.AddAllImpactedObjects(dr,
		shared.ImpactedEntry{ResourceType: resources.ServiceMeshControlPlane, Items: smcps},
		shared.ImpactedEntry{ResourceType: resources.ServiceMeshMemberRoll, Items: smmrs},
		shared.ImpactedEntry{ResourceType: resources.ServiceMeshMember, Items: smms},
	)

	return dr, nil
}
