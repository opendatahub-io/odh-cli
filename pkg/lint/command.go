package lint

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/blang/semver/v4"
	"github.com/fatih/color"
	"github.com/spf13/pflag"

	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/genericiooptions"

	"github.com/opendatahub-io/odh-cli/pkg/api"
	"github.com/opendatahub-io/odh-cli/pkg/cmd"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	resultpkg "github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/lint/checks/components/dashboard"
	"github.com/opendatahub-io/odh-cli/pkg/lint/checks/components/datasciencepipelines"
	"github.com/opendatahub-io/odh-cli/pkg/lint/checks/components/kserve"
	"github.com/opendatahub-io/odh-cli/pkg/lint/checks/components/kueue"
	"github.com/opendatahub-io/odh-cli/pkg/lint/checks/components/llamastack"
	"github.com/opendatahub-io/odh-cli/pkg/lint/checks/components/modelmesh"
	raycomponent "github.com/opendatahub-io/odh-cli/pkg/lint/checks/components/ray"
	"github.com/opendatahub-io/odh-cli/pkg/lint/checks/components/trainingoperator"
	"github.com/opendatahub-io/odh-cli/pkg/lint/checks/dependencies/certmanager"
	"github.com/opendatahub-io/odh-cli/pkg/lint/checks/dependencies/openshift"
	"github.com/opendatahub-io/odh-cli/pkg/lint/checks/dependencies/ossm34"
	"github.com/opendatahub-io/odh-cli/pkg/lint/checks/dependencies/servicemesh"
	"github.com/opendatahub-io/odh-cli/pkg/lint/checks/dependencies/sharedossm"
	"github.com/opendatahub-io/odh-cli/pkg/lint/checks/dependencies/sharedserverless"
	"github.com/opendatahub-io/odh-cli/pkg/lint/checks/platform/datasciencecluster"
	"github.com/opendatahub-io/odh-cli/pkg/lint/checks/platform/dscinitialization"
	datasciencepipelinesworkloads "github.com/opendatahub-io/odh-cli/pkg/lint/checks/workloads/datasciencepipelines"
	"github.com/opendatahub-io/odh-cli/pkg/lint/checks/workloads/guardrails"
	kserveworkloads "github.com/opendatahub-io/odh-cli/pkg/lint/checks/workloads/kserve"
	kueueworkloads "github.com/opendatahub-io/odh-cli/pkg/lint/checks/workloads/kueue"
	llamastackworkloads "github.com/opendatahub-io/odh-cli/pkg/lint/checks/workloads/llamastack"
	"github.com/opendatahub-io/odh-cli/pkg/lint/checks/workloads/notebook"
	"github.com/opendatahub-io/odh-cli/pkg/lint/checks/workloads/ray"
	trainingoperatorworkloads "github.com/opendatahub-io/odh-cli/pkg/lint/checks/workloads/trainingoperator"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/schema"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
	clierrors "github.com/opendatahub-io/odh-cli/pkg/util/errors"
	"github.com/opendatahub-io/odh-cli/pkg/util/iostreams"
	"github.com/opendatahub-io/odh-cli/pkg/util/stdin"
	"github.com/opendatahub-io/odh-cli/pkg/util/version"
)

// Verify Command implements cmd.Command interface at compile time.
var _ cmd.Command = (*Command)(nil)

const (
	msgProhibitedOrBlocking = "prohibited or blocking findings detected: upgrade cannot proceed"
	msgAdvisoryFindings     = "advisory findings detected: review recommended before upgrade"
	msgInfrastructureErrors = "one or more checks failed due to infrastructure errors"
	msgCheckExecErrors      = "check execution errors detected: %w"
)

