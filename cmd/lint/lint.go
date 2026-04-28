package lint

import (
	"fmt"

	"github.com/spf13/cobra"

	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/genericiooptions"

	lintpkg "github.com/opendatahub-io/odh-cli/pkg/lint"
	clierrors "github.com/opendatahub-io/odh-cli/pkg/util/errors"
)

const (
	cmdName  = "lint"
	cmdShort = "Validate current OpenShift AI installation or assess upgrade readiness"
)

const cmdLong = `
Validates the current OpenShift AI installation or assesses upgrade readiness.

INVOCATION:
  The examples below use 'odh-cli'. Depending on your setup, substitute with:
    - Container:       podman|docker run <image> lint ...
    - kubectl plugin:  kubectl odh lint ...
    - Direct binary:   odh-cli lint ...

LINT MODE (without --target-version):
  Validates the current cluster state and reports configuration issues.

UPGRADE MODE (with --target-version):
  Assesses upgrade readiness by comparing current version against target version.

The lint command performs comprehensive validation across four categories:
  - Components: Core OpenShift AI components (Dashboard, Workbenches, etc.)
  - Services: Platform services (OAuth, monitoring, etc.)
  - Dependencies: External dependencies (CertManager, Kueue, etc.)
  - Workloads: User-created custom resources (Notebooks, InferenceServices, etc.)

Each issue is reported with:
  - Severity level (Critical, Warning, Info)
  - Detailed description of the problem
  - Remediation guidance for fixing the issue

Examples:
  # Validate current cluster state
  odh-cli lint

  # Assess upgrade readiness for version 3.0
  odh-cli lint --target-version 3.0

  # Validate with JSON output
  odh-cli lint -o json

  # Validate only component checks
  odh-cli lint --checks "components"
`
const cmdExample = `
  # Validate current cluster state
  odh-cli lint

  # Assess upgrade readiness for version 3.0
  odh-cli lint --target-version 3.0

  # Output results in JSON format
  odh-cli lint -o json

  # Run only dashboard-related checks
  odh-cli lint --checks "*dashboard*"

  # Check upgrade readiness to version 3.1
  odh-cli lint --target-version 3.1
`

// AddCommand adds the lint command to the root command.
func AddCommand(root *cobra.Command, flags *genericclioptions.ConfigFlags) {
	streams := genericiooptions.IOStreams{
		In:     root.InOrStdin(),
		Out:    root.OutOrStdout(),
		ErrOut: root.ErrOrStderr(),
	}

	// Create command with ConfigFlags from parent to ensure CLI auth flags are used
	command := lintpkg.NewCommand(streams, flags)

	cmd := &cobra.Command{
		Use:           cmdName,
		Short:         cmdShort,
		Long:          cmdLong,
		Example:       cmdExample,
		SilenceUsage:  true,
		SilenceErrors: true, // We'll handle error output manually based on --quiet flag
		RunE: func(cmd *cobra.Command, _ []string) error {
			outputFormat := string(command.OutputFormat)

			// Complete phase
			if err := command.Complete(); err != nil {
				if clierrors.WriteStructuredError(cmd.ErrOrStderr(), err, outputFormat) {
					return clierrors.NewAlreadyHandledError(err)
				}

				if command.Verbose {
					_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Error: %v\n", err)
					clierrors.WriteSuggestion(cmd.ErrOrStderr(), err)
				} else {
					clierrors.WriteTextError(cmd.ErrOrStderr(), err)
				}

				return clierrors.NewAlreadyHandledError(err)
			}

			// Validate phase
			if err := command.Validate(); err != nil {
				if clierrors.WriteStructuredError(cmd.ErrOrStderr(), err, outputFormat) {
					return clierrors.NewAlreadyHandledError(err)
				}

				if command.Verbose {
					_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Error: %v\n", err)
					clierrors.WriteSuggestion(cmd.ErrOrStderr(), err)
				} else {
					clierrors.WriteTextError(cmd.ErrOrStderr(), err)
				}

				return clierrors.NewAlreadyHandledError(err)
			}

			// Run phase
			err := command.Run(cmd.Context())
			if err != nil {
				if clierrors.WriteStructuredError(cmd.ErrOrStderr(), err, outputFormat) {
					return clierrors.NewAlreadyHandledError(err)
				}

				if command.Verbose {
					_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Error: %v\n", err)
					clierrors.WriteSuggestion(cmd.ErrOrStderr(), err)
				} else {
					clierrors.WriteTextError(cmd.ErrOrStderr(), err)
				}

				return clierrors.NewAlreadyHandledError(err)
			}

			return nil
		},
	}

	// Register flags using AddFlags method
	command.AddFlags(cmd.Flags())

	root.AddCommand(cmd)
}
