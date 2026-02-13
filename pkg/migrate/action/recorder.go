package action

import "github.com/opendatahub-io/odh-cli/pkg/migrate/action/result"

// StepRecorder provides methods to record migration steps hierarchically.
// Each recorder represents a step in the migration process and can create child recorders for sub-steps.
type StepRecorder interface {
	// Child creates a derived recorder for a sub-step.
	// The child automatically nests under this recorder's step.
	Child(name string, description string) StepRecorder

	// Complete marks this step as complete with status and message.
	// Supports printf-style formatting with variadic arguments.
	Complete(status result.StepStatus, messageFormat string, args ...any)

	// AddDetail adds structured data to this step (for JSON/YAML output).
	AddDetail(key string, value any)

	// Record adds a simple completed sub-step (convenience method for quick recordings).
	// Supports printf-style formatting with variadic arguments.
	Record(name string, messageFormat string, status result.StepStatus, args ...any)
}

// RootRecorder is the top-level recorder that can build the final ActionResult.
type RootRecorder interface {
	StepRecorder
	// Build constructs the final ActionResult with all recorded steps.
	Build() *result.ActionResult
}
