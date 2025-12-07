package doctor

import (
	"context"
	"errors"
	"fmt"

	"github.com/blang/semver/v4"

	"github.com/lburgazzoli/odh-cli/pkg/doctor/check"
	"github.com/lburgazzoli/odh-cli/pkg/doctor/version"
)

// UpgradeOptions contains options for the upgrade command.
type UpgradeOptions struct {
	*SharedOptions

	// TargetVersion is the target OpenShift AI version for upgrade assessment
	TargetVersion string

	// parsedTargetVersion is the parsed semver version
	parsedTargetVersion *semver.Version
}

// NewUpgradeOptions creates a new UpgradeOptions with defaults.
func NewUpgradeOptions(shared *SharedOptions) *UpgradeOptions {
	return &UpgradeOptions{
		SharedOptions: shared,
	}
}

// Complete populates UpgradeOptions and performs pre-validation setup.
func (o *UpgradeOptions) Complete() error {
	// Complete shared options (creates client)
	if err := o.SharedOptions.Complete(); err != nil {
		return fmt.Errorf("completing shared options: %w", err)
	}

	// Parse target version
	targetVer, err := semver.Parse(o.TargetVersion)
	if err != nil {
		return fmt.Errorf("invalid target version %q: %w", o.TargetVersion, err)
	}
	o.parsedTargetVersion = &targetVer

	return nil
}

// Validate checks that all required options are valid.
func (o *UpgradeOptions) Validate() error {
	// Validate shared options
	if err := o.SharedOptions.Validate(); err != nil {
		return fmt.Errorf("validating shared options: %w", err)
	}

	// Validate target version is provided
	if o.TargetVersion == "" {
		return errors.New("target version is required (use --version flag)")
	}

	// Validate target version is parseable (should already be done in Complete, but double-check)
	if o.parsedTargetVersion == nil {
		return errors.New("target version was not properly parsed")
	}

	return nil
}

// Run executes the upgrade command.
func (o *UpgradeOptions) Run(ctx context.Context) error {
	// Create context with timeout to prevent hanging on slow clusters
	ctx, cancel := context.WithTimeout(ctx, o.Timeout)
	defer cancel()

	// Detect current cluster version
	currentVersion, err := version.Detect(ctx, o.Client)
	if err != nil {
		return fmt.Errorf("detecting current cluster version: %w", err)
	}

	_, _ = fmt.Fprintf(o.Out, "Current OpenShift AI version: %s\n", currentVersion)
	_, _ = fmt.Fprintf(o.Out, "Target OpenShift AI version: %s\n\n", o.TargetVersion)

	// Parse current version
	currentVer, err := semver.Parse(currentVersion.Version)
	if err != nil {
		return fmt.Errorf("parsing current version: %w", err)
	}

	// Check if target version is greater than or equal to current
	if o.parsedTargetVersion.LT(currentVer) {
		return fmt.Errorf("target version %s is older than current version %s (downgrades not supported)",
			o.TargetVersion, currentVersion.Version)
	}

	// Check if already at target version
	if o.parsedTargetVersion.EQ(currentVer) {
		_, _ = fmt.Fprintf(o.Out, "Cluster is already at target version %s\n", o.TargetVersion)
		_, _ = fmt.Fprint(o.Out, "No upgrade necessary\n")

		return nil
	}

	_, _ = fmt.Fprintf(o.Out, "Assessing upgrade readiness: %s → %s\n\n", currentVersion.Version, o.TargetVersion)

	// Get the global check registry
	registry := check.GetGlobalRegistry()

	// For upgrade assessment, we run all checks against the TARGET version
	// This allows version-specific checks to determine if they're applicable
	targetVersionInfo := &version.ClusterVersion{
		Version:    o.TargetVersion,
		Source:     version.SourceManual,
		Confidence: version.ConfidenceHigh,
	}

	// Execute checks using target version for applicability filtering
	_, _ = fmt.Fprint(o.Out, "Running upgrade compatibility checks...\n")
	executor := check.NewExecutor(registry)

	// Create check target with BOTH current and target versions for upgrade checks
	checkTarget := &check.CheckTarget{
		Client:         o.Client,
		CurrentVersion: currentVersion,    // The version we're upgrading FROM
		Version:        targetVersionInfo, // The version we're upgrading TO
		Resource:       nil,
	}

	// Execute all checks (components, services, workloads combined)
	// The --checks flag allows users to filter if needed
	results, err := executor.ExecuteSelective(ctx, checkTarget, o.CheckSelector, "")
	if err != nil {
		return fmt.Errorf("executing upgrade checks: %w", err)
	}

	// Group results by category
	resultsByCategory := make(map[check.CheckCategory][]check.CheckExecution)
	for _, result := range results {
		category := result.Check.Category()
		resultsByCategory[category] = append(resultsByCategory[category], result)
	}

	// Filter results by minimum severity if specified
	filteredResults := filterResultsBySeverity(resultsByCategory, o.MinSeverity)

	// Format and output results (reuse lint formatting logic)
	if err := o.formatAndOutputUpgradeResults(currentVersion.Version, filteredResults); err != nil {
		return err
	}

	// Determine if upgrade is recommended
	blockingIssues := 0
	for _, executions := range filteredResults {
		for _, exec := range executions {
			if exec.Result.IsFailing() && exec.Result.Severity != nil && *exec.Result.Severity == check.SeverityCritical {
				blockingIssues++
			}
		}
	}

	if blockingIssues > 0 {
		_, _ = fmt.Fprintf(o.Out, "\n⚠️  Recommendation: Address %d blocking issue(s) before upgrading\n", blockingIssues)
	} else {
		_, _ = fmt.Fprintf(o.Out, "\n✅ Cluster is ready for upgrade to %s\n", o.TargetVersion)
	}

	// Determine exit code based on fail-on flags
	return o.determineExitCode(filteredResults)
}

