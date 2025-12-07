package doctor

import (
	"fmt"

	"github.com/spf13/cobra"

	"k8s.io/cli-runtime/pkg/genericclioptions"

	doctorcmd "github.com/lburgazzoli/odh-cli/pkg/cmd/doctor"
)

const (
	lintCmdName  = "lint"
	lintCmdShort = "Validate current OpenShift AI installation"
	lintCmdLong  = `
Validates the current OpenShift AI installation and reports configuration issues.

The lint command performs comprehensive validation across three categories:
  - Components: Core OpenShift AI components (Dashboard, Workbenches, etc.)
  - Services: Platform services (OAuth, monitoring, etc.)
  - Workloads: User-created custom resources (Notebooks, InferenceServices, etc.)

Each issue is reported with:
  - Severity level (Critical, Warning, Info)
  - Detailed description of the problem
  - Remediation guidance for fixing the issue

Examples:
  # Validate entire cluster
  kubectl odh doctor lint

  # Validate with JSON output
  kubectl odh doctor lint -o json

  # Validate only component checks
  kubectl odh doctor lint --checks "components/*"
`
	lintCmdExample = `
  # Validate entire cluster
  kubectl odh doctor lint

  # Output results in JSON format
  kubectl odh doctor lint -o json

  # Run only dashboard-related checks
  kubectl odh doctor lint --checks "*dashboard*"
`
)

// AddLintCommand adds the lint subcommand to the doctor command.
func AddLintCommand(parent *cobra.Command, flags *genericclioptions.ConfigFlags) {
	streams := genericclioptions.IOStreams{
		In:     parent.InOrStdin(),
		Out:    parent.OutOrStdout(),
		ErrOut: parent.ErrOrStderr(),
	}

	sharedOpts := doctorcmd.NewSharedOptions(streams)
	opts := doctorcmd.NewLintOptions(sharedOpts)

	// Use the ConfigFlags from parent instead of creating new ones
	sharedOpts.ConfigFlags = flags

	cmd := &cobra.Command{
		Use:     lintCmdName,
		Short:   lintCmdShort,
		Long:    lintCmdLong,
		Example: lintCmdExample,
		RunE: func(cmd *cobra.Command, _ []string) error {
			// Complete phase
			if err := opts.Complete(); err != nil {
				return fmt.Errorf("completing lint options: %w", err)
			}

			// Validate phase
			if err := opts.Validate(); err != nil {
				return fmt.Errorf("validating lint options: %w", err)
			}

			// Run phase
			return opts.Run(cmd.Context())
		},
	}

	// Add flags
	cmd.Flags().StringVarP((*string)(&sharedOpts.OutputFormat), "output", "o", string(doctorcmd.OutputFormatTable),
		"Output format (table|json|yaml)")
	cmd.Flags().StringVar(&sharedOpts.CheckSelector, "checks", "*",
		"Glob pattern to filter which checks to run (e.g., 'components/*', '*dashboard*')")
	cmd.Flags().StringVar((*string)(&sharedOpts.MinSeverity), "severity", "",
		"Filter results by minimum severity level (critical|warning|info)")
	cmd.Flags().BoolVar(&sharedOpts.FailOnCritical, "fail-on-critical", true,
		"Exit with non-zero code if Critical findings detected")
	cmd.Flags().BoolVar(&sharedOpts.FailOnWarning, "fail-on-warning", false,
		"Exit with non-zero code if Warning findings detected")
	cmd.Flags().DurationVar(&sharedOpts.Timeout, "timeout", sharedOpts.Timeout,
		"Maximum duration for command execution (e.g., 5m, 10m)")

	parent.AddCommand(cmd)
}
