package migrate

import (
	"context"
	"errors"
	"fmt"

	"github.com/blang/semver/v4"
	"github.com/spf13/pflag"

	"k8s.io/cli-runtime/pkg/genericiooptions"

	"github.com/opendatahub-io/odh-cli/pkg/api"
	"github.com/opendatahub-io/odh-cli/pkg/cmd"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/action"
	"github.com/opendatahub-io/odh-cli/pkg/output"
	clierrors "github.com/opendatahub-io/odh-cli/pkg/util/errors"
	"github.com/opendatahub-io/odh-cli/pkg/util/iostreams"
	"github.com/opendatahub-io/odh-cli/pkg/util/stdin"
	"github.com/opendatahub-io/odh-cli/pkg/util/version"
)

var _ cmd.Command = (*RunCommand)(nil)

type RunCommand struct {
	*SharedOptions

	DryRun        bool
	Yes           bool
	MigrationIDs  []string
	TargetVersion string
	Phase         string
	FromStdin     bool

	parsedTargetVersion *semver.Version
	parsedPhase         action.ActionPhase

	// registry is the action registry for this command instance.
	// Explicitly populated to avoid global state and enable test isolation.
	registry *action.ActionRegistry

	// flags stores the FlagSet for checking explicit flag usage.
	flags *pflag.FlagSet
}

func NewRunCommand(streams genericiooptions.IOStreams) *RunCommand {
	shared := NewSharedOptions(streams)

	return &RunCommand{
		SharedOptions: shared,
		registry:      newDefaultRegistry(),
	}
}

func (c *RunCommand) ActionIDs() []string {
	return c.registry.ActionIDs()
}

func (c *RunCommand) AddFlags(fs *pflag.FlagSet) {
	c.flags = fs
	fs.StringVarP((*string)(&c.OutputFormat), "output", "o", string(OutputFormatTable), "Output format: table, json, yaml")
	_ = fs.SetAnnotation("output", api.AnnotationValidValues, []string{"table", "json", "yaml"})
	fs.BoolVarP(&c.Verbose, "verbose", "v", false, flagDescRunVerbose)
	fs.DurationVar(&c.Timeout, "timeout", c.Timeout, flagDescRunTimeout)
	fs.BoolVar(&c.DryRun, "dry-run", false, flagDescRunDryRun)
	fs.BoolVarP(&c.Yes, "yes", "y", false, flagDescRunYes)
	fs.StringArrayVarP(&c.MigrationIDs, "migration", "m", []string{}, flagDescRunMigration)
	fs.StringVar(&c.TargetVersion, "target-version", "", flagDescRunTargetVersion)
	fs.StringVar(&c.Phase, "phase", "", flagDescRunPhase)
	// Empty string is intentionally excluded: it means "flag not provided" (the default), not a user-selectable value.
	_ = fs.SetAnnotation("phase", api.AnnotationValidValues, []string{"pre-upgrade", "post-upgrade", "pre-enablement"})
	fs.BoolVar(&c.FromStdin, "from-stdin", false, stdin.FlagDesc)

	// Throttling settings
	fs.Float32Var(&c.QPS, "qps", c.QPS, "Kubernetes API QPS limit (queries per second)")
	fs.IntVar(&c.Burst, "burst", c.Burst, "Kubernetes API burst capacity")

	// Let actions register their own flags
	action.RegisterActionFlags(c.registry, fs)
}

// parseStdinConfig reads and applies configuration from stdin.
func (c *RunCommand) parseStdinConfig() error {
	if err := stdin.CheckPiped(c.IO.In()); err != nil {
		return err //nolint:wrapcheck // CheckPiped returns a self-descriptive user-facing error
	}

	var input StdinInput
	if err := stdin.Parse(c.IO.In(), &input); err != nil {
		return fmt.Errorf("parsing stdin: %w", err)
	}

	return c.applyStdinInput(&input)
}

