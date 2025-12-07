package kserve

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

// Category returns the check category.
func (c *ServerlessRemovalCheck) Category() check.CheckCategory {
	return check.CategoryComponent
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
func (c *ServerlessRemovalCheck) Validate(ctx context.Context, target *check.CheckTarget) (*check.DiagnosticResult, error) {
	// Get the DataScienceCluster singleton
	dsc, err := target.Client.GetDataScienceCluster(ctx)
	switch {
	case apierrors.IsNotFound(err):
		return results.DataScienceClusterNotFound(), nil
	case err != nil:
		return nil, fmt.Errorf("getting DataScienceCluster: %w", err)
	}

	// Query kserve component management state using JQ
	kserveState, err := jq.Query(dsc, ".spec.components.kserve.managementState")
	if err != nil || kserveState == nil {
		// KServe component not defined in spec - check passes
		return &check.DiagnosticResult{
			Status:  check.StatusPass,
			Message: "KServe component is not configured in DataScienceCluster",
		}, nil
	}

	kserveStateStr, ok := kserveState.(string)
	if !ok {
		return nil, fmt.Errorf("kserve managementState is not a string: %T", kserveState)
	}

	// Only check serverless if KServe is Managed
	if kserveStateStr != "Managed" {
		// KServe not managed - serverless won't be enabled
		return &check.DiagnosticResult{
			Status:  check.StatusPass,
			Message: fmt.Sprintf("KServe component is not managed (state: %s) - serverless not enabled", kserveStateStr),
			Details: map[string]any{
				"kserveManagementState": kserveStateStr,
			},
		}, nil
	}

	// Query serverless (serving) management state
	servingState, err := jq.Query(dsc, ".spec.components.kserve.serving.managementState")
	if err != nil || servingState == nil {
		// Serverless not configured - check passes
		return &check.DiagnosticResult{
			Status:  check.StatusPass,
			Message: "KServe serverless mode is not configured - ready for RHOAI 3.x upgrade",
			Details: map[string]any{
				"kserveManagementState": kserveStateStr,
			},
		}, nil
	}

	servingStateStr, ok := servingState.(string)
	if !ok {
		return nil, fmt.Errorf("kserve serving managementState is not a string: %T", servingState)
	}

	// Check if serverless (serving) is enabled (Managed or Unmanaged)
	if servingStateStr == "Managed" || servingStateStr == "Unmanaged" {
		severity := check.SeverityCritical

		return &check.DiagnosticResult{
			Status:   check.StatusFail,
			Severity: &severity,
			Message: fmt.Sprintf(
				"KServe serverless mode is enabled (state: %s) but will be removed in RHOAI 3.x",
				servingStateStr,
			),
			Details: map[string]any{
				"kserveManagementState":  kserveStateStr,
				"servingManagementState": servingStateStr,
				"component":              "kserve",
				"targetVersion":          target.Version.Version,
			},
		}, nil
	}

	// Serverless is disabled (Removed) - check passes
	return &check.DiagnosticResult{
		Status:  check.StatusPass,
		Message: fmt.Sprintf("KServe serverless mode is disabled (state: %s) - ready for RHOAI 3.x upgrade", servingStateStr),
		Details: map[string]any{
			"kserveManagementState":  kserveStateStr,
			"servingManagementState": servingStateStr,
		},
	}, nil
}

// Register the check in the global registry.
//
//nolint:gochecknoinits // Required for auto-registration pattern
func init() {
	check.MustRegisterCheck(&ServerlessRemovalCheck{})
}
