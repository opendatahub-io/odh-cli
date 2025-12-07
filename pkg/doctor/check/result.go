package check

import (
	"errors"
	"fmt"
)

// ResultStatus represents the outcome of a check validation.
type ResultStatus string

const (
	StatusPass    ResultStatus = "Pass"
	StatusFail    ResultStatus = "Fail"
	StatusError   ResultStatus = "Error"
	StatusSkipped ResultStatus = "Skipped"
)

// Validate checks if the result status is valid.
func (r ResultStatus) Validate() error {
	switch r {
	case StatusPass, StatusFail, StatusError, StatusSkipped:
		return nil
	default:
		return fmt.Errorf("invalid result status: %s", r)
	}
}

// String returns the string representation of the result status.
func (r ResultStatus) String() string {
	return string(r)
}

// DiagnosticResult represents the outcome of a check validation.
type DiagnosticResult struct {
	// Status indicates the check outcome (Pass, Fail, Error, Skipped)
	Status ResultStatus

	// Severity indicates the severity level (only set when Status is Fail)
	// Nil for Pass/Skipped/Error results
	Severity *Severity

	// Message provides context about the result
	Message string

	// Details contains additional diagnostic information (optional)
	Details map[string]any

	// Remediation provides actionable guidance for addressing failures
	Remediation string
}

// IsFailing returns true if the result represents a failure (status is Fail or Error).
func (r *DiagnosticResult) IsFailing() bool {
	return r.Status == StatusFail || r.Status == StatusError
}

// IsCritical returns true if the result is a critical failure.
func (r *DiagnosticResult) IsCritical() bool {
	return r.Status == StatusFail && r.Severity != nil && *r.Severity == SeverityCritical
}

// Validate checks if the diagnostic result is valid.
func (r *DiagnosticResult) Validate() error {
	if err := r.Status.Validate(); err != nil {
		return err
	}

	// Severity should only be set for Fail status
	if r.Severity != nil {
		if r.Status != StatusFail {
			return errors.New("severity can only be set when status is Fail")
		}
		if err := r.Severity.Validate(); err != nil {
			return err
		}
	} else if r.Status == StatusFail {
		return errors.New("severity must be set when status is Fail")
	}

	return nil
}
