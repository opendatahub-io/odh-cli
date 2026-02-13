package raycluster

import (
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/genericiooptions"

	"github.com/lburgazzoli/odh-cli/cmd/migrate/raycluster/list"
	"github.com/lburgazzoli/odh-cli/cmd/migrate/raycluster/postupgrade"
	"github.com/lburgazzoli/odh-cli/cmd/migrate/raycluster/preupgrade"
)

const (
	cmdName  = "raycluster"
	cmdShort = "RHOAI 2.x to 3.x RayCluster migration"
)

const cmdLong = `
Manage RayCluster migration from RHOAI 2.x to RHOAI 3.x.

  pre-upgrade   Run pre-flight checks and backup RayClusters (run before upgrade)
  post-upgrade  Migrate RayClusters after RHOAI upgrade (live or from backup)
  list          List RayClusters and their migration status

Connection/auth: standard flags (--kubeconfig, --context, --server, --token, etc.) apply.
Use 'kubectl odh --help' for the full list of global flags.
`

const cmdExample = `
  # Before RHOAI upgrade: backup and run pre-flight checks
  kubectl odh migrate raycluster pre-upgrade --output-dir ./raycluster-backups

  # After RHOAI upgrade: migrate (start with one cluster, then namespace, then all)
  kubectl odh migrate raycluster post-upgrade --cluster my-cluster --namespace my-ns --dry-run
  kubectl odh migrate raycluster post-upgrade --cluster my-cluster --namespace my-ns
  kubectl odh migrate raycluster post-upgrade --namespace my-ns
  kubectl odh migrate raycluster post-upgrade

  # Restore from backup
  kubectl odh migrate raycluster post-upgrade --from-backup ./raycluster-backups/rhoai-3.x

  # List migration status
  kubectl odh migrate raycluster list
  kubectl odh migrate raycluster list --namespace my-ns -o json
`

// helpFuncWithShortGlobalFlags renders help for the command but hides the long list
// of root persistent flags (connection/auth). Those flags still apply when running
// the command; they are only omitted from this help output. Callers can run
// 'kubectl odh --help' to see the full list.
func helpFuncWithShortGlobalFlags(c *cobra.Command, args []string) {
	root := c.Root()
	root.PersistentFlags().VisitAll(func(f *pflag.Flag) {
		f.Hidden = true
	})
	defer root.PersistentFlags().VisitAll(func(f *pflag.Flag) {
		f.Hidden = false
	})
	root.HelpFunc()(c, args)
}

// setShortGlobalFlagsHelp sets the custom help func on cmd and all its descendants
// so that help output does not list every global flag.
func setShortGlobalFlagsHelp(cmd *cobra.Command) {
	cmd.SetHelpFunc(helpFuncWithShortGlobalFlags)
	for _, child := range cmd.Commands() {
		setShortGlobalFlagsHelp(child)
	}
}

// AddCommand adds the raycluster subcommand to the migrate command.
func AddCommand(
	parent *cobra.Command,
	flags *genericclioptions.ConfigFlags,
	streams genericiooptions.IOStreams,
) {
	cmd := &cobra.Command{
		Use:           cmdName,
		Short:         cmdShort,
		Long:          cmdLong,
		Example:       cmdExample,
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	preupgrade.AddCommand(cmd, flags, streams)
	postupgrade.AddCommand(cmd, flags, streams)
	list.AddCommand(cmd, flags, streams)

	setShortGlobalFlagsHelp(cmd)
	parent.AddCommand(cmd)
}
