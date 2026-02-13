package version

import (
	"encoding/json"
	"fmt"

	"github.com/opendatahub-io/odh-cli/internal/version"
	"github.com/spf13/cobra"

	"k8s.io/cli-runtime/pkg/genericclioptions"
)

const (
	cmdName  = "version"
	cmdShort = "Show version information"
)

// AddCommand adds the version subcommand to the root command.
func AddCommand(root *cobra.Command, _ *genericclioptions.ConfigFlags) {
	var outputFormat string

	cmd := &cobra.Command{
		Use:          cmdName,
		Short:        cmdShort,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			switch outputFormat {
			case "json":
				encoder := json.NewEncoder(cmd.OutOrStdout())
				encoder.SetIndent("", "  ")

				err := encoder.Encode(map[string]string{
					"version": version.GetVersion(),
					"commit":  version.GetCommit(),
					"date":    version.GetDate(),
				})

				if err != nil {
					return fmt.Errorf("failed to encode version information as JSON: %w", err)
				}

				return nil
			default:
				_, err := fmt.Fprintf(
					cmd.OutOrStdout(),
					"kubectl-odh version %s (commit: %s, built: %s)\n",
					version.GetVersion(),
					version.GetCommit(),
					version.GetDate(),
				)

				if err != nil {
					return fmt.Errorf("failed to write version information: %w", err)
				}

				return nil
			}
		},
	}

	cmd.Flags().StringVarP(&outputFormat, "output", "o", "text", "Output format (text|json)")

	root.AddCommand(cmd)
}
