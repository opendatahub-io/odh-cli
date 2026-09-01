package dashboard

import (
	"context"
	"fmt"
	"strconv"

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

	dashboardDeploymentName = "rhods-dashboard"
	dashboardLabelSelector  = "app=rhods-dashboard"

	msgCapacityBlocking         = "No node has sufficient allocatable resources for dashboard pods (need %s CPU, %s memory; largest node has %s CPU, %s memory). Add larger nodes or reduce pod resource requests"
	msgCapacityAdvisory         = "Dashboard pods fit on cluster nodes but no cluster autoscaler detected - ensure sufficient node capacity for %d container(s) per pod"
	msgCapacityOK               = "Dashboard pods fit on cluster nodes (need %s CPU, %s memory)"
	msgCapacityNoData           = "Could not determine dashboard pod resource requirements"
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

// ResourceCapacityCheck validates that cluster nodes have sufficient allocatable
// resources to schedule the dashboard pods.
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
			CheckDescription: "Validates that cluster nodes have enough allocatable CPU and memory to schedule the new dashboard pods",
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

	var hasAutoscaler *bool

	if autoErr == nil {
		v := len(autoscalers) > 0
		hasAutoscaler = &v
	}

	setCapacityCondition(req, lookup.resources, nodes, lookup.containerCount, hasAutoscaler)

	return nil
}

func setCapacityCondition(
	req *validate.ComponentRequest,
	podReq podResources,
	nodes []podResources,
	containerCount int,
	hasAutoscaler *bool,
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

	if hasAutoscaler != nil && !*hasAutoscaler {
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
	deploy, err := getDashboardDeployment(ctx, cl, namespace)
	if err != nil {
		return resourceLookupResult{}, err
	}

	if deploy == nil {
		return resourceLookupResult{}, nil
	}

	req, count, err := effectivePodRequests(deploy, ".spec.template.spec")
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
		req, count, pErr := effectivePodRequests(pod, ".spec")
		if pErr != nil {
			continue
		}

		if req.cpuMillis > maxReq.cpuMillis {
			maxReq.cpuMillis = req.cpuMillis
		}

		if req.memoryBytes > maxReq.memoryBytes {
			maxReq.memoryBytes = req.memoryBytes
		}

		if count > maxCount {
			maxCount = count
		}
	}

	found := maxReq.cpuMillis > 0 || maxReq.memoryBytes > 0

	return resourceLookupResult{resources: maxReq, containerCount: maxCount, found: found}, nil
}

// effectivePodRequests computes the pod-level request the scheduler actually
// uses, which is not simply the sum of the regular containers. Init containers
// run to completion before the regular containers start, so they never hold
// resources at the same time: for each resource the pod request is the larger
// of the regular-container sum and the biggest single init-container request.
// Native sidecars (init containers with restartPolicy Always) are the exception
// - they keep running alongside the regular containers, so they add to the sum.
//
// It also returns the number of containers that run concurrently, used to
// describe the pod in the advisory message.
func effectivePodRequests(
	obj *unstructured.Unstructured,
	podSpecPath string,
) (podResources, int, error) {
	spec, err := jq.Query[map[string]any](obj, podSpecPath)
	if err != nil {
		return podResources{}, 0, fmt.Errorf("querying pod spec at %s: %w", podSpecPath, err)
	}

	containers, _ := spec["containers"].([]any)
	initContainers, _ := spec["initContainers"].([]any)

	total := podResources{}
	count := len(containers)

	for _, raw := range containers {
		req := containerRequests(raw)
		total.cpuMillis += req.cpuMillis
		total.memoryBytes += req.memoryBytes
	}

	var maxInit podResources

	for _, raw := range initContainers {
		req := containerRequests(raw)

		if isNativeSidecar(raw) {
			total.cpuMillis += req.cpuMillis
			total.memoryBytes += req.memoryBytes
			count++

			continue
		}

		maxInit.cpuMillis = max(maxInit.cpuMillis, req.cpuMillis)
		maxInit.memoryBytes = max(maxInit.memoryBytes, req.memoryBytes)
	}

	return podResources{
		cpuMillis:   max(total.cpuMillis, maxInit.cpuMillis),
		memoryBytes: max(total.memoryBytes, maxInit.memoryBytes),
	}, count, nil
}

// containerRequests reads spec.resources.requests off a single container,
// returning zeroes for anything absent or malformed.
func containerRequests(raw any) podResources {
	ctr, ok := raw.(map[string]any)
	if !ok {
		return podResources{}
	}

	res, _ := ctr["resources"].(map[string]any)
	if res == nil {
		return podResources{}
	}

	requests, _ := res["requests"].(map[string]any)
	if requests == nil {
		return podResources{}
	}

	var out podResources

	if q, ok := parseQuantityAny(requests["cpu"]); ok {
		out.cpuMillis = q.MilliValue()
	}

	if q, ok := parseQuantityAny(requests["memory"]); ok {
		out.memoryBytes = q.Value()
	}

	return out
}

// isNativeSidecar reports whether an init container is a sidecar, i.e. it keeps
// running for the lifetime of the pod instead of completing first.
func isNativeSidecar(raw any) bool {
	ctr, ok := raw.(map[string]any)
	if !ok {
		return false
	}

	policy, _ := ctr["restartPolicy"].(string)

	return policy == "Always"
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
		cpuRaw, _ := jq.Query[any](node, ".status.allocatable.cpu")
		memRaw, _ := jq.Query[any](node, ".status.allocatable.memory")

		var pr podResources

		if q, ok := parseQuantityAny(cpuRaw); ok {
			pr.cpuMillis = q.MilliValue()
		}

		if q, ok := parseQuantityAny(memRaw); ok {
			pr.memoryBytes = q.Value()
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

func parseQuantityAny(raw any) (resource.Quantity, bool) {
	switch v := raw.(type) {
	case string:
		q, err := resource.ParseQuantity(v)

		return q, err == nil
	case int64:
		return *resource.NewQuantity(v, resource.DecimalSI), true
	case float64:
		// Format without an exponent so fractional values such as 0.5 CPU survive
		// instead of being truncated to a whole number.
		q, err := resource.ParseQuantity(strconv.FormatFloat(v, 'f', -1, 64))

		return q, err == nil
	default:
		return resource.Quantity{}, false
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
