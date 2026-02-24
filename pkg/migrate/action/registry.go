package action

import (
	"fmt"
	"path/filepath"
	"sort"
	"sync"
)

type ActionRegistry struct {
	mu      sync.RWMutex
	actions map[string]Action
}

func NewActionRegistry(actions ...Action) (*ActionRegistry, error) {
	r := &ActionRegistry{
		actions: make(map[string]Action),
	}

	if err := r.Register(actions...); err != nil {
		return nil, err
	}

	return r, nil
}

// Register adds one or more actions to the registry.
// The operation is atomic: either all actions are registered or none are.
// Returns an error if an action with the same ID already exists.
func (r *ActionRegistry) Register(actions ...Action) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, a := range actions {
		if _, exists := r.actions[a.ID()]; exists {
			return fmt.Errorf("action with ID %q already registered", a.ID())
		}
	}

	for _, a := range actions {
		r.actions[a.ID()] = a
	}

	return nil
}

func (r *ActionRegistry) Get(id string) (Action, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	action, ok := r.actions[id]

	return action, ok
}

func (r *ActionRegistry) ListAll() []Action {
	r.mu.RLock()
	defer r.mu.RUnlock()

	actions := make([]Action, 0, len(r.actions))
	for _, action := range r.actions {
		actions = append(actions, action)
	}

	sort.Slice(actions, func(i int, j int) bool {
		return actions[i].ID() < actions[j].ID()
	})

	return actions
}

func (r *ActionRegistry) ListByPattern(
	pattern string,
	group ActionGroup,
) ([]Action, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var matched []Action

	for id, action := range r.actions {
		if group != "" && action.Group() != group {
			continue
		}

		match, err := filepath.Match(pattern, id)
		if err != nil {
			return nil, fmt.Errorf("invalid pattern %q: %w", pattern, err)
		}

		if match {
			matched = append(matched, action)
		}
	}

	sort.Slice(matched, func(i int, j int) bool {
		return matched[i].ID() < matched[j].ID()
	})

	return matched, nil
}
