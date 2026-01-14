package jq

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"

	"github.com/itchyny/gojq"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// ErrNotFound is returned when a JQ query doesn't find the requested field.
var ErrNotFound = errors.New("field not found")

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

// Query executes a JQ query against the provided value and returns the first result
// cast to type T. Returns an error if the result cannot be cast to T.
// When the query returns nil/null, returns the zero value of T.
func Query[T any](value any, jqQuery string) (T, error) {
	var zero T

	// Compile the JQ query
	compiledQuery, err := gojq.Parse(jqQuery)
	if err != nil {
		return zero, fmt.Errorf("failed to parse jq query: %w", err)
	}

	// Convert value to JQ-compatible format
	normalizedValue, err := convertValue(value)
	if err != nil {
		return zero, err
	}

	// Run the query against the normalized value
	iter := compiledQuery.Run(normalizedValue)

	// Get the first result
	result, ok := iter.Next()
	if !ok {
		return zero, nil
	}

	// Check for errors
	if err, isErr := result.(error); isErr {
		return zero, fmt.Errorf("jq query error: %w", err)
	}

	// Handle nil result - return ErrNotFound instead of zero value
	if result == nil {
		return zero, ErrNotFound
	}

	// Type assertion to T
	typed, ok := result.(T)
	if !ok {
		return zero, fmt.Errorf("query result type mismatch: expected %T, got %T (value: %v)",
			zero, result, result)
	}

	return typed, nil
}

// Transform applies a JQ update expression to the object, modifying it in place.
// Supports printf-style formatting with variadic arguments.
//
// Examples:
//
//	jq.Transform(obj, ".spec.foo = %q", "bar")
//	jq.Transform(obj, ".metadata.annotations = %s", annotationsJSON)
//	jq.Transform(obj, `.spec.components.kueue.managementState = "Unmanaged"`)
func Transform(obj *unstructured.Unstructured, jqExpressionFormat string, args ...any) error {
	// Format the expression if args provided
	jqExpression := jqExpressionFormat
	if len(args) > 0 {
		jqExpression = fmt.Sprintf(jqExpressionFormat, args...)
	}

	// Convert obj to JQ-compatible format
	normalizedValue, err := convertValue(obj)
	if err != nil {
		return fmt.Errorf("failed to normalize object: %w", err)
	}

	// Parse JQ expression
	compiledQuery, err := gojq.Parse(jqExpression)
	if err != nil {
		return fmt.Errorf("failed to parse jq expression: %w", err)
	}

	// Run the expression
	iter := compiledQuery.Run(normalizedValue)

	result, ok := iter.Next()
	if !ok {
		return errors.New("transform returned no result")
	}

	// Check for errors
	if err, isErr := result.(error); isErr {
		return fmt.Errorf("transform error: %w", err)
	}

	// Update the original object with the result
	resultMap, ok := result.(map[string]any)
	if !ok {
		return fmt.Errorf("transform result is not a map: %T", result)
	}

	obj.Object = resultMap

	return nil
}
