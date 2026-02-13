package lint

import (
	"fmt"

	lintpkg "github.com/opendatahub-io/odh-cli/pkg/lint"
	"github.com/spf13/cobra"

	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/genericiooptions"
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
			// Complete phase
			if err := command.Complete(); err != nil {
				if command.Verbose {
					_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Error: %v\n", err)
				}

				return fmt.Errorf("completing command: %w", err)
			}

			// Validate phase
			if err := command.Validate(); err != nil {
				if command.Verbose {
					_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Error: %v\n", err)
				}

				return fmt.Errorf("validating command: %w", err)
			}

			// Run phase
			err := command.Run(cmd.Context())
			if err != nil {
				if command.Verbose {
					_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Error: %v\n", err)
				}

				return fmt.Errorf("running command: %w", err)
			}

			return nil
		},
	}

	// Register flags using AddFlags method
	command.AddFlags(cmd.Flags())

	root.AddCommand(cmd)
}
