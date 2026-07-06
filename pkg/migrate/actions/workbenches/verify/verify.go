package verify

import (
	"fmt"

	"github.com/spf13/pflag"

	"github.com/opendatahub-io/odh-cli/pkg/migrate/action"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/actions/workbenches"
)

const (
	actionID          = "workbenches.verify-migration"
	actionName        = "Verify workbench migration status"
	actionDescription = "Verifies migration and cleanup status of workbench notebooks " +
		"and classifies them by migration state (LEGACY, MIGRATED, UNRECONCILED, INVALID, UNKNOWN)"

	minTargetMajorVersion = 3

	phaseMigration = "migration"
	phaseCleanup   = "cleanup"
	phaseAll       = "all"
)

// VerifyMigrationAction implements a read-only validation action that assesses
// workbench migration status and classifies notebooks by migration state.
type VerifyMigrationAction struct {
	Scope       *workbenches.SharedScopeOptions
	VerifyPhase string
}

func (a *VerifyMigrationAction) ID() string          { return actionID }
func (a *VerifyMigrationAction) Name() string        { return actionName }
func (a *VerifyMigrationAction) Description() string { return actionDescription }

func (a *VerifyMigrationAction) Group() action.ActionGroup {
	return action.GroupValidation
}

func (a *VerifyMigrationAction) Phase() action.ActionPhase {
	return action.PhasePostUpgrade
}

func (a *VerifyMigrationAction) AddFlags(fs *pflag.FlagSet) {
	workbenches.AddScopeFlags(a.Scope, fs)

	if fs.Lookup("verify-phase") == nil {
		fs.StringVar(&a.VerifyPhase, "verify-phase", phaseMigration,
			"Verification phase: migration, cleanup, or all (default: migration)")
	}
}

func (a *VerifyMigrationAction) CanApply(target action.Target) bool {
	if target.TargetVersion == nil {
		return false
	}

	return target.TargetVersion.Major >= minTargetMajorVersion
}

func (a *VerifyMigrationAction) Prepare() action.Task {
	return nil
}

func (a *VerifyMigrationAction) Run() action.Task {
	return &runTask{action: a}
}

func (a *VerifyMigrationAction) validatePhase() error {
	switch a.VerifyPhase {
	case phaseMigration, phaseCleanup, phaseAll, "":
		return nil
	default:
		return fmt.Errorf("invalid --verify-phase %q: must be one of: migration, cleanup, all", a.VerifyPhase)
	}
}

func (a *VerifyMigrationAction) effectivePhase() string {
	if a.VerifyPhase == "" {
		return phaseMigration
	}

	return a.VerifyPhase
}

func (a *VerifyMigrationAction) includeMigration() bool {
	p := a.effectivePhase()

	return p == phaseMigration || p == phaseAll
}

func (a *VerifyMigrationAction) includeCleanup() bool {
	p := a.effectivePhase()

	return p == phaseCleanup || p == phaseAll
}
