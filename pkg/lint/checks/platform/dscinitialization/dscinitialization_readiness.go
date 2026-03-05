package dscinitialization

import (
	"context"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/opendatahub-io/odh-cli/pkg/constants"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/validate"
)

// DSCInitializationReadinessCheck validates that DSCInitialization is in Ready state before upgrading to RHOAI 3.x.
type DSCInitializationReadinessCheck struct {
	check.BaseCheck
}

func NewDSCInitializationReadinessCheck() *DSCInitializationReadinessCheck {
	return &DSCInitializationReadinessCheck{
		BaseCheck: check.BaseCheck{
			CheckGroup:       check.GroupPlatform,
			Kind:             constants.PlatformDSCI,
			Type:             check.CheckTypeReadiness,
			CheckID:          "platform.dsci.readiness",
			CheckName:        "Platform :: DSCI :: Readiness Check",
			CheckDescription: "Validates that DSCInitialization is in Ready state before upgrading to RHOAI 3.x",
		},
	}
}

func (c DSCInitializationReadinessCheck) CanApply(_ context.Context, _ check.Target) (bool, error) {
	return true, nil
}

func (c DSCInitializationReadinessCheck) Validate(ctx context.Context, target check.Target) (*result.DiagnosticResult, error) {
	return validate.DSCI(c, target).Run(ctx, func(dr *result.DiagnosticResult, dsci *unstructured.Unstructured) error {
		return validatePhaseReady(dr, dsci, "DSCInitialization")
	})
}