// formatAndOutputUpgradeResults formats upgrade assessment results.
func (o *UpgradeOptions) formatAndOutputUpgradeResults(currentVer string, resultsByCategory map[check.CheckCategory][]check.CheckExecution) error {
	switch o.OutputFormat {
	case OutputFormatTable:
		return o.outputUpgradeTable(currentVer, resultsByCategory)
	case OutputFormatJSON:
		return outputJSON(o.Out, resultsByCategory)
	case OutputFormatYAML:
		return outputYAML(o.Out, resultsByCategory)
	default:
		return fmt.Errorf("unsupported output format: %s", o.OutputFormat)
	}
}

// outputUpgradeTable outputs upgrade results in table format with header.
func (o *UpgradeOptions) outputUpgradeTable(currentVer string, resultsByCategory map[check.CheckCategory][]check.CheckExecution) error {
	_, _ = fmt.Fprintln(o.Out)
	_, _ = fmt.Fprintf(o.Out, "UPGRADE READINESS: %s → %s\n", currentVer, o.TargetVersion)
	_, _ = fmt.Fprintln(o.Out, "=============================================================")

	// Reuse the lint table output logic
	return outputTable(o.Out, resultsByCategory)
}

// determineExitCode returns an error if fail-on conditions are met.
func (o *UpgradeOptions) determineExitCode(resultsByCategory map[check.CheckCategory][]check.CheckExecution) error {
	var hasCritical, hasWarning bool

	for _, results := range resultsByCategory {
		for _, result := range results {
			if result.Result.Severity != nil {
				//nolint:revive // exhaustive linter requires explicit SeverityInfo case
				switch *result.Result.Severity {
				case check.SeverityCritical:
					hasCritical = true
				case check.SeverityWarning:
					hasWarning = true
				case check.SeverityInfo:
					// Info doesn't affect exit code
				default:
					// Unknown severities don't affect exit code
				}
			}
		}
	}

	if o.FailOnCritical && hasCritical {
		return errors.New("critical findings detected")
	}

	if o.FailOnWarning && hasWarning {
		return errors.New("warning findings detected")
	}

	return nil
}
