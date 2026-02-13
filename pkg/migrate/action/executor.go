package action

import (
	"context"
	"fmt"

	"github.com/opendatahub-io/odh-cli/pkg/migrate/action/result"
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
	target Target,
) []ActionExecution {
	actions := e.registry.ListAll()

	return e.executeActions(ctx, target, actions)
}

func (e *Executor) ExecuteSelective(
	ctx context.Context,
	target Target,
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
	target Target,
	actions []Action,
) []ActionExecution {
	results := make([]ActionExecution, 0, len(actions))

	for _, action := range actions {
		if !action.CanApply(target) {
			continue
		}

		exec := e.executeAction(ctx, target, action)
		results = append(results, exec)
	}

	return results
}

func (e *Executor) executeAction(
	ctx context.Context,
	target Target,
	action Action,
) ActionExecution {
	runTask := action.Run()
	if runTask == nil {
		errorResult := result.New(
			string(action.Group()),
			action.ID(),
			action.Name(),
			action.Description(),
		)
		errorResult.Status.Error = "Action has no run task"

		return ActionExecution{
			Action: action,
			Result: errorResult,
			Error:  fmt.Errorf("action %s has no run task", action.ID()),
		}
	}

	actionResult, err := runTask.Execute(ctx, target)

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
