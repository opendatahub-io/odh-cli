package inspect

import (
	"errors"
	"fmt"

	"github.com/opendatahub-io/odh-cli/pkg/util/jq"
)

// HasFields checks which of the given JQ expressions resolve to existing (non-nil) fields
// on the provided object. Returns the values of all fields that exist.
// Callers check len(result) to determine if any fields were found.
func HasFields(value any, expressions ...string) ([]any, error) {
	var found []any

	for _, expr := range expressions {
		result, err := jq.Query[any](value, expr)
		if err != nil {
			if errors.Is(err, jq.ErrNotFound) {
				continue
			}

			return nil, fmt.Errorf("querying %s: %w", expr, err)
		}

		found = append(found, result)
	}

	return found, nil
}
