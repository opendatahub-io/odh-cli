package check

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/util/iostreams"
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
	io       iostreams.Interface
}

// NewExecutor creates a new check executor.
func NewExecutor(registry *CheckRegistry, io iostreams.Interface) *Executor {
	return &Executor{
		registry: registry,
		io:       io,
	}
}

// ExecuteAll runs all checks in the registry against the target
// Returns results for all checks, including errors.
func (e *Executor) ExecuteAll(ctx context.Context, target Target) []CheckExecution {
	checks := e.registry.ListAll()

	return e.executeChecks(ctx, target, checks)
}

// ExecuteSelective runs checks matching any of the patterns and group
// Returns results for matching checks only.
// TargetVersion filtering is done via CanApply during execution.
func (e *Executor) ExecuteSelective(
	ctx context.Context,
	target Target,
	patterns []string,
	group CheckGroup,
) ([]CheckExecution, error) {
	checks, err := e.registry.ListByPatterns(patterns, group)
	if err != nil {
		return nil, fmt.Errorf("selecting checks: %w", err)
	}

	return e.executeChecks(ctx, target, checks), nil
}

// executeChecks runs the provided checks against the target sequentially.
func (e *Executor) executeChecks(ctx context.Context, target Target, checks []Check) []CheckExecution {
	results := make([]CheckExecution, 0, len(checks))

	for _, check := range checks {
		// Check context before executing each check
		if err := CheckContextError(ctx); err != nil {
			// Context canceled or timed out - stop executing checks
			break
		}

		// Filter by CanApply before executing
		// Checks can use target.CurrentVersion, target.TargetVersion, or target.Client for filtering
		canApply, err := check.CanApply(ctx, target)
		if err != nil {
			results = append(results, e.buildCanApplyError(check, err))

			continue
		}

		if !canApply {
			continue
		}

		// Execute check sequentially
		exec := e.executeCheck(ctx, target, check)
		results = append(results, exec)
	}

	return results
}

// buildCanApplyError creates a CheckExecution for a CanApply error.
func (e *Executor) buildCanApplyError(check Check, err error) CheckExecution {
	errorResult := result.New(
		string(check.Group()),
		check.CheckKind(),
		check.CheckType(),
		check.Description(),
	)

	errorResult.Status.Conditions = []result.Condition{
		NewCondition(
			ConditionTypeValidated,
			metav1.ConditionUnknown,
			WithReason(ReasonCheckExecutionFailed),
			WithMessage("Check applicability failed: %v", err),
		),
	}

	return CheckExecution{
		Check:  check,
		Result: errorResult,
		Error:  fmt.Errorf("CanApply failed for check %s: %w", check.ID(), err),
	}
}

// executeCheck runs a single check and captures the result or error.
func (e *Executor) executeCheck(ctx context.Context, target Target, check Check) CheckExecution {
	// Ensure target has IOStreams for permission error logging
	if target.IO == nil {
		target.IO = e.io
	}

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
			// Log to stderr if verbose
			if e.io != nil {
				e.io.Errorf("Permission denied: %s - Check: %s", message, check.Name())
			}
		case apierrors.IsUnauthorized(err):
			reason = ReasonAPIAccessDenied
			message = "Authentication required to access cluster resources"
			// Log to stderr if verbose
			if e.io != nil {
				e.io.Errorf("Unauthorized: %s - Check: %s", message, check.Name())
			}
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
			check.CheckKind(),
			check.CheckType(),
			check.Description(),
		)

		var condition result.Condition
		if reason == ReasonCheckExecutionFailed && message == "" {
			condition = NewCondition(
				ConditionTypeValidated,
				metav1.ConditionUnknown,
				WithReason(reason),
				WithMessage("Check execution failed: %v", err),
			)
		} else {
			condition = NewCondition(
				ConditionTypeValidated,
				metav1.ConditionUnknown,
				WithReason(reason),
				WithMessage("%s", message),
			)
		}

		errorResult.Status.Conditions = []result.Condition{condition}

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
			check.CheckKind(),
			check.CheckType(),
			check.Description(),
		)
		invalidResult.Status.Conditions = []result.Condition{
			NewCondition(
				ConditionTypeValidated,
				metav1.ConditionUnknown,
				WithReason(ReasonCheckExecutionFailed),
				WithMessage("Invalid check result: %v", err),
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
