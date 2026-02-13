package result

import (
	"errors"
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/opendatahub-io/odh-cli/pkg/resources"
)

const (
	// Validation error messages.
	errMsgGroupEmpty              = "group must not be empty"
	errMsgKindEmpty               = "kind must not be empty"
	errMsgNameEmpty               = "name must not be empty"
	errMsgConditionsEmpty         = "status.conditions must contain at least one condition"
	errMsgConditionTypeEmpty      = "condition with empty type found"
	errMsgConditionReasonEmpty    = "condition %q has empty reason"
	errMsgConditionInvalidStatus  = "condition %q has invalid status (must be True, False, or Unknown)"
	errMsgAnnotationInvalidFormat = "annotation key %q must be in domain/key format (e.g., openshiftai.io/version)"
)

// Impact represents the upgrade impact level of a diagnostic condition.
type Impact string

// Impact levels for diagnostic conditions.
const (
	ImpactBlocking Impact = "blocking" // Upgrade CANNOT proceed
	ImpactAdvisory Impact = "advisory" // Upgrade CAN proceed with warning
	ImpactNone     Impact = ""         // No impact (omitted from JSON/YAML)
)

// Condition represents a diagnostic condition with severity level.
// It embeds metav1.Condition and adds Impact and Remediation fields to indicate
// the impact level and remediation guidance of the condition result.
type Condition struct {
	metav1.Condition `json:",inline" yaml:",inline"`

	// Impact indicates the upgrade impact level.
	// Auto-derived from Status unless explicitly overridden via WithImpact option.
	Impact Impact `json:"impact,omitempty" yaml:"impact,omitempty"`

	// Remediation provides actionable guidance on how to resolve the condition.
	// Set via WithRemediation option during condition creation.
	Remediation string `json:"remediation,omitempty" yaml:"remediation,omitempty"`
}

// Validate ensures the condition has valid Status/Impact combination.
func (c Condition) Validate() error {
	switch c.Status {
	case metav1.ConditionTrue:
		// True status must have no impact.
		if c.Impact != ImpactNone && c.Impact != "" {
			return fmt.Errorf(
				"condition with Status=True must have Impact=None, got Impact=%q (if there's an impact, the condition is not met)",
				c.Impact,
			)
		}

	case metav1.ConditionFalse, metav1.ConditionUnknown:
		// False/Unknown status must have impact specified.
		if c.Impact == ImpactNone || c.Impact == "" {
			return fmt.Errorf(
				"condition with Status=%q must have Impact specified (Blocking or Advisory), got Impact=%q",
				c.Status, c.Impact,
			)
		}

		// Validate impact values.
		if c.Impact != ImpactBlocking && c.Impact != ImpactAdvisory {
			return fmt.Errorf(
				"invalid Impact=%q, must be %q or %q",
				c.Impact, ImpactBlocking, ImpactAdvisory,
			)
		}

	default:
		return fmt.Errorf("invalid Status: %q", c.Status)
	}

	return nil
}

// DiagnosticSpec describes what the check validates.
type DiagnosticSpec struct {
	// Description provides a detailed explanation of the check purpose and significance
	Description string `json:"description" yaml:"description"`
}

// DiagnosticStatus contains the condition-based validation results.
type DiagnosticStatus struct {
	// Conditions is an array of validation conditions ordered by execution sequence
	Conditions []Condition `json:"conditions" yaml:"conditions"`
}

