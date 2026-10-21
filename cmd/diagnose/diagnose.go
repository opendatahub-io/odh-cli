package diagnose

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/spf13/cobra"

	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/rest"

	"github.com/opendatahub-io/odh-cli/pkg/diagnose"
	utilclient "github.com/opendatahub-io/odh-cli/pkg/util/client"
	utilcobra "github.com/opendatahub-io/odh-cli/pkg/util/cobra"
	clierrors "github.com/opendatahub-io/odh-cli/pkg/util/errors"
)

const (
	cmdName  = "diagnose"
	cmdShort = "Diagnose ODH/RHOAI platform issues"

	defaultTimeout      = 5 * time.Minute
	defaultAppsNS       = "opendatahub"
	defaultOperatorNS   = "opendatahub-operator-system"
	defaultOperatorName = "opendatahub-operator-controller-manager"
)

const cmdLong = `
Runs a 4-step diagnostic flow against the ODH/RHOAI platform:

  1. Triage     - full health check across all sections
  2. Investigate - failure classification + recent warning events
  3. Correlate  - per-component status for unhealthy components
  4. Report     - human-readable or JSON output

Exit code 0 means healthy, 1 means issues found.

For AI-assisted diagnosis use: kubectl odh mcp serve
`

const cmdExample = `
  # Full triage (human-readable)
  kubectl odh diagnose

  # Scope to one component
  kubectl odh diagnose --component kserve

  # Machine-readable output (CI)
  kubectl odh diagnose --json
`

// AddCommand adds the diagnose command to the root command.
func AddCommand(root *cobra.Command, flags *genericclioptions.ConfigFlags, opts ...utilcobra.CommandOption) {
	var (
		jsonOutput  bool
		component   string
		eventsSince time.Duration
	)

	cmd := &cobra.Command{
		Use:           cmdName,
		Short:         cmdShort,
		Long:          cmdLong,
		Example:       cmdExample,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			restConfig, err := utilclient.NewRESTConfig(flags, 0, 0)
			if err != nil {
				return fmt.Errorf("kubeconfig: %w", err)
			}

			return runDiagnostic(cmd, restConfig, jsonOutput, component, eventsSince)
		},
	}

	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output report as JSON")
	cmd.Flags().StringVar(&component, "component", "", "Scope diagnosis to a single component (e.g. kserve)")
	cmd.Flags().DurationVar(&eventsSince, "events-since", diagnose.DefaultEventsSince, "Look-back window for recent events")

	utilcobra.ApplyOptions(cmd, opts...)

	root.AddCommand(cmd)
}

func runDiagnostic(cmd *cobra.Command, restConfig *rest.Config, jsonOutput bool, component string, eventsSince time.Duration) error {
	ctx, cancel := context.WithTimeout(cmd.Context(), defaultTimeout)
	defer cancel()

	cliClient, err := utilclient.NewClientWithConfig(restConfig)
	if err != nil {
		return fmt.Errorf("kube client: %w", err)
	}

	appsNS := defaultAppsNS
	if discovered, nsErr := utilclient.GetApplicationsNamespace(ctx, cliClient); nsErr == nil {
		appsNS = discovered
	}

	operatorNS := defaultOperatorNS
	operatorName := defaultOperatorName

	opInfo, olmErr := utilclient.DiscoverOperatorFromOLM(ctx, cliClient)
	if olmErr != nil {
		slog.Debug("diagnose: OLM discovery failed, using defaults", "error", olmErr)
	} else if opInfo != nil {
		operatorNS = opInfo.Namespace
		if opInfo.DeploymentName != "" {
			operatorName = opInfo.DeploymentName
		}
	}

	crClient, err := utilclient.NewControllerRuntimeClient(restConfig)
	if err != nil {
		return fmt.Errorf("controller-runtime client: %w", err)
	}

	report, err := diagnose.Run(ctx, diagnose.Config{
		Client:          crClient,
		AppsNS:          appsNS,
		OperatorNS:      operatorNS,
		OperatorName:    operatorName,
		TargetComponent: component,
		EventsSince:     eventsSince,
	})
	if err != nil {
		return fmt.Errorf("diagnostic run: %w", err)
	}

	if jsonOutput {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")

		if err := enc.Encode(report); err != nil {
			return fmt.Errorf("json encode: %w", err)
		}
	} else {
		diagnose.Format(cmd.OutOrStdout(), report)
	}

	if !report.Healthy {
		return clierrors.NewAlreadyHandledError(errors.New("issues found")) //nolint:wrapcheck
	}

	return nil
}