// applyStdinInput merges stdin configuration into command options.
// Explicit CLI flags take precedence over stdin values.
func (c *RunCommand) applyStdinInput(input *StdinInput) error {
	// Apply migrations if not set via CLI
	if len(input.Migrations) > 0 && !stdin.FlagChanged(c.flags, "migration") {
		c.MigrationIDs = input.Migrations
	}

	// Apply target version if not set via CLI
	if input.TargetVersion != "" && !stdin.FlagChanged(c.flags, "target-version") {
		c.TargetVersion = input.TargetVersion
	}

	// Apply phase if not set via CLI
	if input.Phase != "" && !stdin.FlagChanged(c.flags, "phase") {
		c.Phase = input.Phase
	}

	// Apply boolean flags if not set via CLI
	if input.DryRun && !stdin.FlagChanged(c.flags, "dry-run") {
		c.DryRun = true
	}

	if input.SkipConfirm && !stdin.FlagChanged(c.flags, "yes") {
		c.Yes = true
	}

	return nil
}

func (c *RunCommand) Complete() error {
	// Parse stdin configuration if --from-stdin is specified
	if c.FromStdin {
		if err := c.parseStdinConfig(); err != nil {
			//nolint:wrapcheck // NewExitCodeError is a same-module constructor
			return clierrors.NewExitCodeError(clierrors.ExitValidation, err)
		}
	}

	if err := c.SharedOptions.Complete(); err != nil {
		return fmt.Errorf("completing shared options: %w", err)
	}

	// Suppress text output for structured formats (matching codebase pattern)
	if c.OutputFormat != OutputFormatTable {
		c.IO = iostreams.NewQuietWrapper(c.IO)
	}

	// Always enable verbose for migrate run (both dry-run and actual execution)
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

	return nil
}

func (c *RunCommand) Validate() error {
	if err := c.SharedOptions.Validate(); err != nil {
		return fmt.Errorf("validating shared options: %w", err)
	}

	if len(c.MigrationIDs) == 0 && c.Phase == "" {
		return errors.New("--migration flag is required (or use --phase to run all actions for a lifecycle phase)")
	}

	if c.TargetVersion == "" {
		return errors.New("--target-version flag is required")
	}

	if err := c.parsedPhase.Validate(); err != nil {
		return fmt.Errorf("validating phase: %w", err)
	}

	return nil
}

func (c *RunCommand) Run(ctx context.Context) error {
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

	return c.runMigrationMode(ctx, currentVersion, c.parsedTargetVersion, effectivePhase)
}

