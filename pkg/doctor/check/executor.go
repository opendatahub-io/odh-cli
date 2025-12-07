package check

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/blang/semver/v4"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

// CheckExecution represents the result of executing a single check.
type CheckExecution struct {
	Check  Check
	Result *DiagnosticResult
	Error  error
}

// Executor orchestrates check execution.
type Executor struct {
	registry *CheckRegistry
}

// NewExecutor creates a new check executor.
func NewExecutor(registry *CheckRegistry) *Executor {
	return &Executor{
		registry: registry,
	}
}

// ExecuteAll runs all checks in the registry against the target
// Returns results for all checks, including errors.
func (e *Executor) ExecuteAll(ctx context.Context, target *CheckTarget) []CheckExecution {
	checks := e.registry.ListAll()

	return e.executeChecks(ctx, target, checks)
}

// ExecuteSelective runs checks matching the pattern and category
// Returns results for matching checks only.
// Version filtering is done via CanApply during execution.
func (e *Executor) ExecuteSelective(
	ctx context.Context,
	target *CheckTarget,
	pattern string,
	category CheckCategory,
) ([]CheckExecution, error) {
	checks, err := e.registry.ListByPattern(pattern, category)
	if err != nil {
		return nil, fmt.Errorf("selecting checks: %w", err)
	}

	return e.executeChecks(ctx, target, checks), nil
}

// executeChecks runs the provided checks against the target.
func (e *Executor) executeChecks(ctx context.Context, target *CheckTarget, checks []Check) []CheckExecution {
	var (
		mu      sync.Mutex
		wg      sync.WaitGroup
		results = make([]CheckExecution, 0, len(checks))
	)

	// Parse versions once for all checks
	var currentVer, targetVer *semver.Version
	if target.CurrentVersion != nil && target.CurrentVersion.Version != "" {
		parsed, err := semver.Parse(strings.TrimPrefix(target.CurrentVersion.Version, "v"))
		if err == nil {
			currentVer = &parsed
		}
	}
	if target.Version != nil && target.Version.Version != "" {
		parsed, err := semver.Parse(strings.TrimPrefix(target.Version.Version, "v"))
		if err == nil {
			targetVer = &parsed
		}
	}

	for _, check := range checks {
		// Filter by CanApply before executing
		// This allows checks to consider both current and target versions
		if !check.CanApply(currentVer, targetVer) {
			// Skip checks that don't apply to this version combination
			continue
		}

		wg.Add(1)

		go func(c Check) {
			defer wg.Done()

			exec := e.executeCheck(ctx, target, c)

			mu.Lock()
			results = append(results, exec)
			mu.Unlock()
		}(check)
	}

	wg.Wait()

	return results
}

// executeCheck runs a single check and captures the result or error.
func (e *Executor) executeCheck(ctx context.Context, target *CheckTarget, check Check) CheckExecution {
	result, err := check.Validate(ctx, target)

	// If check returned an error, convert to Error status with appropriate remediation
	if err != nil {
		remediation := "Check the error message and ensure you have proper access to the cluster resources"

		// Handle specific error types
		switch {
		case apierrors.IsForbidden(err):
			remediation = "Insufficient permissions to access cluster resources. " +
				"Ensure your ServiceAccount or user has the required RBAC permissions. " +
				"Required permissions: get, list on the resource types being checked. " +
				"Contact your cluster administrator to grant access."
		case apierrors.IsTimeout(err):
			remediation = "Request timed out. Check network connectivity to the cluster API server. " +
				"Verify the cluster is responsive and not overloaded."
		case apierrors.IsServiceUnavailable(err) || apierrors.IsServerTimeout(err):
			remediation = "API server is unavailable or overloaded. " +
				"Wait a few moments and try again. " +
				"If the issue persists, check cluster health with 'kubectl get nodes' and 'kubectl get pods -n kube-system'."
		default:
			// Use the default remediation message set above
		}

		return CheckExecution{
			Check: check,
			Result: &DiagnosticResult{
				Status:      StatusError,
				Message:     fmt.Sprintf("Check execution failed: %v", err),
				Remediation: remediation,
			},
			Error: err,
		}
	}

	// Validate the result
	if err := result.Validate(); err != nil {
		return CheckExecution{
			Check: check,
			Result: &DiagnosticResult{
				Status:  StatusError,
				Message: fmt.Sprintf("Invalid check result: %v", err),
				Remediation: "This is an internal error. " +
					"Please report this issue to the OpenShift AI team with the error details.",
			},
			Error: fmt.Errorf("invalid result from check %s: %w", check.ID(), err),
		}
	}

	return CheckExecution{
		Check:  check,
		Result: result,
		Error:  nil,
	}
}
