package components

import (
	"errors"
	"fmt"
	"slices"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/lburgazzoli/odh-cli/pkg/util/jq"
)

// GetManagementState queries a DSC component's management state.
// componentKey is the lowercase key under spec.components (e.g. "kueue", "kserve").
// Returns (state, true, nil) if found, ("", false, nil) if not configured, or ("", false, err) on error.
func GetManagementState(obj client.Object, componentKey string) (string, bool, error) {
	path := fmt.Sprintf(".spec.components.%s.managementState", componentKey)

	state, err := jq.Query[string](obj, path)
	if err != nil {
		if errors.Is(err, jq.ErrNotFound) {
			return "", false, nil
		}

		return "", false, fmt.Errorf("querying %s managementState: %w", componentKey, err)
	}

	return state, true, nil
}

// HasManagementState checks whether a DSC component is configured with a matching management state.
// With states: returns true if the component is configured and its state matches any of the provided values.
// Without states: returns true if the component is configured at all (any state).
func HasManagementState(obj client.Object, componentKey string, states ...string) bool {
	state, configured, err := GetManagementState(obj, componentKey)
	if err != nil || !configured {
		return false
	}

	if len(states) == 0 {
		return true
	}

	return slices.Contains(states, state)
}
