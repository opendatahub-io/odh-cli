package table

import (
	"io"
)

// Column represents a table column with its header name and optional formatters.
// Multiple formatters can be chained and will be applied in sequence.
type Column struct {
	name       string
	formatters []ColumnFormatter
}

// NewColumn creates a new column with the specified header name.
func NewColumn(name string) Column {
	return Column{
		name:       name,
		formatters: []ColumnFormatter{},
	}
}

// JQ appends a JQ query formatter to this column's formatter chain.
// The query will be executed against each row value to extract the column value.
// Can be chained with other formatters: Column().JQ(...).Fn(...)
func (c Column) JQ(query string) Column {
	c.formatters = append(c.formatters, JQFormatter(query))

	return c
}

// Fn appends a custom Go function formatter to this column's formatter chain.
// Can be chained with other formatters: Column().JQ(...).Fn(...)
func (c Column) Fn(formatter ColumnFormatter) Column {
	c.formatters = append(c.formatters, formatter)

	return c
}

// NewWithColumns creates a new table renderer with columns defined using the fluent API.
// This provides a more declarative way to define tables with JQ queries or custom formatters.
// Supports chaining formatters: Column().JQ(...).Fn(...)
//
// Example:
//
//	renderer := table.NewWithColumns(os.Stdout,
//	    table.NewColumn("NAME").JQ(".metadata.name").Fn(strings.ToUpper),
//	    table.NewColumn("TYPE").JQ(".kind"),
//	    table.NewColumn("READY").JQ(`.status.conditions[] | select(.type=="Ready") | .status // "Unknown"`),
//	)
func NewWithColumns[T any](writer io.Writer, columns ...Column) *Renderer[T] {
	headers := make([]string, len(columns))
	options := []Option[T]{WithWriter[T](writer)}

	for i, col := range columns {
		headers[i] = col.name

		// Handle formatters based on count
		switch len(col.formatters) {
		case 0:
			// No formatters, nothing to add
		case 1:
			// Single formatter, use it directly
			options = append(options, WithFormatter[T](col.name, col.formatters[0]))
		default:
			// Multiple formatters, chain them together
			options = append(options, WithFormatter[T](col.name, ChainFormatters(col.formatters...)))
		}
	}

	options = append([]Option[T]{WithHeaders[T](headers...)}, options...)

	return NewRenderer[T](options...)
}