func (c *RunCommand) runMigrationMode(
	ctx context.Context,
	currentVersion *semver.Version,
	targetVersion *semver.Version,
	effectivePhase action.ActionPhase,
) error {
	structured := c.OutputFormat != OutputFormatTable

	c.IO.Errorf("Current OpenShift AI version: %s", currentVersion.String())
	c.IO.Errorf("Target OpenShift AI version: %s", targetVersion.String())
	c.IO.Errorf("Phase: %s\n", string(effectivePhase))

	// Use FullQuietWrapper for action IO to prevent stdout corruption in structured mode
	actionIO := actionIOForMode(c.IO, structured)

	hasSkips := false
	hasFailures := false

	var migrationResults []MigrationResultItem

	for idx, migrationID := range c.MigrationIDs {
		if len(c.MigrationIDs) > 1 {
			c.IO.Errorf("\n=== Migration %d/%d: %s ===\n", idx+1, len(c.MigrationIDs), migrationID)
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

		recorder := action.NewVerboseRootRecorder(actionIO)
		c.IO.Errorf("\n%s:\n", migrationID)

		target := action.Target{
			Client:         c.Client,
			RESTConfig:     c.RESTConfig,
			CurrentVersion: currentVersion,
			TargetVersion:  targetVersion,
			DryRun:         c.DryRun,
			SkipConfirm:    c.Yes,
			Recorder:       recorder,
			IO:             actionIO,
		}

		if c.DryRun {
			c.IO.Errorf("DRY RUN MODE: No changes will be made to the cluster\n")
		} else if c.Yes {
			c.IO.Errorf("Running migration: %s (confirmations skipped)\n", migrationID)
		} else {
			c.IO.Errorf("Preparing migration: %s\n", migrationID)
		}

		runTask := selectedAction.Run()
		if runTask == nil {
			//nolint:wrapcheck // NewExitCodeError is a same-module constructor
			return clierrors.NewExitCodeError(clierrors.ExitValidation,
				fmt.Errorf("migration %q has no run task", migrationID))
		}

		actionResult, err := runTask.Execute(ctx, target)
		if err != nil {
			//nolint:wrapcheck // NewExitCodeError is a same-module constructor
			return clierrors.NewExitCodeError(clierrors.ExitValidation,
				fmt.Errorf("migration failed: %w", err))
		}

		c.IO.Errorln()

		if !actionResult.Status.Completed {
			c.IO.Errorf("Migration %s incomplete - please review the output above", migrationID)

			//nolint:wrapcheck // NewExitCodeError is a same-module constructor
			return clierrors.NewExitCodeError(clierrors.ExitValidation,
				fmt.Errorf("migration halted: %s", migrationID))
		}
		skippedSteps := actionResult.HasSkippedSteps()
		failedSteps := actionResult.HasFailedSteps()

		if skippedSteps {
			hasSkips = true
		}

		if failedSteps {
			c.IO.Errorf("Migration %s completed with failures", migrationID)

			hasFailures = true
		} else if skippedSteps {
			c.IO.Errorf("Migration %s completed with skipped steps", migrationID)
		} else {
			c.IO.Errorf("Migration %s completed successfully!", migrationID)
		}

		migrationResults = append(migrationResults, MigrationResultItem{
			ID:              migrationID,
			Completed:       actionResult.Status.Completed,
			HasSkippedSteps: skippedSteps,
			HasFailedSteps:  failedSteps,
			PhaseMismatch:   phaseMismatch,
		})
	}

	if structured {
		return c.writeRunResult(currentVersion, targetVersion, effectivePhase, migrationResults)
	}

	return c.reportRunSummary(hasFailures, hasSkips)
}

func (c *RunCommand) reportRunSummary(hasFailures, hasSkips bool) error {
	c.IO.Fprintln()

	if hasFailures {
		if hasSkips {
			c.IO.Errorf("All migrations completed (some steps failed, some were skipped). Review the output above for details.")
		} else {
			c.IO.Errorf("All migrations completed (some steps failed). Review the output above for details.")
		}

		return errors.New("one or more migrations completed with failures")
	}

	if hasSkips {
		c.IO.Errorf("All migrations completed (some steps were skipped).")
	} else {
		c.IO.Errorf("All migrations completed successfully!")
	}

	return nil
}

// RunResult is the structured success output for migrate run.
type RunResult struct {
	output.Envelope `json:",inline" yaml:",inline"`

	CurrentVersion string                `json:"currentVersion" yaml:"currentVersion"`
	TargetVersion  string                `json:"targetVersion"  yaml:"targetVersion"`
	Phase          string                `json:"phase"          yaml:"phase"`
	DryRun         bool                  `json:"dryRun"         yaml:"dryRun"`
	Migrations     []MigrationResultItem `json:"migrations"     yaml:"migrations"`
}

func (c *RunCommand) writeRunResult(
	currentVersion *semver.Version,
	targetVersion *semver.Version,
	phase action.ActionPhase,
	migrations []MigrationResultItem,
) error {
	result := RunResult{
		Envelope:       output.NewEnvelope("MigrateRunResult", "migrate run"),
		CurrentVersion: currentVersion.String(),
		TargetVersion:  targetVersion.String(),
		Phase:          string(phase),
		DryRun:         c.DryRun,
		Migrations:     migrations,
	}
	result.SetStatus(countWarnings(migrations), countErrors(migrations))

	if errCount := countErrors(migrations); errCount > 0 {
		if err := writeStructuredOutput(c.IO.Out(), c.OutputFormat, result); err != nil {
			return err
		}

		return errors.New("one or more migrations completed with failures")
	}

	return writeStructuredOutput(c.IO.Out(), c.OutputFormat, result)
}
