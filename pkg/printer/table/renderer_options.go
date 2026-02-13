package table

import (
	"io"
	"strings"

	"github.com/olekukonko/tablewriter"
	"github.com/olekukonko/tablewriter/tw"
	"github.com/opendatahub-io/odh-cli/pkg/util"
	"github.com/opendatahub-io/odh-cli/pkg/util/jq"
)

const (
	// DefaultMaxRowWidth is the default maximum width for table row cells.
	// Messages longer than this will be wrapped at word boundaries.
	DefaultMaxRowWidth = 100
)

// DefaultTableOptions provides a clean, minimal table style with left-aligned headers
// and no borders or separators.
//
//nolint:gochecknoglobals // Shared default table options for consistency across commands
var DefaultTableOptions = []tablewriter.Option{
	tablewriter.WithHeaderAlignment(tw.AlignLeft),
	tablewriter.WithRowAutoWrap(tw.WrapNormal),      // Enable word-wrapping for row content
	tablewriter.WithRowMaxWidth(DefaultMaxRowWidth), // Limit row cells to max width
	tablewriter.WithRendition(tw.Rendition{
		Settings: tw.Settings{
			Separators: tw.Separators{
				BetweenColumns: tw.Off,
				BetweenRows:    tw.Off,
			},
			Lines: tw.Lines{
				ShowTop:        tw.On,
				ShowBottom:     tw.On,
				ShowHeaderLine: tw.On,
			},
		},
	}),
}

// Option is a functional option for configuring a Renderer.
type Option[T any] = util.Option[Renderer[T]]

// WithWriter sets the output writer for the table renderer.
func WithWriter[T any](w io.Writer) Option[T] {
	return util.FunctionalOption[Renderer[T]](func(r *Renderer[T]) {
		r.writer = w
	})
}

// WithHeaders sets the column headers for the table.
func WithHeaders[T any](headers ...string) Option[T] {
	return util.FunctionalOption[Renderer[T]](func(r *Renderer[T]) {
		r.headers = headers
	})
}

// WithFormatter adds a column-specific formatter function.
func WithFormatter[T any](columnName string, formatter ColumnFormatter) Option[T] {
	return util.FunctionalOption[Renderer[T]](func(r *Renderer[T]) {
		if r.formatters == nil {
			r.formatters = make(map[string]ColumnFormatter)
		}

		r.formatters[strings.ToUpper(columnName)] = formatter
	})
}

// WithTableOptions sets the underlying tablewriter options.
func WithTableOptions[T any](values ...tablewriter.Option) Option[T] {
	return util.FunctionalOption[Renderer[T]](func(r *Renderer[T]) {
		r.tableOptions = append(r.tableOptions, values...)
	})
}

// JQFormatter creates a ColumnFormatter that executes a jq query on the input value.
// Uses the jq.Query utility which properly handles unstructured types.
func JQFormatter(query string) ColumnFormatter {
	return func(value any) any {
		result, err := jq.Query[any](value, query)
		if err != nil {
			return err.Error()
		}

		return result
	}
}

// ChainFormatters composes multiple formatters into a single formatter pipeline.
// The output of each formatter is passed as input to the next formatter.
// This enables building transformation pipelines like: JQ extraction → colorization → truncation.
func ChainFormatters(formatters ...ColumnFormatter) ColumnFormatter {
	if len(formatters) == 0 {
		return func(value any) any {
			return value
		}
	}

	if len(formatters) == 1 {
		return formatters[0]
	}

	return func(value any) any {
		result := value
		for _, formatter := range formatters {
			result = formatter(result)
		}

		return result
	}
}
