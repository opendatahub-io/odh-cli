package postupgrade

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/genericiooptions"

	"github.com/lburgazzoli/odh-cli/pkg/migrate/raycluster"
	"github.com/lburgazzoli/odh-cli/pkg/util/client"
	"github.com/lburgazzoli/odh-cli/pkg/util/iostreams"
)

const (
	cmdName  = "post-upgrade"
	cmdShort = "Migrate RayClusters after RHOAI upgrade"
)

const cmdLong = `
Migrate RayClusters after upgrading from RHOAI 2.x to 3.x.

Two modes:
  1. Live migration (default): Updates existing RayClusters in-place (removes TLS/OAuth, adds annotation).
  2. From backup (--from-backup): Deletes and recreates RayClusters from pre-upgrade backup files.

Recommended: test with one cluster first (--cluster NAME --namespace NS --dry-run), then namespace, then all.
`

const cmdExample = `
  # Preview then migrate a single cluster
  kubectl odh migrate raycluster post-upgrade --cluster my-cluster --namespace my-ns --dry-run
  kubectl odh migrate raycluster post-upgrade --cluster my-cluster --namespace my-ns

  # Migrate all clusters in a namespace
  kubectl odh migrate raycluster post-upgrade --namespace my-ns

  # Migrate all clusters (you will be prompted to confirm)
  kubectl odh migrate raycluster post-upgrade

  # Restore from backup
  kubectl odh migrate raycluster post-upgrade --from-backup ./raycluster-backups/rhoai-3.x

  # Skip confirmation prompt
  kubectl odh migrate raycluster post-upgrade --yes
`

type options struct {
	ConfigFlags *genericclioptions.ConfigFlags
	IO          iostreams.Interface
	Client      client.Client

	ClusterName string
	Namespace   string
	DryRun      bool
	Yes         bool
	FromBackup  string
}

func (o *options) Complete() error {
	restConfig, err := client.NewRESTConfig(o.ConfigFlags, client.DefaultQPS, client.DefaultBurst)
	if err != nil {
		return fmt.Errorf("create REST config: %w", err)
	}
	c, err := client.NewClientWithConfig(restConfig)
	if err != nil {
		return fmt.Errorf("create Kubernetes client: %w", err)
	}
	o.Client = c
	if o.Namespace == "" && o.ConfigFlags.Namespace != nil && *o.ConfigFlags.Namespace != "" {
		o.Namespace = *o.ConfigFlags.Namespace
	}

	return nil
}

func (o *options) Validate() error {
	if o.ClusterName != "" && o.Namespace == "" {
		return fmt.Errorf("namespace is required when cluster is specified")
	}
	return nil
}

func (o *options) Run(ctx context.Context) error {
	_, err := raycluster.PostUpgrade(ctx, o.Client, o.ClusterName, o.Namespace, o.DryRun, o.Yes, o.FromBackup, o.IO)
	return err
}

func (o *options) AddFlags(fs *pflag.FlagSet) {
	fs.StringVarP(&o.ClusterName, "cluster", "c", "", "Migrate a specific cluster (requires --namespace)")
	fs.StringVarP(&o.Namespace, "namespace", "n", "", "Migrate all clusters in this namespace")
	fs.BoolVar(&o.DryRun, "dry-run", false, "Preview changes without applying")
	fs.BoolVarP(&o.Yes, "yes", "y", false, "Skip confirmation prompt")
	fs.StringVar(&o.FromBackup, "from-backup", "", "Restore from backup file or directory (deletes existing cluster first)")
}

// AddCommand adds the post-upgrade subcommand to the raycluster command.
func AddCommand(
	parent *cobra.Command,
	flags *genericclioptions.ConfigFlags,
	streams genericiooptions.IOStreams,
) {
	o := &options{
		ConfigFlags: flags,
		IO:          iostreams.NewIOStreams(streams.In, streams.Out, streams.ErrOut),
	}

	cmd := &cobra.Command{
		Use:           cmdName,
		Short:         cmdShort,
		Long:          cmdLong,
		Example:       cmdExample,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := o.Complete(); err != nil {
				return err
			}
			if err := o.Validate(); err != nil {
				return err
			}
			return o.Run(cmd.Context())
		},
	}

	o.AddFlags(cmd.Flags())
	parent.AddCommand(cmd)
}
