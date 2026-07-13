package prepare

import (
	"github.com/spf13/cobra"

	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/genericiooptions"

	"github.com/opendatahub-io/odh-cli/pkg/migrate"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/action"
	clierrors "github.com/opendatahub-io/odh-cli/pkg/util/errors"
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

  # Prepare all pre-upgrade migrations
  kubectl odh migrate prepare --phase pre-upgrade --target-version 3.0.0

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

	_ = cmd.RegisterFlagCompletionFunc("migration",
		func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
			return command.ActionIDs(), cobra.ShellCompDirectiveNoFileComp
		},
	)

	parent.AddCommand(cmd)
}
