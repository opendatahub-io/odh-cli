package migrate

import (
	"github.com/spf13/cobra"

	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/genericiooptions"

	"github.com/opendatahub-io/odh-cli/cmd/migrate/list"
	"github.com/opendatahub-io/odh-cli/cmd/migrate/prepare"
	"github.com/opendatahub-io/odh-cli/cmd/migrate/run"
	utilcobra "github.com/opendatahub-io/odh-cli/pkg/util/cobra"
)

const (
	cmdName  = "migrate"
	cmdShort = "Manage cluster migrations"
)

const cmdLong = `
The migrate command manages cluster migrations for OpenShift AI components.

Use 'migrate list' to see available migrations filtered by version compatibility.
Use 'migrate prepare' to backup resources before migration.
Use 'migrate run' to execute one or more migrations sequentially.

Migrations are version-aware and only execute when applicable to the current
cluster state. Each migration can be run in dry-run mode to preview changes
before applying them.

Available subcommands:
  list     List available migrations for a target version
  prepare  Execute preparation steps (backups) for migrations
  run      Execute one or more migrations

Available migrations include RayCluster backup/migrate (raycluster.backup,
raycluster.migrate), Kueue RHBOK, and model serving actions.
`

const cmdExample = `
  # List available migrations for version 3.0
  kubectl odh migrate list --target-version 3.0.0

  # List only pre-upgrade migrations
  kubectl odh migrate list --target-version 3.0.0 --phase pre-upgrade

  # List all migrations including non-applicable ones
  kubectl odh migrate list --all

  # Prepare for migration (creates backups)
  kubectl odh migrate prepare --migration kueue.rhbok.migrate --target-version 3.0.0

  # Run a migration with confirmation prompts
  kubectl odh migrate run --migration kueue.rhbok.migrate --target-version 3.0.0

  # Run all pre-upgrade migrations (includes RayCluster backup)
  kubectl odh migrate run --phase pre-upgrade --target-version 3.0.0

  # Backup RayClusters before RHOAI upgrade
  kubectl odh migrate run -m raycluster.backup --target-version 3.0.0

  # Migrate RayClusters after RHOAI upgrade
  kubectl odh migrate run -m raycluster.migrate --target-version 3.0.0

  # Migrate RayClusters from backup
  kubectl odh migrate run -m raycluster.migrate --target-version 3.0.0 --raycluster-from-backup ./raycluster-backups/rhoai-3.x

  # Run migration in dry-run mode (preview changes only)
  kubectl odh migrate run --migration kueue.rhbok.migrate --target-version 3.0.0 --dry-run

  # Check for running training workloads before upgrade
  kubectl odh migrate run -m training.verify-workloads --target-version 3.0.0
`

// AddCommand adds the migrate command to the root command.
func AddCommand(root *cobra.Command, flags *genericclioptions.ConfigFlags, opts ...utilcobra.CommandOption) {
	streams := genericiooptions.IOStreams{
		In:     root.InOrStdin(),
		Out:    root.OutOrStdout(),
		ErrOut: root.ErrOrStderr(),
	}

	cmd := &cobra.Command{
		Use:           cmdName,
		Short:         cmdShort,
		Long:          cmdLong,
		Example:       cmdExample,
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	list.AddCommand(cmd, flags, streams)
	prepare.AddCommand(cmd, flags, streams)
	run.AddCommand(cmd, flags, streams)

	utilcobra.ApplyOptions(cmd, opts...)

	root.AddCommand(cmd)
}
