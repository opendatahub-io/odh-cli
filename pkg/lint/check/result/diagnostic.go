package result

import (
	"errors"
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// Validation error messages.
	errMsgGroupEmpty              = "metadata.group must not be empty"
	errMsgKindEmpty               = "metadata.kind must not be empty"
	errMsgNameEmpty               = "metadata.name must not be empty"
	errMsgConditionsEmpty         = "status.conditions must contain at least one condition"
	errMsgConditionTypeEmpty      = "condition with empty type found"
	errMsgConditionReasonEmpty    = "condition %q has empty reason"
	errMsgConditionInvalidStatus  = "condition %q has invalid status (must be True, False, or Unknown)"
	errMsgAnnotationInvalidFormat = "annotation key %q must be in domain/key format (e.g., openshiftai.io/version)"
)

// DiagnosticMetadata contains CR-like metadata identifying the diagnostic target and check.
type DiagnosticMetadata struct {
	// Group is the diagnostic target category (e.g., "components", "services", "workloads")
	Group string `json:"group" yaml:"group"`

	// Kind is the specific target being checked (e.g., "kserve", "auth", "cert-manager")
	Kind string `json:"kind" yaml:"kind"`

	// Name is the check identifier (e.g., "version-compatibility", "configuration-valid")
	Name string `json:"name" yaml:"name"`

	// Annotations contains optional key-value metadata with domain-qualified keys
	Annotations map[string]string `json:"annotations,omitempty" yaml:"annotations,omitempty"`
}

// Validate checks if the diagnostic metadata is valid.
func (m *DiagnosticMetadata) Validate() error {
	if m.Group == "" {
		return errors.New(errMsgGroupEmpty)
	}
	if m.Kind == "" {
		return errors.New(errMsgKindEmpty)
	}
	if m.Name == "" {
		return errors.New(errMsgNameEmpty)
	}

	// Validate annotation keys follow domain/key format
	for key := range m.Annotations {
		if !isValidAnnotationKey(key) {
			return fmt.Errorf(errMsgAnnotationInvalidFormat, key)
		}
	}

	return nil
}

// isValidAnnotationKey validates that an annotation key follows the domain/key format.
// Valid examples: openshiftai.io/version, example.com/name
// Invalid examples: version, /name, example.com/.
func isValidAnnotationKey(key string) bool {
	// Must contain exactly one '/' separating domain and key
	parts := strings.Split(key, "/")
	if len(parts) != 2 {
		return false
	}

	domain, name := parts[0], parts[1]

	// Domain and name must both be non-empty
	if domain == "" || name == "" {
		return false
	}

	// Domain must contain at least one dot (e.g., example.com, openshiftai.io)
	if !strings.Contains(domain, ".") {
		return false
	}

	return true
}

// DiagnosticSpec describes what the check validates.
type DiagnosticSpec struct {
	// Description provides a detailed explanation of the check purpose and significance
	Description string `json:"description" yaml:"description"`
}

// DiagnosticStatus contains the condition-based validation results.
type DiagnosticStatus struct {
	// Conditions is an array of validation conditions ordered by execution sequence
	Conditions []metav1.Condition `json:"conditions" yaml:"conditions"`
}

// DiagnosticResult represents a diagnostic check result following Kubernetes Custom Resource conventions.
type DiagnosticResult struct {
	// Metadata contains CR-like metadata identifying the diagnostic target and check
	Metadata DiagnosticMetadata `json:"metadata" yaml:"metadata"`

	// Spec describes what the check validates
	Spec DiagnosticSpec `json:"spec" yaml:"spec"`

	// Status contains the condition-based validation results
	Status DiagnosticStatus `json:"status" yaml:"status"`
}

// Validate checks if the diagnostic result is valid.
func (r *DiagnosticResult) Validate() error {
	// Validate metadata using dedicated method
	if err := r.Metadata.Validate(); err != nil {
		return err
	}

	// Validate conditions array
	if len(r.Status.Conditions) == 0 {
		return errors.New(errMsgConditionsEmpty)
	}

	// Validate each condition
	for i := range r.Status.Conditions {
		condition := &r.Status.Conditions[i]

		if condition.Type == "" {
			return errors.New(errMsgConditionTypeEmpty)
		}
		if condition.Status != metav1.ConditionTrue &&
			condition.Status != metav1.ConditionFalse &&
			condition.Status != metav1.ConditionUnknown {
			return fmt.Errorf(errMsgConditionInvalidStatus, condition.Type)
		}
		if condition.Reason == "" {
			return fmt.Errorf(errMsgConditionReasonEmpty, condition.Type)
		}
	}

	return nil
}

