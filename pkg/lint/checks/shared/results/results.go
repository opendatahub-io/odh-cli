package results

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/lburgazzoli/odh-cli/pkg/lint/check"
	"github.com/lburgazzoli/odh-cli/pkg/lint/check/result"
)

// DataScienceClusterNotFound returns a standard passing result when DataScienceCluster is not found.
// This is used by component checks that require DSC to exist.
func DataScienceClusterNotFound(group string, kind string, name string, description string) *result.DiagnosticResult {
	dr := result.New(group, kind, name, description)
	dr.Status.Conditions = []metav1.Condition{
		check.NewCondition(
			check.ConditionTypeAvailable,
			metav1.ConditionFalse,
			check.ReasonResourceNotFound,
			"No DataScienceCluster found",
		),
	}

	return dr
}

// DSCInitializationNotFound returns a standard passing result when DSCInitialization is not found.
// This is used by service checks that require DSCInitialization to exist.
func DSCInitializationNotFound(group string, kind string, name string, description string) *result.DiagnosticResult {
	dr := result.New(group, kind, name, description)
	dr.Status.Conditions = []metav1.Condition{
		check.NewCondition(
			check.ConditionTypeAvailable,
			metav1.ConditionFalse,
			check.ReasonResourceNotFound,
			"No DSCInitialization found",
		),
	}

	return dr
}
