package iostreams_test

import (
	"bytes"
	"testing"

	"github.com/lburgazzoli/odh-cli/pkg/util/iostreams"

	. "github.com/onsi/gomega"
)

// T004: Test IOStreams.Fprintf with and without formatting args.
func TestIOStreams_Fprintf(t *testing.T) {
	t.Run("with formatting arguments", func(t *testing.T) {
		g := NewWithT(t)
		var out bytes.Buffer
		io := iostreams.NewIOStreams(nil, &out, nil)

		io.Fprintf("Hello %s, you have %d messages", "Alice", 5)

		g.Expect(out.String()).To(Equal("Hello Alice, you have 5 messages\n"))
	})

	t.Run("without formatting arguments", func(t *testing.T) {
		g := NewWithT(t)
		var out bytes.Buffer
		io := iostreams.NewIOStreams(nil, &out, nil)

		io.Fprintf("Static message")

		g.Expect(out.String()).To(Equal("Static message\n"))
	})

	t.Run("empty string", func(t *testing.T) {
		g := NewWithT(t)
		var out bytes.Buffer
		io := iostreams.NewIOStreams(nil, &out, nil)

		io.Fprintf("")

		g.Expect(out.String()).To(Equal("\n"))
	})
}

// T005: Test IOStreams.Fprintln for plain output.
func TestIOStreams_Fprintln(t *testing.T) {
	t.Run("single argument", func(t *testing.T) {
		g := NewWithT(t)
		var out bytes.Buffer
		io := iostreams.NewIOStreams(nil, &out, nil)

		io.Fprintln("Hello World")

		g.Expect(out.String()).To(Equal("Hello World\n"))
	})

	t.Run("multiple arguments", func(t *testing.T) {
		g := NewWithT(t)
		var out bytes.Buffer
		io := iostreams.NewIOStreams(nil, &out, nil)

		io.Fprintln("Hello", "World", 123)

		g.Expect(out.String()).To(Equal("Hello World 123\n"))
	})

	t.Run("no arguments", func(t *testing.T) {
		g := NewWithT(t)
		var out bytes.Buffer
		io := iostreams.NewIOStreams(nil, &out, nil)

		io.Fprintln()

		g.Expect(out.String()).To(Equal("\n"))
	})
}

// T006: Test IOStreams.Errorf for error output to stderr.
func TestIOStreams_Errorf(t *testing.T) {
	t.Run("with formatting arguments", func(t *testing.T) {
		g := NewWithT(t)
		var errOut bytes.Buffer
		io := iostreams.NewIOStreams(nil, nil, &errOut)

		io.Errorf("Error: %s failed with code %d", "operation", 42)

		g.Expect(errOut.String()).To(Equal("Error: operation failed with code 42\n"))
	})

	t.Run("without formatting arguments", func(t *testing.T) {
		g := NewWithT(t)
		var errOut bytes.Buffer
		io := iostreams.NewIOStreams(nil, nil, &errOut)

		io.Errorf("Static error message")

		g.Expect(errOut.String()).To(Equal("Static error message\n"))
	})

	t.Run("empty string", func(t *testing.T) {
		g := NewWithT(t)
		var errOut bytes.Buffer
		io := iostreams.NewIOStreams(nil, nil, &errOut)

		io.Errorf("")

		g.Expect(errOut.String()).To(Equal("\n"))
	})
}

// T007: Test IOStreams.Errorln for plain error output.
func TestIOStreams_Errorln(t *testing.T) {
	t.Run("single argument", func(t *testing.T) {
		g := NewWithT(t)
		var errOut bytes.Buffer
		io := iostreams.NewIOStreams(nil, nil, &errOut)

		io.Errorln("Error occurred")

		g.Expect(errOut.String()).To(Equal("Error occurred\n"))
	})

	t.Run("multiple arguments", func(t *testing.T) {
		g := NewWithT(t)
		var errOut bytes.Buffer
		io := iostreams.NewIOStreams(nil, nil, &errOut)

		io.Errorln("Error", "code", 500)

		g.Expect(errOut.String()).To(Equal("Error code 500\n"))
	})

	t.Run("no arguments", func(t *testing.T) {
		g := NewWithT(t)
		var errOut bytes.Buffer
		io := iostreams.NewIOStreams(nil, nil, &errOut)

		io.Errorln()

		g.Expect(errOut.String()).To(Equal("\n"))
	})
}

// T008: Test nil writer validation (edge case from spec).
func TestIOStreams_NilWriterValidation(t *testing.T) {
	t.Run("nil Out writer should not panic", func(t *testing.T) {
		io := iostreams.NewIOStreams(nil, nil, nil)

		// Should not panic - implementation should handle gracefully
		// Note: The actual behavior will be defined during implementation
		// This test documents the edge case identified in the spec
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("Fprintf with nil Out writer caused panic: %v", r)
			}
		}()

		io.Fprintf("test")
	})

	t.Run("nil ErrOut writer should not panic", func(t *testing.T) {
		io := iostreams.NewIOStreams(nil, nil, nil)

		defer func() {
			if r := recover(); r != nil {
				t.Errorf("Errorf with nil ErrOut writer caused panic: %v", r)
			}
		}()

		io.Errorf("test")
	})
}