// Command contains the lint command configuration.
type Command struct {
	*SharedOptions
	schema.OutputOptions

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

	// flags stores the FlagSet for checking explicitly set flags
	flags *pflag.FlagSet
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
	// Platform (2)
	registry.MustRegister(dscinitialization.NewDSCInitializationReadinessCheck())
	registry.MustRegister(datasciencecluster.NewDataScienceClusterReadinessCheck())

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
	// Deferred: re-enable when a future 3.3.x release supports Unmanaged + Red Hat build of Kueue Operator.
	// registry.MustRegister(kueue.NewOperatorInstalledCheck())
	registry.MustRegister(llamastack.NewRemovalCheck())
	registry.MustRegister(modelmesh.NewRemovalCheck())
	registry.MustRegister(trainingoperator.NewDeprecationCheck())

	// Dependencies (6)
	registry.MustRegister(certmanager.NewCheck())
	registry.MustRegister(openshift.NewCheck())
	registry.MustRegister(ossm34.NewCheck())
	registry.MustRegister(servicemesh.NewCheck())
	registry.MustRegister(sharedossm.NewCheck())
	registry.MustRegister(sharedserverless.NewCheck())

	// Workloads (20)
	registry.MustRegister(ray.NewAppWrapperCleanupCheck())
	registry.MustRegister(datasciencepipelinesworkloads.NewInstructLabRemovalCheck())
	registry.MustRegister(datasciencepipelinesworkloads.NewStoredVersionRemovalCheck())
	registry.MustRegister(guardrails.NewImpactedWorkloadsCheck())
	registry.MustRegister(guardrails.NewOtelMigrationCheck())
	registry.MustRegister(kserveworkloads.NewInferenceServiceConfigCheck())
	registry.MustRegister(kserveworkloads.NewAcceleratorMigrationCheck())
	registry.MustRegister(kserveworkloads.NewHardwareProfileMigrationCheck())
	registry.MustRegister(kserveworkloads.NewImpactedWorkloadsCheck())
	registry.MustRegister(kueueworkloads.NewDataIntegrityCheck())
	registry.MustRegister(llamastackworkloads.NewConfigCheck())
	registry.MustRegister(notebook.NewAcceleratorMigrationCheck())
	registry.MustRegister(notebook.NewContainerNameCheck())
	registry.MustRegister(notebook.NewHardwareProfileMigrationCheck())
	registry.MustRegister(notebook.NewConnectionIntegrityCheck())
	registry.MustRegister(notebook.NewHardwareProfileIntegrityCheck())
	registry.MustRegister(notebook.NewImpactedWorkloadsCheck())
	registry.MustRegister(notebook.NewNonStoppedWorkloadsCheck())
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
	c.flags = fs // Store for checking explicitly set flags in applyStdinInput
	fs.StringVar(&c.TargetVersion, "target-version", "", flagDescTargetVersion)
	fs.StringVarP((*string)(&c.OutputFormat), "output", "o", string(OutputFormatTable), flagDescOutput)
	_ = fs.SetAnnotation("output", api.AnnotationValidValues, []string{"table", "json", "yaml"})
	fs.StringVar((*string)(&c.SeverityLevel), "severity", string(SeverityLevelInfo), flagDescSeverity)
	_ = fs.SetAnnotation("severity", api.AnnotationValidValues, []string{"prohibited", "critical", "warning", "info"})
	fs.StringArrayVar(&c.CheckSelectors, "checks", []string{"*"}, flagDescChecks)
	fs.BoolVarP(&c.Verbose, "verbose", "v", false, flagDescVerbose)
	fs.BoolVarP(&c.Quiet, "quiet", "q", false, flagDescQuiet)
	fs.BoolVar(&c.Debug, "debug", false, flagDescDebug)
	fs.BoolVar(&c.NoColor, "no-color", false, flagDescNoColor)
	fs.DurationVar(&c.Timeout, "timeout", c.Timeout, flagDescTimeout)
	fs.StringVar(&c.ISVCDeploymentMode, "isvc-deployment-mode", "all", flagDescISVCDeploymentMode)
	_ = fs.SetAnnotation("isvc-deployment-mode", api.AnnotationValidValues, []string{"all", "serverless", "modelmesh"})
	fs.BoolVar(&c.FromStdin, "from-stdin", false, stdin.FlagDesc)

	// Throttling settings
	fs.Float32Var(&c.QPS, "qps", c.QPS, flagDescQPS)
	fs.IntVar(&c.Burst, "burst", c.Burst, flagDescBurst)

	// Schema output
	c.OutputOptions.AddFlags(fs)
}

// parseStdinConfig reads and applies configuration from stdin.
func (c *Command) parseStdinConfig() error {
	if err := stdin.CheckPiped(c.IO.In()); err != nil {
		return err //nolint:wrapcheck // CheckPiped returns a self-descriptive user-facing error
	}

	var input StdinInput
	if err := stdin.Parse(c.IO.In(), &input); err != nil {
		return fmt.Errorf("parsing stdin: %w", err)
	}

	// Merge stdin values into command options (stdin overrides defaults)
	if err := c.applyStdinInput(&input); err != nil {
		return err
	}

	return nil
}

