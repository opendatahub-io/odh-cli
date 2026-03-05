package check

import (
	"fmt"
	"path"
)

// Selector shortcut names used in CLI --checks flag.
// These are plural user-facing names (except for platform) that map to internal CheckGroup values.
const (
	SelectorComponents   = "components"
	SelectorDependencies = "dependencies"
	SelectorPlatform     = "platform"
	SelectorServices     = "services"
	SelectorWorkloads    = "workloads"
)

// matchesPattern returns true if the check matches the selector pattern
// Pattern can be:
//   - Wildcard: "*" matches all checks
//   - Group shortcut: "components", "services", "workloads", "dependencies", "platform"
//   - Exact ID: "components.dashboard"
//   - Glob pattern: "components.*", "*dashboard*", "*.dashboard"
func matchesPattern(check Check, pattern string) (bool, error) {
	// Wildcard matches all
	if pattern == "*" {
		return true, nil
	}

	// Group shortcuts
	switch pattern {
	case SelectorComponents:
		return check.Group() == GroupComponent, nil
	case SelectorDependencies:
		return check.Group() == GroupDependency, nil
	case SelectorPlatform:
		return check.Group() == GroupPlatform, nil
	case SelectorServices:
		return check.Group() == GroupService, nil
	case SelectorWorkloads:
		return check.Group() == GroupWorkload, nil
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
