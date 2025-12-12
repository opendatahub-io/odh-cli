package action

import (
	"context"
	"fmt"
	"strings"

	"github.com/blang/semver/v4"

	"github.com/lburgazzoli/odh-cli/pkg/migrate/action/result"
)

type ActionExecution struct {
	Action Action
	Result *result.ActionResult
	Error  error
}

type Executor struct {
	registry *ActionRegistry
}

func NewExecutor(registry *ActionRegistry) *Executor {
	return &Executor{
		registry: registry,
	}
}

func (e *Executor) ExecuteAll(
	ctx context.Context,
	target *ActionTarget,
) []ActionExecution {
	actions := e.registry.ListAll()

	return e.executeActions(ctx, target, actions)
}

func (e *Executor) ExecuteSelective(
	ctx context.Context,
	target *ActionTarget,
	pattern string,
	group ActionGroup,
) ([]ActionExecution, error) {
	actions, err := e.registry.ListByPattern(pattern, group)
	if err != nil {
		return nil, fmt.Errorf("selecting actions: %w", err)
	}

	return e.executeActions(ctx, target, actions), nil
}

func (e *Executor) executeActions(
	ctx context.Context,
	target *ActionTarget,
	actions []Action,
) []ActionExecution {
	results := make([]ActionExecution, 0, len(actions))

	var currentVer, targetVer *semver.Version
	if target.CurrentVersion != nil && target.CurrentVersion.Version != "" {
		parsed, err := semver.Parse(strings.TrimPrefix(target.CurrentVersion.Version, "v"))
		if err == nil {
			currentVer = &parsed
		}
	}
	if target.TargetVersion != nil && target.TargetVersion.Version != "" {
		parsed, err := semver.Parse(strings.TrimPrefix(target.TargetVersion.Version, "v"))
		if err == nil {
			targetVer = &parsed
		}
	}

	for _, action := range actions {
		if !action.CanApply(currentVer, targetVer) {
			continue
		}

		exec := e.executeAction(ctx, target, action)
		results = append(results, exec)
	}

	return results
}

func (e *Executor) executeAction(
	ctx context.Context,
	target *ActionTarget,
	action Action,
) ActionExecution {
	actionResult, err := action.Execute(ctx, target)

	if err != nil {
		errorResult := result.New(
			string(action.Group()),
			action.ID(),
			action.Name(),
			action.Description(),
		)
		errorResult.Status.Error = fmt.Sprintf("Action execution failed: %v", err)

		return ActionExecution{
			Action: action,
			Result: errorResult,
			Error:  err,
		}
	}

	return ActionExecution{
		Action: action,
		Result: actionResult,
		Error:  nil,
	}
}