// applyStdinInput merges stdin configuration into command options.
// Explicit CLI flags take precedence over stdin values.
// Returns an error if stdin contains invalid values.
func (c *Command) applyStdinInput(input *StdinInput) error {
	if len(input.Checks) > 0 && !stdin.FlagChanged(c.flags, "checks") {
		c.CheckSelectors = input.Checks
	}

	if input.Severity != "" && !stdin.FlagChanged(c.flags, "severity") {
		level := SeverityLevel(input.Severity)
		if err := level.Validate(); err != nil {
			return fmt.Errorf("stdin input: %w", err)
		}
		c.SeverityLevel = level
	}

	if input.TargetVersion != "" && !stdin.FlagChanged(c.flags, "target-version") {
		c.TargetVersion = input.TargetVersion
	}

	if input.Verbose && !stdin.FlagChanged(c.flags, "verbose") {
		c.Verbose = true
	}

	if input.Quiet && !stdin.FlagChanged(c.flags, "quiet") {
		c.Quiet = true
	}

	if input.Output != "" && !stdin.FlagChanged(c.flags, "output") {
		format := OutputFormat(input.Output)
		if err := format.Validate(); err != nil {
			return fmt.Errorf("stdin input: %w", err)
		}
		c.OutputFormat = format
	}

	return nil
}

