package migrate

import (
	"context"
	"errors"
	"fmt"

	"github.com/blang/semver/v4"
	"github.com/spf13/pflag"

	"k8s.io/cli-runtime/pkg/genericiooptions"

	"github.com/opendatahub-io/odh-cli/pkg/cmd"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/action"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/actions/dashboard/redirect"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/actions/kueue/rhbok"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/actions/modelserving"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/actions/workbenches/upgrade"
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

	parsedTargetVersion *semver.Version
	parsedPhase         action.ActionPhase

	// registry is the action registry for this command instance.
	// Explicitly populated to avoid global state and enable test isolation.
	registry *action.ActionRegistry
}

func NewRunCommand(streams genericiooptions.IOStreams) *RunCommand {
	shared := NewSharedOptions(streams)
	registry := action.NewActionRegistry()

	// Explicitly register all actions (no global state, full test isolation)
	registry.MustRegister(&rhbok.RHBOKMigrationAction{})
	registry.MustRegister(&modelserving.ServerlessToRawAction{})
	registry.MustRegister(&modelserving.ModelMeshToRawAction{})
	registry.MustRegister(&modelserving.HardwareProfilesIgnorelistAction{})
	registry.MustRegister(&modelserving.AddOwnerReferencesAction{})
	registry.MustRegister(&modelserving.ManagedISVCConfigAction{})
	registry.MustRegister(&upgrade.WorkbenchUpgradeAction{})
	registry.MustRegister(&redirect.DashboardRedirectAction{})

	return &RunCommand{
		SharedOptions: shared,
		registry:      registry,
	}
}

func (c *RunCommand) ActionIDs() []string {
	return c.registry.ActionIDs()
}

func (c *RunCommand) AddFlags(fs *pflag.FlagSet) {
	fs.BoolVarP(&c.Verbose, "verbose", "v", false, flagDescRunVerbose)
	fs.DurationVar(&c.Timeout, "timeout", c.Timeout, flagDescRunTimeout)
	fs.BoolVar(&c.DryRun, "dry-run", false, flagDescRunDryRun)
	fs.BoolVarP(&c.Yes, "yes", "y", false, flagDescRunYes)
	fs.StringArrayVarP(&c.MigrationIDs, "migration", "m", []string{}, flagDescRunMigration)
	fs.StringVar(&c.TargetVersion, "target-version", "", flagDescRunTargetVersion)
	fs.StringVar(&c.Phase, "phase", "", flagDescRunPhase)

	// Throttling settings
	fs.Float32Var(&c.QPS, "qps", c.QPS, "Kubernetes API QPS limit (queries per second)")
	fs.IntVar(&c.Burst, "burst", c.Burst, "Kubernetes API burst capacity")

	// Let actions register their own flags
	action.RegisterActionFlags(c.registry, fs)
}

func (c *RunCommand) Complete() error {
	if err := c.SharedOptions.Complete(); err != nil {
		return fmt.Errorf("completing shared options: %w", err)
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
		return fmt.Errorf("no applicable migrations found for phase %s", string(effectivePhase))
	}

	return c.runMigrationMode(ctx, currentVersion, c.parsedTargetVersion, effectivePhase)
}

func (c *RunCommand) runMigrationMode(
	ctx context.Context,
	currentVersion *semver.Version,
	targetVersion *semver.Version,
	effectivePhase action.ActionPhase,
) error {
	c.IO.Errorf("Current OpenShift AI version: %s", currentVersion.String())
	c.IO.Errorf("Target OpenShift AI version: %s", targetVersion.String())
	c.IO.Errorf("Phase: %s\n", string(effectivePhase))

	hasSkips := false

	for idx, migrationID := range c.MigrationIDs {
		if len(c.MigrationIDs) > 1 {
			c.IO.Errorf("\n=== Migration %d/%d: %s ===\n", idx+1, len(c.MigrationIDs), migrationID)
		}

		selectedAction, ok := c.registry.Get(migrationID)
		if !ok {
			return fmt.Errorf("migration %q not found", migrationID)
		}

		if selectedAction.Phase() != effectivePhase {
			c.IO.Errorf("WARNING: migration %s has phase %s but effective phase is %s; proceeding because --migration was explicit",
				migrationID, string(selectedAction.Phase()), string(effectivePhase))
		}

		// Use verbose recorder for real-time streaming output
		recorder := action.NewVerboseRootRecorder(c.IO)
		c.IO.Errorf("\n%s:\n", migrationID)

		target := action.Target{
			Client:         c.Client,
			CurrentVersion: currentVersion,
			TargetVersion:  targetVersion,
			DryRun:         c.DryRun,
			SkipConfirm:    c.Yes,
			Recorder:       recorder,
			IO:             c.IO,
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
			return fmt.Errorf("migration %q has no run task", migrationID)
		}

		actionResult, err := runTask.Execute(ctx, target)
		if err != nil {
			return fmt.Errorf("migration failed: %w", err)
		}

		// Output has already been streamed during execution, no need to render again
		c.IO.Fprintln()
		if !actionResult.Status.Completed {
			c.IO.Errorf("Migration %s incomplete - please review the output above", migrationID)

			return fmt.Errorf("migration halted: %s", migrationID)
		}
		if actionResult.HasSkippedSteps() {
			c.IO.Errorf("Migration %s completed with skipped steps", migrationID)

			hasSkips = true
		} else {
			c.IO.Errorf("Migration %s completed successfully!", migrationID)
		}
	}

	c.IO.Fprintln()

	if hasSkips {
		c.IO.Errorf("All migrations completed (some steps were skipped).")
	} else {
		c.IO.Errorf("All migrations completed successfully!")
	}

	return nil
}
