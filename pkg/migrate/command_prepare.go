package migrate

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/blang/semver/v4"
	"github.com/spf13/pflag"

	"k8s.io/cli-runtime/pkg/genericiooptions"

	"github.com/opendatahub-io/odh-cli/pkg/api"
	"github.com/opendatahub-io/odh-cli/pkg/cmd"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/action"
	"github.com/opendatahub-io/odh-cli/pkg/output"
	clierrors "github.com/opendatahub-io/odh-cli/pkg/util/errors"
	"github.com/opendatahub-io/odh-cli/pkg/util/iostreams"
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
	Phase         string

	parsedTargetVersion *semver.Version
	parsedPhase         action.ActionPhase

	// registry is the action registry for this command instance.
	// Explicitly populated to avoid global state and enable test isolation.
	registry *action.ActionRegistry
}

func NewPrepareCommand(streams genericiooptions.IOStreams) *PrepareCommand {
	shared := NewSharedOptions(streams)

	return &PrepareCommand{
		SharedOptions: shared,
		registry:      newDefaultRegistry(),
	}
}

func (c *PrepareCommand) ActionIDs() []string {
	return c.registry.ActionIDs()
}

func (c *PrepareCommand) AddFlags(fs *pflag.FlagSet) {
	fs.StringVarP((*string)(&c.OutputFormat), "output", "o", string(OutputFormatTable), "Output format: table, json, yaml")
	_ = fs.SetAnnotation("output", api.AnnotationValidValues, []string{"table", "json", "yaml"})
	fs.BoolVarP(&c.Verbose, "verbose", "v", false, flagDescPrepareVerbose)
	fs.DurationVar(&c.Timeout, "timeout", c.Timeout, flagDescPrepareTimeout)
	fs.BoolVar(&c.DryRun, "dry-run", false, flagDescPrepareDryRun)
	fs.BoolVarP(&c.Yes, "yes", "y", false, flagDescPrepareYes)
	fs.StringVar(&c.OutputDir, "output-dir", "", flagDescPrepareOutputDir)
	fs.StringArrayVarP(&c.MigrationIDs, "migration", "m", []string{}, flagDescPrepareMigration)
	fs.StringVar(&c.TargetVersion, "target-version", "", flagDescPrepareTargetVersion)
	fs.StringVar(&c.Phase, "phase", "", flagDescPreparePhase)
	// Empty string is intentionally excluded: it means "flag not provided" (the default), not a user-selectable value.
	_ = fs.SetAnnotation("phase", api.AnnotationValidValues, []string{"pre-upgrade", "post-upgrade", "pre-enablement"})

	// Throttling settings
	fs.Float32Var(&c.QPS, "qps", c.QPS, "Kubernetes API QPS limit (queries per second)")
	fs.IntVar(&c.Burst, "burst", c.Burst, "Kubernetes API burst capacity")

	// Let actions register their own flags
	action.RegisterActionFlags(c.registry, fs)
}

