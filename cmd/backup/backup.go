package backup

import (
	"fmt"

	backuppkg "github.com/opendatahub-io/odh-cli/pkg/backup"
	"github.com/spf13/cobra"

	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/genericiooptions"
)

const (
	cmdName  = "backup"
	cmdShort = "Backup OpenShift AI workloads and dependencies"
)

const cmdLong = `
Backs up OpenShift AI workloads (notebooks, etc.) and their dependencies
(ConfigMaps, Secrets, PVCs) to a directory structure.

The backup command:
  - Discovers workload resources based on --includes/--exclude filters
  - For each workload, identifies and backs up referenced dependencies
  - Strips cluster-specific metadata for portability
  - Organizes backups by namespace: $output-dir/$namespace/$GVR-$name.yaml

Examples:
  # Backup all notebooks to /tmp/backup
  odh-cli backup --output-dir /tmp/backup

  # Backup to stdout
  odh-cli backup > backup.yaml

  # Backup with custom includes/excludes
  odh-cli backup --output-dir /backup \
    --includes notebooks.kubeflow.org \
    --includes inferenceservices.serving.kserve.io \
    --exclude datasciencepipelinesapplications.opendatahub.io

  # Strip additional fields
  odh-cli backup --output-dir /backup \
    --strip ".spec.customField"
`

const cmdExample = `
  # Backup all notebooks to /tmp/backup
  odh-cli backup --output-dir /tmp/backup

  # Backup to stdout
  odh-cli backup > backup.yaml

  # Backup with verbose output
  odh-cli backup --output-dir /backup -v

  # Strip additional fields
  odh-cli backup --output-dir /backup --strip ".spec.customField"
`

// AddCommand adds the backup command to the root command.
func AddCommand(root *cobra.Command, flags *genericclioptions.ConfigFlags) {
	streams := genericiooptions.IOStreams{
		In:     root.InOrStdin(),
		Out:    root.OutOrStdout(),
		ErrOut: root.ErrOrStderr(),
	}

	command := backuppkg.NewCommand(streams)
	command.ConfigFlags = flags

	cmd := &cobra.Command{
		Use:           cmdName,
		Short:         cmdShort,
		Long:          cmdLong,
		Example:       cmdExample,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := command.Complete(); err != nil {
				return fmt.Errorf("completing command: %w", err)
			}
			if err := command.Validate(); err != nil {
				return fmt.Errorf("validating command: %w", err)
			}

			return command.Run(cmd.Context())
		},
	}

	command.AddFlags(cmd.Flags())
	root.AddCommand(cmd)
}
