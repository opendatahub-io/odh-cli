package acceleratorprofile

import (
	"context"
	"fmt"
	"strconv"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/lburgazzoli/odh-cli/pkg/lint/check"
	"github.com/lburgazzoli/odh-cli/pkg/lint/check/result"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/shared/base"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/shared/results"
	"github.com/lburgazzoli/odh-cli/pkg/resources"
	"github.com/lburgazzoli/odh-cli/pkg/util/client"
	"github.com/lburgazzoli/odh-cli/pkg/util/version"
)

const (
	// ConditionTypeMigrationRequired indicates AcceleratorProfiles require migration.
	ConditionTypeMigrationRequired = "MigrationRequired"

	// ReasonMigrationPending indicates AcceleratorProfiles will be auto-migrated.
	ReasonMigrationPending = "MigrationPending"

	// ReasonNoMigrationRequired indicates no AcceleratorProfiles exist.
	ReasonNoMigrationRequired = "NoMigrationRequired"

	// minMigrationMajorVersion is the minimum major version for this check to apply.
	minMigrationMajorVersion = 3
)

// MigrationCheck detects AcceleratorProfiles that will be auto-migrated to HardwareProfiles
// during upgrade to RHOAI 3.x.
type MigrationCheck struct {
	base.BaseCheck
}

// NewMigrationCheck creates a new MigrationCheck instance.
func NewMigrationCheck() *MigrationCheck {
	return &MigrationCheck{
		BaseCheck: base.BaseCheck{
			CheckGroup:       check.GroupConfigurations,
			Kind:             check.ConfigurationAcceleratorProfile,
			CheckType:        check.CheckTypeMigration,
			CheckID:          "configuration.acceleratorprofile.migration",
			CheckName:        "Configuration :: AcceleratorProfile :: Migration (3.x)",
			CheckDescription: "Lists AcceleratorProfiles that will be auto-migrated to HardwareProfiles during upgrade",
			CheckRemediation: "AcceleratorProfiles will be automatically migrated to HardwareProfiles during upgrade - no manual action required",
		},
	}
}

// CanApply returns whether this check should run for the given target.
// Only applies when upgrading to 3.x or later.
func (c *MigrationCheck) CanApply(target check.Target) bool {
	return version.IsVersionAtLeast(target.TargetVersion, minMigrationMajorVersion, 0)
}

// Validate executes the check against the provided target.
func (c *MigrationCheck) Validate(
	ctx context.Context,
	target check.Target,
) (*result.DiagnosticResult, error) {
	dr := c.NewResult()

	if target.TargetVersion != nil {
		dr.Annotations[check.AnnotationCheckTargetVersion] = target.TargetVersion.String()
	}

	acceleratorProfiles, err := c.listAcceleratorProfiles(ctx, target)
	if err != nil {
		return nil, err
	}

	totalCount := len(acceleratorProfiles)
	dr.Annotations[check.AnnotationImpactedWorkloadCount] = strconv.Itoa(totalCount)

	// Add condition based on findings.
	if totalCount == 0 {
		results.SetCondition(dr, check.NewCondition(
			ConditionTypeMigrationRequired,
			metav1.ConditionFalse,
			ReasonNoMigrationRequired,
			"No AcceleratorProfiles found - no migration required",
		))

		return dr, nil
	}

	// AcceleratorProfiles found - advisory notice about auto-migration.
	results.SetCondition(dr, check.NewCondition(
		ConditionTypeMigrationRequired,
		metav1.ConditionTrue,
		ReasonMigrationPending,
		"Found %d AcceleratorProfile(s) that will be automatically migrated to HardwareProfiles during upgrade",
		totalCount,
		check.WithImpact(result.ImpactAdvisory),
	))

	// Populate ImpactedObjects.
	results.PopulateImpactedObjects(dr, resources.AcceleratorProfile, acceleratorProfiles)

	return dr, nil
}

// listAcceleratorProfiles retrieves all AcceleratorProfiles in the cluster.
func (c *MigrationCheck) listAcceleratorProfiles(
	ctx context.Context,
	target check.Target,
) ([]types.NamespacedName, error) {
	// Use ListMetadata since we only need namespace/name.
	profiles, err := target.Client.ListMetadata(ctx, resources.AcceleratorProfile)
	if err != nil {
		if client.IsResourceTypeNotFound(err) {
			// AcceleratorProfile CRD doesn't exist - nothing to migrate.
			return nil, nil
		}

		return nil, fmt.Errorf("listing AcceleratorProfiles: %w", err)
	}

	profileNames := make([]types.NamespacedName, 0, len(profiles))

	for _, profile := range profiles {
		profileNames = append(profileNames, types.NamespacedName{
			Namespace: profile.GetNamespace(),
			Name:      profile.GetName(),
		})
	}

	return profileNames, nil
}
