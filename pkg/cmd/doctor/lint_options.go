package doctor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"sigs.k8s.io/yaml"

	"github.com/lburgazzoli/odh-cli/pkg/doctor/check"
	"github.com/lburgazzoli/odh-cli/pkg/doctor/discovery"
	"github.com/lburgazzoli/odh-cli/pkg/doctor/version"
)

// LintOptions contains options for the lint command.
type LintOptions struct {
	*SharedOptions
}

// NewLintOptions creates a new LintOptions with defaults.
func NewLintOptions(shared *SharedOptions) *LintOptions {
	return &LintOptions{
		SharedOptions: shared,
	}
}

// Complete populates LintOptions and performs pre-validation setup.
func (o *LintOptions) Complete() error {
	// Complete shared options (creates client)
	if err := o.SharedOptions.Complete(); err != nil {
		return fmt.Errorf("completing shared options: %w", err)
	}

	return nil
}

// Validate checks that all required options are valid.
func (o *LintOptions) Validate() error {
	// Validate shared options
	if err := o.SharedOptions.Validate(); err != nil {
		return fmt.Errorf("validating shared options: %w", err)
	}

	return nil
}

// Run executes the lint command.
func (o *LintOptions) Run(ctx context.Context) error {
	// Create context with timeout to prevent hanging on slow clusters
	ctx, cancel := context.WithTimeout(ctx, o.Timeout)
	defer cancel()

	// Detect cluster version
	clusterVersion, err := version.Detect(ctx, o.Client)
	if err != nil {
		return fmt.Errorf("detecting cluster version: %w", err)
	}

	_, _ = fmt.Fprintf(o.Out, "Detected OpenShift AI version: %s\n\n", clusterVersion)

	// Discover components and services
	_, _ = fmt.Fprint(o.Out, "Discovering OpenShift AI components and services...\n")
	components, err := discovery.DiscoverComponentsAndServices(ctx, o.Client)
	if err != nil {
		return fmt.Errorf("discovering components and services: %w", err)
	}
	_, _ = fmt.Fprintf(o.Out, "Found %d API groups\n", len(components))
	for _, comp := range components {
		_, _ = fmt.Fprintf(o.Out, "  - %s/%s (%d resources)\n", comp.APIGroup, comp.Version, len(comp.Resources))
	}
	_, _ = fmt.Fprintln(o.Out)

	// Discover workloads
	_, _ = fmt.Fprint(o.Out, "Discovering workload custom resources...\n")
	workloads, err := discovery.DiscoverWorkloads(ctx, o.Client)
	if err != nil {
		return fmt.Errorf("discovering workloads: %w", err)
	}
	_, _ = fmt.Fprintf(o.Out, "Found %d workload types\n", len(workloads))
	for _, gvr := range workloads {
		_, _ = fmt.Fprintf(o.Out, "  - %s/%s %s\n", gvr.Group, gvr.Version, gvr.Resource)
	}
	_, _ = fmt.Fprintln(o.Out)

	// Get the global check registry
	registry := check.GetGlobalRegistry()

	// Execute component and service checks (Resource: nil)
	_, _ = fmt.Fprint(o.Out, "Running component and service checks...\n")
	componentTarget := &check.CheckTarget{
		Client:         o.Client,
		CurrentVersion: clusterVersion, // For lint, current = target
		Version:        clusterVersion,
		Resource:       nil, // No specific resource for component/service checks
	}

	executor := check.NewExecutor(registry)

	// Execute all component checks
	componentResults, err := executor.ExecuteSelective(ctx, componentTarget, o.CheckSelector, check.CategoryComponent)
	if err != nil {
		// Log error but continue with other checks
		_, _ = fmt.Fprintf(o.ErrOut, "Warning: Failed to execute component checks: %v\n", err)
		componentResults = []check.CheckExecution{}
	}

	// Execute all service checks
	serviceResults, err := executor.ExecuteSelective(ctx, componentTarget, o.CheckSelector, check.CategoryService)
	if err != nil {
		// Log error but continue with other checks
		_, _ = fmt.Fprintf(o.ErrOut, "Warning: Failed to execute service checks: %v\n", err)
		serviceResults = []check.CheckExecution{}
	}

	// Execute all dependency checks
	dependencyResults, err := executor.ExecuteSelective(ctx, componentTarget, o.CheckSelector, check.CategoryDependency)
	if err != nil {
		// Log error but continue with other checks
		_, _ = fmt.Fprintf(o.ErrOut, "Warning: Failed to execute dependency checks: %v\n", err)
		dependencyResults = []check.CheckExecution{}
	}

	// Execute workload checks for each discovered workload instance
	_, _ = fmt.Fprint(o.Out, "Running workload checks...\n")
	var workloadResults []check.CheckExecution

	for _, gvr := range workloads {
		// List all instances of this workload type
		instances, err := o.Client.ListResources(ctx, gvr)
		if err != nil {
			// Skip workloads we can't access
			_, _ = fmt.Fprintf(o.ErrOut, "Warning: Failed to list %s: %v\n", gvr.Resource, err)

			continue
		}

		// Run workload checks for each instance
		for i := range instances {
			workloadTarget := &check.CheckTarget{
				Client:         o.Client,
				CurrentVersion: clusterVersion, // For lint, current = target
				Version:        clusterVersion,
				Resource:       &instances[i],
			}

			results, err := executor.ExecuteSelective(ctx, workloadTarget, o.CheckSelector, check.CategoryWorkload)
			if err != nil {
				return fmt.Errorf("executing workload checks: %w", err)
			}

			workloadResults = append(workloadResults, results...)
		}
	}

	// Group results by category
	resultsByCategory := map[check.CheckCategory][]check.CheckExecution{
		check.CategoryComponent:  componentResults,
		check.CategoryService:    serviceResults,
		check.CategoryDependency: dependencyResults,
		check.CategoryWorkload:   workloadResults,
	}

	// Filter results by minimum severity if specified
	filteredResults := filterResultsBySeverity(resultsByCategory, o.MinSeverity)

	// Format and output results based on output format
	if err := o.formatAndOutputResults(filteredResults); err != nil {
		return err
	}

	// Determine exit code based on fail-on flags
	return o.determineExitCode(filteredResults)
}

