package dashboard

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"

	"k8s.io/apimachinery/pkg/api/resource"
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
	conditionTypeResourceCapacity = "ResourceCapacity"
	conditionTypeRolloutStrategy  = "RolloutStrategy"

	dashboardDeploymentName = "rhods-dashboard"
	dashboardLabelSelector  = "app=rhods-dashboard"

	msgCapacityBlocking         = "No node has sufficient allocatable resources for dashboard pods (need %s CPU, %s memory; largest node has %s CPU, %s memory). Add larger nodes or reduce pod resource requests"
	msgCapacityAdvisory         = "Dashboard pods fit on cluster nodes but no cluster autoscaler detected - ensure sufficient node capacity for %d container(s) per pod"
	msgCapacityOK               = "Dashboard pods fit on cluster nodes (need %s CPU, %s memory)"
	msgCapacityNoData           = "Could not determine dashboard pod resource requirements"
	msgRolloutDeadlock          = "Deployment %q has %d replica(s) with maxUnavailable=%s which rounds to 0 - rolling update will stall if new pods cannot schedule alongside old pods"
	msgRolloutOK                = "Deployment rollout strategy allows progress"
	remediationCapacityBlocking = "Add nodes with at least %s CPU and %s memory allocatable, or enable cluster autoscaler with a machine pool using larger instance types"
	remediationCapacityAdvisory = "Ensure cluster has nodes with sufficient CPU/memory for dashboard pods. Consider enabling cluster autoscaler with larger instance types"
)

type podResources struct {
	cpuMillis   int64
	memoryBytes int64
}

type resourceLookupResult struct {
	resources      podResources
	containerCount int
	found          bool
}

// ResourceCapacityCheck validates that cluster nodes have sufficient resources
// for the new dashboard pod spec and that the rollout strategy won't deadlock.
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
			CheckDescription: "Validates that cluster nodes can schedule the new dashboard pods and rollout strategy allows progress",
			CheckRemediation: remediationCapacityAdvisory,
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
		WithApplicationsNamespace().
		Run(ctx, c.checkCapacity)
}

func (c *ResourceCapacityCheck) checkCapacity(
	ctx context.Context,
	req *validate.ComponentRequest,
) error {
	lookup, err := getRequiredResources(ctx, req.Client, req.ApplicationsNamespace)
	if err != nil {
		return err
	}

	if !lookup.found {
		req.Result.SetCondition(check.NewCondition(
			conditionTypeResourceCapacity,
			metav1.ConditionTrue,
			check.WithReason(check.ReasonRequirementsMet),
			check.WithMessage(msgCapacityNoData),
		))

		return nil
	}

	nodes, err := getNodeAllocatable(ctx, req.Client)
	if err != nil {
		return err
	}

	autoscalers, autoErr := req.Client.List(ctx, resources.ClusterAutoscaler)
	hasAutoscaler := autoErr == nil && len(autoscalers) > 0

	setCapacityCondition(req, lookup.resources, nodes, lookup.containerCount, hasAutoscaler)

	return checkRolloutDeadlock(ctx, req)
}

func setCapacityCondition(
	req *validate.ComponentRequest,
	podReq podResources,
	nodes []podResources,
	containerCount int,
	hasAutoscaler bool,
) {
	cpuStr := formatCPU(podReq.cpuMillis)
	memStr := formatMemory(podReq.memoryBytes)

	if !anyNodeFits(podReq, nodes) {
		var largestCPU, largestMem int64
		for _, n := range nodes {
			if n.cpuMillis > largestCPU {
				largestCPU = n.cpuMillis
			}
			if n.memoryBytes > largestMem {
				largestMem = n.memoryBytes
			}
		}

		req.Result.SetCondition(check.NewCondition(
			conditionTypeResourceCapacity,
			metav1.ConditionFalse,
			check.WithReason(check.ReasonInsufficientCapacity),
			check.WithMessage(msgCapacityBlocking, cpuStr, memStr, formatCPU(largestCPU), formatMemory(largestMem)),
			check.WithImpact(result.ImpactBlocking),
			check.WithRemediation(fmt.Sprintf(remediationCapacityBlocking, cpuStr, memStr)),
		))

		return
	}

	if !hasAutoscaler {
		req.Result.SetCondition(check.NewCondition(
			conditionTypeResourceCapacity,
			metav1.ConditionFalse,
			check.WithReason(check.ReasonWorkloadsImpacted),
			check.WithMessage(msgCapacityAdvisory, containerCount),
			check.WithImpact(result.ImpactAdvisory),
			check.WithRemediation(remediationCapacityAdvisory),
		))

		return
	}

	req.Result.SetCondition(check.NewCondition(
		conditionTypeResourceCapacity,
		metav1.ConditionTrue,
		check.WithReason(check.ReasonRequirementsMet),
		check.WithMessage(msgCapacityOK, cpuStr, memStr),
	))
}

