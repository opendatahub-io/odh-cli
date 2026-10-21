package version

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"runtime"

	"github.com/spf13/cobra"

	"k8s.io/cli-runtime/pkg/genericclioptions"

	"github.com/opendatahub-io/odh-cli/internal/version"
	"github.com/opendatahub-io/odh-cli/pkg/api"
	"github.com/opendatahub-io/odh-cli/pkg/schema"
	utilcobra "github.com/opendatahub-io/odh-cli/pkg/util/cobra"
)

const (
	cmdName  = "version"
	cmdShort = "Show version information"
)

func writeJSONVersion(out io.Writer, data map[string]string) error {
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")

	if err := encoder.Encode(data); err != nil {
		return fmt.Errorf("failed to encode version information as JSON: %w", err)
	}

	return nil
}

func writeTextVersion(out io.Writer, format string, args ...any) error {
	if _, err := fmt.Fprintf(out, format, args...); err != nil {
		return fmt.Errorf("failed to write version information: %w", err)
	}

	return nil
}

// AddCommand adds the version subcommand to the root command.
func AddCommand(root *cobra.Command, _ *genericclioptions.ConfigFlags, opts ...utilcobra.CommandOption) {
	var (
		outputFormat string
		outputSchema bool
		verbose      bool
		quiet        bool
	)

	cmd := &cobra.Command{
		Use:           cmdName,
		Short:         cmdShort,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			// Validate flag combinations before any output
			if verbose && quiet {
				return errors.New("--verbose and --quiet are mutually exclusive")
			}

			if outputSchema && quiet {
				return errors.New("--schema and --quiet cannot be used together")
			}

			// Short-circuit if --schema was requested
			if outputSchema {
				schemaType := schema.SchemaVersionInfo
				if verbose {
					schemaType = schema.SchemaVersionInfoVerbose
				}

				if err := schema.WriteTo(cmd.OutOrStdout(), schemaType); err != nil {
					return fmt.Errorf("outputting schema: %w", err)
				}

				return nil
			}

			// Determine output writer (suppress if quiet)
			out := cmd.OutOrStdout()
			if quiet {
				out = io.Discard
			}

			data := map[string]string{
				"name":    version.GetCLIName(),
				"version": version.GetVersion(),
				"commit":  version.GetCommit(),
				"date":    version.GetDate(),
			}

			if verbose {
				data["goVersion"] = runtime.Version()
				data["platform"] = runtime.GOOS + "/" + runtime.GOARCH
			}

			if outputFormat == "json" {
				return writeJSONVersion(out, data)
			}

			if verbose {
				return writeTextVersion(out,
					"%s version %s\n  Commit:     %s\n  Built:      %s\n  Go version: %s\n  Platform:   %s\n",
					version.GetCLIName(), data["version"], data["commit"], data["date"], data["goVersion"], data["platform"])
			}

			return writeTextVersion(out,
				"%s version %s (commit: %s, built: %s)\n",
				version.GetCLIName(), data["version"], data["commit"], data["date"])
		},
	}

	cmd.Flags().StringVarP(&outputFormat, "output", "o", "text", "Output format (text|json)")
	_ = cmd.Flags().SetAnnotation("output", api.AnnotationValidValues, []string{"text", "json"})
	cmd.Flags().BoolVar(&outputSchema, "schema", false, "Output JSON Schema for the command's structured output format")
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose output")
	cmd.Flags().BoolVarP(&quiet, "quiet", "q", false, "Suppress all output")

	utilcobra.ApplyOptions(cmd, opts...)

	root.AddCommand(cmd)
}
