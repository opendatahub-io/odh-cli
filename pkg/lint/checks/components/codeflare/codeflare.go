package codeflare

import (
	"context"
	"fmt"

	"github.com/blang/semver/v4"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/lburgazzoli/odh-cli/pkg/lint/check"
	"github.com/lburgazzoli/odh-cli/pkg/lint/check/result"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/shared/results"
	"github.com/lburgazzoli/odh-cli/pkg/util/jq"
)

const (
	checkID          = "components.codeflare.removal"
	checkName        = "Components :: CodeFlare :: Removal (3.x)"
	checkDescription = "Validates that CodeFlare is disabled before upgrading from RHOAI 2.x to 3.x (component will be removed)"
)

// RemovalCheck validates that CodeFlare is disabled before upgrading to 3.x.
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
	return check.GroupComponent
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
func (c *RemovalCheck) Validate(ctx context.Context, target *check.CheckTarget) (*result.DiagnosticResult, error) {
	dr := result.New(
		string(check.GroupComponent),
		"codeflare",
		"removal",
		checkDescription,
	)

	// Get the DataScienceCluster singleton
	dsc, err := target.Client.GetDataScienceCluster(ctx)
	switch {
	case apierrors.IsNotFound(err):
		return results.DataScienceClusterNotFound(string(check.GroupComponent), "codeflare", "removal", checkDescription), nil
	case err != nil:
		return nil, fmt.Errorf("getting DataScienceCluster: %w", err)
	}

	// Query codeflare component management state using JQ
	managementState, err := jq.Query(dsc, ".spec.components.codeflare.managementState")
	if err != nil || managementState == nil {
		// CodeFlare component not defined in spec - check passes
		dr.Status.Conditions = []metav1.Condition{
			check.NewCondition(
				check.ConditionTypeConfigured,
				metav1.ConditionFalse,
				check.ReasonResourceNotFound,
				"CodeFlare component is not configured in DataScienceCluster",
			),
		}

		return dr, nil
	}

	managementStateStr, ok := managementState.(string)
	if !ok {
		return nil, fmt.Errorf("managementState is not a string: %T", managementState)
	}

	// Add management state as annotation
	dr.Metadata.Annotations["component.opendatahub.io/management-state"] = managementStateStr
	if target.Version != nil {
		dr.Metadata.Annotations["check.opendatahub.io/target-version"] = target.Version.Version
	}

	// Check if codeflare is enabled (Managed or Unmanaged)
	if managementStateStr == "Managed" || managementStateStr == "Unmanaged" {
		dr.Status.Conditions = []metav1.Condition{
			check.NewCondition(
				check.ConditionTypeCompatible,
				metav1.ConditionFalse,
				check.ReasonVersionIncompatible,
				fmt.Sprintf("CodeFlare is enabled (state: %s) but will be removed in RHOAI 3.x", managementStateStr),
			),
		}

		return dr, nil
	}

	// CodeFlare is disabled (Removed) - check passes
	dr.Status.Conditions = []metav1.Condition{
		check.NewCondition(
			check.ConditionTypeCompatible,
			metav1.ConditionTrue,
			check.ReasonVersionCompatible,
			fmt.Sprintf("CodeFlare is disabled (state: %s) - ready for RHOAI 3.x upgrade", managementStateStr),
		),
	}

	return dr, nil
}

// Register the check in the global registry.
//
//nolint:gochecknoinits // Required for auto-registration pattern
func init() {
	check.MustRegisterCheck(&RemovalCheck{})
}
