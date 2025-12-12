package lint

import (
	"context"
	"errors"
	"fmt"

	"github.com/blang/semver/v4"
	"github.com/spf13/pflag"

	"k8s.io/cli-runtime/pkg/genericiooptions"

	"github.com/lburgazzoli/odh-cli/pkg/cmd"
	"github.com/lburgazzoli/odh-cli/pkg/doctor/discovery"
	"github.com/lburgazzoli/odh-cli/pkg/lint/check"
	"github.com/lburgazzoli/odh-cli/pkg/lint/version"
	"github.com/lburgazzoli/odh-cli/pkg/util/iostreams"
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

	// parsedTargetVersion is the parsed semver version (upgrade mode only)
	parsedTargetVersion *semver.Version

	// currentClusterVersion stores the detected cluster version (populated during Run)
	currentClusterVersion string
}

// NewCommand creates a new Command with defaults.
// Per FR-014, SharedOptions are initialized internally.
func NewCommand(streams genericiooptions.IOStreams) *Command {
	shared := NewSharedOptions(streams)

	return &Command{
		SharedOptions: shared,
	}
}

// AddFlags registers command-specific flags with the provided FlagSet.
func (c *Command) AddFlags(fs *pflag.FlagSet) {
	fs.StringVar(&c.TargetVersion, "target-version", "",
		"Target version for upgrade assessment (enables upgrade mode)")
	fs.StringVarP((*string)(&c.OutputFormat), "output", "o", string(OutputFormatTable),
		"Output format (table|json|yaml)")
	fs.StringVar(&c.CheckSelector, "checks", "*",
		"Glob pattern to filter which checks to run (e.g., 'components/*', '*dashboard*')")
	fs.StringVar((*string)(&c.MinSeverity), "severity", "",
		"Filter results by minimum severity level (critical|warning|info)")
	fs.BoolVar(&c.FailOnCritical, "fail-on-critical", true,
		"Exit with non-zero code if Critical findings detected")
	fs.BoolVar(&c.FailOnWarning, "fail-on-warning", false,
		"Exit with non-zero code if Warning findings detected")
	fs.BoolVarP(&c.Verbose, "verbose", "v", false,
		"Show progress messages (default: quiet, only show results)")
	fs.DurationVar(&c.Timeout, "timeout", c.Timeout,
		"Maximum duration for command execution (e.g., 5m, 10m)")
}

