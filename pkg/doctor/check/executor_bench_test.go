package check_test

import (
	"context"
	"testing"

	"github.com/blang/semver/v4"

	"k8s.io/apimachinery/pkg/runtime"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	"github.com/lburgazzoli/odh-cli/pkg/doctor/check"
	"github.com/lburgazzoli/odh-cli/pkg/doctor/version"
	"github.com/lburgazzoli/odh-cli/pkg/util/client"
)

// BenchmarkExecuteSelective_FullSuite benchmarks execution of all checks.
func BenchmarkExecuteSelective_FullSuite(b *testing.B) {
	registry := setupBenchmarkRegistry()
	executor := check.NewExecutor(registry)

	scheme := runtime.NewScheme()
	dynamicClient := dynamicfake.NewSimpleDynamicClient(scheme)
	c := &client.Client{
		Dynamic: dynamicClient,
	}

	target := &check.CheckTarget{
		Client:   c,
		Version:  &version.ClusterVersion{Version: "2.17.0"},
		Resource: nil,
	}

	ctx := context.Background()

	for b.Loop() {
		_, err := executor.ExecuteSelective(ctx, target, "*", "")
		if err != nil {
			b.Fatalf("ExecuteSelective failed: %v", err)
		}
	}
}

// BenchmarkExecuteSelective_CategoryFilter benchmarks execution with category filter.
func BenchmarkExecuteSelective_CategoryFilter(b *testing.B) {
	registry := setupBenchmarkRegistry()
	executor := check.NewExecutor(registry)

	scheme := runtime.NewScheme()
	dynamicClient := dynamicfake.NewSimpleDynamicClient(scheme)
	c := &client.Client{
		Dynamic: dynamicClient,
	}

	target := &check.CheckTarget{
		Client:   c,
		Version:  &version.ClusterVersion{Version: "2.17.0"},
		Resource: nil,
	}

	ctx := context.Background()

	for b.Loop() {
		_, err := executor.ExecuteSelective(ctx, target, "components", "")
		if err != nil {
			b.Fatalf("ExecuteSelective failed: %v", err)
		}
	}
}

// BenchmarkExecuteSelective_SingleCheck benchmarks execution of a single check.
func BenchmarkExecuteSelective_SingleCheck(b *testing.B) {
	registry := setupBenchmarkRegistry()
	executor := check.NewExecutor(registry)

	scheme := runtime.NewScheme()
	dynamicClient := dynamicfake.NewSimpleDynamicClient(scheme)
	c := &client.Client{
		Dynamic: dynamicClient,
	}

	target := &check.CheckTarget{
		Client:   c,
		Version:  &version.ClusterVersion{Version: "2.17.0"},
		Resource: nil,
	}

	ctx := context.Background()

	for b.Loop() {
		_, err := executor.ExecuteSelective(ctx, target, "components.dashboard", "")
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
	id       string
	category check.CheckCategory
}

func newBenchmarkCheck(categoryStr string, index int) *benchmarkCheck {
	var category check.CheckCategory
	switch categoryStr {
	case "components":
		category = check.CategoryComponent
	case "services":
		category = check.CategoryService
	case "workloads":
		category = check.CategoryWorkload
	}

	return &benchmarkCheck{
		id:       categoryStr + ".bench" + string(rune('0'+index)),
		category: category,
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

func (c *benchmarkCheck) Category() check.CheckCategory {
	return c.category
}

func (c *benchmarkCheck) CanApply(currentVersion *semver.Version, targetVersion *semver.Version) bool {
	return true // Always applicable
}

func (c *benchmarkCheck) Validate(_ context.Context, _ *check.CheckTarget) (*check.DiagnosticResult, error) {
	return &check.DiagnosticResult{
		Status:  check.StatusPass,
		Message: "Benchmark check passed",
	}, nil
}
