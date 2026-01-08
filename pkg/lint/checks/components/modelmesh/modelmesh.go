package modelmesh

import (
	"context"
	"fmt"

	"github.com/blang/semver/v4"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/lburgazzoli/odh-cli/pkg/lint/check"
	"github.com/lburgazzoli/odh-cli/pkg/lint/check/result"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/shared/base"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/shared/results"
	"github.com/lburgazzoli/odh-cli/pkg/util/jq"
)

// RemovalCheck validates that ModelMesh is disabled before upgrading to 3.x.
type RemovalCheck struct {
	base.BaseCheck
}

func NewRemovalCheck() *RemovalCheck {
	return &RemovalCheck{
		BaseCheck: base.BaseCheck{
			CheckGroup:       check.GroupComponent,
			Kind:             check.ComponentModelMesh,
			CheckType:        check.CheckTypeRemoval,
			CheckID:          "components.modelmesh.removal",
			CheckName:        "Components :: ModelMesh :: Removal (3.x)",
			CheckDescription: "Validates that ModelMesh is disabled before upgrading from RHOAI 2.x to 3.x (component will be removed)",
		},
	}
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
	dr := c.NewResult()

	// Get the DataScienceCluster singleton
	dsc, err := target.Client.GetDataScienceCluster(ctx)
	switch {
	case apierrors.IsNotFound(err):
		return results.DataScienceClusterNotFound(string(c.Group()), c.Kind, c.CheckType, c.Description()), nil
	case err != nil:
		return nil, fmt.Errorf("getting DataScienceCluster: %w", err)
	}

	// Query modelmeshserving component management state using JQ
	managementStateStr, err := jq.Query[string](dsc, ".spec.components.modelmeshserving.managementState")
	if err != nil {
		return nil, fmt.Errorf("querying modelmeshserving managementState: %w", err)
	}

	if managementStateStr == "" {
		// ModelMesh component not defined in spec - check passes
		results.SetComponentNotConfigured(dr, "ModelMesh")

		return dr, nil
	}

	// Add management state as annotation
	dr.Annotations[check.AnnotationComponentManagementState] = managementStateStr
	if target.Version != nil {
		dr.Annotations[check.AnnotationCheckTargetVersion] = target.Version.Version
	}

	// Check if modelmesh is enabled (Managed or Unmanaged)
	if managementStateStr == check.ManagementStateManaged || managementStateStr == check.ManagementStateUnmanaged {
		results.SetCompatibilityFailuref(dr, "ModelMesh is enabled (state: %s) but will be removed in RHOAI 3.x", managementStateStr)

		return dr, nil
	}

	// ModelMesh is disabled (Removed) - check passes
	results.SetCompatibilitySuccessf(dr, "ModelMesh is disabled (state: %s) - ready for RHOAI 3.x upgrade", managementStateStr)

	return dr, nil
}

// Register the check in the global registry.
//
//nolint:gochecknoinits // Required for auto-registration pattern
func init() {
	check.MustRegisterCheck(NewRemovalCheck())
}
