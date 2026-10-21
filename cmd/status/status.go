package status

import (
	"io"

	"github.com/spf13/cobra"

	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/genericiooptions"

	statuspkg "github.com/opendatahub-io/odh-cli/pkg/status"
	utilcobra "github.com/opendatahub-io/odh-cli/pkg/util/cobra"
	clierrors "github.com/opendatahub-io/odh-cli/pkg/util/errors"
)

// handleErr writes the error in structured or text format and returns an already-handled error.
//
//nolint:wrapcheck // NewAlreadyHandledError is a sentinel, not meant to be wrapped
func handleErr(w io.Writer, err error, outputFormat string) error {
	if clierrors.WriteStructuredError(w, err, outputFormat) {
		return clierrors.NewAlreadyHandledError(err)
	}

	clierrors.WriteTextError(w, err)

	return clierrors.NewAlreadyHandledError(err)
}

const (
	cmdName  = "status"
	cmdShort = "Show platform health and version information"
)

const cmdLong = `
Shows the health status of an OpenShift AI / Open Data Hub installation.

Runs health checks across eight sections and displays a summary table:
  Operator, DSCI, DSC, Nodes, Deployments, Pods, Quotas, Events

Status indicators:
  ✓  Section healthy (all checks passed)
  ✗  Section unhealthy (problems found)
  ?  Section skipped (insufficient permissions)

Sections are grouped into layers:
  infrastructure: nodes
  workload:       deployments, pods, events, quotas
  operator:       operator, dsci, dsc

The applications and operator namespaces are auto-detected from the
DSCInitialization resource and OLM ClusterServiceVersions. Use
--apps-namespace and --operator-namespace to override.

By default, also checks required operator dependencies (ServiceMesh,
Serverless, etc.) and shows which components need them.

Use --wait-for=healthy to poll until the platform reaches healthy status.
This is useful for automation and agent workflows that need to wait for
the platform to stabilize after changes. The --timeout flag controls
how long to wait (default 30s; use --timeout=0 for no timeout).
Exit code 5 indicates a timeout.

Use --watch with -o json or -o yaml to stream status changes. Each state
change emits one JSON line (NDJSON) or YAML document. Only changes are
emitted; duplicate states are suppressed. Agents consume this as a stream.

Examples:
  # Show platform health summary
  kubectl odh status

  # Show detailed per-item output
  kubectl odh status --verbose

  # Check only nodes and deployments
  kubectl odh status --section nodes --section deployments

  # Check only operator-related sections
  kubectl odh status --layer operator

  # Output full report as JSON or YAML
  kubectl odh status -o json
  kubectl odh status -o yaml
`

const cmdExample = `
  # Show platform health summary
  kubectl odh status

  # Verbose output with per-item details
  kubectl odh status --verbose

  # Filter to specific sections
  kubectl odh status --section operator --section dsci --section dsc

  # Filter by layer (infrastructure, workload, operator)
  kubectl odh status --layer operator

  # JSON or YAML output for scripting
  kubectl odh status -o json
  kubectl odh status -o yaml

  # Skip dependency checking
  kubectl odh status --include-deps=false

  # Override namespace detection
  kubectl odh status --apps-namespace my-apps --operator-namespace my-operator

  # Wait until the platform is healthy (5 minute timeout)
  kubectl odh status --wait-for=healthy --timeout=300s

  # Wait with custom poll interval
  kubectl odh status --wait-for=healthy --timeout=300s --poll-interval=5s

  # Wait indefinitely (no timeout)
  kubectl odh status --wait-for=healthy --timeout=0

  # Watch status changes as streaming JSON (NDJSON)
  kubectl odh status --watch -o json

  # Watch with custom poll interval
  kubectl odh status --watch -o json --poll-interval=30s

  # Watch as YAML stream
  kubectl odh status --watch -o yaml
`

// AddCommand adds the status command to the root command.
func AddCommand(root *cobra.Command, flags *genericclioptions.ConfigFlags, opts ...utilcobra.CommandOption) {
	streams := genericiooptions.IOStreams{
		In:     root.InOrStdin(),
		Out:    root.OutOrStdout(),
		ErrOut: root.ErrOrStderr(),
	}

	command := statuspkg.NewCommand(streams, flags)

	cmd := &cobra.Command{
		Use:           cmdName,
		Short:         cmdShort,
		Long:          cmdLong,
		Example:       cmdExample,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			errOut := cmd.ErrOrStderr()
			outputFormat := string(command.OutputFormat)
			command.TimeoutExplicit = cmd.Flags().Changed("timeout")

			if err := command.Complete(); err != nil {
				return handleErr(errOut, err, outputFormat)
			}

			if err := command.Validate(); err != nil {
				return handleErr(errOut, err, outputFormat)
			}

			if err := command.Run(cmd.Context()); err != nil {
				return handleErr(errOut, err, outputFormat)
			}

			return nil
		},
	}

	command.AddFlags(cmd.Flags())

	utilcobra.ApplyOptions(cmd, opts...)

	root.AddCommand(cmd)
}
