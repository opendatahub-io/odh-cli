package datasciencecluster

import (
	"github.com/opendatahub-io/odh-cli/pkg/migrate/action"
	"github.com/opendatahub-io/odh-cli/pkg/util/version"
)

const (
	actionID          = "dsc.storage-version.migrate"
	actionName        = "Migrate DataScienceCluster storage version"
	actionDescription = "Rewrite stored DataScienceClusters and remove obsolete CRD storage history"

	targetMajor = 3
	targetMinor = 6
)

// StorageVersionAction rewrites DSCs using the installed CRD's storage version.
type StorageVersionAction struct{}

func (a *StorageVersionAction) ID() string {
	return actionID
}

func (a *StorageVersionAction) Name() string {
	return actionName
}

func (a *StorageVersionAction) Description() string {
	return actionDescription
}

func (a *StorageVersionAction) Group() action.ActionGroup {
	return action.GroupMigration
}

func (a *StorageVersionAction) Phase() action.ActionPhase {
	return action.PhasePreUpgrade
}

func (a *StorageVersionAction) CanApply(target action.Target) bool {
	return version.IsVersionAtLeast(target.TargetVersion, targetMajor, targetMinor)
}

func (a *StorageVersionAction) Prepare() action.Task {
	return nil
}

func (a *StorageVersionAction) Run() action.Task {
	return &storageVersionTask{}
}
