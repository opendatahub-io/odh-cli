package jq

import (
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/itchyny/gojq"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// convertValue converts a value to a JQ-compatible format.
// It handles special types like unstructured.Unstructured by extracting their Object field,
// and passes through maps and slices directly without marshaling/unmarshaling.
func convertValue(value any) (any, error) {
	if value == nil {
		return nil, nil
	}

	// Handle unstructured.Unstructured by value
	if v, ok := value.(unstructured.Unstructured); ok {
		return v.Object, nil
	}

	// Handle *unstructured.Unstructured by pointer
	if v, ok := value.(*unstructured.Unstructured); ok {
		return v.Object, nil
	}

	// Check the kind of the value
	rv := reflect.ValueOf(value)
	kind := rv.Kind()

	// Handle maps - pass through directly
	if kind == reflect.Map {
		return value, nil
	}

	// Handle slices
	if kind == reflect.Slice {
		// For non-byte slices, convert to []any for gojq compatibility
		if _, isByteSlice := value.([]byte); !isByteSlice {
			slice := make([]any, rv.Len())
			for i := range rv.Len() {
				slice[i] = rv.Index(i).Interface()
			}

			return slice, nil
		}
		// For []byte, fall through to JSON marshal/unmarshal
	}

	// For other types, use JSON marshal/unmarshal to normalize
	var normalizedValue any
	jsonBytes, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal value: %w", err)
	}

	if err := json.Unmarshal(jsonBytes, &normalizedValue); err != nil {
		return nil, fmt.Errorf("failed to unmarshal value: %w", err)
	}

	return normalizedValue, nil
}

// Query executes a JQ query against the provided value and returns the first result.
// The value is converted to a JQ-compatible format before processing.
func Query(value any, jqQuery string) (any, error) {
	// Compile the JQ query
	compiledQuery, err := gojq.Parse(jqQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to parse jq query: %w", err)
	}

	// Convert value to JQ-compatible format
	normalizedValue, err := convertValue(value)
	if err != nil {
		return nil, err
	}

	// Run the query against the normalized value
	iter := compiledQuery.Run(normalizedValue)

	// Get the first result
	result, ok := iter.Next()
	if !ok {
		return nil, nil
	}

	// Check for errors
	if err, isErr := result.(error); isErr {
		return nil, fmt.Errorf("jq query error: %w", err)
	}

	return result, nil
}