// filterResultsBySeverity filters check results based on minimum severity level.
func filterResultsBySeverity(
	resultsByCategory map[check.CheckCategory][]check.CheckExecution,
	minSeverity MinimumSeverity,
) map[check.CheckCategory][]check.CheckExecution {
	// If no filtering requested, return original results
	if minSeverity == MinimumSeverityAll {
		return resultsByCategory
	}

	filtered := make(map[check.CheckCategory][]check.CheckExecution)
	for category, results := range resultsByCategory {
		var categoryResults []check.CheckExecution
		for _, result := range results {
			// Always include pass/error results (no severity)
			// Include results that match the minimum severity filter
			if minSeverity.ShouldInclude(result.Result.Severity) {
				categoryResults = append(categoryResults, result)
			}
		}
		filtered[category] = categoryResults
	}

	return filtered
}

// determineExitCode returns an error if fail-on conditions are met.
func (o *LintOptions) determineExitCode(resultsByCategory map[check.CheckCategory][]check.CheckExecution) error {
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

// formatAndOutputResults formats and outputs check results based on the output format.
func (o *LintOptions) formatAndOutputResults(resultsByCategory map[check.CheckCategory][]check.CheckExecution) error {
	switch o.OutputFormat {
	case OutputFormatTable:
		return o.outputTable(resultsByCategory)
	case OutputFormatJSON:
		return o.outputJSON(resultsByCategory)
	case OutputFormatYAML:
		return o.outputYAML(resultsByCategory)
	default:
		return fmt.Errorf("unsupported output format: %s", o.OutputFormat)
	}
}

// outputTable outputs results in table format.
func (o *LintOptions) outputTable(resultsByCategory map[check.CheckCategory][]check.CheckExecution) error {
	_, _ = fmt.Fprintln(o.Out)
	_, _ = fmt.Fprintln(o.Out, "Check Results:")
	_, _ = fmt.Fprintln(o.Out, "==============")

	return outputTable(o.Out, resultsByCategory)
}

// CheckResultOutput represents a check result for JSON/YAML output.
type CheckResultOutput struct {
	CheckID     string         `json:"checkId"               yaml:"checkId"`
	CheckName   string         `json:"checkName"             yaml:"checkName"`
	Category    string         `json:"category"              yaml:"category"`
	Status      string         `json:"status"                yaml:"status"`
	Severity    *string        `json:"severity,omitempty"    yaml:"severity,omitempty"`
	Message     string         `json:"message"               yaml:"message"`
	Remediation string         `json:"remediation,omitempty" yaml:"remediation,omitempty"`
	Details     map[string]any `json:"details,omitempty"     yaml:"details,omitempty"`
}

// LintOutput represents the full lint output for JSON/YAML.
type LintOutput struct {
	Components   []CheckResultOutput `json:"components"   yaml:"components"`
	Services     []CheckResultOutput `json:"services"     yaml:"services"`
	Dependencies []CheckResultOutput `json:"dependencies" yaml:"dependencies"`
	Workloads    []CheckResultOutput `json:"workloads"    yaml:"workloads"`
	Summary      struct {
		Total  int `json:"total"  yaml:"total"`
		Passed int `json:"passed" yaml:"passed"`
		Failed int `json:"failed" yaml:"failed"`
	} `json:"summary" yaml:"summary"`
}

// convertToOutputFormat converts check executions to output format.
func convertToOutputFormat(resultsByCategory map[check.CheckCategory][]check.CheckExecution) *LintOutput {
	output := &LintOutput{
		Components:   make([]CheckResultOutput, 0),
		Services:     make([]CheckResultOutput, 0),
		Dependencies: make([]CheckResultOutput, 0),
		Workloads:    make([]CheckResultOutput, 0),
	}

	for category, results := range resultsByCategory {
		for _, exec := range results {
			var severityStr *string
			if exec.Result.Severity != nil {
				s := string(*exec.Result.Severity)
				severityStr = &s
			}

			result := CheckResultOutput{
				CheckID:     exec.Check.ID(),
				CheckName:   exec.Check.Name(),
				Category:    string(exec.Check.Category()),
				Status:      string(exec.Result.Status),
				Severity:    severityStr,
				Message:     exec.Result.Message,
				Remediation: exec.Result.Remediation,
				Details:     exec.Result.Details,
			}

			output.Summary.Total++
			if exec.Result.IsFailing() {
				output.Summary.Failed++
			} else {
				output.Summary.Passed++
			}

			switch category {
			case check.CategoryComponent:
				output.Components = append(output.Components, result)
			case check.CategoryService:
				output.Services = append(output.Services, result)
			case check.CategoryDependency:
				output.Dependencies = append(output.Dependencies, result)
			case check.CategoryWorkload:
				output.Workloads = append(output.Workloads, result)
			default:
				// Unreachable: all check categories are handled above
			}
		}
	}

	return output
}

// outputJSON outputs results in JSON format.
func (o *LintOptions) outputJSON(resultsByCategory map[check.CheckCategory][]check.CheckExecution) error {
	return outputJSON(o.Out, resultsByCategory)
}

// outputYAML outputs results in YAML format.
func (o *LintOptions) outputYAML(resultsByCategory map[check.CheckCategory][]check.CheckExecution) error {
	return outputYAML(o.Out, resultsByCategory)
}

// Shared output functions used by both lint and upgrade commands

// outputTable is a shared function for outputting check results in table format.
func outputTable(out io.Writer, resultsByCategory map[check.CheckCategory][]check.CheckExecution) error {
	categories := []check.CheckCategory{
		check.CategoryComponent,
		check.CategoryService,
		check.CategoryDependency,
		check.CategoryWorkload,
	}

	totalChecks := 0
	totalPassed := 0
	totalFailed := 0

	for _, category := range categories {
		results := resultsByCategory[category]
		if len(results) == 0 {
			continue
		}

		_, _ = fmt.Fprintf(out, "\n%s Checks:\n", category)
		_, _ = fmt.Fprintln(out, "---")

		for _, exec := range results {
			totalChecks++

			status := "✓"
			if exec.Result.IsFailing() {
				status = "✗"
				totalFailed++
			} else {
				totalPassed++
			}

			severity := ""
			if exec.Result.Severity != nil {
				severity = fmt.Sprintf("[%s] ", *exec.Result.Severity)
			}

			_, _ = fmt.Fprintf(out, "%s %s %s- %s\n", status, exec.Check.Name(), severity, exec.Result.Message)

			if exec.Result.Remediation != "" && exec.Result.IsFailing() {
				_, _ = fmt.Fprintf(out, "  Remediation: %s\n", exec.Result.Remediation)
			}
		}
	}

	_, _ = fmt.Fprintln(out)
	_, _ = fmt.Fprintln(out, "Summary:")
	_, _ = fmt.Fprintf(out, "  Total: %d | Passed: %d | Failed: %d\n", totalChecks, totalPassed, totalFailed)

	return nil
}

// outputJSON is a shared function for outputting check results in JSON format.
func outputJSON(out io.Writer, resultsByCategory map[check.CheckCategory][]check.CheckExecution) error {
	output := convertToOutputFormat(resultsByCategory)

	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")

	if err := encoder.Encode(output); err != nil {
		return fmt.Errorf("encoding JSON: %w", err)
	}

	return nil
}

// outputYAML is a shared function for outputting check results in YAML format.
func outputYAML(out io.Writer, resultsByCategory map[check.CheckCategory][]check.CheckExecution) error {
	output := convertToOutputFormat(resultsByCategory)

	yamlBytes, err := yaml.Marshal(output)
	if err != nil {
		return fmt.Errorf("encoding YAML: %w", err)
	}

	_, _ = fmt.Fprint(out, string(yamlBytes))

	return nil
}
