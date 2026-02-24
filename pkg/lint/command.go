package lint

import (
	"context"
	"fmt"
	"slices"

	"github.com/blang/semver/v4"
	"github.com/spf13/pflag"

	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/genericiooptions"

	"github.com/opendatahub-io/odh-cli/pkg/cmd"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	resultpkg "github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/lint/checks/components/dashboard"
	"github.com/opendatahub-io/odh-cli/pkg/lint/checks/components/datasciencepipelines"
	"github.com/opendatahub-io/odh-cli/pkg/lint/checks/components/kserve"
	"github.com/opendatahub-io/odh-cli/pkg/lint/checks/components/kueue"
	"github.com/opendatahub-io/odh-cli/pkg/lint/checks/components/modelmesh"
	raycomponent "github.com/opendatahub-io/odh-cli/pkg/lint/checks/components/ray"
	"github.com/opendatahub-io/odh-cli/pkg/lint/checks/components/trainingoperator"
	"github.com/opendatahub-io/odh-cli/pkg/lint/checks/dependencies/certmanager"
	"github.com/opendatahub-io/odh-cli/pkg/lint/checks/dependencies/openshift"
	datasciencepipelinesworkloads "github.com/opendatahub-io/odh-cli/pkg/lint/checks/workloads/datasciencepipelines"
	"github.com/opendatahub-io/odh-cli/pkg/lint/checks/workloads/guardrails"
	kserveworkloads "github.com/opendatahub-io/odh-cli/pkg/lint/checks/workloads/kserve"
	llamastackworkloads "github.com/opendatahub-io/odh-cli/pkg/lint/checks/workloads/llamastack"
	"github.com/opendatahub-io/odh-cli/pkg/lint/checks/workloads/notebook"
	"github.com/opendatahub-io/odh-cli/pkg/lint/checks/workloads/ray"
	trainingoperatorworkloads "github.com/opendatahub-io/odh-cli/pkg/lint/checks/workloads/trainingoperator"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
	"github.com/opendatahub-io/odh-cli/pkg/util/iostreams"
	"github.com/opendatahub-io/odh-cli/pkg/util/version"
)

// Verify Command implements cmd.Command interface at compile time.
var _ cmd.Command = (*Command)(nil)

// Command contains the lint command configuration.
type Command struct {
	*SharedOptions

	// TargetVersion is the optional target version for upgrade assessment.
	// If empty, runs in lint mode (validates current state).
	// If set, runs in upgrade mode (assesses upgrade readiness to target version).
	TargetVersion string

	// ISVCDeploymentMode filters InferenceService display by deployment mode.
	// Valid values: "all" (default), "serverless", "modelmesh".
	ISVCDeploymentMode string

	// parsedTargetVersion is the parsed semver version (upgrade mode only)
	parsedTargetVersion *semver.Version

	// currentClusterVersion stores the detected OpenShift AI version (populated during Run)
	currentClusterVersion string

	// currentOpenShiftVersion stores the detected OpenShift platform version (populated during Run)
	currentOpenShiftVersion string

	// registry is the check registry for this command instance.
	// Explicitly populated to avoid global state and enable test isolation.
	registry *check.CheckRegistry
}

