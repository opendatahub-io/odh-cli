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

// MustRegister registers a check and panics if registration fails.
// Use this for check registration in command construction where failure is unrecoverable.
func (r *CheckRegistry) MustRegister(check Check) {
	if err := r.Register(check); err != nil {
		panic(fmt.Sprintf("failed to register check %s: %v", check.ID(), err))
	}
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
// TargetVersion filtering is handled by CanApply in the executor.
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

// ListByPatterns returns checks matching any of the selector patterns and group.
// Each pattern can be:
//   - Wildcard: "*" matches all checks
//   - Group shortcut: "components", "dependencies", "platform", "services", "workloads"
//   - Exact ID: "components.dashboard"
//   - Glob pattern: "components.*", "*dashboard*", "*.dashboard"
//
// A check is included if it matches ANY of the provided patterns (union semantics).
// If group is empty, all groups are included.
// TargetVersion filtering is handled by CanApply in the executor.
func (r *CheckRegistry) ListByPatterns(
	patterns []string,
	group CheckGroup,
) ([]Check, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]Check, 0, len(r.checks))

	for _, check := range r.checks {
		// Filter by group first (cheaper than pattern matching)
		if group != "" && check.Group() != group {
			continue
		}

		// Match against any pattern
		for _, pattern := range patterns {
			matched, err := matchesPattern(check, pattern)
			if err != nil {
				return nil, fmt.Errorf("pattern matching for check %s: %w", check.ID(), err)
			}

			if matched {
				result = append(result, check)

				break
			}
		}
	}

	return result, nil
}

// ListByPattern returns checks matching a single selector pattern and group.
// For matching against multiple patterns, use ListByPatterns.
func (r *CheckRegistry) ListByPattern(
	pattern string,
	group CheckGroup,
) ([]Check, error) {
	return r.ListByPatterns([]string{pattern}, group)
}
