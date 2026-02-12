package preupgrade

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
	cmdName  = "pre-upgrade"
	cmdShort = "Backup RayClusters and run pre-flight checks before RHOAI upgrade"
)

const cmdLong = `
Run pre-flight checks and backup RayCluster configurations before upgrading from RHOAI 2.x to 3.x.

Creates two backup subdirectories:
  <output-dir>/rhoai-2.x  - RHOAI 2.x compatible (for rollback)
  <output-dir>/rhoai-3.x  - RHOAI 3.x compatible (use with post-upgrade --from-backup)

Run this BEFORE performing the RHOAI upgrade. After the upgrade, use 'post-upgrade' to migrate.
`

const cmdExample = `
  # Backup all RayClusters (default directory ./raycluster-backups)
  kubectl odh migrate raycluster pre-upgrade

  # Backup to a specific directory
  kubectl odh migrate raycluster pre-upgrade --output-dir ./my-backups

  # Backup a specific namespace
  kubectl odh migrate raycluster pre-upgrade --namespace my-ns

  # Backup a single cluster
  kubectl odh migrate raycluster pre-upgrade --cluster my-cluster --namespace my-ns
`

type options struct {
	ConfigFlags *genericclioptions.ConfigFlags
	IO          iostreams.Interface
	Client      client.Client

	OutputDir    string
	ClusterName  string
	Namespace    string
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

	if o.OutputDir == "" {
		o.OutputDir = raycluster.DefaultBackupDir
	}
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
	checks := raycluster.RunPreUpgradeChecks(ctx, o.Client)
	saved, err := raycluster.PreUpgrade(ctx, o.Client, o.OutputDir, o.ClusterName, o.Namespace, checks, o.IO)
	if err != nil {
		return err
	}
	if len(saved) == 0 {
		return nil
	}
	o.IO.Errorf("")
	o.IO.Errorf("Backup complete: %d RayCluster(s) saved to %s", len(saved), o.OutputDir)
	o.IO.Errorf("")
	o.IO.Errorf("Backup directory structure:")
	o.IO.Errorf("  %s/", o.OutputDir)
	o.IO.Errorf("    %s/  - RHOAI 2.x compatible (use if you did not proceed with the 3.x upgrade)", raycluster.BackupSubdirRHOAI2x)
	o.IO.Errorf("    %s/  - RHOAI 3.x compatible (use with post-upgrade --from-backup)", raycluster.BackupSubdirRHOAI3x)
	o.IO.Errorf("")
	o.IO.Errorf("WARNING: The 'rhoai-2.x/' backups contain CodeFlare-operator components.")
	o.IO.Errorf("         Use 'rhoai-2.x/' ONLY if attempting to restore RayClusters but did not proceed with the RHOAI 3.x upgrade.")
	o.IO.Errorf("         Use 'rhoai-3.x/' for proceeding with the RHOAI 3.x upgrade.")
	o.IO.Errorf("")
	o.IO.Errorf("Next steps:")
	o.IO.Errorf("  1. Perform the RHOAI upgrade")
	o.IO.Errorf("  2. Run 'post-upgrade' to migrate the RayClusters")
	return nil
}

func (o *options) AddFlags(fs *pflag.FlagSet) {
	fs.StringVar(&o.OutputDir, "output-dir", "", "Directory for backup YAML files (default: ./raycluster-backups)")
	fs.StringVarP(&o.ClusterName, "cluster", "c", "", "Backup a specific cluster (requires --namespace)")
	fs.StringVarP(&o.Namespace, "namespace", "n", "", "Backup all clusters in this namespace")
}

// AddCommand adds the pre-upgrade subcommand to the raycluster command.
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
