package check_test

import (
	"context"
	"testing"

	"github.com/blang/semver/v4"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
)

// BenchmarkExecuteSelective_FullSuite benchmarks execution of all checks.
func BenchmarkExecuteSelective_FullSuite(b *testing.B) {
	registry := setupBenchmarkRegistry()
	executor := check.NewExecutor(registry, nil)

	scheme := runtime.NewScheme()
	dynamicClient := dynamicfake.NewSimpleDynamicClient(scheme)
	c := client.NewForTesting(client.TestClientConfig{
		Dynamic: dynamicClient,
	})

	ver := semver.MustParse("2.17.0")
	target := check.Target{
		Client:        c,
		TargetVersion: &ver,
		Resource:      nil,
	}

	ctx := context.Background()

	for b.Loop() {
		_, err := executor.ExecuteSelective(ctx, target, []string{"*"}, "")
		if err != nil {
			b.Fatalf("ExecuteSelective failed: %v", err)
		}
	}
}

// BenchmarkExecuteSelective_GroupFilter benchmarks execution with group filter.
func BenchmarkExecuteSelective_GroupFilter(b *testing.B) {
	registry := setupBenchmarkRegistry()
	executor := check.NewExecutor(registry, nil)

	scheme := runtime.NewScheme()
	dynamicClient := dynamicfake.NewSimpleDynamicClient(scheme)
	c := client.NewForTesting(client.TestClientConfig{
		Dynamic: dynamicClient,
	})

	ver := semver.MustParse("2.17.0")
	target := check.Target{
		Client:        c,
		TargetVersion: &ver,
		Resource:      nil,
	}

	ctx := context.Background()

	for b.Loop() {
		_, err := executor.ExecuteSelective(ctx, target, []string{"components"}, "")
		if err != nil {
			b.Fatalf("ExecuteSelective failed: %v", err)
		}
	}
}

// BenchmarkExecuteSelective_SingleCheck benchmarks execution of a single check.
func BenchmarkExecuteSelective_SingleCheck(b *testing.B) {
	registry := setupBenchmarkRegistry()
	executor := check.NewExecutor(registry, nil)

	scheme := runtime.NewScheme()
	dynamicClient := dynamicfake.NewSimpleDynamicClient(scheme)
	c := client.NewForTesting(client.TestClientConfig{
		Dynamic: dynamicClient,
	})

	ver := semver.MustParse("2.17.0")
	target := check.Target{
		Client:        c,
		TargetVersion: &ver,
		Resource:      nil,
	}

	ctx := context.Background()

	for b.Loop() {
		_, err := executor.ExecuteSelective(ctx, target, []string{"components.dashboard"}, "")
		if err != nil {
			b.Fatalf("ExecuteSelective failed: %v", err)
		}
	}
}

// setupBenchmarkRegistry creates a registry with representative checks.
func setupBenchmarkRegistry() *check.CheckRegistry {
	registry := check.NewRegistry()

	// Register multiple checks across categories to simulate realistic load
	// Components (5 checks)
	for i := range 5 {
		_ = registry.Register(newBenchmarkCheck("components", i))
	}

	// Services (5 checks)
	for i := range 5 {
		_ = registry.Register(newBenchmarkCheck("services", i))
	}

	// Workloads (5 checks)
	for i := range 5 {
		_ = registry.Register(newBenchmarkCheck("workloads", i))
	}

	return registry
}

// benchmarkCheck is a minimal check implementation for benchmarking.
type benchmarkCheck struct {
	id    string
	group check.CheckGroup
}

func newBenchmarkCheck(categoryStr string, index int) *benchmarkCheck {
	var group check.CheckGroup
	switch categoryStr {
	case "components":
		group = check.GroupComponent
	case "services":
		group = check.GroupService
	case "workloads":
		group = check.GroupWorkload
	}

	return &benchmarkCheck{
		id:    categoryStr + ".bench" + string(rune('0'+index)),
		group: group,
	}
}

func (c *benchmarkCheck) ID() string {
	return c.id
}

func (c *benchmarkCheck) Name() string {
	return "Benchmark Check " + c.id
}

func (c *benchmarkCheck) Description() string {
	return "Benchmark check for performance testing"
}

func (c *benchmarkCheck) Group() check.CheckGroup {
	return c.group
}

func (c *benchmarkCheck) CheckKind() string {
	return "bench" + string(rune('0'+len(c.id)))
}

func (c *benchmarkCheck) CheckType() string {
	return "benchmark"
}

func (c *benchmarkCheck) CanApply(_ context.Context, _ check.Target) (bool, error) {
	return true, nil // Always applicable
}

func (c *benchmarkCheck) Validate(_ context.Context, _ check.Target) (*result.DiagnosticResult, error) {
	dr := result.New(
		string(c.group),
		"bench"+string(rune('0'+len(c.id))),
		c.id,
		c.Description(),
	)
	dr.Status.Conditions = []result.Condition{
		check.NewCondition(
			check.ConditionTypeValidated,
			metav1.ConditionTrue,
			check.WithReason(check.ReasonResourceFound),
			check.WithMessage("Benchmark check passed"),
		),
	}

	return dr, nil
}
