package migrate

import (
	"context"
	"fmt"

	"github.com/blang/semver/v4"
	"github.com/spf13/pflag"

	"k8s.io/cli-runtime/pkg/genericiooptions"

	"github.com/lburgazzoli/odh-cli/pkg/cmd"
	"github.com/lburgazzoli/odh-cli/pkg/lint/version"
	"github.com/lburgazzoli/odh-cli/pkg/migrate/action"
	"github.com/lburgazzoli/odh-cli/pkg/migrate/action/result"
	"github.com/lburgazzoli/odh-cli/pkg/util/iostreams"
)

var _ cmd.Command = (*Command)(nil)

type Command struct {
	*SharedOptions

	DryRun        bool
	Prepare       bool
	Yes           bool
	BackupPath    string
	MigrationID   string
	TargetVersion string

	parsedTargetVersion *semver.Version
}

func NewCommand(streams genericiooptions.IOStreams) *Command {
	shared := NewSharedOptions(streams)

	return &Command{
		SharedOptions: shared,
		BackupPath:    "./backups",
	}
}

func (c *Command) AddFlags(fs *pflag.FlagSet) {
	fs.StringVarP((*string)(&c.OutputFormat), "output", "o", string(OutputFormatTable),
		"Output format (table|json|yaml)")
	fs.BoolVarP(&c.Verbose, "verbose", "v", false,
		"Show detailed progress")
	fs.DurationVar(&c.Timeout, "timeout", c.Timeout,
		"Operation timeout (e.g., 10m, 30m)")

	fs.BoolVar(&c.DryRun, "dry-run", false,
		"Show what would be done without making changes")
	fs.BoolVar(&c.Prepare, "prepare", false,
		"Run pre-flight checks and backup resources (does not execute migration)")
	fs.BoolVarP(&c.Yes, "yes", "y", false,
		"Skip confirmation prompts")
	fs.StringVar(&c.BackupPath, "backup-path", c.BackupPath,
		"Path to store backup files (used with --prepare)")
	fs.StringVar(&c.MigrationID, "migration", "",
		"Migration ID to execute (e.g., kueue-to-rhbok)")
	fs.StringVar(&c.TargetVersion, "target-version", "",
		"Target version for migration (required)")
}

func (c *Command) Complete() error {
	if err := c.SharedOptions.Complete(); err != nil {
		return fmt.Errorf("completing shared options: %w", err)
	}

	if !c.Verbose {
		c.IO = iostreams.NewQuietWrapper(c.IO)
	}

	if c.TargetVersion != "" {
		targetVer, err := semver.Parse(c.TargetVersion)
		if err != nil {
			return fmt.Errorf("invalid target version %q: %w", c.TargetVersion, err)
		}
		c.parsedTargetVersion = &targetVer
	}

	return nil
}

func (c *Command) Validate() error {
	if err := c.SharedOptions.Validate(); err != nil {
		return fmt.Errorf("validating shared options: %w", err)
	}

	if c.MigrationID == "" {
		return fmt.Errorf("--migration flag is required")
	}

	if c.TargetVersion == "" {
		return fmt.Errorf("--target-version flag is required")
	}

	return nil
}

func (c *Command) Run(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()

	currentVersion, err := version.Detect(ctx, c.Client)
	if err != nil {
		return fmt.Errorf("detecting cluster version: %w", err)
	}

	targetVersionInfo := &version.ClusterVersion{
		Version:    c.TargetVersion,
		Source:     version.SourceManual,
		Confidence: version.ConfidenceHigh,
	}

	registry := action.GetGlobalRegistry()

	target := &action.ActionTarget{
		Client:         c.Client,
		CurrentVersion: currentVersion,
		TargetVersion:  targetVersionInfo,
		DryRun:         c.DryRun,
		BackupPath:     c.BackupPath,
		SkipConfirm:    c.Yes,
		IO:             c.IO,
	}

	if c.Prepare {
		return c.runPrepareMode(ctx, registry, target)
	}

	return c.runMigrationMode(ctx, registry, target)
}

func (c *Command) runPrepareMode(
	ctx context.Context,
	registry *action.ActionRegistry,
	target *action.ActionTarget,
) error {
	c.IO.Errorf("Running pre-flight checks for migration: %s\n", c.MigrationID)

	selectedAction, ok := registry.Get(c.MigrationID)
	if !ok {
		return fmt.Errorf("migration %q not found", c.MigrationID)
	}

	result, err := selectedAction.Validate(ctx, target)
	if err != nil {
		return fmt.Errorf("pre-flight validation failed: %w", err)
	}

	c.IO.Fprintln()
	c.IO.Fprintln("Pre-flight Validation:")
	for _, step := range result.Status.Steps {
		c.outputStep(step)
	}

	c.IO.Fprintln()
	c.IO.Errorf("Preparation complete. Run without --prepare to execute migration.")

	return nil
}

func (c *Command) runMigrationMode(
	ctx context.Context,
	registry *action.ActionRegistry,
	target *action.ActionTarget,
) error {
	c.IO.Errorf("Current OpenShift AI version: %s", target.CurrentVersion.Version)
	c.IO.Errorf("Target OpenShift AI version: %s\n", target.TargetVersion.Version)

	selectedAction, ok := registry.Get(c.MigrationID)
	if !ok {
		return fmt.Errorf("migration %q not found", c.MigrationID)
	}

	if target.DryRun {
		c.IO.Errorf("DRY RUN MODE: No changes will be made to the cluster\n")
	} else if target.SkipConfirm {
		c.IO.Errorf("Running migration: %s (confirmations skipped)\n", c.MigrationID)
	} else {
		c.IO.Errorf("Preparing migration: %s\n", c.MigrationID)
	}

	result, err := selectedAction.Execute(ctx, target)
	if err != nil {
		return fmt.Errorf("migration failed: %w", err)
	}

	c.IO.Fprintln()
	for i, step := range result.Status.Steps {
		c.IO.Errorf("[Step %d/%d] %s", i+1, len(result.Status.Steps), step.Description)
		c.outputStep(step)
	}

	c.IO.Fprintln()
	if result.Status.Completed {
		c.IO.Errorf("Migration completed successfully!")
	} else {
		c.IO.Errorf("Migration incomplete - please review the output above")
	}

	return nil
}

func (c *Command) outputStep(step result.ActionStep) {
	switch step.Status {
	case result.StepCompleted:
		c.IO.Errorf("✓ %s", step.Message)
	case result.StepFailed:
		c.IO.Errorf("✗ %s", step.Message)
	case result.StepSkipped:
		c.IO.Errorf("→ %s", step.Message)
	default:
		c.IO.Errorf("  %s", step.Message)
	}
}
