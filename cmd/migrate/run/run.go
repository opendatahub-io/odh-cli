package run

import (
	"github.com/opendatahub-io/odh-cli/pkg/migrate"
	"github.com/spf13/cobra"

	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/genericiooptions"
)

const (
	cmdName  = "run"
	cmdShort = "Execute one or more migrations"
)

const cmdLong = `
Execute one or more migrations sequentially for OpenShift AI components.

Migrations are executed in the order specified. If any migration fails, execution
stops immediately. Each migration can require user confirmation unless --yes is specified.

Use --dry-run to preview changes without applying them.
Use 'migrate prepare' to backup resources before running migrations.
`

const cmdExample = `
  # Run a single migration with confirmation prompts
  kubectl odh migrate run --migration kueue.rhbok.migrate --target-version 3.0.0

  # Run migration in dry-run mode (verbose is automatically enabled)
  kubectl odh migrate run --migration kueue.rhbok.migrate --target-version 3.0.0 --dry-run

  # Run migration without confirmation prompts
  kubectl odh migrate run --migration kueue.rhbok.migrate --target-version 3.0.0 --yes

  # Run multiple migrations sequentially
  kubectl odh migrate run -m kueue.rhbok.migrate -m other.migration --target-version 3.0.0

  # Typical workflow: prepare first, then run
  kubectl odh migrate prepare --migration kueue.rhbok.migrate --target-version 3.0.0
  kubectl odh migrate run --migration kueue.rhbok.migrate --target-version 3.0.0 --yes
`

// AddCommand adds the run subcommand to the migrate command.
func AddCommand(
	parent *cobra.Command,
	flags *genericclioptions.ConfigFlags,
	streams genericiooptions.IOStreams,
) {
	command := migrate.NewRunCommand(streams)
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
