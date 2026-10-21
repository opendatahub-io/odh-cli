package get

import (
	"github.com/spf13/cobra"

	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/genericiooptions"

	getpkg "github.com/opendatahub-io/odh-cli/pkg/get"
	utilcobra "github.com/opendatahub-io/odh-cli/pkg/util/cobra"
	clierrors "github.com/opendatahub-io/odh-cli/pkg/util/errors"
)

const (
	cmdName  = "get"
	cmdShort = "Get ODH/RHOAI resources"
	maxArgs  = 2
)

const cmdLong = `
Display one or more ODH/RHOAI resources without knowing CRD group/version details.

Supported resources:
  notebooks (nb)                              Kubeflow Notebooks
  inferenceservices (isvc)                     KServe InferenceServices
  servingruntimes (sr)                         KServe ServingRuntimes
  datasciencepipelinesapplications (pipeline)  Data Science Pipelines

Table output shows ODH-relevant columns tuned for each resource type.
If the CRD is not installed, a friendly message is shown instead of an error.
`

const cmdExample = `
  # List notebooks in the current namespace
  kubectl odh get nb

  # List inference services across all namespaces
  kubectl odh get isvc -A

  # Get a specific notebook in a namespace
  kubectl odh get nb my-notebook -n my-project

  # List serving runtimes with label filter
  kubectl odh get sr -l app=my-model

  # List pipelines as JSON
  kubectl odh get pipeline -o json
`

// AddCommand adds the get command to the root command.
func AddCommand(root *cobra.Command, flags *genericclioptions.ConfigFlags, opts ...utilcobra.CommandOption) {
	streams := genericiooptions.IOStreams{
		In:     root.InOrStdin(),
		Out:    root.OutOrStdout(),
		ErrOut: root.ErrOrStderr(),
	}

	command := getpkg.NewCommand(streams, flags)

	cmd := &cobra.Command{
		Use:           "get RESOURCE [NAME]",
		Short:         cmdShort,
		Long:          cmdLong,
		Example:       cmdExample,
		SilenceUsage:  true,
		SilenceErrors: true,
		// Note: This closure captures `command` which is mutated by flag binding.
		// This works because Cobra parses flags before calling Args validation.
		Args: func(cmd *cobra.Command, args []string) error {
			// Allow 0 args when --schema is set, but still enforce max
			if command.OutputSchema {
				return cobra.RangeArgs(0, maxArgs)(cmd, args)
			}

			return cobra.RangeArgs(1, maxArgs)(cmd, args)
		},
		ValidArgs: getpkg.Names(),
		ValidArgsFunction: func(_ *cobra.Command, args []string, _ string) ([]string, cobra.ShellCompDirective) {
			if len(args) == 0 {
				return getpkg.Names(), cobra.ShellCompDirectiveNoFileComp
			}

			return nil, cobra.ShellCompDirectiveNoFileComp
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				command.ResourceName = args[0]
			}
			if len(args) > 1 {
				command.ItemName = args[1]
			}

			outputFormat := command.OutputFormat

			if err := command.Complete(); err != nil {
				if clierrors.WriteStructuredError(cmd.ErrOrStderr(), err, outputFormat) {
					return clierrors.NewAlreadyHandledError(err)
				}

				clierrors.WriteTextError(cmd.ErrOrStderr(), err)

				return clierrors.NewAlreadyHandledError(err)
			}

			if err := command.Validate(); err != nil {
				if clierrors.WriteStructuredError(cmd.ErrOrStderr(), err, outputFormat) {
					return clierrors.NewAlreadyHandledError(err)
				}

				clierrors.WriteTextError(cmd.ErrOrStderr(), err)

				return clierrors.NewAlreadyHandledError(err)
			}

			if err := command.Run(cmd.Context()); err != nil {
				if clierrors.WriteStructuredError(cmd.ErrOrStderr(), err, outputFormat) {
					return clierrors.NewAlreadyHandledError(err)
				}

				clierrors.WriteTextError(cmd.ErrOrStderr(), err)

				return clierrors.NewAlreadyHandledError(err)
			}

			return nil
		},
	}

	command.AddFlags(cmd.Flags())

	utilcobra.ApplyOptions(cmd, opts...)

	root.AddCommand(cmd)
}
