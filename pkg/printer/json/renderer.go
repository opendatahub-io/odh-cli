package json

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/opendatahub-io/odh-cli/pkg/util"
)

// Renderer provides a generic interface for rendering values as JSON.
type Renderer[T any] struct {
	writer io.Writer
	indent string
}

// Option is a functional option for configuring a Renderer.
type Option[T any] = util.Option[Renderer[T]]

// NewRenderer creates a new JSON renderer with the given options.
func NewRenderer[T any](opts ...Option[T]) *Renderer[T] {
	r := &Renderer[T]{
		writer: os.Stdout,
		indent: "  ",
	}

	for _, opt := range opts {
		opt.ApplyTo(r)
	}

	return r
}

// WithWriter sets the output writer for the JSON renderer.
func WithWriter[T any](w io.Writer) Option[T] {
	return util.FunctionalOption[Renderer[T]](func(r *Renderer[T]) {
		r.writer = w
	})
}

// WithIndent sets the indentation string for JSON output.
func WithIndent[T any](indent string) Option[T] {
	return util.FunctionalOption[Renderer[T]](func(r *Renderer[T]) {
		r.indent = indent
	})
}

// Render marshals the value to JSON and writes it to the configured writer.
func (r *Renderer[T]) Render(value T) error {
	encoder := json.NewEncoder(r.writer)
	encoder.SetIndent("", r.indent)

	if err := encoder.Encode(value); err != nil {
		return fmt.Errorf("failed to marshal value to JSON: %w", err)
	}

	return nil
}
