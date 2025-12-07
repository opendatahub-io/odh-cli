package table

import (
	"errors"
	"fmt"
	"io"
	"os"
	"reflect"
	"strings"

	mapstructure "github.com/go-viper/mapstructure/v2"
	"github.com/olekukonko/tablewriter"
	"github.com/olekukonko/tablewriter/tw"
)

// ColumnFormatter is a function that transforms a value for display in a specific column.
type ColumnFormatter func(value any) any

// Renderer provides a flexible interface for creating and rendering tables.
// T is the type of objects that will be appended to the table.
type Renderer[T any] struct {
	writer       io.Writer
	headers      []string
	formatters   map[string]ColumnFormatter
	table        *tablewriter.Table
	tableOptions []tablewriter.Option
}

// NewRenderer creates a new table renderer with the given tableOptions.
func NewRenderer[T any](opts ...Option[T]) *Renderer[T] {
	r := &Renderer[T]{
		writer:     os.Stdout,
		formatters: make(map[string]ColumnFormatter),
	}

	// Apply tableOptions first to set basic configuration
	for _, opt := range opts {
		opt.ApplyTo(r)
	}

	r.table = tablewriter.NewTable(r.writer)

	if len(r.tableOptions) == 0 {
		r.table = r.table.Options(tablewriter.WithRendition(
			tw.Rendition{
				Settings: tw.Settings{
					Separators: tw.Separators{
						BetweenColumns: tw.Off,
					},
				},
			}),
		)
	} else {
		r.table = r.table.Options(r.tableOptions...)
	}

	if len(r.headers) > 0 {
		r.table.Header(r.headers)
	}

	return r
}

// Append adds a single row to the table.
// Accepts either []any (legacy) or a struct (auto-extracted via mapstructure).
func (r *Renderer[T]) Append(value T) error {
	// Check if all headers have formatters
	allHaveFormatters := true
	for _, header := range r.headers {
		if _, exists := r.formatters[strings.ToUpper(header)]; !exists {
			allHaveFormatters = false

			break
		}
	}

	var values []any
	var err error

	if allHaveFormatters {
		// If all columns have formatters, pass the whole value to each formatter
		// This allows JQ formatters to work directly on complex objects
		values = make([]any, len(r.headers))
		for i := range r.headers {
			values[i] = value
		}
	} else {
		// Otherwise, extract values from the object first
		values, err = r.extractValues(value)
		if err != nil {
			return err
		}
	}

	row := make([]any, 0, len(r.headers))

	for i := range r.headers {
		v := values[i]
		h := strings.ToUpper(r.headers[i])

		// Apply formatter if one exists for this column
		if formatter, exists := r.formatters[h]; exists {
			v = formatter(v)
		}

		row = append(row, v)
	}

	if err := r.table.Append(row); err != nil {
		return fmt.Errorf("failed to append row to table: %w", err)
	}

	return nil
}

// extractValues extracts column values from either a slice or a struct.
func (r *Renderer[T]) extractValues(value any) ([]any, error) {
	if value == nil {
		return nil, errors.New("cannot append nil value")
	}

	// Check if it's a slice
	v := reflect.ValueOf(value)
	if v.Kind() == reflect.Slice {
		// Legacy behavior: treat as []any
		values := make([]any, v.Len())
		for i := range v.Len() {
			values[i] = v.Index(i).Interface()
		}

		if len(values) != len(r.headers) {
			return nil, fmt.Errorf("column count mismatch: expected %d, got %d", len(r.headers), len(values))
		}

		return values, nil
	}

	// For struct types, convert to map and extract by column name
	var dataMap map[string]any
	if err := mapstructure.Decode(value, &dataMap); err != nil {
		return nil, fmt.Errorf("failed to decode value to map: %w", err)
	}

	values := make([]any, 0, len(r.headers))
	for _, header := range r.headers {
		val, err := r.extractFieldValue(dataMap, header)
		if err != nil {
			return nil, fmt.Errorf("column %q: %w", header, err)
		}
		values = append(values, val)
	}

	return values, nil
}

// extractFieldValue extracts a single field value from the map by column name.
// Uses case-insensitive matching.
func (r *Renderer[T]) extractFieldValue(data map[string]any, columnName string) (any, error) {
	// Try exact match first
	if val, ok := data[columnName]; ok {
		return val, nil
	}

	// Try case-insensitive match
	lowerColumn := strings.ToLower(columnName)
	for key, val := range data {
		if strings.ToLower(key) == lowerColumn {
			return val, nil
		}
	}

	return nil, errors.New("field not found")
}

// AppendAll adds multiple rows to the table in a single operation.
// Each item in the slice can be either []any or a struct.
func (r *Renderer[T]) AppendAll(rows []T) error {
	for _, value := range rows {
		if err := r.Append(value); err != nil {
			return err
		}
	}

	return nil
}

// Render outputs the table to the configured writer.
func (r *Renderer[T]) Render() error {
	if err := r.table.Render(); err != nil {
		return fmt.Errorf("failed to render table: %w", err)
	}

	return nil
}

// SetHeaders updates the table headers (useful for dynamic header configuration).
func (r *Renderer[T]) SetHeaders(headers ...string) {
	r.headers = headers
	r.table.Header(headers)
}

// GetHeaders returns the current headers.
func (r *Renderer[T]) GetHeaders() []string {
	return r.headers
}
