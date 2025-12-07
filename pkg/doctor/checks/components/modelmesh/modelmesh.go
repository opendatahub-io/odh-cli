package modelmesh

import (
	"context"
	"fmt"

	"github.com/blang/semver/v4"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/lburgazzoli/odh-cli/pkg/doctor/check"
	"github.com/lburgazzoli/odh-cli/pkg/doctor/checks/shared/results"
	"github.com/lburgazzoli/odh-cli/pkg/util/jq"
)

const (
	checkID          = "components.modelmesh.removal"
	checkName        = "Components :: ModelMesh :: Removal (3.x)"
	checkDescription = "Validates that ModelMesh Serving is disabled before upgrading from RHOAI 2.x to 3.x (component will be removed)"
)

// RemovalCheck validates that ModelMesh Serving is disabled before upgrading to 3.x.
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

// Category returns the check category.
func (c *RemovalCheck) Category() check.CheckCategory {
	return check.CategoryComponent
}

// CanApply returns whether this check should run for the given versions.
// This check only applies when upgrading FROM 2.x TO 3.x.
func (c *RemovalCheck) CanApply(currentVersion *semver.Version, targetVersion *semver.Version) bool {
	// If no current version provided (lint mode), don't run this check
	if currentVersion == nil || targetVersion == nil {
		return false
	}

	// Only apply when upgrading FROM 2.x TO 3.x
	return currentVersion.Major == 2 && targetVersion.Major >= 3
}

// Validate executes the check against the provided target.
func (c *RemovalCheck) Validate(ctx context.Context, target *check.CheckTarget) (*check.DiagnosticResult, error) {
	// Get the DataScienceCluster singleton
	dsc, err := target.Client.GetDataScienceCluster(ctx)
	switch {
	case apierrors.IsNotFound(err):
		return results.DataScienceClusterNotFound(), nil
	case err != nil:
		return nil, fmt.Errorf("getting DataScienceCluster: %w", err)
	}

	// Query modelmeshserving component management state using JQ
	managementState, err := jq.Query(dsc, ".spec.components.modelmeshserving.managementState")
	if err != nil || managementState == nil {
		// ModelMesh component not defined in spec - check passes
		return &check.DiagnosticResult{
			Status:  check.StatusPass,
			Message: "ModelMesh Serving component is not configured in DataScienceCluster",
		}, nil
	}

	managementStateStr, ok := managementState.(string)
	if !ok {
		return nil, fmt.Errorf("managementState is not a string: %T", managementState)
	}

	// Check if modelmesh is enabled (Managed or Unmanaged)
	if managementStateStr == "Managed" || managementStateStr == "Unmanaged" {
		severity := check.SeverityCritical

		return &check.DiagnosticResult{
			Status:   check.StatusFail,
			Severity: &severity,
			Message: fmt.Sprintf(
				"ModelMesh Serving is still enabled (state: %s) but will be removed in RHOAI 3.x",
				managementStateStr,
			),
			Details: map[string]any{
				"managementState": managementStateStr,
				"component":       "modelmeshserving",
				"targetVersion":   target.Version.Version,
			},
		}, nil
	}

	// ModelMesh is disabled (Removed) - check passes
	return &check.DiagnosticResult{
		Status:  check.StatusPass,
		Message: fmt.Sprintf("ModelMesh Serving is disabled (state: %s) - ready for RHOAI 3.x upgrade", managementStateStr),
		Details: map[string]any{
			"managementState": managementStateStr,
		},
	}, nil
}

// Register the check in the global registry.
//
//nolint:gochecknoinits // Required for auto-registration pattern
func init() {
	check.MustRegisterCheck(&RemovalCheck{})
}