// DiagnosticResult represents a diagnostic check result with flattened metadata fields.
type DiagnosticResult struct {
	// Group is the diagnostic target category (e.g., "components", "services", "workloads")
	Group string `json:"group" yaml:"group"`

	// Kind is the specific target being checked (e.g., "kserve", "auth", "cert-manager")
	Kind string `json:"kind" yaml:"kind"`

	// Name is the check identifier (e.g., "version-compatibility", "configuration-valid")
	Name string `json:"name" yaml:"name"`

	// Annotations contains optional key-value metadata with domain-qualified keys
	Annotations map[string]string `json:"annotations,omitempty" yaml:"annotations,omitempty"`

	// Spec describes what the check validates
	Spec DiagnosticSpec `json:"spec" yaml:"spec"`

	// Status contains the condition-based validation results
	Status DiagnosticStatus `json:"status" yaml:"status"`

	// ImpactedObjects contains references to resources impacted by this diagnostic.
	// Uses PartialObjectMetadata to store minimal object info with optional annotations
	// for additional context (e.g., deployment mode, configuration details).
	ImpactedObjects []metav1.PartialObjectMetadata `json:"impactedObjects,omitempty" yaml:"impactedObjects,omitempty"`
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

// validateCondition validates a single condition.
func validateCondition(condition *Condition) error {
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

	return nil
}

// Validate checks if the diagnostic result is valid.
func (r *DiagnosticResult) Validate() error {
	// Validate flattened metadata fields
	if r.Group == "" {
		return errors.New(errMsgGroupEmpty)
	}
	if r.Kind == "" {
		return errors.New(errMsgKindEmpty)
	}
	if r.Name == "" {
		return errors.New(errMsgNameEmpty)
	}

	// Validate annotation keys follow domain/key format
	for key := range r.Annotations {
		if !isValidAnnotationKey(key) {
			return fmt.Errorf(errMsgAnnotationInvalidFormat, key)
		}
	}

	// Validate conditions array
	if len(r.Status.Conditions) == 0 {
		return errors.New(errMsgConditionsEmpty)
	}

	// Validate each condition
	for i := range r.Status.Conditions {
		if err := validateCondition(&r.Status.Conditions[i]); err != nil {
			return err
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
		Group:       group,
		Kind:        kind,
		Name:        name,
		Annotations: make(map[string]string),
		Spec: DiagnosticSpec{
			Description: description,
		},
		Status: DiagnosticStatus{
			Conditions: []Condition{},
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

// GetImpact returns the highest impact level across all conditions.
// Returns nil if there are no conditions.
// Impact is always explicit (set during condition creation), never derived.
func (r *DiagnosticResult) GetImpact() *string {
	if len(r.Status.Conditions) == 0 {
		return nil
	}

	var hasBlocking bool
	var hasAdvisory bool

	for _, cond := range r.Status.Conditions {
		switch cond.Impact {
		case ImpactBlocking:
			hasBlocking = true
		case ImpactAdvisory:
			hasAdvisory = true
		case ImpactNone:
			// No impact - continue checking other conditions
		}
	}

	var result Impact
	if hasBlocking {
		result = ImpactBlocking
	} else if hasAdvisory {
		result = ImpactAdvisory
	} else {
		result = ImpactNone
	}

	resultStr := string(result)

	return &resultStr
}

// GetRemediation returns remediation guidance from the first condition that has it set.
func (r *DiagnosticResult) GetRemediation() string {
	for _, cond := range r.Status.Conditions {
		if cond.Remediation != "" {
			return cond.Remediation
		}
	}

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

// SetCondition updates or adds a condition to the diagnostic result.
// If a condition with the same type already exists, it updates it.
// If no condition with that type exists, it adds a new one.
func (r *DiagnosticResult) SetCondition(condition Condition) {
	for i := range r.Status.Conditions {
		if r.Status.Conditions[i].Type == condition.Type {
			r.Status.Conditions[i] = condition

			return
		}
	}

	r.Status.Conditions = append(r.Status.Conditions, condition)
}

// SetImpactedObjects replaces all impacted objects from a list of NamespacedNames.
func (r *DiagnosticResult) SetImpactedObjects(
	resourceType resources.ResourceType,
	names []types.NamespacedName,
) {
	r.ImpactedObjects = make([]metav1.PartialObjectMetadata, 0, len(names))

	for _, n := range names {
		r.ImpactedObjects = append(r.ImpactedObjects, metav1.PartialObjectMetadata{
			TypeMeta: resourceType.TypeMeta(),
			ObjectMeta: metav1.ObjectMeta{
				Namespace: n.Namespace,
				Name:      n.Name,
			},
		})
	}
}

// AddImpactedObjects appends impacted objects from a list of NamespacedNames.
func (r *DiagnosticResult) AddImpactedObjects(
	resourceType resources.ResourceType,
	names []types.NamespacedName,
) {
	for _, n := range names {
		r.ImpactedObjects = append(r.ImpactedObjects, metav1.PartialObjectMetadata{
			TypeMeta: resourceType.TypeMeta(),
			ObjectMeta: metav1.ObjectMeta{
				Namespace: n.Namespace,
				Name:      n.Name,
			},
		})
	}
}

// DiagnosticResultList represents a list of diagnostic results.
type DiagnosticResultList struct {
	ClusterVersion *string             `json:"clusterVersion,omitempty" yaml:"clusterVersion,omitempty"`
	TargetVersion  *string             `json:"targetVersion,omitempty"  yaml:"targetVersion,omitempty"`
	Results        []*DiagnosticResult `json:"results"                  yaml:"results"`
}

// NewDiagnosticResultList creates a new list.
func NewDiagnosticResultList(clusterVersion *string, targetVersion *string) *DiagnosticResultList {
	return &DiagnosticResultList{
		ClusterVersion: clusterVersion,
		TargetVersion:  targetVersion,
		Results:        make([]*DiagnosticResult, 0),
	}
}
