package check

import (
	"fmt"
	"path"
)

// matchesPattern returns true if the check matches the selector pattern
// Pattern can be:
//   - Wildcard: "*" matches all checks
//   - Category shortcut: "components", "services", "workloads", "dependencies"
//   - Exact ID: "components.dashboard"
//   - Glob pattern: "components.*", "*dashboard*", "*.dashboard"
func matchesPattern(check Check, pattern string) (bool, error) {
	// Wildcard matches all
	if pattern == "*" {
		return true, nil
	}

	// Category shortcuts
	switch pattern {
	case "components":
		return check.Category() == CategoryComponent, nil
	case "services":
		return check.Category() == CategoryService, nil
	case "workloads":
		return check.Category() == CategoryWorkload, nil
	case "dependencies":
		return check.Category() == CategoryDependency, nil
	}

	// Exact ID match
	if pattern == check.ID() {
		return true, nil
	}

	// Glob pattern match
	matched, err := path.Match(pattern, check.ID())
	if err != nil {
		return false, fmt.Errorf("invalid pattern %q: %w", pattern, err)
	}

	return matched, nil
}
