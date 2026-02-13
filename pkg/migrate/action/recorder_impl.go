package action

import (
	"fmt"
	"sync"
	"time"

	"github.com/fatih/color"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/action/result"
	"github.com/opendatahub-io/odh-cli/pkg/util/iostreams"
)

// stepRecorderImpl implements both StepRecorder and RootRecorder interfaces.
type stepRecorderImpl struct {
	step     *result.ActionStep
	parent   *stepRecorderImpl
	children []*stepRecorderImpl
	mu       sync.Mutex
	io       iostreams.Interface // For real-time output
	verbose  bool                // Whether to output steps in real-time
}

// NewRootRecorder creates a new root recorder for collecting migration steps.
func NewRootRecorder() RootRecorder {
	return &stepRecorderImpl{
		step:     nil, // Root has no step
		children: make([]*stepRecorderImpl, 0),
	}
}

// NewVerboseRootRecorder creates a root recorder that outputs steps in real-time.
func NewVerboseRootRecorder(io iostreams.Interface) RootRecorder {
	return &stepRecorderImpl{
		step:     nil,
		children: make([]*stepRecorderImpl, 0),
		io:       io,
		verbose:  true,
	}
}

// Child creates a derived recorder for a sub-step.
func (r *stepRecorderImpl) Child(name string, description string) StepRecorder {
	r.mu.Lock()
	defer r.mu.Unlock()

	step := &result.ActionStep{
		Name:        name,
		Description: description,
		Status:      result.StepRunning,
		Message:     "",
		Timestamp:   time.Now(),
		Children:    []result.ActionStep{},
		Details:     make(map[string]any),
	}

	child := &stepRecorderImpl{
		step:     step,
		parent:   r,
		children: make([]*stepRecorderImpl, 0),
		io:       r.io,
		verbose:  r.verbose,
	}

	r.children = append(r.children, child)

	// Output step start in real-time if verbose
	if r.verbose && r.io != nil {
		r.io.Errorf("  → %s", description)
	}

	return child
}

// Complete marks this step as complete with status and message.
// Supports printf-style formatting with variadic arguments.
func (r *stepRecorderImpl) Complete(status result.StepStatus, messageFormat string, args ...any) {
	r.mu.Lock()
	defer r.mu.Unlock()

	message := messageFormat
	if len(args) > 0 {
		message = fmt.Sprintf(messageFormat, args...)
	}

	if r.step != nil {
		r.step.Status = status
		r.step.Message = message
	}

	// Output completion in real-time if verbose
	if r.verbose && r.io != nil && message != "" {
		statusIcon := getStatusIcon(status)
		r.io.Errorf("    %s %s", statusIcon, message)
	}
}

const iconInProgress = "⋯"

func getStatusIcon(status result.StepStatus) string {
	switch status {
	case result.StepCompleted:
		return color.GreenString("✓")
	case result.StepFailed:
		return color.RedString("✗")
	case result.StepSkipped:
		return color.YellowString("→")
	case result.StepPending, result.StepRunning:
		return color.CyanString(iconInProgress)
	}

	return color.CyanString(iconInProgress)
}

// AddDetail adds structured data to this step.
func (r *stepRecorderImpl) AddDetail(key string, value any) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.step != nil {
		r.step.Details[key] = value
	}
}

// Record adds a simple completed sub-step (convenience method).
// Supports printf-style formatting with variadic arguments.
func (r *stepRecorderImpl) Record(name string, messageFormat string, status result.StepStatus, args ...any) {
	message := messageFormat
	if len(args) > 0 {
		message = fmt.Sprintf(messageFormat, args...)
	}

	child := r.Child(name, message)
	child.Complete(status, message)
}

// Build constructs the final ActionResult with all recorded steps.
func (r *stepRecorderImpl) Build() *result.ActionResult {
	r.mu.Lock()
	defer r.mu.Unlock()

	actionResult := &result.ActionResult{
		Status: result.ActionStatus{
			Steps:     r.buildSteps(),
			Completed: true,
		},
	}

	return actionResult
}

// buildSteps recursively builds the step tree.
func (r *stepRecorderImpl) buildSteps() []result.ActionStep {
	steps := make([]result.ActionStep, 0, len(r.children))

	for _, child := range r.children {
		if child.step != nil {
			// Build this step
			step := *child.step

			// Recursively build children
			step.Children = child.buildSteps()

			steps = append(steps, step)
		}
	}

	return steps
}
