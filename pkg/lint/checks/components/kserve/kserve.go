package kserve

import (
	"context"
	"fmt"

	"github.com/blang/semver/v4"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/lburgazzoli/odh-cli/pkg/lint/check"
	"github.com/lburgazzoli/odh-cli/pkg/lint/check/result"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/shared/results"
	"github.com/lburgazzoli/odh-cli/pkg/util/jq"
)

const (
	checkID          = "components.kserve.serverless-removal"
	checkName        = "Components :: KServe :: Serverless Removal (3.x)"
	checkDescription = "Validates that KServe serverless mode is disabled before upgrading from RHOAI 2.x to 3.x (serverless support will be removed)"
)

// ServerlessRemovalCheck validates that KServe serverless is disabled before upgrading to 3.x.
type ServerlessRemovalCheck struct{}

// ID returns the unique identifier for this check.
func (c *ServerlessRemovalCheck) ID() string {
	return checkID
}

// Name returns the human-readable check name.
func (c *ServerlessRemovalCheck) Name() string {
	return checkName
}

// Description returns what this check validates.
func (c *ServerlessRemovalCheck) Description() string {
	return checkDescription
}

// Group returns the check group.
func (c *ServerlessRemovalCheck) Group() check.CheckGroup {
	return check.GroupComponent
}

// CanApply returns whether this check should run for the given versions.
// This check only applies when upgrading FROM 2.x TO 3.x.
func (c *ServerlessRemovalCheck) CanApply(currentVersion *semver.Version, targetVersion *semver.Version) bool {
	// If no current version provided (lint mode), don't run this check
	if currentVersion == nil || targetVersion == nil {
		return false
	}

	// Only apply when upgrading FROM 2.x TO 3.x
	return currentVersion.Major == 2 && targetVersion.Major >= 3
}

// Validate executes the check against the provided target.
func (c *ServerlessRemovalCheck) Validate(ctx context.Context, target *check.CheckTarget) (*result.DiagnosticResult, error) {
	dr := result.New(
		string(check.GroupComponent),
		check.ComponentKServe,
		check.CheckTypeServerlessRemoval,
		checkDescription,
	)

	// Get the DataScienceCluster singleton
	dsc, err := target.Client.GetDataScienceCluster(ctx)
	switch {
	case apierrors.IsNotFound(err):
		return results.DataScienceClusterNotFound(string(check.GroupComponent), check.ComponentKServe, check.CheckTypeServerlessRemoval, checkDescription), nil
	case err != nil:
		return nil, fmt.Errorf("getting DataScienceCluster: %w", err)
	}

	// Query kserve component management state using JQ
	kserveStateStr, err := jq.Query[string](dsc, ".spec.components.kserve.managementState")
	if err != nil {
		return nil, fmt.Errorf("querying kserve managementState: %w", err)
	}

	if kserveStateStr == "" {
		// KServe component not defined in spec - check passes
		results.SetComponentNotConfigured(dr, "KServe")

		return dr, nil
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
		return nil, fmt.Errorf("querying kserve serving managementState: %w", err)
	}

	if servingStateStr == "" {
		// Serverless not configured - check passes
		results.SetCompatibilitySuccessf(dr, "KServe serverless mode is not configured - ready for RHOAI 3.x upgrade")

		return dr, nil
	}

	dr.Annotations[check.AnnotationComponentServingState] = servingStateStr
	if target.Version != nil {
		dr.Annotations[check.AnnotationCheckTargetVersion] = target.Version.Version
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
	check.MustRegisterCheck(&ServerlessRemovalCheck{})
}
