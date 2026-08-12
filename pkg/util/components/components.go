package components

import (
	"errors"
	"fmt"
	"slices"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/opendatahub-io/odh-cli/pkg/constants"
	"github.com/opendatahub-io/odh-cli/pkg/util/jq"
)

// GetManagementState queries a DSC component's management state.
// componentKey is the lowercase key under spec.components (e.g. "kueue", "kserve").
// Returns the state if found, "Removed" if not configured (semantically equivalent),
// or an error on query failure.
func GetManagementState(obj client.Object, componentKey string) (string, error) {
	path := fmt.Sprintf(".spec.components.%s.managementState", componentKey)

	state, err := jq.Query[string](obj, path)
	if err != nil {
		if errors.Is(err, jq.ErrNotFound) {
			// Treat "not configured" as "Removed" - both mean component is not active.
			return constants.ManagementStateRemoved, nil
		}

		return "", fmt.Errorf("querying %s managementState: %w", componentKey, err)
	}

	return state, nil
}

// HasManagementState checks whether a DSC component is configured with a matching management state.
// With states: returns true if the component's state matches any of the provided values.
// Without states: returns true always (component exists or defaults to Removed).
func HasManagementState(obj client.Object, componentKey string, states ...string) bool {
	state, err := GetManagementState(obj, componentKey)
	if err != nil {
		return false
	}

	if len(states) == 0 {
		return true
	}

	return slices.Contains(states, state)
}

// GetSubComponentManagementState queries a DSC sub-component's management state.
// parentKey is the top-level component key (e.g. "aigateway").
// subKey is the nested sub-component key (e.g. "modelsAsAService").
// Returns "Removed" if not configured (semantically equivalent), or an error on query failure.
func GetSubComponentManagementState(obj client.Object, parentKey, subKey string) (string, error) {
	path := fmt.Sprintf(".spec.components.%s.%s.managementState", parentKey, subKey)

	state, err := jq.Query[string](obj, path)
	if err != nil {
		if errors.Is(err, jq.ErrNotFound) {
			return constants.ManagementStateRemoved, nil
		}

		return "", fmt.Errorf("querying %s.%s managementState: %w", parentKey, subKey, err)
	}

	return state, nil
}

// HasSubComponentManagementState checks whether a DSC sub-component is configured with a matching management state.
// With states: returns true if the sub-component's state matches any of the provided values.
// Without states: returns true always (sub-component exists or defaults to Removed).
func HasSubComponentManagementState(obj client.Object, parentKey, subKey string, states ...string) bool {
	state, err := GetSubComponentManagementState(obj, parentKey, subKey)
	if err != nil {
		return false
	}

	if len(states) == 0 {
		return true
	}

	return slices.Contains(states, state)
}
