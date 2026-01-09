package check

import (
	"context"
	"fmt"
	"strings"

	"github.com/blang/semver/v4"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/lburgazzoli/odh-cli/pkg/lint/check/result"
)

// CheckExecution bundles a check with its execution result and any error encountered.
type CheckExecution struct {
	Check  Check
	Result *result.DiagnosticResult
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

// ExecuteSelective runs checks matching the pattern and group
// Returns results for matching checks only.
// Version filtering is done via CanApply during execution.
func (e *Executor) ExecuteSelective(
	ctx context.Context,
	target *CheckTarget,
	pattern string,
	group CheckGroup,
) ([]CheckExecution, error) {
	checks, err := e.registry.ListByPattern(pattern, group)
	if err != nil {
		return nil, fmt.Errorf("selecting checks: %w", err)
	}

	return e.executeChecks(ctx, target, checks), nil
}

// executeChecks runs the provided checks against the target sequentially.
func (e *Executor) executeChecks(ctx context.Context, target *CheckTarget, checks []Check) []CheckExecution {
	results := make([]CheckExecution, 0, len(checks))

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

		// Execute check sequentially
		exec := e.executeCheck(ctx, target, check)
		results = append(results, exec)
	}

	return results
}

// executeCheck runs a single check and captures the result or error.
func (e *Executor) executeCheck(ctx context.Context, target *CheckTarget, check Check) CheckExecution {
	checkResult, err := check.Validate(ctx, target)

	// If check returned an error, create a diagnostic result with error condition
	if err != nil {
		var message string
		var reason string

		// Handle specific error types
		switch {
		case apierrors.IsForbidden(err):
			reason = ReasonAPIAccessDenied
			message = "Insufficient permissions to access cluster resources"
		case apierrors.IsTimeout(err):
			reason = ReasonCheckExecutionFailed
			message = "Request timed out"
		case apierrors.IsServiceUnavailable(err) || apierrors.IsServerTimeout(err):
			reason = ReasonCheckExecutionFailed
			message = "API server is unavailable or overloaded"
		default:
			reason = ReasonCheckExecutionFailed
		}

		errorResult := result.New(
			string(check.Group()),
			check.ID(),
			check.Name(),
			check.Description(),
		)

		var condition metav1.Condition
		if reason == ReasonCheckExecutionFailed && message == "" {
			condition = NewCondition(
				ConditionTypeValidated,
				metav1.ConditionUnknown,
				reason,
				"Check execution failed: %v",
				err,
			)
		} else {
			condition = NewCondition(
				ConditionTypeValidated,
				metav1.ConditionUnknown,
				reason,
				message,
			)
		}

		errorResult.Status.Conditions = []metav1.Condition{condition}

		return CheckExecution{
			Check:  check,
			Result: errorResult,
			Error:  err,
		}
	}

	// Validate the result
	if err := checkResult.Validate(); err != nil {
		invalidResult := result.New(
			string(check.Group()),
			check.ID(),
			check.Name(),
			check.Description(),
		)
		invalidResult.Status.Conditions = []metav1.Condition{
			NewCondition(
				ConditionTypeValidated,
				metav1.ConditionUnknown,
				ReasonCheckExecutionFailed,
				"Invalid check result: %v",
				err,
			),
		}

		return CheckExecution{
			Check:  check,
			Result: invalidResult,
			Error:  fmt.Errorf("invalid result from check %s: %w", check.ID(), err),
		}
	}

	return CheckExecution{
		Check:  check,
		Result: checkResult,
		Error:  nil,
	}
}