// NewCommand creates a new Command with defaults.
// Per FR-014, SharedOptions are initialized internally.
// ConfigFlags must be provided to ensure CLI auth flags are properly propagated.
// Optional configuration can be provided via functional options (e.g., WithTargetVersion).
func NewCommand(
	streams genericiooptions.IOStreams,
	configFlags *genericclioptions.ConfigFlags,
	options ...CommandOption,
) *Command {
	shared := NewSharedOptions(streams, configFlags)
	registry := check.NewRegistry()

	// Explicitly register all checks (no global state, full test isolation)
	// Components (13)
	registry.MustRegister(raycomponent.NewCodeFlareRemovalCheck())
	registry.MustRegister(dashboard.NewAcceleratorProfileMigrationCheck())
	registry.MustRegister(dashboard.NewHardwareProfileMigrationCheck())
	registry.MustRegister(datasciencepipelines.NewRenamingCheck())
	registry.MustRegister(kserve.NewServerlessRemovalCheck())
	registry.MustRegister(kserve.NewKuadrantReadinessCheck())
	registry.MustRegister(kserve.NewAuthorinoTLSReadinessCheck())
	registry.MustRegister(kserve.NewServiceMeshOperatorCheck())
	registry.MustRegister(kserve.NewServiceMeshRemovalCheck())
	registry.MustRegister(kueue.NewManagementStateCheck())
	registry.MustRegister(kueue.NewOperatorInstalledCheck())
	registry.MustRegister(modelmesh.NewRemovalCheck())
	registry.MustRegister(trainingoperator.NewDeprecationCheck())

	// Dependencies (2)
	registry.MustRegister(certmanager.NewCheck())
	registry.MustRegister(openshift.NewCheck())

	// Workloads (16)
	registry.MustRegister(ray.NewAppWrapperCleanupCheck())
	registry.MustRegister(datasciencepipelinesworkloads.NewInstructLabRemovalCheck())
	registry.MustRegister(datasciencepipelinesworkloads.NewStoredVersionRemovalCheck())
	registry.MustRegister(guardrails.NewImpactedWorkloadsCheck())
	registry.MustRegister(guardrails.NewOtelMigrationCheck())
	registry.MustRegister(kserveworkloads.NewInferenceServiceConfigCheck())
	registry.MustRegister(kserveworkloads.NewAcceleratorMigrationCheck())
	registry.MustRegister(kserveworkloads.NewHardwareProfileMigrationCheck())
	registry.MustRegister(kserveworkloads.NewImpactedWorkloadsCheck())
	registry.MustRegister(llamastackworkloads.NewConfigCheck())
	registry.MustRegister(notebook.NewAcceleratorMigrationCheck())
	registry.MustRegister(notebook.NewContainerNameCheck())
	registry.MustRegister(notebook.NewHardwareProfileMigrationCheck())
	registry.MustRegister(notebook.NewConnectionIntegrityCheck())
	registry.MustRegister(notebook.NewHardwareProfileIntegrityCheck())
	registry.MustRegister(notebook.NewImpactedWorkloadsCheck())
	registry.MustRegister(notebook.NewRunningWorkloadsCheck())
	registry.MustRegister(ray.NewImpactedWorkloadsCheck())
	registry.MustRegister(trainingoperatorworkloads.NewImpactedWorkloadsCheck())

	c := &Command{
		SharedOptions:      shared,
		registry:           registry,
		ISVCDeploymentMode: "all",
	}

	// Apply functional options
	for _, opt := range options {
		opt(c)
	}

	return c
}

// AddFlags registers command-specific flags with the provided FlagSet.
func (c *Command) AddFlags(fs *pflag.FlagSet) {
	fs.StringVar(&c.TargetVersion, "target-version", "", flagDescTargetVersion)
	fs.StringVarP((*string)(&c.OutputFormat), "output", "o", string(OutputFormatTable), flagDescOutput)
	fs.StringArrayVar(&c.CheckSelectors, "checks", []string{"*"}, flagDescChecks)
	fs.BoolVarP(&c.Verbose, "verbose", "v", false, flagDescVerbose)
	fs.BoolVar(&c.Debug, "debug", false, flagDescDebug)
	fs.DurationVar(&c.Timeout, "timeout", c.Timeout, flagDescTimeout)
	fs.StringVar(&c.ISVCDeploymentMode, "isvc-deployment-mode", "all", flagDescISVCDeploymentMode)

	// Throttling settings
	fs.Float32Var(&c.QPS, "qps", c.QPS, flagDescQPS)
	fs.IntVar(&c.Burst, "burst", c.Burst, flagDescBurst)
}

// Complete populates Options and performs pre-validation setup.
func (c *Command) Complete() error {
	// Complete shared options (creates client)
	if err := c.SharedOptions.Complete(); err != nil {
		return fmt.Errorf("completing shared options: %w", err)
	}

	// Wrap IO with QuietWrapper if NOT in verbose or debug mode (default is quiet)
	if !c.Verbose && !c.Debug {
		c.IO = iostreams.NewQuietWrapper(c.IO)
	}

	// Parse target version if provided (upgrade mode)
	if c.TargetVersion != "" {
		// Use ParseTolerant to accept partial versions (e.g., "3.0" → "3.0.0")
		targetVer, err := semver.ParseTolerant(c.TargetVersion)
		if err != nil {
			return fmt.Errorf("invalid target version %q: %w", c.TargetVersion, err)
		}
		c.parsedTargetVersion = &targetVer
	}
	// If no target version provided, we're in lint mode (will use current version)

	return nil
}