func getRequiredResources(
	ctx context.Context,
	cl client.Reader,
	namespace string,
) (resourceLookupResult, error) {
	res, err := getDeploymentPodResources(ctx, cl, namespace)
	if err != nil {
		return resourceLookupResult{}, err
	}

	if res.found {
		return res, nil
	}

	return getPodMaxResources(ctx, cl, namespace)
}

func getDeploymentPodResources(
	ctx context.Context,
	cl client.Reader,
	namespace string,
) (resourceLookupResult, error) {
	deploy, err := cl.GetResource(ctx, resources.Deployment, dashboardDeploymentName,
		client.InNamespace(namespace))
	if err != nil || deploy == nil {
		return resourceLookupResult{}, nil //nolint:nilerr // Not-found is expected
	}

	req, count, err := sumContainerRequests(deploy, ".spec.template.spec.containers")
	if err != nil {
		return resourceLookupResult{}, fmt.Errorf("reading deployment container requests: %w", err)
	}

	return resourceLookupResult{resources: req, containerCount: count, found: true}, nil
}

func getPodMaxResources(
	ctx context.Context,
	cl client.Reader,
	namespace string,
) (resourceLookupResult, error) {
	pods, err := cl.List(ctx, resources.Pod,
		client.WithNamespace(namespace),
		client.WithLabelSelector(dashboardLabelSelector))
	if err != nil {
		return resourceLookupResult{}, fmt.Errorf("listing dashboard pods: %w", err)
	}

	if len(pods) == 0 {
		return resourceLookupResult{}, nil
	}

	var maxReq podResources
	var maxCount int

	for _, pod := range pods {
		req, count, pErr := sumContainerRequests(pod, ".spec.containers")
		if pErr != nil {
			continue
		}

		if req.cpuMillis > maxReq.cpuMillis || req.memoryBytes > maxReq.memoryBytes {
			maxReq = req
			maxCount = count
		}
	}

	found := maxReq.cpuMillis > 0 || maxReq.memoryBytes > 0

	return resourceLookupResult{resources: maxReq, containerCount: maxCount, found: found}, nil
}

func sumContainerRequests(
	obj *unstructured.Unstructured,
	containersPath string,
) (podResources, int, error) {
	containers, err := jq.Query[[]any](obj, containersPath)
	if err != nil {
		return podResources{}, 0, fmt.Errorf("querying containers at %s: %w", containersPath, err)
	}

	var total podResources

	for _, raw := range containers {
		ctr, ok := raw.(map[string]any)
		if !ok {
			continue
		}

		res, _ := ctr["resources"].(map[string]any)
		if res == nil {
			continue
		}

		requests, _ := res["requests"].(map[string]any)
		if requests == nil {
			continue
		}

		if cpuStr, ok := requests["cpu"].(string); ok {
			if q, pErr := resource.ParseQuantity(cpuStr); pErr == nil {
				total.cpuMillis += q.MilliValue()
			}
		}

		if memStr, ok := requests["memory"].(string); ok {
			if q, pErr := resource.ParseQuantity(memStr); pErr == nil {
				total.memoryBytes += q.Value()
			}
		}
	}

	return total, len(containers), nil
}

