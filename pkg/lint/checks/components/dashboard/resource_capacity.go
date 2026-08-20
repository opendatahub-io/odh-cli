package dashboard

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/opendatahub-io/odh-cli/pkg/constants"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/validate"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/version"
)

const (
	conditionTypeResourceCapacity = "ResourceCapacity"

	msgResourceCapacityWithAutoscaler    = "RHOAI 3.5 dashboard pods run 9 containers (up from 3 in 2.25), requiring additional CPU/memory. Cluster autoscaler detected"
	msgResourceCapacityWithoutAutoscaler = "RHOAI 3.5 dashboard pods run 9 containers (up from 3 in 2.25), requiring additional CPU/memory. No cluster autoscaler detected - ensure sufficient node capacity"
)

// ResourceCapacityCheck advises about increased resource requirements for dashboard pods in 3.x.
type ResourceCapacityCheck struct {
	check.BaseCheck
}

func NewResourceCapacityCheck() *ResourceCapacityCheck {
	return &ResourceCapacityCheck{
		BaseCheck: check.BaseCheck{
			CheckGroup:       check.GroupComponent,
			Kind:             constants.ComponentDashboard,
			Type:             check.CheckTypeResourceCapacity,
			CheckID:          "components.dashboard.resource-capacity",
			CheckName:        "Components :: Dashboard :: Resource Capacity (3.x)",
			CheckDescription: "Advises about increased resource requirements for dashboard pods in RHOAI 3.x",
			CheckRemediation: "Ensure cluster has sufficient CPU/memory capacity for dashboard pods with 9 containers. Consider enabling cluster autoscaler if not already active",
		},
	}
}

func (c *ResourceCapacityCheck) CanApply(_ context.Context, target check.Target) (bool, error) {
	return version.IsUpgradeFrom2xTo3x(target.CurrentVersion, target.TargetVersion), nil
}

func (c *ResourceCapacityCheck) Validate(
	ctx context.Context,
	target check.Target,
) (*result.DiagnosticResult, error) {
	return validate.Component(c, target).
		Run(ctx, c.checkCapacity)
}

func (c *ResourceCapacityCheck) checkCapacity(
	ctx context.Context,
	req *validate.ComponentRequest,
) error {
	autoscalers, err := req.Client.List(ctx, resources.ClusterAutoscaler)

	hasAutoscaler := err == nil && len(autoscalers) > 0

	msg := msgResourceCapacityWithoutAutoscaler
	if hasAutoscaler {
		msg = msgResourceCapacityWithAutoscaler
	}

	req.Result.SetCondition(check.NewCondition(
		conditionTypeResourceCapacity,
		metav1.ConditionFalse,
		check.WithReason(check.ReasonWorkloadsImpacted),
		check.WithMessage("%s", msg),
		check.WithImpact(result.ImpactAdvisory),
		check.WithRemediation(c.CheckRemediation),
	))

	return nil
}