// Complete populates Options and performs pre-validation setup.
func (c *Command) Complete() error {
	// Complete shared options (creates client)
	if err := c.SharedOptions.Complete(); err != nil {
		return fmt.Errorf("completing shared options: %w", err)
	}

	// Wrap IO with QuietWrapper if NOT in verbose mode (default is quiet)
	if !c.Verbose {
		c.IO = iostreams.NewQuietWrapper(c.IO)
	}

	// Parse target version if provided (upgrade mode)
	if c.TargetVersion != "" {
		targetVer, err := semver.Parse(c.TargetVersion)
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
	c.currentClusterVersion = currentVersion.Version

	// Determine mode: upgrade (with --target-version) or lint (without --target-version)
	if c.TargetVersion != "" {
		return c.runUpgradeMode(ctx, currentVersion)
	}

	return c.runLintMode(ctx, currentVersion)
}

// runLintMode validates current cluster state.
func (c *Command) runLintMode(ctx context.Context, clusterVersion *version.ClusterVersion) error {
	c.IO.Errorf("Detected OpenShift AI version: %s\n", clusterVersion)

	// Discover components and services
	c.IO.Errorf("Discovering OpenShift AI components and services...")
	components, err := discovery.DiscoverComponentsAndServices(ctx, c.Client)
	if err != nil {
		return fmt.Errorf("discovering components and services: %w", err)
	}
	c.IO.Errorf("Found %d API groups", len(components))
	for _, comp := range components {
		c.IO.Errorf("  - %s/%s (%d resources)", comp.APIGroup, comp.Version, len(comp.Resources))
	}
	c.IO.Fprintln()

	// Discover workloads
	c.IO.Errorf("Discovering workload custom resources...")
	workloads, err := discovery.DiscoverWorkloads(ctx, c.Client)
	if err != nil {
		return fmt.Errorf("discovering workloads: %w", err)
	}
	c.IO.Errorf("Found %d workload types", len(workloads))
	for _, gvr := range workloads {
		c.IO.Errorf("  - %s/%s %s", gvr.Group, gvr.Version, gvr.Resource)
	}
	c.IO.Fprintln()

	// Get the global check registry
	registry := check.GetGlobalRegistry()

	// Execute component and service checks (Resource: nil)
	c.IO.Errorf("Running component and service checks...")
	componentTarget := &check.CheckTarget{
		Client:         c.Client,
		CurrentVersion: clusterVersion, // For lint mode, current = target
		Version:        clusterVersion,
		Resource:       nil, // No specific resource for component/service checks
	}

	executor := check.NewExecutor(registry)

	// Execute all component checks
	componentResults, err := executor.ExecuteSelective(ctx, componentTarget, c.CheckSelector, check.GroupComponent)
	if err != nil {
		// Log error but continue with other checks
		c.IO.Errorf("Warning: Failed to execute component checks: %v", err)
		componentResults = []check.CheckExecution{}
	}

	// Execute all service checks
	serviceResults, err := executor.ExecuteSelective(ctx, componentTarget, c.CheckSelector, check.GroupService)
	if err != nil {
		// Log error but continue with other checks
		c.IO.Errorf("Warning: Failed to execute service checks: %v", err)
		serviceResults = []check.CheckExecution{}
	}

	// Execute all dependency checks
	dependencyResults, err := executor.ExecuteSelective(ctx, componentTarget, c.CheckSelector, check.GroupDependency)
	if err != nil {
		// Log error but continue with other checks
		c.IO.Errorf("Warning: Failed to execute dependency checks: %v", err)
		dependencyResults = []check.CheckExecution{}
	}

	// Execute workload checks for each discovered workload instance
	c.IO.Errorf("Running workload checks...")
	var workloadResults []check.CheckExecution

	for _, gvr := range workloads {
		// List all instances of this workload type
		instances, err := c.Client.ListResources(ctx, gvr)
		if err != nil {
			// Skip workloads we can't access
			c.IO.Errorf("Warning: Failed to list %s: %v", gvr.Resource, err)

			continue
		}

		// Run workload checks for each instance
		for i := range instances {
			workloadTarget := &check.CheckTarget{
				Client:         c.Client,
				CurrentVersion: clusterVersion, // For lint mode, current = target
				Version:        clusterVersion,
				Resource:       &instances[i],
			}

			results, err := executor.ExecuteSelective(ctx, workloadTarget, c.CheckSelector, check.GroupWorkload)
			if err != nil {
				return fmt.Errorf("executing workload checks: %w", err)
			}

			workloadResults = append(workloadResults, results...)
		}
	}

	// Group results by group
	resultsByGroup := map[check.CheckGroup][]check.CheckExecution{
		check.GroupComponent:  componentResults,
		check.GroupService:    serviceResults,
		check.GroupDependency: dependencyResults,
		check.GroupWorkload:   workloadResults,
	}

	// Filter results by minimum severity if specified
	filteredResults := FilterResultsBySeverity(resultsByGroup, c.MinSeverity)

	// Format and output results based on output format
	if err := c.formatAndOutputResults(filteredResults); err != nil {
		return err
	}

	// Determine exit code based on fail-on flags
	return c.determineExitCode(filteredResults)
}

// runUpgradeMode assesses upgrade readiness for a target version.
func (c *Command) runUpgradeMode(ctx context.Context, currentVersion *version.ClusterVersion) error {
	c.IO.Errorf("Current OpenShift AI version: %s", currentVersion)
	c.IO.Errorf("Target OpenShift AI version: %s\n", c.TargetVersion)

	// Parse current version for comparison
	currentVer, err := semver.Parse(currentVersion.Version)
	if err != nil {
		return fmt.Errorf("parsing current version: %w", err)
	}

	// Check if target version is greater than or equal to current
	if c.parsedTargetVersion.LT(currentVer) {
		return fmt.Errorf("target version %s is older than current version %s (downgrades not supported)",
			c.TargetVersion, currentVersion.Version)
	}

	// Check if already at target version
	if c.parsedTargetVersion.EQ(currentVer) {
		c.IO.Errorf("Cluster is already at target version %s", c.TargetVersion)
		c.IO.Errorf("No upgrade necessary")

		return nil
	}

	c.IO.Errorf("Assessing upgrade readiness: %s → %s\n", currentVersion.Version, c.TargetVersion)

	// Get the global check registry
	registry := check.GetGlobalRegistry()

	// For upgrade assessment, we run all checks against the TARGET version
	// This allows version-specific checks to determine if they're applicable
	targetVersionInfo := &version.ClusterVersion{
		Version:    c.TargetVersion,
		Source:     version.SourceManual,
		Confidence: version.ConfidenceHigh,
	}

	// Execute checks using target version for applicability filtering
	c.IO.Errorf("Running upgrade compatibility checks...")
	executor := check.NewExecutor(registry)

	// Create check target with BOTH current and target versions for upgrade checks
	checkTarget := &check.CheckTarget{
		Client:         c.Client,
		CurrentVersion: currentVersion,    // The version we're upgrading FROM
		Version:        targetVersionInfo, // The version we're upgrading TO
		Resource:       nil,
	}

	// Execute all checks (components, services, dependencies, workloads combined)
	// The --checks flag allows users to filter if needed
	results, err := executor.ExecuteSelective(ctx, checkTarget, c.CheckSelector, "")
	if err != nil {
		return fmt.Errorf("executing upgrade checks: %w", err)
	}

	// Group results by group
	resultsByGroup := make(map[check.CheckGroup][]check.CheckExecution)
	for _, result := range results {
		group := result.Check.Group()
		resultsByGroup[group] = append(resultsByGroup[group], result)
	}

	// Filter results by minimum severity if specified
	filteredResults := FilterResultsBySeverity(resultsByGroup, c.MinSeverity)

	// Format and output results
	if err := c.formatAndOutputUpgradeResults(currentVersion.Version, filteredResults); err != nil {
		return err
	}

	// Determine if upgrade is recommended
	blockingIssues := 0
	for _, executions := range filteredResults {
		for _, exec := range executions {
			severity := exec.Result.GetSeverity()
			if exec.Result.IsFailing() && severity != nil && *severity == string(check.SeverityCritical) {
				blockingIssues++
			}
		}
	}

	if blockingIssues > 0 {
		c.IO.Errorf("\n⚠️  Recommendation: Address %d blocking issue(s) before upgrading", blockingIssues)
	} else {
		c.IO.Errorf("\n✅ Cluster is ready for upgrade to %s", c.TargetVersion)
	}

	// Determine exit code based on fail-on flags
	return c.determineExitCode(filteredResults)
}

// determineExitCode returns an error if fail-on conditions are met.
func (c *Command) determineExitCode(resultsByGroup map[check.CheckGroup][]check.CheckExecution) error {
	var hasCritical, hasWarning bool

	for _, results := range resultsByGroup {
		for _, result := range results {
			severity := result.Result.GetSeverity()
			if severity != nil {
				//nolint:revive // exhaustive linter requires explicit Info case
				switch *severity {
				case string(check.SeverityCritical):
					hasCritical = true
				case string(check.SeverityWarning):
					hasWarning = true
				case string(check.SeverityInfo):
					// Info doesn't affect exit code
				default:
					// Unknown severities don't affect exit code
				}
			}
		}
	}

	if c.FailOnCritical && hasCritical {
		return errors.New("critical findings detected")
	}

	if c.FailOnWarning && hasWarning {
		return errors.New("warning findings detected")
	}

	return nil
}

// formatAndOutputResults formats and outputs check results based on the output format.
func (c *Command) formatAndOutputResults(resultsByGroup map[check.CheckGroup][]check.CheckExecution) error {
	clusterVer := &c.currentClusterVersion
	var targetVer *string
	if c.TargetVersion != "" {
		targetVer = &c.TargetVersion
	}

	// Flatten results to sorted array
	flatResults := FlattenResults(resultsByGroup)

	switch c.OutputFormat {
	case OutputFormatTable:
		return c.outputTable(flatResults)
	case OutputFormatJSON:
		if err := OutputJSON(c.IO.Out(), flatResults, clusterVer, targetVer); err != nil {
			return fmt.Errorf("outputting JSON: %w", err)
		}

		return nil
	case OutputFormatYAML:
		if err := OutputYAML(c.IO.Out(), flatResults, clusterVer, targetVer); err != nil {
			return fmt.Errorf("outputting YAML: %w", err)
		}

		return nil
	default:
		return fmt.Errorf("unsupported output format: %s", c.OutputFormat)
	}
}

// outputTable outputs results in table format.
func (c *Command) outputTable(results []check.CheckExecution) error {
	c.IO.Fprintln()
	c.IO.Fprintln("Check Results:")
	c.IO.Fprintln("==============")

	if err := OutputTable(c.IO.Out(), results, c.Verbose); err != nil {
		return fmt.Errorf("outputting table: %w", err)
	}

	return nil
}

// formatAndOutputUpgradeResults formats upgrade assessment results.
func (c *Command) formatAndOutputUpgradeResults(currentVer string, resultsByGroup map[check.CheckGroup][]check.CheckExecution) error {
	clusterVer := &c.currentClusterVersion
	targetVer := &c.TargetVersion

	// Flatten results to sorted array
	flatResults := FlattenResults(resultsByGroup)

	switch c.OutputFormat {
	case OutputFormatTable:
		return c.outputUpgradeTable(currentVer, flatResults)
	case OutputFormatJSON:
		if err := OutputJSON(c.IO.Out(), flatResults, clusterVer, targetVer); err != nil {
			return fmt.Errorf("outputting JSON: %w", err)
		}

		return nil
	case OutputFormatYAML:
		if err := OutputYAML(c.IO.Out(), flatResults, clusterVer, targetVer); err != nil {
			return fmt.Errorf("outputting YAML: %w", err)
		}

		return nil
	default:
		return fmt.Errorf("unsupported output format: %s", c.OutputFormat)
	}
}

// outputUpgradeTable outputs upgrade results in table format with header.
func (c *Command) outputUpgradeTable(currentVer string, results []check.CheckExecution) error {
	c.IO.Fprintln()
	c.IO.Errorf("UPGRADE READINESS: %s → %s", currentVer, c.TargetVersion)
	c.IO.Errorf("=============================================================")

	// Reuse the lint table output logic
	if err := OutputTable(c.IO.Out(), results, c.Verbose); err != nil {
		return fmt.Errorf("outputting table: %w", err)
	}

	return nil
}
