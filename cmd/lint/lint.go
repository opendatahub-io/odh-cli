package lint

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/genericiooptions"

	lintpkg "github.com/opendatahub-io/odh-cli/pkg/lint"
	utilcobra "github.com/opendatahub-io/odh-cli/pkg/util/cobra"
	clierrors "github.com/opendatahub-io/odh-cli/pkg/util/errors"
)

const (
	cmdName  = "lint"
	cmdShort = "Validate current OpenShift AI installation or assess upgrade readiness"
)

const cmdLong = `
Validates the current OpenShift AI installation or assesses upgrade readiness.

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
  kubectl odh lint

  # Assess upgrade readiness for version 3.0
  kubectl odh lint --target-version 3.0

  # Validate with JSON output
  kubectl odh lint -o json

  # Validate only component checks
  kubectl odh lint --checks "components"
`
const cmdExample = `
  # Validate current cluster state
  kubectl odh lint

  # Assess upgrade readiness for version 3.0
  kubectl odh lint --target-version 3.0

  # Output results in JSON format
  kubectl odh lint -o json

  # Run only dashboard-related checks
  kubectl odh lint --checks "*dashboard*"

  # Check upgrade readiness to version 3.1
  kubectl odh lint --target-version 3.1
`

// wrapHandledError wraps an error as already-handled with its derived exit code,
// used when the error has been rendered to output and should not be printed again.
func wrapHandledError(err error) error {
	//nolint:wrapcheck // NewAlreadyHandledError is a same-module constructor
	return clierrors.NewAlreadyHandledError(
		clierrors.NewExitCodeError(clierrors.ExitCodeFromError(err), err),
	)
}

// AddCommand adds the lint command to the root command.
func AddCommand(root *cobra.Command, flags *genericclioptions.ConfigFlags, opts ...utilcobra.CommandOption) {
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
					return wrapHandledError(err)
				}

				if command.Verbose {
					_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Error: %v\n", err)
					clierrors.WriteSuggestion(cmd.ErrOrStderr(), err)
				} else {
					clierrors.WriteTextError(cmd.ErrOrStderr(), err)
				}

				return wrapHandledError(err)
			}

			// Validate phase
			if err := command.Validate(); err != nil {
				exitErr := clierrors.NewExitCodeError(clierrors.ExitValidation, err)

				if clierrors.WriteStructuredError(cmd.ErrOrStderr(), exitErr, outputFormat) {
					return clierrors.NewAlreadyHandledError(exitErr)
				}

				if command.Verbose {
					_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Error: %v\n", err)
					clierrors.WriteSuggestion(cmd.ErrOrStderr(), err)
				} else {
					clierrors.WriteTextError(cmd.ErrOrStderr(), err)
				}

				return clierrors.NewAlreadyHandledError(exitErr)
			}

			// Run phase
			err := command.Run(cmd.Context())
			if err != nil {
				// Verdict errors (findings already rendered) propagate directly
				if errors.Is(err, clierrors.ErrAlreadyHandled) {
					return err //nolint:wrapcheck // already wrapped by NewAlreadyHandledError
				}

				if clierrors.WriteStructuredError(cmd.ErrOrStderr(), err, outputFormat) {
					return wrapHandledError(err)
				}

				if command.Verbose {
					_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Error: %v\n", err)
					clierrors.WriteSuggestion(cmd.ErrOrStderr(), err)
				} else {
					clierrors.WriteTextError(cmd.ErrOrStderr(), err)
				}

				return wrapHandledError(err)
			}

			return nil
		},
	}

	// Register flags using AddFlags method
	command.AddFlags(cmd.Flags())

	utilcobra.ApplyOptions(cmd, opts...)

	root.AddCommand(cmd)
}
