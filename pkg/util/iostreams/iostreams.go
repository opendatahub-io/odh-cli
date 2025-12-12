package iostreams

import (
	"fmt"
	"io"
)

// Interface defines the contract for structured IO streams.
type Interface interface {
	// Fprintf writes formatted output to Out with automatic newline
	Fprintf(format string, args ...any)
	// Fprintln writes output to Out with automatic newline
	Fprintln(args ...any)
	// Errorf writes formatted error output to ErrOut with automatic newline
	Errorf(format string, args ...any)
	// Errorln writes error output to ErrOut with automatic newline
	Errorln(args ...any)
	// Out returns the output writer (stdout)
	Out() io.Writer
	// In returns the input reader (stdin)
	In() io.Reader
	// ErrOut returns the error output writer (stderr)
	ErrOut() io.Writer
}

// IOStreams provides structured access to standard input/output/error streams
// with convenience methods for formatted output.
type IOStreams struct {
	// in is the input stream (stdin)
	in io.Reader
	// out is the output stream (stdout)
	out io.Writer
	// errOut is the error output stream (stderr)
	errOut io.Writer
}

// NewIOStreams creates a new IOStreams with the given readers/writers.
func NewIOStreams(in io.Reader, out io.Writer, errOut io.Writer) *IOStreams {
	return &IOStreams{
		in:     in,
		out:    out,
		errOut: errOut,
	}
}

// Fprintf writes formatted output to Out with automatic newline.
// If args are provided, the format string is processed with fmt.Sprintf.
// If no args are provided, the format string is written directly.
// Per constitution: automatically appends newline, conditionally uses fmt.Sprintf.
func (s *IOStreams) Fprintf(format string, args ...any) {
	if s.out == nil {
		// Gracefully handle nil writer - silently ignore
		return
	}

	var message string
	if len(args) > 0 {
		message = fmt.Sprintf(format, args...)
	} else {
		message = format
	}

	_, _ = fmt.Fprintln(s.out, message)
}

// Fprintln writes output to Out with automatic newline.
// This is a direct pass-through to fmt.Fprintln.
func (s *IOStreams) Fprintln(args ...any) {
	if s.out == nil {
		// Gracefully handle nil writer - silently ignore
		return
	}

	_, _ = fmt.Fprintln(s.out, args...)
}

// Errorf writes formatted error output to ErrOut with automatic newline.
// If args are provided, the format string is processed with fmt.Sprintf.
// If no args are provided, the format string is written directly.
// Per constitution: automatically appends newline, conditionally uses fmt.Sprintf.
func (s *IOStreams) Errorf(format string, args ...any) {
	if s.errOut == nil {
		// Gracefully handle nil writer - silently ignore
		return
	}

	var message string
	if len(args) > 0 {
		message = fmt.Sprintf(format, args...)
	} else {
		message = format
	}

	_, _ = fmt.Fprintln(s.errOut, message)
}

// Errorln writes error output to ErrOut with automatic newline.
// This is a direct pass-through to fmt.Fprintln on the error stream.
func (s *IOStreams) Errorln(args ...any) {
	if s.errOut == nil {
		// Gracefully handle nil writer - silently ignore
		return
	}

	_, _ = fmt.Fprintln(s.errOut, args...)
}

// Out returns the output writer (stdout).
func (s *IOStreams) Out() io.Writer {
	return s.out
}

// In returns the input reader (stdin).
func (s *IOStreams) In() io.Reader {
	return s.in
}

// ErrOut returns the error output writer (stderr).
func (s *IOStreams) ErrOut() io.Writer {
	return s.errOut
}

// QuietWrapper wraps an IOStreams and suppresses error output (Errorf/Errorln).
// Regular output (Fprintf/Fprintln) is passed through unchanged.
type QuietWrapper struct {
	delegate Interface
}

// NewQuietWrapper creates a new QuietWrapper that suppresses error output.
func NewQuietWrapper(delegate Interface) *QuietWrapper {
	return &QuietWrapper{delegate: delegate}
}

// Fprintf passes through to the delegate unchanged.
func (q *QuietWrapper) Fprintf(format string, args ...any) {
	q.delegate.Fprintf(format, args...)
}

// Fprintln passes through to the delegate unchanged.
func (q *QuietWrapper) Fprintln(args ...any) {
	q.delegate.Fprintln(args...)
}

// Errorf is suppressed (no-op) in quiet mode.
func (q *QuietWrapper) Errorf(_ string, _ ...any) {
	// Suppress - no output
}

// Errorln is suppressed (no-op) in quiet mode.
func (q *QuietWrapper) Errorln(_ ...any) {
	// Suppress - no output
}

// Out returns the output writer from the delegate.
func (q *QuietWrapper) Out() io.Writer {
	return q.delegate.Out()
}

// In returns the input reader from the delegate.
func (q *QuietWrapper) In() io.Reader {
	return q.delegate.In()
}

// ErrOut returns the error output writer from the delegate.
func (q *QuietWrapper) ErrOut() io.Writer {
	return q.delegate.ErrOut()
}
