package kserve

import (
	"context"
	"errors"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/lburgazzoli/odh-cli/pkg/lint/check"
	"github.com/lburgazzoli/odh-cli/pkg/lint/check/result"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/shared/base"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/shared/results"
	"github.com/lburgazzoli/odh-cli/pkg/util/jq"
	"github.com/lburgazzoli/odh-cli/pkg/util/version"
)

// ServerlessRemovalCheck validates that KServe serverless is disabled before upgrading to 3.x.
type ServerlessRemovalCheck struct {
	base.BaseCheck
}

func NewServerlessRemovalCheck() *ServerlessRemovalCheck {
	return &ServerlessRemovalCheck{
		BaseCheck: base.BaseCheck{
			CheckGroup:       check.GroupComponent,
			Kind:             check.ComponentKServe,
			CheckType:        check.CheckTypeServerlessRemoval,
			CheckID:          "components.kserve.serverless-removal",
			CheckName:        "Components :: KServe :: Serverless Removal (3.x)",
			CheckDescription: "Validates that KServe serverless mode is disabled before upgrading from RHOAI 2.x to 3.x (serverless support will be removed)",
		},
	}
}

// CanApply returns whether this check should run for the given target.
// This check only applies when upgrading FROM 2.x TO 3.x.
func (c *ServerlessRemovalCheck) CanApply(target check.Target) bool {
	return version.IsUpgradeFrom2xTo3x(target.CurrentVersion, target.TargetVersion)
}

// Validate executes the check against the provided target.
func (c *ServerlessRemovalCheck) Validate(ctx context.Context, target check.Target) (*result.DiagnosticResult, error) {
	dr := c.NewResult()

	// Get the DataScienceCluster singleton
	dsc, err := target.Client.GetDataScienceCluster(ctx)
	switch {
	case apierrors.IsNotFound(err):
		return results.DataScienceClusterNotFound(string(c.Group()), c.Kind, c.CheckType, c.Description()), nil
	case err != nil:
		return nil, fmt.Errorf("getting DataScienceCluster: %w", err)
	}

	// Query kserve component management state using JQ
	kserveStateStr, err := jq.Query[string](dsc, ".spec.components.kserve.managementState")
	if err != nil {
		if errors.Is(err, jq.ErrNotFound) {
			// KServe component not defined in spec - check passes
			results.SetComponentNotConfigured(dr, "KServe")

			return dr, nil
		}

		return nil, fmt.Errorf("querying kserve managementState: %w", err)
	}

	dr.Annotations[check.AnnotationComponentKServeState] = kserveStateStr

	// Only check serverless if KServe is Managed
	if kserveStateStr != check.ManagementStateManaged {
		// KServe not managed - serverless won't be enabled
		results.SetComponentNotManaged(dr, "KServe", kserveStateStr)

		return dr, nil
	}

	// Query serverless (serving) management state
	servingStateStr, err := jq.Query[string](dsc, ".spec.components.kserve.serving.managementState")
	if err != nil {
		if errors.Is(err, jq.ErrNotFound) {
			// Serverless not configured - check passes
			results.SetCompatibilitySuccessf(dr, "KServe serverless mode is not configured - ready for RHOAI 3.x upgrade")

			return dr, nil
		}

		return nil, fmt.Errorf("querying kserve serving managementState: %w", err)
	}

	dr.Annotations[check.AnnotationComponentServingState] = servingStateStr
	if target.TargetVersion != nil {
		dr.Annotations[check.AnnotationCheckTargetVersion] = target.TargetVersion.String()
	}

	// Check if serverless (serving) is enabled (Managed or Unmanaged)
	if servingStateStr == check.ManagementStateManaged || servingStateStr == check.ManagementStateUnmanaged {
		results.SetCompatibilityFailuref(dr, "KServe serverless mode is enabled (state: %s) but will be removed in RHOAI 3.x", servingStateStr)

		return dr, nil
	}

	// Serverless is disabled (Removed) - check passes
	results.SetCompatibilitySuccessf(dr, "KServe serverless mode is disabled (state: %s) - ready for RHOAI 3.x upgrade", servingStateStr)

	return dr, nil
}

// Register the check in the global registry.
//
//nolint:gochecknoinits // Required for auto-registration pattern
func init() {
	check.MustRegisterCheck(NewServerlessRemovalCheck())
}
