package check

import (
	"fmt"
	"sync"
)

// CheckRegistry manages the collection of available diagnostic checks.
type CheckRegistry struct {
	mu     sync.RWMutex
	checks map[string]Check
}

// NewRegistry creates a new check registry.
func NewRegistry() *CheckRegistry {
	return &CheckRegistry{
		checks: make(map[string]Check),
	}
}

// Register adds a check to the registry
// Returns error if a check with the same ID already exists.
func (r *CheckRegistry) Register(check Check) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.checks[check.ID()]; exists {
		return fmt.Errorf("check with ID %s already registered", check.ID())
	}

	r.checks[check.ID()] = check

	return nil
}

// Get looks up a check by ID, returning the check and whether it exists.
func (r *CheckRegistry) Get(id string) (Check, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	check, exists := r.checks[id]

	return check, exists
}

// ListByGroup returns all checks for a specific group.
func (r *CheckRegistry) ListByGroup(group CheckGroup) []Check {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []Check
	for _, check := range r.checks {
		if check.Group() == group {
			result = append(result, check)
		}
	}

	return result
}

// ListBySelector returns checks matching group
// If group is empty, all groups are included
// Version filtering is handled by CanApply in the executor.
func (r *CheckRegistry) ListBySelector(group CheckGroup) []Check {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]Check, 0, len(r.checks))
	for _, check := range r.checks {
		// Filter by group if specified
		if group != "" && check.Group() != group {
			continue
		}

		result = append(result, check)
	}

	return result
}

// ListAll returns all registered checks.
func (r *CheckRegistry) ListAll() []Check {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]Check, 0, len(r.checks))
	for _, check := range r.checks {
		result = append(result, check)
	}

	return result
}

// ListByPattern returns checks matching the selector pattern and group
// Pattern can be:
//   - Wildcard: "*" matches all checks
//   - Group shortcut: "components", "services", "workloads", "dependencies"
//   - Exact ID: "components.dashboard"
//   - Glob pattern: "components.*", "*dashboard*", "*.dashboard"
//
// If group is empty, all groups are included
// Version filtering is handled by CanApply in the executor.
func (r *CheckRegistry) ListByPattern(
	pattern string,
	group CheckGroup,
) ([]Check, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]Check, 0, len(r.checks))
	for _, check := range r.checks {
		// Filter by pattern
		matched, err := matchesPattern(check, pattern)
		if err != nil {
			return nil, fmt.Errorf("pattern matching for check %s: %w", check.ID(), err)
		}
		if !matched {
			continue
		}

		// Filter by group if specified
		if group != "" && check.Group() != group {
			continue
		}

		result = append(result, check)
	}

	return result, nil
}
