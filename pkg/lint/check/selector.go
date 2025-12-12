package check

import (
	"fmt"
	"path"
)

// matchesPattern returns true if the check matches the selector pattern
// Pattern can be:
//   - Wildcard: "*" matches all checks
//   - Group shortcut: "components", "services", "workloads", "dependencies"
//   - Exact ID: "components.dashboard"
//   - Glob pattern: "components.*", "*dashboard*", "*.dashboard"
func matchesPattern(check Check, pattern string) (bool, error) {
	// Wildcard matches all
	if pattern == "*" {
		return true, nil
	}

	// Group shortcuts
	switch pattern {
	case "components":
		return check.Group() == GroupComponent, nil
	case "services":
		return check.Group() == GroupService, nil
	case "workloads":
		return check.Group() == GroupWorkload, nil
	case "dependencies":
		return check.Group() == GroupDependency, nil
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
