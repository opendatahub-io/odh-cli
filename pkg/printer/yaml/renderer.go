package yaml

import (
	"fmt"
	"io"
	"os"

	"github.com/opendatahub-io/odh-cli/pkg/util"
	"sigs.k8s.io/yaml"
)

// Renderer provides a generic interface for rendering values as YAML.
type Renderer[T any] struct {
	writer io.Writer
}

// Option is a functional option for configuring a Renderer.
type Option[T any] = util.Option[Renderer[T]]

// NewRenderer creates a new YAML renderer with the given options.
func NewRenderer[T any](opts ...Option[T]) *Renderer[T] {
	r := &Renderer[T]{
		writer: os.Stdout,
	}

	for _, opt := range opts {
		opt.ApplyTo(r)
	}

	return r
}

// WithWriter sets the output writer for the YAML renderer.
func WithWriter[T any](w io.Writer) Option[T] {
	return util.FunctionalOption[Renderer[T]](func(r *Renderer[T]) {
		r.writer = w
	})
}

// Render marshals the value to YAML and writes it to the configured writer.
func (r *Renderer[T]) Render(value T) error {
	data, err := yaml.Marshal(value)
	if err != nil {
		return fmt.Errorf("failed to marshal value to YAML: %w", err)
	}

	if _, err := r.writer.Write(data); err != nil {
		return fmt.Errorf("failed to write YAML output: %w", err)
	}

	return nil
}
