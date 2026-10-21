package events

import (
	"io"

	"github.com/spf13/cobra"

	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/genericiooptions"

	eventspkg "github.com/opendatahub-io/odh-cli/pkg/events"
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
	cmdName  = "events"
	cmdShort = "Show events for ODH resources"
)

const cmdLong = `
Shows Kubernetes events related to Open Data Hub / OpenShift AI resources.

Events are fetched from ODH namespaces (applications, operator, monitoring)
using the clusterhealth library. The namespaces are auto-detected from the
DSCInitialization resource.
`

const cmdExample = `
  # Show recent events
  kubectl odh events

  # Show events from the last 30 minutes
  kubectl odh events --since 30m

  # Show only warnings
  kubectl odh events --type Warning

  # Filter by component
  kubectl odh events --component kserve

  # Stream events in real-time
  kubectl odh events -f

  # Combine flags
  kubectl odh events --component dashboard -f

  # All ODH namespaces, YAML output
  kubectl odh events -A -o yaml
`

// AddCommand adds the events command to the root command.
func AddCommand(root *cobra.Command, flags *genericclioptions.ConfigFlags, opts ...utilcobra.CommandOption) {
	streams := genericiooptions.IOStreams{
		In:     root.InOrStdin(),
		Out:    root.OutOrStdout(),
		ErrOut: root.ErrOrStderr(),
	}

	command := eventspkg.NewCommand(streams, flags)

	cmd := &cobra.Command{
		Use:           cmdName,
		Short:         cmdShort,
		Long:          cmdLong,
		Example:       cmdExample,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			errOut := cmd.ErrOrStderr()
			outputFormat := command.OutputFormat

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

	// Shell completion for --component flag
	_ = cmd.RegisterFlagCompletionFunc("component", func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
		return eventspkg.ValidComponents(), cobra.ShellCompDirectiveNoFileComp
	})

	utilcobra.ApplyOptions(cmd, opts...)

	root.AddCommand(cmd)
}
