package action

import (
	"context"

	"github.com/blang/semver/v4"

	"github.com/opendatahub-io/odh-cli/pkg/migrate/action/result"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
	"github.com/opendatahub-io/odh-cli/pkg/util/iostreams"
)

type ActionGroup string

const (
	GroupMigration  ActionGroup = "migration"
	GroupBackup     ActionGroup = "backup"
	GroupValidation ActionGroup = "validation"
)

// Task represents a single executable phase (prepare or run) with validation and execution.
type Task interface {
	Validate(ctx context.Context, target Target) (*result.ActionResult, error)
	Execute(ctx context.Context, target Target) (*result.ActionResult, error)
}

type Action interface {
	ID() string
	Name() string
	Description() string
	Group() ActionGroup

	// CanApply returns whether this action should run for the given target context.
	// Actions can use target.CurrentVersion, target.TargetVersion, or target.Client for filtering.
	CanApply(target Target) bool

	// Prepare returns the Task for the preparation phase (e.g., backups, pre-migration setup).
	// Returns nil if this action has no prepare phase.
	Prepare() Task

	// Run returns the Task for the migration execution phase.
	Run() Task
}

// Target holds all context needed for executing migration actions.
type Target struct {
	Client         client.Client
	CurrentVersion *semver.Version // Version being migrated FROM
	TargetVersion  *semver.Version // Version being migrated TO
	DryRun         bool
	SkipConfirm    bool
	OutputDir      string // Output directory for backups (used in prepare phase)
	Recorder       StepRecorder
	IO             iostreams.Interface
}
