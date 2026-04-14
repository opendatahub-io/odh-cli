package mustgather

import (
	"fmt"

	"github.com/spf13/cobra"

	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/genericiooptions"

	mustgatherpkg "github.com/opendatahub-io/odh-cli/pkg/cmd/mustgather"
)

const (
	cmdName  = "must-gather"
	cmdShort = "Collect diagnostic information from OpenShift AI clusters"
)

const cmdLong = `
Collects diagnostic information from OpenShift AI clusters for troubleshooting
and support purposes.

IMPORTANT: This command requires the must-gather collection scripts for the current implementation.
NOTE: Currently only the 'llm-d' component is supported.

Usage options:
  1. Container image (recommended) - Scripts pre-bundled at /opt/must-gather
  2. Local binary - Use --scripts-path to point to a local must-gather clone
     Example: git clone https://github.com/red-hat-data-services/must-gather.git
              kubectl-odh must-gather --scripts-path ./must-gather/collection-scripts

INVOCATION:
  Depending on your setup:
    - Container:       podman run -v $KUBECONFIG:/kubeconfig <image> must-gather --component llm-d
    - kubectl plugin:  kubectl odh must-gather --component llm-d
    - Direct binary:   kubectl-odh must-gather --scripts-path <path> --component llm-d

Environment Variables:
  Command-line flags always take precedence over environment variables.
  - COMPONENT: Overridden by --component flag
  - MUST_GATHER_SINCE: Overridden by --since flag

Output is written to /tmp/must-gather by default.
`

const cmdExample = `
  # Container image - mount output directory (requires permissions)
  GATHER_DIR="./must-gather.local.$(date +%s)"
  mkdir -p "$GATHER_DIR" && chmod 777 "$GATHER_DIR"
  podman run --rm -ti \
    -v $KUBECONFIG:/kubeconfig:Z \
    -v "$GATHER_DIR":/tmp/must-gather:Z \
    <image> must-gather --component llm-d

  # Container image - copy files after (cleaner, no permission issues)
  podman run -ti --name mg -v $KUBECONFIG:/kubeconfig:Z <image> must-gather --component llm-d
  podman cp mg:/tmp/must-gather ./must-gather.local.$(date +%s)
  podman rm mg

  # Local binary (requires must-gather repository clone)
  git clone https://github.com/red-hat-data-services/must-gather.git
  kubectl odh must-gather \
    --scripts-path ./must-gather/collection-scripts \
    --component llm-d

  # Collect with time range for logs (last 1 hour)
  kubectl odh must-gather \
    --scripts-path ./must-gather/collection-scripts \
    --component llm-d \
    --since 1h

  # List available components (doesn't require scripts)
  kubectl odh must-gather --list-components
`

// AddCommand adds the must-gather command to the root command.
func AddCommand(root *cobra.Command, flags *genericclioptions.ConfigFlags) {
	streams := genericiooptions.IOStreams{
		In:     root.InOrStdin(),
		Out:    root.OutOrStdout(),
		ErrOut: root.ErrOrStderr(),
	}

	command := mustgatherpkg.NewCommand(streams, flags)

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
