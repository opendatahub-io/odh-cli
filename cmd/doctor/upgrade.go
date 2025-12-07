package doctor

import (
	"fmt"

	"github.com/spf13/cobra"

	"k8s.io/cli-runtime/pkg/genericclioptions"

	doctorcmd "github.com/lburgazzoli/odh-cli/pkg/cmd/doctor"
)

const (
	upgradeCmdName  = "upgrade"
	upgradeCmdShort = "Assess upgrade readiness for target OpenShift AI version"
	upgradeCmdLong  = `
Assesses upgrade readiness for a target OpenShift AI version.

The upgrade command identifies compatibility issues before upgrading:
  - API version deprecations and removals
  - Component configuration changes
  - Breaking changes in custom resources
  - Required migration steps

Each issue is categorized as:
  - Blocking: Must be resolved before upgrade
  - Warning: Should be addressed (non-breaking)
  - Info: Advisory notices about changes

Examples:
  # Check upgrade readiness to version 3.0
  kubectl odh doctor upgrade --version 3.0

  # Check upgrade with JSON output
  kubectl odh doctor upgrade --version 3.1 -o json

  # Show only blocking issues
  kubectl odh doctor upgrade --version 3.0 --severity critical
`
	upgradeCmdExample = `
  # Check upgrade readiness to version 3.0
  kubectl odh doctor upgrade --version 3.0

  # Output results in JSON format
  kubectl odh doctor upgrade --version 3.1 --output json

  # Show only blocking issues
  kubectl odh doctor upgrade --version 3.0 --severity critical
`
)

// AddUpgradeCommand adds the upgrade subcommand to the doctor command.
func AddUpgradeCommand(parent *cobra.Command, flags *genericclioptions.ConfigFlags) {
	streams := genericclioptions.IOStreams{
		In:     parent.InOrStdin(),
		Out:    parent.OutOrStdout(),
		ErrOut: parent.ErrOrStderr(),
	}

	sharedOpts := doctorcmd.NewSharedOptions(streams)
	opts := doctorcmd.NewUpgradeOptions(sharedOpts)

	// Use the ConfigFlags from parent instead of creating new ones
	sharedOpts.ConfigFlags = flags

	cmd := &cobra.Command{
		Use:     upgradeCmdName,
		Short:   upgradeCmdShort,
		Long:    upgradeCmdLong,
		Example: upgradeCmdExample,
		RunE: func(cmd *cobra.Command, _ []string) error {
			// Complete phase
			if err := opts.Complete(); err != nil {
				return fmt.Errorf("completing upgrade options: %w", err)
			}

			// Validate phase
			if err := opts.Validate(); err != nil {
				return fmt.Errorf("validating upgrade options: %w", err)
			}

			// Run phase
			return opts.Run(cmd.Context())
		},
	}

	// Add flags
	cmd.Flags().StringVar(&opts.TargetVersion, "version", "",
		"Target OpenShift AI version for upgrade assessment (required)")
	_ = cmd.MarkFlagRequired("version")

	cmd.Flags().StringVarP((*string)(&sharedOpts.OutputFormat), "output", "o", string(doctorcmd.OutputFormatTable),
		"Output format (table|json|yaml)")
	cmd.Flags().StringVar(&sharedOpts.CheckSelector, "checks", "*",
		"Glob pattern to filter which checks to run (e.g., 'components/*', '*upgrade*')")
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
