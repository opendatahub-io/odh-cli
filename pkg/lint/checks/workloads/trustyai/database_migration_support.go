package trustyai

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/validate"
)

func hasDatabaseStorage(obj *unstructured.Unstructured) (bool, error) {
	format, _, _ := unstructured.NestedString(obj.Object, "spec", "storage", "format")

	return format == "DATABASE", nil
}

func (c *DatabaseStorageMigrationCheck) newDatabaseMigrationCondition(
	_ context.Context,
	req *validate.WorkloadRequest[*unstructured.Unstructured],
) ([]result.Condition, error) {
	count := len(req.Items)

	if count == 0 {
		return []result.Condition{check.NewCondition(
			check.ConditionTypeCompatible,
			metav1.ConditionTrue,
			check.WithReason(check.ReasonResourceNotFound),
			check.WithMessage("No TrustyAIService instances using database storage found"),
		)}, nil
	}

	return []result.Condition{check.NewCondition(
		check.ConditionTypeCompatible,
		metav1.ConditionFalse,
		check.WithReason(check.ReasonMigrationPending),
		check.WithMessage("Found %d TrustyAIService instance(s) using database storage - verify the databaseConfigurations secret remains accessible after upgrade", count),
		check.WithImpact(result.ImpactAdvisory),
		check.WithRemediation(c.CheckRemediation),
	)}, nil
}
