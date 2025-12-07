package check

import "fmt"

// Severity represents the severity level of a diagnostic finding.
type Severity string

const (
	SeverityCritical Severity = "Critical"
	SeverityWarning  Severity = "Warning"
	SeverityInfo     Severity = "Info"
)

// Validate checks if the severity is valid.
func (s Severity) Validate() error {
	switch s {
	case SeverityCritical, SeverityWarning, SeverityInfo:
		return nil
	default:
		return fmt.Errorf("invalid severity: %s", s)
	}
}

// String returns the string representation of the severity.
func (s Severity) String() string {
	return string(s)
}