func getNodeAllocatable(
	ctx context.Context,
	cl client.Reader,
) ([]podResources, error) {
	nodes, err := cl.List(ctx, resources.Node)
	if err != nil {
		return nil, fmt.Errorf("listing nodes: %w", err)
	}

	capacities := make([]podResources, 0, len(nodes))

	for _, node := range nodes {
		cpuStr, _ := jq.Query[string](node, ".status.allocatable.cpu")
		memStr, _ := jq.Query[string](node, ".status.allocatable.memory")

		var pr podResources

		if cpuStr != "" {
			if q, pErr := resource.ParseQuantity(cpuStr); pErr == nil {
				pr.cpuMillis = q.MilliValue()
			}
		}

		if memStr != "" {
			if q, pErr := resource.ParseQuantity(memStr); pErr == nil {
				pr.memoryBytes = q.Value()
			}
		}

		if pr.cpuMillis > 0 || pr.memoryBytes > 0 {
			capacities = append(capacities, pr)
		}
	}

	return capacities, nil
}

func anyNodeFits(podReq podResources, nodes []podResources) bool {
	for _, n := range nodes {
		if n.cpuMillis >= podReq.cpuMillis && n.memoryBytes >= podReq.memoryBytes {
			return true
		}
	}

	return false
}

func checkRolloutDeadlock(
	ctx context.Context,
	req *validate.ComponentRequest,
) error {
	deploy, err := req.Client.GetResource(ctx, resources.Deployment, dashboardDeploymentName,
		client.InNamespace(req.ApplicationsNamespace))
	if err != nil || deploy == nil {
		return nil //nolint:nilerr // Not-found is fine — no rollout to check
	}

	replicas, _ := jq.Query[float64](deploy, ".spec.replicas")
	if replicas < 2 {
		req.Result.SetCondition(check.NewCondition(
			conditionTypeRolloutStrategy,
			metav1.ConditionTrue,
			check.WithReason(check.ReasonRequirementsMet),
			check.WithMessage(msgRolloutOK),
		))

		return nil
	}

	maxUnavailRaw, _ := jq.Query[any](deploy, ".spec.strategy.rollingUpdate.maxUnavailable")

	effective := computeEffectiveMaxUnavailable(maxUnavailRaw, int(replicas))

	if effective == 0 {
		maxUnavailStr := fmt.Sprintf("%v", maxUnavailRaw)

		req.Result.SetCondition(check.NewCondition(
			conditionTypeRolloutStrategy,
			metav1.ConditionFalse,
			check.WithReason(check.ReasonWorkloadsImpacted),
			check.WithMessage(msgRolloutDeadlock, dashboardDeploymentName, int(replicas), maxUnavailStr),
			check.WithImpact(result.ImpactAdvisory),
			check.WithRemediation("Set maxUnavailable to at least 1 in the deployment rollout strategy, or ensure new pods can schedule alongside existing pods"),
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

func computeEffectiveMaxUnavailable(raw any, replicas int) int {
	switch v := raw.(type) {
	case float64:
		return int(v)
	case string:
		if pctStr, ok := strings.CutSuffix(v, "%"); ok {
			pct, err := strconv.ParseFloat(pctStr, 64)
			if err != nil {
				return 1
			}

			return int(math.Floor(pct / 100.0 * float64(replicas)))
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

const millicoresPerCore = 1000

func formatCPU(millis int64) string {
	if millis%millicoresPerCore == 0 {
		return strconv.FormatInt(millis/millicoresPerCore, 10)
	}

	return strconv.FormatInt(millis, 10) + "m"
}

func formatMemory(bytes int64) string {
	const (
		gi = 1024 * 1024 * 1024
		mi = 1024 * 1024
	)

	if bytes%gi == 0 {
		return strconv.FormatInt(bytes/gi, 10) + "Gi"
	}

	return strconv.FormatInt(bytes/mi, 10) + "Mi"
}
