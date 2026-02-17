package migrate

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/blang/semver/v4"
	"github.com/spf13/pflag"

	"k8s.io/cli-runtime/pkg/genericiooptions"

	"github.com/opendatahub-io/odh-cli/pkg/cmd"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/action"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/actions/kueue/rhbok"
	"github.com/opendatahub-io/odh-cli/pkg/util/version"
)

var _ cmd.Command = (*PrepareCommand)(nil)

type PrepareCommand struct {
	*SharedOptions

	DryRun        bool
	Yes           bool
	OutputDir     string
	MigrationIDs  []string
	TargetVersion string

	parsedTargetVersion *semver.Version

	// registry is the action registry for this command instance.
	// Explicitly populated to avoid global state and enable test isolation.
	registry *action.ActionRegistry
}

func NewPrepareCommand(streams genericiooptions.IOStreams) *PrepareCommand {
	shared := NewSharedOptions(streams)
	registry := action.NewActionRegistry()

	// Explicitly register all actions (no global state, full test isolation)
	registry.MustRegister(&rhbok.RHBOKMigrationAction{})

	return &PrepareCommand{
		SharedOptions: shared,
		registry:      registry,
	}
}

func (c *PrepareCommand) AddFlags(fs *pflag.FlagSet) {
	fs.BoolVarP(&c.Verbose, "verbose", "v", false, flagDescPrepareVerbose)
	fs.DurationVar(&c.Timeout, "timeout", c.Timeout, flagDescPrepareTimeout)
	fs.BoolVar(&c.DryRun, "dry-run", false, flagDescPrepareDryRun)
	fs.BoolVarP(&c.Yes, "yes", "y", false, flagDescPrepareYes)
	fs.StringVar(&c.OutputDir, "output-dir", "", flagDescPrepareOutputDir)
	fs.StringArrayVarP(&c.MigrationIDs, "migration", "m", []string{}, flagDescPrepareMigration)
	fs.StringVar(&c.TargetVersion, "target-version", "", flagDescPrepareTargetVersion)

	// Throttling settings
	fs.Float32Var(&c.QPS, "qps", c.QPS, "Kubernetes API QPS limit (queries per second)")
	fs.IntVar(&c.Burst, "burst", c.Burst, "Kubernetes API burst capacity")
}

func (c *PrepareCommand) Complete() error {
	if err := c.SharedOptions.Complete(); err != nil {
		return fmt.Errorf("completing shared options: %w", err)
	}

	// Always enable verbose for migrate prepare
	c.Verbose = true

	if c.TargetVersion != "" {
		// Use ParseTolerant to accept partial versions (e.g., "3.0" → "3.0.0")
		targetVer, err := semver.ParseTolerant(c.TargetVersion)
		if err != nil {
			return fmt.Errorf("invalid target version %q: %w", c.TargetVersion, err)
		}
		c.parsedTargetVersion = &targetVer
	}

	// Set default output directory if not specified
	if c.OutputDir == "" {
		timestamp := time.Now().Format("20060102-150405")
		c.OutputDir = filepath.Join(".", "backup-migrate-"+timestamp)
	}

	return nil
}

func (c *PrepareCommand) Validate() error {
	if err := c.SharedOptions.Validate(); err != nil {
		return fmt.Errorf("validating shared options: %w", err)
	}

	if len(c.MigrationIDs) == 0 {
		return errors.New("--migration flag is required")
	}

	if c.TargetVersion == "" {
		return errors.New("--target-version flag is required")
	}

	return nil
}

func (c *PrepareCommand) Run(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()

	currentVersion, err := version.Detect(ctx, c.Client)
	if err != nil {
		return fmt.Errorf("detecting cluster version: %w", err)
	}

	c.IO.Errorf("Current OpenShift AI version: %s", currentVersion.String())
	c.IO.Errorf("Target OpenShift AI version: %s", c.parsedTargetVersion.String())
	c.IO.Errorf("Backup directory: %s\n", c.OutputDir)

	for idx, migrationID := range c.MigrationIDs {
		if len(c.MigrationIDs) > 1 {
			c.IO.Errorf("\n=== Preparation %d/%d: %s ===\n", idx+1, len(c.MigrationIDs), migrationID)
		}

		selectedAction, ok := c.registry.Get(migrationID)
		if !ok {
			return fmt.Errorf("migration %q not found", migrationID)
		}

		prepareTask := selectedAction.Prepare()
		if prepareTask == nil {
			c.IO.Errorf("Migration %s has no prepare phase (skipped)\n", migrationID)

			continue
		}

		// Use verbose recorder for real-time streaming output
		recorder := action.NewVerboseRootRecorder(c.IO)
		c.IO.Errorf("\n%s:\n", migrationID)

		target := action.Target{
			Client:         c.Client,
			CurrentVersion: currentVersion,
			TargetVersion:  c.parsedTargetVersion,
			DryRun:         c.DryRun,
			SkipConfirm:    c.Yes,
			OutputDir:      c.OutputDir,
			Recorder:       recorder,
			IO:             c.IO,
		}

		if c.DryRun {
			c.IO.Errorf("DRY RUN MODE: No files will be written\n")
		}

		actionResult, err := prepareTask.Execute(ctx, target)
		if err != nil {
			return fmt.Errorf("preparation failed: %w", err)
		}

		// Output has already been streamed during execution
		c.IO.Fprintln()
		if !actionResult.Status.Completed {
			c.IO.Errorf("Preparation %s incomplete - please review the output above", migrationID)

			return fmt.Errorf("preparation halted: %s", migrationID)
		}
		c.IO.Errorf("Preparation %s completed successfully!", migrationID)
	}

	c.IO.Fprintln()
	if c.DryRun {
		c.IO.Errorf("Dry-run complete. Run without --dry-run to create backups.")
	} else {
		c.IO.Errorf("All preparations completed successfully!")
		c.IO.Errorf("Backups saved to: %s", c.OutputDir)
		c.IO.Errorf("\nRun 'migrate run' to execute the migration.")
	}

	return nil
}