func (c *PrepareCommand) Complete() error {
	if err := c.SharedOptions.Complete(); err != nil {
		return fmt.Errorf("completing shared options: %w", err)
	}

	// Suppress text output for structured formats (matching codebase pattern)
	if c.OutputFormat != OutputFormatTable {
		c.IO = iostreams.NewQuietWrapper(c.IO)
	}

	// Always enable verbose for migrate prepare
	c.Verbose = true

	c.parsedPhase = ""
	if c.Phase != "" {
		c.parsedPhase = action.ActionPhase(c.Phase)
	}

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

	if len(c.MigrationIDs) == 0 && c.Phase == "" {
		return errors.New("--migration flag is required (or use --phase to prepare all actions for a lifecycle phase)")
	}

	if c.TargetVersion == "" {
		return errors.New("--target-version flag is required")
	}

	if err := c.parsedPhase.Validate(); err != nil {
		return fmt.Errorf("validating phase: %w", err)
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

	effectivePhase, resolvedIDs, err := resolvePhaseAndMigrations(phaseResolverInput{
		ParsedPhase:    c.parsedPhase,
		MigrationIDs:   c.MigrationIDs,
		CurrentVersion: currentVersion,
		TargetVersion:  c.parsedTargetVersion,
		Registry:       c.registry,
		Client:         c.Client,
		IO:             c.IO,
	})
	if err != nil {
		return err
	}

	c.MigrationIDs = resolvedIDs

	if len(c.MigrationIDs) == 0 {
		//nolint:wrapcheck // NewExitCodeError is a same-module constructor
		return clierrors.NewExitCodeError(clierrors.ExitValidation,
			fmt.Errorf("no applicable migrations found for phase %s", string(effectivePhase)))
	}

	return c.runPrepareMode(ctx, currentVersion, c.parsedTargetVersion, effectivePhase)
}

func (c *PrepareCommand) runPrepareMode(
	ctx context.Context,
	currentVersion *semver.Version,
	targetVersion *semver.Version,
	effectivePhase action.ActionPhase,
) error {
	structured := c.OutputFormat != OutputFormatTable

	c.IO.Errorf("Current OpenShift AI version: %s", currentVersion.String())
	c.IO.Errorf("Target OpenShift AI version: %s", targetVersion.String())
	c.IO.Errorf("Phase: %s", string(effectivePhase))
	c.IO.Errorf("Backup directory: %s\n", c.OutputDir)

	// Use FullQuietWrapper for action IO to prevent stdout corruption in structured mode
	actionIO := actionIOForMode(c.IO, structured)

	var migrationResults []MigrationResultItem

	for idx, migrationID := range c.MigrationIDs {
		if len(c.MigrationIDs) > 1 {
			c.IO.Errorf("\n=== Preparation %d/%d: %s ===\n", idx+1, len(c.MigrationIDs), migrationID)
		}

		selectedAction, ok := c.registry.Get(migrationID)
		if !ok {
			//nolint:wrapcheck // NewExitCodeError is a same-module constructor
			return clierrors.NewExitCodeError(clierrors.ExitValidation,
				fmt.Errorf("migration %q not found", migrationID))
		}

		var phaseMismatch bool
		if selectedAction.Phase() != effectivePhase {
			phaseMismatch = true
			c.IO.Errorf("WARNING: migration %s has phase %s but effective phase is %s; proceeding because --migration was explicit",
				migrationID, string(selectedAction.Phase()), string(effectivePhase))
		}

		prepareTask := selectedAction.Prepare()
		if prepareTask == nil {
			c.IO.Errorf("Migration %s has no prepare phase (skipped)\n", migrationID)
			migrationResults = append(migrationResults, MigrationResultItem{
				ID:            migrationID,
				Skipped:       true,
				PhaseMismatch: phaseMismatch,
			})

			continue
		}

		recorder := action.NewVerboseRootRecorder(actionIO)
		c.IO.Errorf("\n%s:\n", migrationID)

		target := action.Target{
			Client:         c.Client,
			RESTConfig:     c.RESTConfig,
			CurrentVersion: currentVersion,
			TargetVersion:  targetVersion,
			DryRun:         c.DryRun,
			SkipConfirm:    c.Yes,
			OutputDir:      c.OutputDir,
			Recorder:       recorder,
			IO:             actionIO,
		}

		if c.DryRun {
			c.IO.Errorf("DRY RUN MODE: No files will be written\n")
		}

		actionResult, err := prepareTask.Execute(ctx, target)
		if err != nil {
			//nolint:wrapcheck // NewExitCodeError is a same-module constructor
			return clierrors.NewExitCodeError(clierrors.ExitValidation,
				fmt.Errorf("preparation failed: %w", err))
		}

		c.IO.Errorln()

		if !actionResult.Status.Completed {
			c.IO.Errorf("Preparation %s incomplete - please review the output above", migrationID)

			//nolint:wrapcheck // NewExitCodeError is a same-module constructor
			return clierrors.NewExitCodeError(clierrors.ExitValidation,
				fmt.Errorf("preparation halted: %s", migrationID))
		}

		c.IO.Errorf("Preparation %s completed successfully!", migrationID)
		migrationResults = append(migrationResults, MigrationResultItem{
			ID:              migrationID,
			Completed:       actionResult.Status.Completed,
			HasSkippedSteps: actionResult.HasSkippedSteps(),
			PhaseMismatch:   phaseMismatch,
		})
	}

	if structured {
		return c.writePrepareResult(currentVersion, targetVersion, effectivePhase, migrationResults)
	}

	c.IO.Fprintln()
	if c.DryRun {
		c.IO.Errorf("Dry-run complete. Run without --dry-run to create backups.")
	} else {
		c.IO.Errorf("All preparations completed successfully!")

		entries, err := os.ReadDir(c.OutputDir)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			c.IO.Errorf("Warning: could not read backup directory: %v", err)
		} else if len(entries) > 0 {
			c.IO.Errorf("Backups saved to: %s", c.OutputDir)
		} else {
			c.IO.Errorf("No backups were created — all backup steps were skipped (see output above for details).")
		}

		c.IO.Errorf("\nRun 'migrate run' to execute the migration.")
	}

	return nil
}

