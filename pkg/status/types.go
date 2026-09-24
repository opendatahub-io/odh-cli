package status

import (
	"github.com/opendatahub-io/opendatahub-operator/pkg/clusterhealth"

	"github.com/opendatahub-io/odh-cli/pkg/deps"
	"github.com/opendatahub-io/odh-cli/pkg/output"
)

// StatusReport wraps the cluster health report with a standard envelope.
type StatusReport struct {
	output.Envelope

	Report       *clusterhealth.Report   `json:"report"                 yaml:"report"`
	Dependencies []deps.DependencyStatus `json:"dependencies,omitempty" yaml:"dependencies,omitempty"`
}

// NewStatusReport creates a new StatusReport with envelope fields populated.
func NewStatusReport(report *clusterhealth.Report, depStatuses []deps.DependencyStatus) *StatusReport {
	sr := &StatusReport{
		Envelope:     output.NewEnvelope("StatusReport", "status"),
		Report:       report,
		Dependencies: depStatuses,
	}
	sr.computeStatus()

	return sr
}

// computeStatus calculates warnings/errors from the report sections and dependencies.
func (sr *StatusReport) computeStatus() {
	errors := sr.countSectionErrors()
	warnings := 0

	// Count dependency issues
	for _, dep := range sr.Dependencies {
		if dep.Error != "" {
			errors++

			continue
		}

		switch dep.Status {
		case deps.StatusMissing:
			errors++
		case deps.StatusUnknown, deps.StatusPending:
			warnings++
		case deps.StatusInstalled, deps.StatusOptional:
			// No action needed
		}
	}

	sr.SetStatus(warnings, errors)
}

// countSectionErrors counts the number of sections with errors.
// Uses direct field access to avoid allocating a temporary slice.
func (sr *StatusReport) countSectionErrors() int {
	if sr.Report == nil {
		return 0
	}

	count := 0
	r := sr.Report

	if r.Nodes.Error != "" {
		count++
	}

	if r.Deployments.Error != "" {
		count++
	}

	if r.Pods.Error != "" {
		count++
	}

	if r.Events.Error != "" {
		count++
	}

	if r.Quotas.Error != "" {
		count++
	}

	if r.Operator.Error != "" {
		count++
	}

	if r.DSCI.Error != "" {
		count++
	}

	if r.DSC.Error != "" {
		count++
	}

	return count
}
