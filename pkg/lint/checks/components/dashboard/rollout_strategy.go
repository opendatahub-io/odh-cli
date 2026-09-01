package dashboard

import (
	"context"
	"fmt"
	"math"
	"strconv"
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
	"github.com/opendatahub-io/odh-cli/pkg/util/jq"
	"github.com/opendatahub-io/odh-cli/pkg/util/version"
)

const (
	conditionTypeRolloutStrategy = "RolloutStrategy"

	// strategyRecreate tears down all pods before creating new ones, so
	// maxUnavailable and maxSurge do not apply.
	strategyRecreate = "Recreate"

	// defaultMaxUnavailable and defaultMaxSurge are the values Kubernetes applies
	// when a RollingUpdate strategy leaves the fields unset.
	defaultMaxUnavailable = "25%"
	defaultMaxSurge       = "25%"

	// minReplicasForDeadlock is the replica count below which a rolling update
	// cannot stall on maxUnavailable, since a single pod is always replaced.
	minReplicasForDeadlock = 2

	msgRolloutSurgeOnly    = "Deployment %q has %d replica(s) with maxUnavailable=%s, which resolves to 0 - the rolling update can only progress by surging %d extra pod(s), and will stall if the cluster cannot schedule them alongside the existing pods"
	msgRolloutOK           = "Deployment rollout strategy allows progress"
	msgRolloutRecreate     = "Deployment uses the Recreate strategy - no rolling update constraints apply"
	msgRolloutNoDeployment = "Dashboard deployment not found - rollout strategy cannot be evaluated"

	remediationRollout = "Set maxUnavailable to at least 1 in the deployment rollout strategy, or make sure the cluster has headroom to schedule the surge pods alongside the existing ones"
)

// RolloutStrategyCheck detects a rolling update deadlock on the dashboard
// deployment, where the effective maxUnavailable rounds to 0 (for example 25%
// with 2 replicas) so no old pod can be evicted to make room for a new one.
type RolloutStrategyCheck struct {
	check.BaseCheck
}

func NewRolloutStrategyCheck() *RolloutStrategyCheck {
	return &RolloutStrategyCheck{
		BaseCheck: check.BaseCheck{
			CheckGroup:       check.GroupComponent,
			Kind:             constants.ComponentDashboard,
			Type:             check.CheckTypeRolloutStrategy,
			CheckID:          "components.dashboard.rollout-strategy",
			CheckName:        "Components :: Dashboard :: Rollout Strategy (3.x)",
			CheckDescription: "Detects a dashboard rolling update deadlock caused by an effective maxUnavailable of 0",
			CheckRemediation: remediationRollout,
		},
	}
}

func (c *RolloutStrategyCheck) CanApply(_ context.Context, target check.Target) (bool, error) {
	return version.IsUpgradeFrom2xTo3x(target.CurrentVersion, target.TargetVersion), nil
}

func (c *RolloutStrategyCheck) Validate(
	ctx context.Context,
	target check.Target,
) (*result.DiagnosticResult, error) {
	return validate.Component(c, target).
		WithApplicationsNamespace().
		Run(ctx, c.checkRolloutStrategy)
}

