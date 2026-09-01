package trustyai

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/validate"
)

func hasPVCStorage(obj *unstructured.Unstructured) (bool, error) {
	format, _, _ := unstructured.NestedString(obj.Object, "spec", "storage", "format")

	return format == "PVC", nil
}

func (c *PVCStorageMigrationCheck) newPVCMigrationCondition(
	_ context.Context,
	req *validate.WorkloadRequest[*unstructured.Unstructured],
) ([]result.Condition, error) {
	count := len(req.Items)

	if count == 0 {
		return []result.Condition{check.NewCondition(
			check.ConditionTypeCompatible,
			metav1.ConditionTrue,
			check.WithReason(check.ReasonResourceNotFound),
			check.WithMessage("No TrustyAIService instances using PVC storage found"),
		)}, nil
	}

	return []result.Condition{check.NewCondition(
		check.ConditionTypeCompatible,
		metav1.ConditionFalse,
		check.WithReason(check.ReasonMigrationPending),
		check.WithMessage("Found %d TrustyAIService instance(s) using PVC storage - back up data before upgrading to prevent loss during volume reconfiguration", count),
		check.WithImpact(result.ImpactBlocking),
		check.WithRemediation(c.CheckRemediation),
	)}, nil
}
