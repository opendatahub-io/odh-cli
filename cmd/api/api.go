package api

import (
	"github.com/spf13/cobra"

	"k8s.io/cli-runtime/pkg/genericclioptions"

	apipkg "github.com/opendatahub-io/odh-cli/pkg/api"
	utilcobra "github.com/opendatahub-io/odh-cli/pkg/util/cobra"
)

const (
	cmdName  = "api"
	cmdShort = "Output a JSON manifest describing all CLI commands and flags"
)

// AddCommand adds the api subcommand to the root command.
func AddCommand(root *cobra.Command, _ *genericclioptions.ConfigFlags, opts ...utilcobra.CommandOption) {
	cmd := &cobra.Command{
		Use:           cmdName,
		Short:         cmdShort,
		Hidden:        true,
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return apipkg.Run(root, cmd.OutOrStdout())
		},
	}

	utilcobra.ApplyOptions(cmd, opts...)

	root.AddCommand(cmd)
}