// Validate checks that all required options are valid.
func (c *Command) Validate() error {
	// Validate shared options
	if err := c.SharedOptions.Validate(); err != nil {
		return fmt.Errorf("validating shared options: %w", err)
	}

	// Validate ISVC deployment mode filter
	validModes := []string{"all", "serverless", "modelmesh"}
	if !slices.Contains(validModes, c.ISVCDeploymentMode) {
		return fmt.Errorf("invalid isvc-deployment-mode: %s (must be one of: all, serverless, modelmesh)", c.ISVCDeploymentMode)
	}

	return nil
}

// Run executes the lint command in either lint or upgrade mode.
func (c *Command) Run(ctx context.Context) error {
	// Create context with timeout to prevent hanging on slow clusters
	ctx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()

	// Detect current cluster version (needed for both modes)
	currentVersion, err := version.Detect(ctx, c.Client)
	if err != nil {
		return fmt.Errorf("detecting cluster version: %w", err)
	}

	// Store current version for output formatting
	c.currentClusterVersion = currentVersion.String()

	// Detect OpenShift platform version (informational, non-fatal)
	ocpVersion, err := version.DetectOpenShiftVersion(ctx, c.Client)
	if err != nil {
		c.IO.Errorf("Warning: Failed to detect OpenShift version: %v", err)
	} else {
		c.currentOpenShiftVersion = ocpVersion.String()
	}

	// Determine effective target version (defaults to current for lint mode)
	targetVersion := currentVersion
	if c.parsedTargetVersion != nil {
		targetVersion = c.parsedTargetVersion
	}

	// Same major.minor means no upgrade checks are needed (checked before
	// the downgrade guard so that e.g. --target-version 2.25 with current
	// 2.25.2 is treated as "same version", not as a downgrade).
	if version.SameMajorMinor(currentVersion, targetVersion) {
		return c.runLintMode(ctx, currentVersion)
	}

	// Reject downgrades when explicit --target-version is provided
	if targetVersion.LT(*currentVersion) {
		return fmt.Errorf("target version %s is older than current version %s (downgrades not supported)",
			c.TargetVersion, currentVersion.String())
	}

	return c.runUpgradeMode(ctx, currentVersion)
}

// configureCheckSettings applies command-level settings to specific checks.
func (c *Command) configureCheckSettings() {
	// Apply ISVC deployment mode filter to the KServe impacted workloads check
	for _, chk := range c.registry.ListAll() {
		if isvcCheck, ok := chk.(*kserveworkloads.ImpactedWorkloadsCheck); ok {
			isvcCheck.SetDeploymentModeFilter(c.ISVCDeploymentMode)
		}
	}
}

// runLintMode validates current cluster state.
//
//nolint:unparam // keep explicit error return value
func (c *Command) runLintMode(_ context.Context, currentVersion *semver.Version) error {
	c.IO.Fprintln()
	outputVersionInfo(c.IO.Out(), &VersionInfo{
		RHOAICurrentVersion: currentVersion.String(),
		OpenShiftVersion:    c.currentOpenShiftVersion,
	})

	c.IO.Fprintln()
	c.IO.Fprintf("Current and target versions are the same (%s), no checks will be executed.",
		version.MajorMinorLabel(currentVersion))

	return nil
}

// runUpgradeMode assesses upgrade readiness for a target version.
func (c *Command) runUpgradeMode(ctx context.Context, currentVersion *semver.Version) error {
	c.IO.Errorf("Assessing upgrade readiness: %s → %s\n", currentVersion.String(), c.TargetVersion)

	// Configure check-specific settings
	c.configureCheckSettings()

	// Execute checks using target version for applicability filtering
	c.IO.Errorf("Running upgrade compatibility checks...")
	executor := check.NewExecutor(c.registry, c.IO)

	// Create check target with BOTH current and target versions for upgrade checks
	checkTarget := check.Target{
		Client:         c.Client,
		CurrentVersion: currentVersion,        // The version we're upgrading FROM
		TargetVersion:  c.parsedTargetVersion, // The version we're upgrading TO
		Resource:       nil,
		IO:             c.IO,
		Debug:          c.Debug,
	}

	// Execute checks in canonical order: dependencies → services → components → workloads
	resultsByGroup := make(map[check.CheckGroup][]check.CheckExecution)

	for _, group := range check.CanonicalGroupOrder {
		results, err := executor.ExecuteSelective(ctx, checkTarget, c.CheckSelectors, group)
		if err != nil {
			return fmt.Errorf("executing %s checks: %w", group, err)
		}

		resultsByGroup[group] = results
	}

	// Format and output results
	if err := c.formatAndOutputUpgradeResults(ctx, currentVersion.String(), resultsByGroup); err != nil {
		return err
	}

	// Print verdict and determine exit code
	return c.printVerdictAndExit(resultsByGroup)
}

