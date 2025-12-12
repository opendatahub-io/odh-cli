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

func NewActionRegistry() *ActionRegistry {
	return &ActionRegistry{
		actions: make(map[string]Action),
	}
}

func (r *ActionRegistry) Register(action Action) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	id := action.ID()
	if _, exists := r.actions[id]; exists {
		return fmt.Errorf("action with ID %q already registered", id)
	}

	r.actions[id] = action

	return nil
}

func (r *ActionRegistry) MustRegister(action Action) {
	if err := r.Register(action); err != nil {
		panic(err)
	}
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