func (c *RolloutStrategyCheck) checkRolloutStrategy(
	ctx context.Context,
	req *validate.ComponentRequest,
) error {
	deploy, err := getDashboardDeployment(ctx, req.Client, req.ApplicationsNamespace)
	if err != nil {
		return err
	}

	if deploy == nil {
		req.Result.SetCondition(check.NewCondition(
			conditionTypeRolloutStrategy,
			metav1.ConditionTrue,
			check.WithReason(check.ReasonResourceNotFound),
			check.WithMessage(msgRolloutNoDeployment),
		))

		return nil
	}

	if strategyType, _ := jq.Query[string](deploy, ".spec.strategy.type"); strategyType == strategyRecreate {
		req.Result.SetCondition(check.NewCondition(
			conditionTypeRolloutStrategy,
			metav1.ConditionTrue,
			check.WithReason(check.ReasonRequirementsMet),
			check.WithMessage(msgRolloutRecreate),
		))

		return nil
	}

	replicas, _ := jq.Query[float64](deploy, ".spec.replicas")
	if int(replicas) < minReplicasForDeadlock {
		req.Result.SetCondition(check.NewCondition(
			conditionTypeRolloutStrategy,
			metav1.ConditionTrue,
			check.WithReason(check.ReasonRequirementsMet),
			check.WithMessage(msgRolloutOK),
		))

		return nil
	}

	maxUnavailRaw, _ := jq.Query[any](deploy, ".spec.strategy.rollingUpdate.maxUnavailable")
	maxSurgeRaw, _ := jq.Query[any](deploy, ".spec.strategy.rollingUpdate.maxSurge")

	// maxUnavailable rounds down and maxSurge rounds up, matching the scheduler.
	maxUnavailable := resolveFencepost(maxUnavailRaw, int(replicas), defaultMaxUnavailable, math.Floor)
	maxSurge := resolveFencepost(maxSurgeRaw, int(replicas), defaultMaxSurge, math.Ceil)

	// Kubernetes' own ResolveFenceposts forces maxUnavailable to 1 when both
	// fenceposts resolve to 0, so that combination cannot actually stall. What is
	// left is a surge-only rollout, which progresses only if the cluster can fit
	// the extra pods.
	if maxUnavailable == 0 && maxSurge > 0 {
		req.Result.SetCondition(check.NewCondition(
			conditionTypeRolloutStrategy,
			metav1.ConditionFalse,
			check.WithReason(check.ReasonWorkloadsImpacted),
			check.WithMessage(msgRolloutSurgeOnly,
				dashboardDeploymentName, int(replicas), describeFencepost(maxUnavailRaw, defaultMaxUnavailable), maxSurge),
			check.WithImpact(result.ImpactAdvisory),
			check.WithRemediation(c.CheckRemediation),
		))

		return nil
	}

	req.Result.SetCondition(check.NewCondition(
		conditionTypeRolloutStrategy,
		metav1.ConditionTrue,
		check.WithReason(check.ReasonRequirementsMet),
		check.WithMessage(msgRolloutOK),
	))

	return nil
}

// getDashboardDeployment returns the dashboard deployment, or nil when it is absent.
func getDashboardDeployment(
	ctx context.Context,
	cl client.Reader,
	namespace string,
) (*unstructured.Unstructured, error) {
	deploy, err := cl.GetResource(ctx, resources.Deployment, dashboardDeploymentName,
		client.InNamespace(namespace))
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil //nolint:nilnil // absent deployment is not an error
		}

		return nil, fmt.Errorf("getting dashboard deployment: %w", err)
	}

	return deploy, nil
}

// resolveFencepost turns a maxUnavailable/maxSurge value - an int or a
// percentage string - into a pod count. An absent field takes the Kubernetes
// default rather than a neutral value, so that omitting the field and setting it
// explicitly to the same percentage are reported identically. round is
// math.Floor for maxUnavailable and math.Ceil for maxSurge, matching Kubernetes.
// Unparseable values fall back to 1 so the check never claims a stall it cannot
// prove.
func resolveFencepost(raw any, replicas int, defaultValue string, round func(float64) float64) int {
	if raw == nil {
		raw = defaultValue
	}

	switch v := raw.(type) {
	case float64:
		return int(v)
	case string:
		if pctStr, ok := strings.CutSuffix(v, "%"); ok {
			pct, err := strconv.ParseFloat(pctStr, 64)
			if err != nil {
				return 1
			}

			return int(round(pct / 100.0 * float64(replicas)))
		}

		n, err := strconv.Atoi(v)
		if err != nil {
			return 1
		}

		return n
	default:
		return 1
	}
}

// describeFencepost renders the configured value for the diagnostic message,
// making it explicit when the value came from the Kubernetes default.
func describeFencepost(raw any, defaultValue string) string {
	if raw == nil {
		return defaultValue + " (Kubernetes default)"
	}

	return fmt.Sprintf("%v", raw)
}
