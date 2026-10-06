package mcpserver

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/validate"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/version"
)

const (
	kind = "mcpserver"

	// mcpGAMajor and mcpGAMinor identify the RHOAI release in which the MCP
	// lifecycle operator goes GA and the operand NetworkPolicy default flips
	// to the restricted, deny-by-default ingress posture. The check only warns
	// when an upgrade crosses into that release. 3.6 is the current plan of
	// record; adjust these constants if the GA release number changes.
	mcpGAMajor = 3
	mcpGAMinor = 6
)

// NetworkPolicyPostureCheck warns that upgrading into the MCP lifecycle
// operator GA release flips the operand ingress NetworkPolicy to a
// deny-by-default posture, which can cut off direct, non-gateway consumers of
// existing MCPServer workloads.
type NetworkPolicyPostureCheck struct {
	check.BaseCheck
}

func NewNetworkPolicyPostureCheck() *NetworkPolicyPostureCheck {
	return &NetworkPolicyPostureCheck{
		BaseCheck: check.BaseCheck{
			CheckGroup:       check.GroupWorkload,
			Kind:             kind,
			Type:             check.CheckTypeImpactedWorkloads,
			CheckID:          "workloads.mcpserver.networkpolicy-posture",
			CheckName:        "Workloads :: MCPServer :: NetworkPolicy Posture",
			CheckDescription: "Warns that MCPServer ingress becomes deny-by-default when upgrading into the MCP lifecycle operator GA release",
			CheckRemediation: "For MCPServers that need direct (non-gateway) access, add spec.network.ingressFrom entries, or have a cluster admin create an allowing NetworkPolicy, before upgrading",
		},
	}
}

// CanApply returns whether this check should run for the given target.
// It only applies when an upgrade crosses into the GA release where the
// restricted NetworkPolicy posture ships.
func (c *NetworkPolicyPostureCheck) CanApply(_ context.Context, target check.Target) (bool, error) {
	if target.CurrentVersion == nil || target.TargetVersion == nil {
		return false, nil
	}

	crossesIntoGA := version.IsVersionAtLeast(target.TargetVersion, mcpGAMajor, mcpGAMinor) &&
		!version.IsVersionAtLeast(target.CurrentVersion, mcpGAMajor, mcpGAMinor)

	return crossesIntoGA, nil
}

// Validate executes the check against the provided target.
func (c *NetworkPolicyPostureCheck) Validate(ctx context.Context, target check.Target) (*result.DiagnosticResult, error) {
	return validate.WorkloadsMetadata(c, target, resources.MCPServer).
		Complete(ctx, c.newPostureCondition)
}

func (c *NetworkPolicyPostureCheck) newPostureCondition(
	_ context.Context,
	req *validate.WorkloadRequest[*metav1.PartialObjectMetadata],
) ([]result.Condition, error) {
	count := len(req.Items)

	if count == 0 {
		return []result.Condition{check.NewCondition(
			check.ConditionTypeCompatible,
			metav1.ConditionTrue,
			check.WithReason(check.ReasonRequirementsMet),
			check.WithMessage("No MCPServer resources found - the restricted NetworkPolicy posture has no impact"),
		)}, nil
	}

	return []result.Condition{check.NewCondition(
		check.ConditionTypeCompatible,
		metav1.ConditionFalse,
		check.WithReason(check.ReasonWorkloadsImpacted),
		check.WithMessage(
			"Found %d MCPServer(s). After the upgrade, MCPServer ingress becomes deny-by-default: "+
				"traffic that does not originate from the gateway or is not listed in spec.network.ingressFrom will be blocked. "+
				"Add ingressFrom entries, or a cluster-admin NetworkPolicy, where direct (non-gateway) access is required.",
			count),
		check.WithImpact(result.ImpactAdvisory),
		check.WithRemediation(c.CheckRemediation),
	)}, nil
}