// PrepareResult is the structured success output for migrate prepare.
type PrepareResult struct {
	output.Envelope `json:",inline" yaml:",inline"`

	CurrentVersion string                `json:"currentVersion" yaml:"currentVersion"`
	TargetVersion  string                `json:"targetVersion"  yaml:"targetVersion"`
	Phase          string                `json:"phase"          yaml:"phase"`
	DryRun         bool                  `json:"dryRun"         yaml:"dryRun"`
	OutputDir      string                `json:"outputDir"      yaml:"outputDir"`
	Migrations     []MigrationResultItem `json:"migrations"     yaml:"migrations"`
}

// MigrationResultItem represents the result of a single migration in structured output.
type MigrationResultItem struct {
	ID              string `json:"id"                        yaml:"id"`
	Completed       bool   `json:"completed"                 yaml:"completed"`
	Skipped         bool   `json:"skipped,omitempty"         yaml:"skipped,omitempty"`
	HasSkippedSteps bool   `json:"hasSkippedSteps,omitempty" yaml:"hasSkippedSteps,omitempty"`
	HasFailedSteps  bool   `json:"hasFailedSteps,omitempty"  yaml:"hasFailedSteps,omitempty"`
	PhaseMismatch   bool   `json:"phaseMismatch,omitempty"   yaml:"phaseMismatch,omitempty"`
}

func countWarnings(migrations []MigrationResultItem) int {
	warnings := 0

	for _, m := range migrations {
		if m.Skipped || m.HasSkippedSteps || m.PhaseMismatch {
			warnings++
		}
	}

	return warnings
}

func countErrors(migrations []MigrationResultItem) int {
	errs := 0

	for _, m := range migrations {
		if m.HasFailedSteps {
			errs++
		}
	}

	return errs
}

func (c *PrepareCommand) writePrepareResult(
	currentVersion *semver.Version,
	targetVersion *semver.Version,
	phase action.ActionPhase,
	migrations []MigrationResultItem,
) error {
	result := PrepareResult{
		Envelope:       output.NewEnvelope("MigratePrepareResult", "migrate prepare"),
		CurrentVersion: currentVersion.String(),
		TargetVersion:  targetVersion.String(),
		Phase:          string(phase),
		DryRun:         c.DryRun,
		OutputDir:      c.OutputDir,
		Migrations:     migrations,
	}
	result.SetStatus(countWarnings(migrations), 0)

	return writeStructuredOutput(c.IO.Out(), c.OutputFormat, result)
}
