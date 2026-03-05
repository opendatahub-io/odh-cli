package datasciencecluster

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/opendatahub-io/odh-cli/pkg/constants"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/validate"
)

// DataScienceClusterReadinessCheck validates that DataScienceCluster is in Ready state.
type DataScienceClusterReadinessCheck struct {
	check.BaseCheck
}

// NewDataScienceClusterReadinessCheck creates a new DataScienceClusterReadinessCheck.
func NewDataScienceClusterReadinessCheck() *DataScienceClusterReadinessCheck {
	return &DataScienceClusterReadinessCheck{
		BaseCheck: check.BaseCheck{
			CheckGroup:       check.GroupPlatform,
			Kind:             constants.PlatformDSC,
			Type:             check.CheckTypeReadiness,
			CheckID:          "platform.dsc.readiness",
			CheckName:        "Platform :: DSC :: Readiness Check",
			CheckDescription: "Validates that DataScienceCluster is in Ready state",
		},
	}
}

// CanApply returns true for all targets since DSC readiness is always relevant.
func (c DataScienceClusterReadinessCheck) CanApply(_ context.Context, _ check.Target) (bool, error) {
	return true, nil
}

// Validate executes the check against the provided target.
func (c DataScienceClusterReadinessCheck) Validate(ctx context.Context, target check.Target) (*result.DiagnosticResult, error) {
	return validate.DSC(c, target).Run(ctx, func(dr *result.DiagnosticResult, dsc *unstructured.Unstructured) error {
		return validateReadyCondition(dr, dsc, "DataScienceCluster", metav1.ConditionTrue)
	})
}
