package deps

import (
	"github.com/spf13/cobra"

	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/genericiooptions"

	"github.com/opendatahub-io/odh-cli/pkg/cmd"
	depspkg "github.com/opendatahub-io/odh-cli/pkg/deps"
	clierrors "github.com/opendatahub-io/odh-cli/pkg/util/errors"
)

const (
	cmdName  = "deps"
	cmdShort = "Manage operator dependencies for ODH/RHOAI"
)

const cmdLong = `
Manage operator dependencies required by ODH/RHOAI components.

Without a subcommand, displays dependency status by querying OLM operator resources.
Each dependency shows:
  - Installation status (installed, missing, optional)
  - Installed version (if available)
  - Namespace where the operator runs
  - Components that require this dependency

Subcommands:
  install     Install missing operator dependencies via OLM
  graph       Show dependency relationships
`

const cmdExample = `
  # Show all dependencies
  kubectl odh deps

  # Show dependencies for a specific version
  kubectl odh deps --version 3.4.0

  # Output as JSON
  kubectl odh deps -o json

  # Dry run (show manifest data without cluster query)
  kubectl odh deps --dry-run

  # Install all missing required dependencies
  kubectl odh deps install

  # Install a specific dependency
  kubectl odh deps install cert-manager

  # Show what would be installed without executing
  kubectl odh deps install --dry-run

  # Show dependency graph
  kubectl odh deps graph
`

// runCommand executes the Complete/Validate/Run lifecycle with error handling.
//
//nolint:wrapcheck // HandleError returns an already-handled error
func runCommand(cobraCmd *cobra.Command, c cmd.Command, outputFormat string) error {
	if err := c.Complete(); err != nil {
		return clierrors.HandleError(cobraCmd, err, outputFormat)
	}

	if err := c.Validate(); err != nil {
		return clierrors.HandleError(cobraCmd, err, outputFormat)
	}

	if err := c.Run(cobraCmd.Context()); err != nil {
		return clierrors.HandleError(cobraCmd, err, outputFormat)
	}

	return nil
}

// AddCommand adds the deps command to the root command.
func AddCommand(root *cobra.Command, flags *genericclioptions.ConfigFlags) {
	streams := genericiooptions.IOStreams{
		In:     root.InOrStdin(),
		Out:    root.OutOrStdout(),
		ErrOut: root.ErrOrStderr(),
	}

	// Default list command (backward compatibility)
	listCommand := depspkg.NewCommand(streams, flags)

	depsCmd := &cobra.Command{
		Use:           cmdName,
		Short:         cmdShort,
		Long:          cmdLong,
		Example:       cmdExample,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cobraCmd *cobra.Command, _ []string) error {
			return runCommand(cobraCmd, listCommand, listCommand.Output)
		},
	}

	listCommand.AddFlags(depsCmd.Flags())

	// Add subcommands
	addInstallCommand(depsCmd, flags, streams)
	addGraphCommand(depsCmd, flags, streams)

	root.AddCommand(depsCmd)
}

// addInstallCommand adds the install subcommand.
// Note: This command is text-only (progress messages) and does not support --output flag.
func addInstallCommand(parent *cobra.Command, flags *genericclioptions.ConfigFlags, streams genericiooptions.IOStreams) {
	installCommand := depspkg.NewInstallCommand(streams, flags)

	installCmd := &cobra.Command{
		Use:   "install [DEPENDENCY]",
		Short: "Install missing operator dependencies via OLM",
		Long: `Install missing operator dependencies required by ODH/RHOAI.

Creates the resources required by the cluster's OLM API for each missing dependency:
an OperatorGroup and Subscription for OLM v0, or a ClusterExtension for OLM v1.
Waits for the operator installation to succeed.

OLM v1 requires a preconfigured ServiceAccount in each operator namespace with
permissions for the operator bundle. The default name is odh-cli-dependency-installer;
use --service-account to select a different account.
When both OLM APIs are available, OLM v0 is used by default; use --olm-mode=v1
to install with ClusterExtension.

If a dependency name is provided, only that dependency is installed.
Otherwise, all missing required dependencies are installed.

Note: This command outputs progress text only; the parent's --output flag does not apply.`,
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.MaximumNArgs(1),
		RunE: func(cobraCmd *cobra.Command, args []string) error {
			installCommand.TargetDep = ""
			if len(args) > 0 {
				installCommand.TargetDep = args[0]
			}

			return runCommand(cobraCmd, installCommand, "")
		},
	}

	installCommand.AddFlags(installCmd.Flags())
	parent.AddCommand(installCmd)
}

// addGraphCommand adds the graph subcommand.
// Note: This command is text-only (ASCII visualization) and does not support --output flag.
func addGraphCommand(parent *cobra.Command, flags *genericclioptions.ConfigFlags, streams genericiooptions.IOStreams) {
	graphCommand := depspkg.NewGraphCommand(streams, flags)

	graphCmd := &cobra.Command{
		Use:   "graph",
		Short: "Show dependency relationships visually",
		Long: `Display dependency relationships as a formatted list.

Shows which components require which operators, and which operators
depend on other operators (transitive dependencies).

Note: This command outputs text visualization only; the parent's --output flag does not apply.`,
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.NoArgs,
		RunE: func(cobraCmd *cobra.Command, _ []string) error {
			return runCommand(cobraCmd, graphCommand, "")
		},
	}

	graphCommand.AddFlags(graphCmd.Flags())
	parent.AddCommand(graphCmd)
}
