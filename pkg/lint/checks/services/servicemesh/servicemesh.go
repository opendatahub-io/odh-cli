package servicemesh

import (
	"context"
	"errors"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/lburgazzoli/odh-cli/pkg/lint/check"
	"github.com/lburgazzoli/odh-cli/pkg/lint/check/result"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/shared/results"
	"github.com/lburgazzoli/odh-cli/pkg/util/jq"
	"github.com/lburgazzoli/odh-cli/pkg/util/version"
)

const (
	checkID          = "services.servicemesh.removal"
	checkName        = "Services :: ServiceMesh :: Removal (3.x)"
	checkDescription = "Validates that ServiceMesh is disabled before upgrading from RHOAI 2.x to 3.x (service mesh will be removed)"
)

// RemovalCheck validates that ServiceMesh is disabled before upgrading to 3.x.
type RemovalCheck struct{}

// ID returns the unique identifier for this check.
func (c *RemovalCheck) ID() string {
	return checkID
}

// Name returns the human-readable check name.
func (c *RemovalCheck) Name() string {
	return checkName
}

// Description returns what this check validates.
func (c *RemovalCheck) Description() string {
	return checkDescription
}

// Group returns the check group.
func (c *RemovalCheck) Group() check.CheckGroup {
	return check.GroupService
}

// CanApply returns whether this check should run for the given target.
// This check only applies when upgrading FROM 2.x TO 3.x.
func (c *RemovalCheck) CanApply(target check.Target) bool {
	return version.IsUpgradeFrom2xTo3x(target.CurrentVersion, target.TargetVersion)
}

// Validate executes the check against the provided target.
func (c *RemovalCheck) Validate(ctx context.Context, target check.Target) (*result.DiagnosticResult, error) {
	dr := result.New(
		string(check.GroupService),
		check.ServiceServiceMesh,
		check.CheckTypeRemoval,
		checkDescription,
	)

	// Get the DSCInitialization singleton
	dsci, err := target.Client.GetDSCInitialization(ctx)
	switch {
	case apierrors.IsNotFound(err):
		return results.DSCInitializationNotFound(string(check.GroupService), check.ServiceServiceMesh, check.CheckTypeRemoval, checkDescription), nil
	case err != nil:
		return nil, fmt.Errorf("getting DSCInitialization: %w", err)
	}

	// Query servicemesh management state using JQ
	managementStateStr, err := jq.Query[string](dsci, ".spec.serviceMesh.managementState")
	if err != nil {
		if errors.Is(err, jq.ErrNotFound) {
			// ServiceMesh not defined in spec - check passes
			results.SetServiceNotConfigured(dr, "ServiceMesh")

			return dr, nil
		}

		return nil, fmt.Errorf("querying servicemesh managementState: %w", err)
	}

	// Add management state as annotation
	dr.Annotations[check.AnnotationServiceManagementState] = managementStateStr

	// Check if servicemesh is enabled (Managed or Unmanaged)
	if managementStateStr == check.ManagementStateManaged || managementStateStr == check.ManagementStateUnmanaged {
		results.SetCompatibilityFailuref(dr, "ServiceMesh is enabled (state: %s) but will be removed in RHOAI 3.x", managementStateStr)

		return dr, nil
	}

	// ServiceMesh is disabled (Removed) - check passes
	results.SetCompatibilitySuccessf(dr, "ServiceMesh is disabled (state: %s) - ready for RHOAI 3.x upgrade", managementStateStr)

	return dr, nil
}

// Register the check in the global registry.
//
//nolint:gochecknoinits // Required for auto-registration pattern
func init() {
	check.MustRegisterCheck(&RemovalCheck{})
}
