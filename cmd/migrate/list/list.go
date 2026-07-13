package list

import (
	"github.com/spf13/cobra"

	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/genericiooptions"

	"github.com/opendatahub-io/odh-cli/pkg/migrate"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/action"
	clierrors "github.com/opendatahub-io/odh-cli/pkg/util/errors"
)

const (
	cmdName  = "list"
	cmdShort = "List available migrations"
)

const cmdLong = `
List available migrations filtered by version compatibility.

By default, only migrations applicable to the current and target versions are shown.
Use --all to see all registered migrations regardless of applicability.

Note: --all and --target-version are mutually exclusive. Use --all to list all
migrations without version filtering, or --target-version to filter by applicability.
`

const cmdExample = `
  # List applicable migrations for version 3.0
  kubectl odh migrate list --target-version 3.0.0

  # List only pre-upgrade migrations
  kubectl odh migrate list --target-version 3.0.0 --phase pre-upgrade

  # List all migrations without version filtering
  kubectl odh migrate list --all

  # List with JSON output
  kubectl odh migrate list --target-version 3.0.0 -o json
`

// AddCommand adds the list subcommand to the migrate command.
func AddCommand(
	parent *cobra.Command,
	flags *genericclioptions.ConfigFlags,
	streams genericiooptions.IOStreams,
) {
	command := migrate.NewListCommand(streams)
	command.ConfigFlags = flags

	cmd := &cobra.Command{
		Use:           cmdName,
		Short:         cmdShort,
		Long:          cmdLong,
		Example:       cmdExample,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			outputFormat := string(command.OutputFormat)

			if err := command.Complete(); err != nil {
				return clierrors.HandleError(cmd, err, outputFormat)
			}

			if err := command.Validate(); err != nil {
				return clierrors.HandleError(cmd, err, outputFormat)
			}

			if err := command.Run(cmd.Context()); err != nil {
				return clierrors.HandleError(cmd, err, outputFormat)
			}

			return nil
		},
	}

	command.AddFlags(cmd.Flags())

	_ = cmd.RegisterFlagCompletionFunc("phase",
		func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
			return action.PhaseValues(), cobra.ShellCompDirectiveNoFileComp
		},
	)

	parent.AddCommand(cmd)
}
