package result

import (
	"time"
)

type StepStatus string

const (
	StepPending   StepStatus = "Pending"
	StepRunning   StepStatus = "Running"
	StepCompleted StepStatus = "Completed"
	StepFailed    StepStatus = "Failed"
	StepSkipped   StepStatus = "Skipped"
)

type ActionResult struct {
	Metadata ActionMetadata `json:"metadata" yaml:"metadata"`
	Spec     ActionSpec     `json:"spec"     yaml:"spec"`
	Status   ActionStatus   `json:"status"   yaml:"status"`
}

type ActionMetadata struct {
	Group       string            `json:"group"                 yaml:"group"`
	Kind        string            `json:"kind"                  yaml:"kind"`
	Name        string            `json:"name"                  yaml:"name"`
	Annotations map[string]string `json:"annotations,omitempty" yaml:"annotations,omitempty"`
}

type ActionSpec struct {
	Description string `json:"description" yaml:"description"`
	DryRun      bool   `json:"dryRun"      yaml:"dryRun"`
}

type ActionStatus struct {
	Steps     []ActionStep `json:"steps"           yaml:"steps"`
	Completed bool         `json:"completed"       yaml:"completed"`
	Error     string       `json:"error,omitempty" yaml:"error,omitempty"`
}

type ActionStep struct {
	Name        string         `json:"name"               yaml:"name"`
	Description string         `json:"description"        yaml:"description"`
	Status      StepStatus     `json:"status"             yaml:"status"`
	Message     string         `json:"message,omitempty"  yaml:"message,omitempty"`
	Timestamp   time.Time      `json:"timestamp"          yaml:"timestamp"`
	Children    []ActionStep   `json:"children,omitempty" yaml:"children,omitempty"`
	Details     map[string]any `json:"details,omitempty"  yaml:"details,omitempty"`
}

func New(
	group string,
	kind string,
	name string,
	description string,
) *ActionResult {
	return &ActionResult{
		Metadata: ActionMetadata{
			Group:       group,
			Kind:        kind,
			Name:        name,
			Annotations: make(map[string]string),
		},
		Spec: ActionSpec{
			Description: description,
		},
		Status: ActionStatus{
			Steps:     []ActionStep{},
			Completed: false,
		},
	}
}

func (r *ActionResult) HasSkippedSteps() bool {
	return hasSkipped(r.Status.Steps)
}

func hasSkipped(steps []ActionStep) bool {
	for _, s := range steps {
		if s.Status == StepSkipped {
			return true
		}

		if hasSkipped(s.Children) {
			return true
		}
	}

	return false
}

func NewStep(
	name string,
	description string,
	status StepStatus,
	message string,
) ActionStep {
	return ActionStep{
		Name:        name,
		Description: description,
		Status:      status,
		Message:     message,
		Timestamp:   time.Now(),
		Children:    []ActionStep{},
		Details:     make(map[string]any),
	}
}
