package migrate

import (
	"github.com/spf13/cobra"

	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/genericiooptions"

	"github.com/lburgazzoli/odh-cli/pkg/cmd/migrate"

	// Import action packages to trigger init() auto-registration.
	// These blank imports are REQUIRED for actions to register with the global registry.
	// DO NOT REMOVE - they appear unused but are essential for runtime action discovery.
	_ "github.com/lburgazzoli/odh-cli/pkg/migrate/actions/kueue/rhbok"
)

const (
	cmdName  = "migrate"
	cmdShort = "Run cluster migrations"
	cmdLong  = `
The migrate command performs cluster migrations for OpenShift AI components.

Migrations are version-aware and only execute when applicable to the current
cluster state. Each migration can be run in dry-run mode to preview changes
before applying them.

Examples:
  # Run RHBOK migration with confirmation prompts
  kubectl odh migrate --migration kueue-to-rhbok --target-version 3.0.0

  # Run migration in dry-run mode (preview changes only)
  kubectl odh migrate --migration kueue-to-rhbok --target-version 3.0.0 --dry-run

  # Run migration without confirmation prompts
  kubectl odh migrate --migration kueue-to-rhbok --target-version 3.0.0 --yes

  # Prepare for migration (run pre-flight checks and backup resources)
  kubectl odh migrate --migration kueue-to-rhbok --target-version 3.0.0 --prepare

  # Run migration with custom backup path
  kubectl odh migrate --migration kueue-to-rhbok --target-version 3.0.0 --backup-path /path/to/backups
`
	cmdExample = `
  # Migrate from OpenShift AI built-in Kueue to Red Hat Build of Kueue
  kubectl odh migrate --migration kueue-to-rhbok --target-version 3.0.0

  # Preview migration changes without applying them
  kubectl odh migrate --migration kueue-to-rhbok --target-version 3.0.0 --dry-run

  # Prepare for migration (validate and backup)
  kubectl odh migrate --migration kueue-to-rhbok --target-version 3.0.0 --prepare --backup-path ./my-backups
`
)

func AddCommand(root *cobra.Command, flags *genericclioptions.ConfigFlags) {
	streams := genericiooptions.IOStreams{
		In:     root.InOrStdin(),
		Out:    root.OutOrStdout(),
		ErrOut: root.ErrOrStderr(),
	}

	command := migrate.NewCommand(streams)
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
	root.AddCommand(cmd)
}
