package list

import (
	"github.com/spf13/cobra"

	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/genericiooptions"

	"github.com/opendatahub-io/odh-cli/pkg/migrate"
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
			//nolint:wrapcheck // Errors from Complete and Validate are already contextualized
			if err := command.Complete(); err != nil {
				return err
			}
			//nolint:wrapcheck // Errors from Validate are already contextualized
			if err := command.Validate(); err != nil {
				return err
			}

			return command.Run(cmd.Context())
		},
	}

	command.AddFlags(cmd.Flags())
	parent.AddCommand(cmd)
}
