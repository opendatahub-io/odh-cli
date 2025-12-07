package results

import "github.com/lburgazzoli/odh-cli/pkg/doctor/check"

// DataScienceClusterNotFound returns a standard passing result when DataScienceCluster is not found.
// This is used by component checks that require DSC to exist.
func DataScienceClusterNotFound() *check.DiagnosticResult {
	return &check.DiagnosticResult{
		Status:  check.StatusPass,
		Message: "No DataScienceCluster found",
	}
}

// DSCInitializationNotFound returns a standard passing result when DSCInitialization is not found.
// This is used by service checks that require DSCInitialization to exist.
func DSCInitializationNotFound() *check.DiagnosticResult {
	return &check.DiagnosticResult{
		Status:  check.StatusPass,
		Message: "No DSCInitialization found",
	}
}
