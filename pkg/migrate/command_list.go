package migrate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/blang/semver/v4"
	"github.com/spf13/pflag"
	"sigs.k8s.io/yaml"

	"k8s.io/cli-runtime/pkg/genericiooptions"

	"github.com/opendatahub-io/odh-cli/pkg/cmd"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/action"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/actions/dashboard/redirect"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/actions/kueue/rhbok"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/actions/modelserving"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/actions/workbenches/upgrade"
	"github.com/opendatahub-io/odh-cli/pkg/printer/table"
	"github.com/opendatahub-io/odh-cli/pkg/util/iostreams"
	"github.com/opendatahub-io/odh-cli/pkg/util/version"
)

var _ cmd.Command = (*ListCommand)(nil)

type migrationRow struct {
	ID          string
	Name        string
	Phase       string
	Applicable  string
	Description string
}

type ListCommand struct {
	*SharedOptions

	TargetVersion string
	ShowAll       bool
	Phase         string

	parsedTargetVersion *semver.Version

	// registry is the action registry for this command instance.
	// Explicitly populated to avoid global state and enable test isolation.
	registry *action.ActionRegistry
}

func NewListCommand(streams genericiooptions.IOStreams) *ListCommand {
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

	return &ListCommand{
		SharedOptions: shared,
		registry:      registry,
	}
}

func (c *ListCommand) AddFlags(fs *pflag.FlagSet) {
	fs.StringVarP((*string)(&c.OutputFormat), "output", "o", string(OutputFormatTable), flagDescListOutput)
	fs.BoolVarP(&c.Verbose, "verbose", "v", false, flagDescListVerbose)
	fs.StringVar(&c.TargetVersion, "target-version", "", flagDescListTargetVersion)
	fs.BoolVar(&c.ShowAll, "all", false, flagDescListAll)
	fs.StringVar(&c.Phase, "phase", "", flagDescListPhase)

	// Throttling settings
	fs.Float32Var(&c.QPS, "qps", c.QPS, "Kubernetes API QPS limit (queries per second)")
	fs.IntVar(&c.Burst, "burst", c.Burst, "Kubernetes API burst capacity")
}

func (c *ListCommand) Complete() error {
	if err := c.SharedOptions.Complete(); err != nil {
		return fmt.Errorf("completing shared options: %w", err)
	}

	if !c.Verbose {
		c.IO = iostreams.NewQuietWrapper(c.IO)
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

func (c *ListCommand) Validate() error {
	if err := c.SharedOptions.Validate(); err != nil {
		return fmt.Errorf("validating shared options: %w", err)
	}

	if c.ShowAll && c.TargetVersion != "" {
		return errors.New("--all and --target-version are mutually exclusive")
	}

	if !c.ShowAll && c.TargetVersion == "" {
		return errors.New("--target-version flag is required")
	}

	if err := action.ActionPhase(c.Phase).Validate(); err != nil {
		return fmt.Errorf("validating phase: %w", err)
	}

	return nil
}

func (c *ListCommand) Run(ctx context.Context) error {
	var currentVersion *semver.Version
	var err error

	if !c.ShowAll {
		currentVersion, err = version.Detect(ctx, c.Client)
		if err != nil {
			return fmt.Errorf("detecting cluster version: %w", err)
		}
	}

	allActions := c.registry.ListAll()

	if len(allActions) == 0 {
		c.IO.Errorf("No migrations registered")

		return nil
	}

	allActions = filterByPhase(allActions, action.ActionPhase(c.Phase))

	if len(allActions) == 0 {
		c.IO.Errorf("No migrations found for phase %q", c.Phase)

		return nil
	}

	rows := c.buildRows(allActions, currentVersion)

	if len(rows) == 0 {
		c.IO.Errorf("no migrations applicable for version %s", c.TargetVersion)

		return nil
	}

	if err := c.printRows(rows); err != nil {
		return err
	}

	c.printPhaseHint(currentVersion)

	return nil
}

func (c *ListCommand) printPhaseHint(currentVersion *semver.Version) {
	if c.Phase != "" || currentVersion == nil || c.OutputFormat != OutputFormatTable {
		return
	}

	detectedPhase, err := DetectPhase(currentVersion, c.parsedTargetVersion)
	if err != nil {
		return
	}

	c.IO.Fprintf("\nNote: 'migrate run' will auto-detect phase as %q for this cluster.\n", string(detectedPhase))
	c.IO.Fprintf("Use --phase to filter the list by lifecycle phase.\n")
}

func (c *ListCommand) buildRows(actions []action.Action, currentVersion *semver.Version) []migrationRow {
	rows := make([]migrationRow, 0)

	for _, act := range actions {
		var applicableStr string

		if c.ShowAll && c.parsedTargetVersion == nil {
			applicableStr = "N/A"
		} else {
			target := action.Target{
				Client:         c.Client,
				CurrentVersion: currentVersion,
				TargetVersion:  c.parsedTargetVersion,
			}

			applicable := act.CanApply(target)

			if !c.ShowAll && !applicable {
				continue
			}

			if applicable {
				applicableStr = "Yes"
			} else {
				applicableStr = "No"
			}
		}

		rows = append(rows, migrationRow{
			ID:          act.ID(),
			Name:        act.Name(),
			Phase:       string(act.Phase()),
			Applicable:  applicableStr,
			Description: act.Description(),
		})
	}

	return rows
}

func (c *ListCommand) printRows(rows []migrationRow) error {
	switch c.OutputFormat {
	case OutputFormatTable:
		return c.printTable(rows)
	case OutputFormatJSON:
		return c.printJSON(rows)
	case OutputFormatYAML:
		return c.printYAML(rows)
	default:
		return fmt.Errorf("unsupported output format: %s", c.OutputFormat)
	}
}

func (c *ListCommand) printTable(rows []migrationRow) error {
	renderer := table.NewRenderer(
		table.WithWriter[migrationRow](c.IO.Out()),
		table.WithHeaders[migrationRow]("ID", "NAME", "PHASE", "APPLICABLE", "DESCRIPTION"),
		table.WithTableOptions[migrationRow](table.DefaultTableOptions...),
	)

	for _, row := range rows {
		if err := renderer.Append(row); err != nil {
			return fmt.Errorf("failed to append row: %w", err)
		}
	}

	if err := renderer.Render(); err != nil {
		return fmt.Errorf("failed to render table: %w", err)
	}

	return nil
}

func (c *ListCommand) printJSON(rows []migrationRow) error {
	//nolint:musttag // Table rows don't need JSON tags
	data, err := json.MarshalIndent(rows, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling JSON: %w", err)
	}

	c.IO.Fprintf("%s\n", string(data))

	return nil
}

func (c *ListCommand) printYAML(rows []migrationRow) error {
	data, err := yaml.Marshal(rows)
	if err != nil {
		return fmt.Errorf("marshaling YAML: %w", err)
	}

	c.IO.Fprintf("%s", string(data))

	return nil
}