// printVerdictAndExit prints a prominent result verdict for table output and returns
// an error if fail-on conditions are met (to control exit code).
func (c *Command) printVerdictAndExit(resultsByGroup map[check.CheckGroup][]check.CheckExecution) error {
	var hasBlocking, hasAdvisory bool

	for _, results := range resultsByGroup {
		for _, exec := range results {
			impact := exec.Result.GetImpact()
			if impact != nil {
				switch *impact {
				case string(resultpkg.ImpactBlocking):
					hasBlocking = true
				case string(resultpkg.ImpactAdvisory):
					hasAdvisory = true
				default:
					// ImpactNone and unknown impacts don't affect exit code
				}
			}
		}
	}

	if c.OutputFormat == OutputFormatTable {
		printVerdict(c.IO.Out(), hasBlocking, hasAdvisory)
	}

	return nil
}

// openShiftVersionPtr returns the OpenShift version as *string, or nil if empty.
func (c *Command) openShiftVersionPtr() *string {
	if c.currentOpenShiftVersion == "" {
		return nil
	}

	return &c.currentOpenShiftVersion
}

// formatAndOutputUpgradeResults formats upgrade assessment results.
func (c *Command) formatAndOutputUpgradeResults(
	ctx context.Context,
	currentVer string,
	resultsByGroup map[check.CheckGroup][]check.CheckExecution,
) error {
	clusterVer := &c.currentClusterVersion
	targetVer := &c.TargetVersion
	ocpVer := c.openShiftVersionPtr()

	// Flatten results to sorted array
	flatResults := FlattenResults(resultsByGroup)

	switch c.OutputFormat {
	case OutputFormatTable:
		return c.outputUpgradeTable(ctx, currentVer, flatResults)
	case OutputFormatJSON:
		if err := OutputJSON(c.IO.Out(), flatResults, clusterVer, targetVer, ocpVer); err != nil {
			return fmt.Errorf("outputting JSON: %w", err)
		}

		return nil
	case OutputFormatYAML:
		if err := OutputYAML(c.IO.Out(), flatResults, clusterVer, targetVer, ocpVer); err != nil {
			return fmt.Errorf("outputting YAML: %w", err)
		}

		return nil
	default:
		return fmt.Errorf("unsupported output format: %s", c.OutputFormat)
	}
}

// outputUpgradeTable outputs upgrade results in table format with header.
func (c *Command) outputUpgradeTable(ctx context.Context, _ string, results []check.CheckExecution) error {
	c.IO.Fprintln()

	opts := TableOutputOptions{
		ShowImpactedObjects: c.Verbose,
		VersionInfo: &VersionInfo{
			RHOAICurrentVersion: c.currentClusterVersion,
			RHOAITargetVersion:  c.TargetVersion,
			OpenShiftVersion:    c.currentOpenShiftVersion,
		},
	}

	if c.Verbose {
		opts.NamespaceRequesters = collectNamespaceRequesters(ctx, c.Client, results)
	}

	// Reuse the lint table output logic
	if err := OutputTable(c.IO.Out(), results, opts); err != nil {
		return fmt.Errorf("outputting table: %w", err)
	}

	return nil
}

// collectNamespaceRequesters fetches the openshift.io/requester annotation for each
// unique namespace referenced by impacted objects in the results.
func collectNamespaceRequesters(
	ctx context.Context,
	reader client.Reader,
	results []check.CheckExecution,
) map[string]string {
	// Collect unique namespaces from impacted objects.
	namespaces := make(map[string]struct{})

	for _, exec := range results {
		for _, obj := range exec.Result.ImpactedObjects {
			if obj.Namespace != "" {
				namespaces[obj.Namespace] = struct{}{}
			}
		}
	}

	if len(namespaces) == 0 {
		return nil
	}

	requesters := make(map[string]string, len(namespaces))

	for ns := range namespaces {
		meta, err := reader.GetResourceMetadata(ctx, resources.Namespace, ns)
		if err != nil {
			continue
		}

		if requester, ok := meta.Annotations["openshift.io/requester"]; ok {
			requesters[ns] = requester
		}
	}

	return requesters
}