// New creates a new diagnostic result.
func New(
	group string,
	kind string,
	name string,
	description string,
) *DiagnosticResult {
	return &DiagnosticResult{
		Metadata: DiagnosticMetadata{
			Group:       group,
			Kind:        kind,
			Name:        name,
			Annotations: make(map[string]string),
		},
		Spec: DiagnosticSpec{
			Description: description,
		},
		Status: DiagnosticStatus{
			Conditions: []metav1.Condition{},
		},
	}
}

// IsFailing returns true if any condition has status False or Unknown.
func (r *DiagnosticResult) IsFailing() bool {
	for _, cond := range r.Status.Conditions {
		if cond.Status == metav1.ConditionFalse || cond.Status == metav1.ConditionUnknown {
			return true
		}
	}

	return false
}

// GetMessage returns a summary message from all conditions.
func (r *DiagnosticResult) GetMessage() string {
	if len(r.Status.Conditions) == 0 {
		return ""
	}
	// Return the first condition's message as the primary message
	return r.Status.Conditions[0].Message
}

// GetSeverity returns the severity level based on condition statuses.
// Critical: Any ConditionFalse (indicates a failure)
// Warning: Any ConditionUnknown (indicates an error or inability to determine)
// Info: All ConditionTrue (indicates success/informational)
// Returns nil if there are no conditions.
func (r *DiagnosticResult) GetSeverity() *string {
	if len(r.Status.Conditions) == 0 {
		return nil
	}

	var hasFalse bool
	var hasUnknown bool

	for _, cond := range r.Status.Conditions {
		switch cond.Status {
		case metav1.ConditionFalse:
			hasFalse = true
		case metav1.ConditionUnknown:
			hasUnknown = true
		case metav1.ConditionTrue:
			hasFalse = false
			hasUnknown = false
		default:
			continue
		}
	}

	var severity string
	if hasFalse {
		severity = "critical"
	} else if hasUnknown {
		severity = "warning"
	} else {
		severity = "info"
	}

	return &severity
}

// GetRemediation returns remediation guidance.
// Currently returns empty string as remediation is not part of the CR pattern.
// Remediation can be inferred from condition reasons and messages.
func (r *DiagnosticResult) GetRemediation() string {
	return ""
}

// GetStatusString returns a string representation of the overall status.
// Pass: All conditions are True
// Fail: Any condition is False
// Error: Any condition is Unknown.
func (r *DiagnosticResult) GetStatusString() string {
	if len(r.Status.Conditions) == 0 {
		return "Unknown"
	}

	for _, cond := range r.Status.Conditions {
		if cond.Status == metav1.ConditionFalse {
			return "Fail"
		}
		if cond.Status == metav1.ConditionUnknown {
			return "Error"
		}
	}

	return "Pass"
}

// DiagnosticResultListMetadata contains metadata for the list of diagnostic results.
type DiagnosticResultListMetadata struct {
	ClusterVersion *string `json:"clusterVersion,omitempty" yaml:"clusterVersion,omitempty"`
	TargetVersion  *string `json:"targetVersion,omitempty"  yaml:"targetVersion,omitempty"`
}

// DiagnosticResultList represents a list of diagnostic results.
type DiagnosticResultList struct {
	Kind     string                       `json:"kind"     yaml:"kind"`
	Metadata DiagnosticResultListMetadata `json:"metadata" yaml:"metadata"`
	Items    []*DiagnosticResult          `json:"items"    yaml:"items"`
}

// NewDiagnosticResultList creates a new list.
func NewDiagnosticResultList(clusterVersion *string, targetVersion *string) *DiagnosticResultList {
	return &DiagnosticResultList{
		Kind: "DiagnosticResultList",
		Metadata: DiagnosticResultListMetadata{
			ClusterVersion: clusterVersion,
			TargetVersion:  targetVersion,
		},
		Items: make([]*DiagnosticResult, 0),
	}
}
