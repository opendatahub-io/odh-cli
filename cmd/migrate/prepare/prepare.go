package prepare

import (
	"github.com/opendatahub-io/odh-cli/pkg/migrate"
	"github.com/spf13/cobra"

	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/genericiooptions"
)

const (
	cmdName  = "prepare"
	cmdShort = "Execute preparation steps for migrations"
)

const cmdLong = `
Execute preparation steps for one or more migrations, such as backing up
resources and performing pre-migration setup.

This command should be run before 'migrate run' to ensure the cluster is
properly prepared. Preparation steps typically include:
- Backing up critical resources (ClusterQueues, LocalQueues, ConfigMaps)
- Validating cluster prerequisites
- Creating necessary namespaces or resources

Use --dry-run to preview what would be backed up without making changes.
Use --output-dir to specify where backups should be written.
`

const cmdExample = `
  # Prepare for a single migration (creates timestamped backup directory)
  kubectl odh migrate prepare --migration kueue.rhbok.migrate --target-version 3.0.0

  # Prepare with custom backup directory
  kubectl odh migrate prepare --migration kueue.rhbok.migrate --target-version 3.0.0 --output-dir /path/to/backups

  # Preview what would be backed up (dry-run mode)
  kubectl odh migrate prepare --migration kueue.rhbok.migrate --target-version 3.0.0 --dry-run

  # Prepare without confirmation prompts
  kubectl odh migrate prepare --migration kueue.rhbok.migrate --target-version 3.0.0 --yes

  # Prepare multiple migrations sequentially
  kubectl odh migrate prepare -m kueue.rhbok.migrate -m other.migration --target-version 3.0.0
`

// AddCommand adds the prepare subcommand to the migrate command.
func AddCommand(
	parent *cobra.Command,
	flags *genericclioptions.ConfigFlags,
	streams genericiooptions.IOStreams,
) {
	command := migrate.NewPrepareCommand(streams)
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
