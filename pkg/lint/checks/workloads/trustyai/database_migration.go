package trustyai

import (
	"context"
	"fmt"

	"github.com/opendatahub-io/odh-cli/pkg/constants"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/validate"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
	"github.com/opendatahub-io/odh-cli/pkg/util/components"
	"github.com/opendatahub-io/odh-cli/pkg/util/version"
)

// DatabaseStorageMigrationCheck detects TrustyAIService CRs using DATABASE storage.
// The DB credentials secret referenced by spec.storage.databaseConfigurations must
// remain accessible in the target namespace after the CRD is updated to 3.x.
type DatabaseStorageMigrationCheck struct {
	check.BaseCheck
}

func NewDatabaseStorageMigrationCheck() *DatabaseStorageMigrationCheck {
	return &DatabaseStorageMigrationCheck{
		BaseCheck: check.BaseCheck{
			CheckGroup:       check.GroupWorkload,
			Kind:             kind,
			Type:             check.CheckTypeDataIntegrity,
			CheckID:          "workloads.trustyai.database-storage-migration",
			CheckName:        "Workloads :: TrustyAI :: Database Storage Migration (3.x)",
			CheckDescription: "Detects TrustyAIService CRs using database storage whose credentials secret must be verified before upgrading to RHOAI 3.x",
			CheckRemediation: "Ensure the secret named in spec.storage.databaseConfigurations exists in the same namespace and will remain accessible after the upgrade",
		},
	}
}

// CanApply returns true when upgrading from 2.x to 3.x and TrustyAI is Managed.
func (c *DatabaseStorageMigrationCheck) CanApply(ctx context.Context, target check.Target) (bool, error) {
	if !version.IsUpgradeFrom2xTo3x(target.CurrentVersion, target.TargetVersion) {
		return false, nil
	}

	dsc, err := client.GetDataScienceCluster(ctx, target.Client)
	if err != nil {
		return false, fmt.Errorf("getting DataScienceCluster: %w", err)
	}

	return components.HasManagementState(dsc, "trustyai", constants.ManagementStateManaged), nil
}

// Validate lists TrustyAIService CRs with DATABASE storage and warns if any are found.
func (c *DatabaseStorageMigrationCheck) Validate(
	ctx context.Context,
	target check.Target,
) (*result.DiagnosticResult, error) {
	return validate.Workloads(c, target, resources.TrustyAIService).
		Filter(hasDatabaseStorage).
		Complete(ctx, c.newDatabaseMigrationCondition)
}