// Complete populates Options and performs pre-validation setup.
func (c *Command) Complete() error {
	// Skip client creation when only outputting schema
	if c.OutputSchema {
		return nil
	}

	// Parse stdin configuration if --from-stdin is specified
	if c.FromStdin {
		if err := c.parseStdinConfig(); err != nil {
			//nolint:wrapcheck // NewExitCodeError is a same-module constructor
			return clierrors.NewExitCodeError(clierrors.ExitValidation, err)
		}
	}

	// Validate mutual exclusivity of verbose and quiet
	if c.Verbose && c.Quiet {
		return errors.New("--verbose and --quiet are mutually exclusive")
	}

	// Complete shared options (creates client)
	if err := c.SharedOptions.Complete(); err != nil {
		return fmt.Errorf("completing shared options: %w", err)
	}
	// Disable color for structured output; fatih/color handles NO_COLOR env and non-TTY detection.
	if c.OutputFormat == OutputFormatJSON || c.OutputFormat == OutputFormatYAML {
		c.NoColor = true
	}
	color.NoColor = c.NoColor

	// Wrap IO based on verbosity settings
	switch {
	case c.Quiet:
		c.IO = iostreams.NewFullQuietWrapper(c.IO)
	case !c.Verbose && !c.Debug:
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
	// Skip validation when only outputting schema
	if c.OutputSchema {
		return nil
	}

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
	// Short-circuit if --schema was requested (no cluster connection needed)
	if c.OutputSchema {
		if err := schema.WriteTo(c.IO.Out(), schema.SchemaDiagnosticResultList); err != nil {
			return fmt.Errorf("outputting schema: %w", err)
		}

		return nil
	}

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
		//nolint:wrapcheck // NewExitCodeError is a same-module constructor, not an external error
		return clierrors.NewExitCodeError(clierrors.ExitValidation,
			fmt.Errorf("target version %s is older than current version %s (downgrades not supported)",
				c.TargetVersion, currentVersion.String()))
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

	// Validate selectors match at least one registered check (skip for default wildcard)
	if !isDefaultSelector(c.CheckSelectors) {
		matched, err := c.registry.MatchesAnyCheck(c.CheckSelectors)
		if err != nil {
			return fmt.Errorf("validating check selectors: %w", err)
		}

		if !matched {
			noun := "selector"
			if len(c.CheckSelectors) > 1 {
				noun = "selectors"
			}

			return fmt.Errorf("no registered checks match %s: %v\n\nAvailable check IDs:\n  %s",
				noun, c.CheckSelectors, strings.Join(c.registry.AllCheckIDs(), "\n  "))
		}
	}

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

	// Execute checks in canonical order: dependencies → services → platform → components → workloads
	resultsByGroup := make(map[check.CheckGroup][]check.CheckExecution)

	for _, group := range check.CanonicalGroupOrder {
		results, err := executor.ExecuteSelective(ctx, checkTarget, c.CheckSelectors, group)
		if err != nil {
			return fmt.Errorf("executing %s checks: %w", group, err)
		}

		resultsByGroup[group] = results
	}

	// Flatten results and compute the highest-priority exit code from execution
	// errors BEFORE filtering, so failures with Result == nil are not dropped.
	flatResults := FlattenResults(resultsByGroup)
	execSummary := highestPriorityExecError(flatResults)

	// Strip nil results and apply severity filter for display/verdict
	flatResults = slices.DeleteFunc(flatResults, func(exec check.CheckExecution) bool {
		return exec.Result == nil
	})
	flatResults = FilterBySeverity(flatResults, c.SeverityLevel)

	// Format and output results
	if err := c.formatAndOutputUpgradeResults(ctx, currentVersion.String(), flatResults); err != nil {
		return err
	}

	// Print verdict and determine exit code from findings
	findingsErr := c.evaluateVerdict(flatResults)

	return resolveExitError(execSummary, findingsErr, c.OutputFormat)
}

// evaluateVerdict prints a prominent result verdict for table output and returns
// an error carrying the appropriate ExitCode when fail-on conditions are met.
func (c *Command) evaluateVerdict(results []check.CheckExecution) error {
	var hasProhibited, hasBlocking, hasAdvisory bool

	for _, exec := range results {
		if exec.Result == nil {
			continue
		}

		switch exec.Result.GetImpact() {
		case resultpkg.ImpactProhibited:
			hasProhibited = true
		case resultpkg.ImpactBlocking:
			hasBlocking = true
		case resultpkg.ImpactAdvisory:
			hasAdvisory = true
		case resultpkg.ImpactNone:
			// No impact on exit code
		}
	}

	if c.OutputFormat == OutputFormatTable {
		printVerdict(c.IO.Out(), hasProhibited, hasBlocking, hasAdvisory)
	}

	if hasProhibited || hasBlocking {
		//nolint:wrapcheck // NewExitCodeError is a same-module constructor
		return clierrors.NewExitCodeError(
			clierrors.ExitError,
			fmt.Errorf("%w: %s", clierrors.ErrLintBlocked, msgProhibitedOrBlocking),
		)
	}

	if hasAdvisory {
		//nolint:wrapcheck // NewExitCodeError is a same-module constructor
		return clierrors.NewExitCodeError(
			clierrors.ExitWarning,
			fmt.Errorf("%w: %s", clierrors.ErrLintAdvisory, msgAdvisoryFindings),
		)
	}

	return nil
}

// execErrorSummary holds the highest-priority execution error info.
type execErrorSummary struct {
	exitCode clierrors.ExitCode
	err      error
}

// highestPriorityExecError scans check executions for infrastructure errors
// (auth, connection, etc.) and returns the highest-priority exit code found.
func highestPriorityExecError(results []check.CheckExecution) execErrorSummary {
	summary := execErrorSummary{exitCode: clierrors.ExitSuccess}

	for _, exec := range results {
		if exec.Error != nil {
			code := clierrors.ExitCodeFromError(exec.Error)
			if clierrors.IsHigherPriority(code, summary.exitCode) {
				summary.exitCode = code
				summary.err = exec.Error
			}
		}
	}

	return summary
}

// resolveExitError determines the final error to return based on execution
// errors and findings verdict, preferring infrastructure errors when they
// have higher or equal priority.
//
// When exit codes are equal (e.g., both ExitError), infrastructure errors
// take precedence because they indicate a check couldn't run properly,
// which is more actionable than findings from checks that did complete.
func resolveExitError(execSummary execErrorSummary, findingsErr error, outputFormat OutputFormat) error {
	if execSummary.exitCode != clierrors.ExitSuccess {
		findingsExitCode := clierrors.ExitCodeFromError(findingsErr)
		// Prefer exec errors when priority is higher OR equal (see comment above)
		if clierrors.IsHigherPriority(execSummary.exitCode, findingsExitCode) ||
			execSummary.exitCode == findingsExitCode {
			if findingsErr != nil {
				//nolint:wrapcheck // NewExitCodeError is a same-module constructor
				return clierrors.NewExitCodeError(execSummary.exitCode,
					fmt.Errorf(msgCheckExecErrors, execSummary.err))
			}

			//nolint:wrapcheck // NewExitCodeError is a same-module constructor
			return clierrors.NewExitCodeError(execSummary.exitCode,
				fmt.Errorf(msgInfrastructureErrors+": %w", execSummary.err))
		}
	}

	if findingsErr != nil {
		if outputFormat == OutputFormatTable {
			return clierrors.NewAlreadyHandledError(findingsErr) //nolint:wrapcheck // wrapping is done by NewAlreadyHandledError
		}

		return findingsErr
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
	results []check.CheckExecution,
) error {
	clusterVer := &c.currentClusterVersion
	targetVer := &c.TargetVersion
	ocpVer := c.openShiftVersionPtr()

	switch c.OutputFormat {
	case OutputFormatTable:
		return c.outputUpgradeTable(ctx, currentVer, results)
	case OutputFormatJSON:
		if err := OutputJSON(c.IO.Out(), results, clusterVer, targetVer, ocpVer); err != nil {
			return fmt.Errorf("outputting JSON: %w", err)
		}

		return nil
	case OutputFormatYAML:
		if err := OutputYAML(c.IO.Out(), results, clusterVer, targetVer, ocpVer); err != nil {
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

// isDefaultSelector returns true if the selectors are the default wildcard ["*"].
func isDefaultSelector(selectors []string) bool {
	return len(selectors) == 1 && selectors[0] == "*"
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
