package lint

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"time"

	"github.com/fatih/color"
	"github.com/olekukonko/tablewriter"
	"github.com/olekukonko/tablewriter/tw"
	"sigs.k8s.io/yaml"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/genericiooptions"

	"github.com/lburgazzoli/odh-cli/pkg/lint/check"
	"github.com/lburgazzoli/odh-cli/pkg/lint/check/result"
	"github.com/lburgazzoli/odh-cli/pkg/printer/table"
	"github.com/lburgazzoli/odh-cli/pkg/util/client"
	"github.com/lburgazzoli/odh-cli/pkg/util/iostreams"
)

// OutputFormat represents the output format for doctor commands.
type OutputFormat string

const (
	OutputFormatTable OutputFormat = "table"
	OutputFormatJSON  OutputFormat = "json"
	OutputFormatYAML  OutputFormat = "yaml"

	// DefaultTimeout is the default timeout for doctor commands.
	DefaultTimeout = 5 * time.Minute
)

// Validate checks if the output format is valid.
func (o OutputFormat) Validate() error {
	switch o {
	case OutputFormatTable, OutputFormatJSON, OutputFormatYAML:
		return nil
	default:
		return fmt.Errorf("invalid output format: %s (must be one of: table, json, yaml)", o)
	}
}

// MinimumSeverity represents the minimum severity level to display in results.
type MinimumSeverity string

const (
	MinimumSeverityCritical MinimumSeverity = "critical"
	MinimumSeverityWarning  MinimumSeverity = "warning"
	MinimumSeverityInfo     MinimumSeverity = "info"
	MinimumSeverityAll      MinimumSeverity = "" // Empty string means show all
)

// Validate checks if the minimum severity is valid.
func (m MinimumSeverity) Validate() error {
	switch m {
	case MinimumSeverityCritical, MinimumSeverityWarning, MinimumSeverityInfo, MinimumSeverityAll:
		return nil
	default:
		return fmt.Errorf("invalid minimum severity: %s (must be one of: critical, warning, info)", m)
	}
}

// ShouldInclude returns true if a check result with the given severity should be included.
func (m MinimumSeverity) ShouldInclude(severity *string) bool {
	// Always include pass/error results
	if severity == nil {
		return true
	}

	// If showing all or info (which includes all), return true
	if m == MinimumSeverityAll || m == MinimumSeverityInfo {
		return true
	}

	// For critical filter, only show critical
	if m == MinimumSeverityCritical {
		return *severity == string(check.SeverityCritical)
	}

	// For warning filter, show critical and warning
	if m == MinimumSeverityWarning {
		return *severity == string(check.SeverityCritical) || *severity == string(check.SeverityWarning)
	}

	// Default: show all
	return true
}

// SharedOptions contains options common to all doctor subcommands.
type SharedOptions struct {
	// IO provides structured access to stdin, stdout, stderr with convenience methods
	IO iostreams.Interface

	// ConfigFlags provides access to kubeconfig and context
	ConfigFlags *genericclioptions.ConfigFlags

	// OutputFormat specifies the output format (table, json, yaml)
	OutputFormat OutputFormat

	// CheckSelector filters which checks to run (glob pattern)
	CheckSelector string

	// MinSeverity filters results by minimum severity level
	MinSeverity MinimumSeverity

	// FailOnCritical exits with non-zero code if critical findings detected
	FailOnCritical bool

	// FailOnWarning exits with non-zero code if warning findings detected
	FailOnWarning bool

	// Verbose enables progress messages (default: false, quiet by default)
	Verbose bool

	// Timeout is the maximum duration for command execution
	Timeout time.Duration

	// Client is the Kubernetes client (populated during Complete)
	Client *client.Client
}

// NewSharedOptions creates a new SharedOptions with defaults.
func NewSharedOptions(streams genericiooptions.IOStreams) *SharedOptions {
	return &SharedOptions{
		ConfigFlags:    genericclioptions.NewConfigFlags(true),
		OutputFormat:   OutputFormatTable,
		CheckSelector:  "*",                // Run all checks by default
		MinSeverity:    MinimumSeverityAll, // Show all severity levels by default
		FailOnCritical: true,               // Exit with error on critical findings (default)
		FailOnWarning:  false,              // Don't exit on warnings by default
		Timeout:        DefaultTimeout,     // Default timeout to prevent hanging on slow clusters
		IO:             iostreams.NewIOStreams(streams.In, streams.Out, streams.ErrOut),
	}
}

// Complete populates the client and performs pre-validation setup.
func (o *SharedOptions) Complete() error {
	// Create the unified client
	c, err := client.NewClient(o.ConfigFlags)
	if err != nil {
		return fmt.Errorf("failed to create Kubernetes client: %w", err)
	}

	o.Client = c

	return nil
}

// Validate checks that all required options are valid.
func (o *SharedOptions) Validate() error {
	// Validate output format
	if err := o.OutputFormat.Validate(); err != nil {
		return err
	}

	// Validate check selector
	if err := ValidateCheckSelector(o.CheckSelector); err != nil {
		return err
	}

	// Validate minimum severity
	if err := o.MinSeverity.Validate(); err != nil {
		return err
	}

	// Validate timeout
	if o.Timeout <= 0 {
		return errors.New("timeout must be greater than 0")
	}

	return nil
}

// ValidateCheckSelector validates the check selector pattern.
func ValidateCheckSelector(selector string) error {
	if selector == "" {
		return errors.New("check selector cannot be empty")
	}

	// Allow category shortcuts
	if selector == "components" || selector == "services" || selector == "workloads" || selector == "dependencies" {
		return nil
	}

	// Allow wildcard (default)
	if selector == "*" {
		return nil
	}

	// Validate glob pattern
	_, err := path.Match(selector, "test.check")
	if err != nil {
		return fmt.Errorf("invalid check selector pattern %q: %w", selector, err)
	}

	return nil
}

// CheckResultOutput represents a check result for JSON/YAML output.
type CheckResultOutput struct {
	CheckID     string         `json:"checkId"               yaml:"checkId"`
	CheckName   string         `json:"checkName"             yaml:"checkName"`
	Group       string         `json:"group"                 yaml:"group"`
	Status      string         `json:"status"                yaml:"status"`
	Severity    *string        `json:"severity,omitempty"    yaml:"severity,omitempty"`
	Message     string         `json:"message"               yaml:"message"`
	Remediation string         `json:"remediation,omitempty" yaml:"remediation,omitempty"`
	Details     map[string]any `json:"details,omitempty"     yaml:"details,omitempty"`
}

// CheckResultTableRow represents a single condition row for table output.
// Each row represents one condition from a diagnostic result.
type CheckResultTableRow struct {
	Status      string
	Group       string
	Kind        string
	Check       string
	Severity    string
	Message     string
	Description string
}

// LintOutput represents the full lint output for JSON/YAML.
type LintOutput struct {
	ClusterVersion *string             `json:"clusterVersion,omitempty" yaml:"clusterVersion,omitempty"`
	TargetVersion  *string             `json:"targetVersion,omitempty"  yaml:"targetVersion,omitempty"`
	Components     []CheckResultOutput `json:"components"               yaml:"components"`
	Services       []CheckResultOutput `json:"services"                 yaml:"services"`
	Dependencies   []CheckResultOutput `json:"dependencies"             yaml:"dependencies"`
	Workloads      []CheckResultOutput `json:"workloads"                yaml:"workloads"`
	Summary        struct {
		Total  int `json:"total"  yaml:"total"`
		Passed int `json:"passed" yaml:"passed"`
		Failed int `json:"failed" yaml:"failed"`
	} `json:"summary" yaml:"summary"`
}

// FilterResultsBySeverity filters check results based on minimum severity level.
func FilterResultsBySeverity(
	resultsByGroup map[check.CheckGroup][]check.CheckExecution,
	minSeverity MinimumSeverity,
) map[check.CheckGroup][]check.CheckExecution {
	// If no filtering requested, return original results
	if minSeverity == MinimumSeverityAll {
		return resultsByGroup
	}

	filtered := make(map[check.CheckGroup][]check.CheckExecution)
	for group, results := range resultsByGroup {
		var groupResults []check.CheckExecution
		for _, res := range results {
			// Always include pass/error results (no severity)
			// Include results that match the minimum severity filter
			if minSeverity.ShouldInclude(res.Result.GetSeverity()) {
				groupResults = append(groupResults, res)
			}
		}
		filtered[group] = groupResults
	}

	return filtered
}

// FlattenResults converts a map of results by group to a flat sorted array.
// Results are sorted by group in the order: Component, Service, Dependency, Workload.
func FlattenResults(resultsByGroup map[check.CheckGroup][]check.CheckExecution) []check.CheckExecution {
	groups := []check.CheckGroup{
		check.GroupComponent,
		check.GroupService,
		check.GroupDependency,
		check.GroupWorkload,
	}

	flattened := make([]check.CheckExecution, 0)
	for _, group := range groups {
		flattened = append(flattened, resultsByGroup[group]...)
	}

	return flattened
}

// OutputTable is a shared function for outputting check results in table format.
//
//nolint:revive // verbose boolean is clear and appropriate for controlling output detail level
func OutputTable(out io.Writer, results []check.CheckExecution, verbose bool) error {
	totalChecks := 0
	totalPassed := 0
	totalFailed := 0

	// Pre-compute color formatters
	var (
		statusPass   = color.New(color.FgGreen).Sprint("✓")
		statusFail   = color.New(color.FgRed).Sprint("✗")
		severityCrit = color.New(color.FgRed).Sprint("critical")
		severityWarn = color.New(color.FgYellow).Sprint("warning")
		severityInfo = color.New(color.FgCyan).Sprint("info")
	)

	// Build headers based on verbose mode
	var headers []string
	if verbose {
		headers = []string{"STATUS", "GROUP", "KIND", "CHECK", "SEVERITY", "MESSAGE", "DESCRIPTION"}
	} else {
		headers = []string{"STATUS", "GROUP", "KIND", "CHECK", "SEVERITY", "MESSAGE"}
	}

	// Create single table renderer for all results
	renderer := table.NewRenderer[CheckResultTableRow](
		table.WithWriter[CheckResultTableRow](out),
		table.WithHeaders[CheckResultTableRow](headers...),
		table.WithTableOptions[CheckResultTableRow](
			tablewriter.WithHeaderAlignment(tw.AlignLeft),
			tablewriter.WithRendition(tw.Rendition{
				Settings: tw.Settings{
					Separators: tw.Separators{
						BetweenColumns: tw.Off,
						BetweenRows:    tw.Off,
					},
				},
			}),
		),
	)

	// Append all results to single table - one row per condition
	for _, exec := range results {
		// Each diagnostic result can have multiple conditions
		// Create one table row per condition
		for _, condition := range exec.Result.Status.Conditions {
			totalChecks++

			// Determine status symbol based on condition status
			var status string
			if condition.Status == metav1.ConditionFalse || condition.Status == metav1.ConditionUnknown {
				status = statusFail
				totalFailed++
			} else {
				status = statusPass
				totalPassed++
			}

			// Determine severity from condition status
			var severity string

			switch condition.Status {
			case metav1.ConditionFalse:
				severity = severityCrit
			case metav1.ConditionTrue:
				severity = severityInfo
			case metav1.ConditionUnknown:
				severity = severityWarn
			}

			msg := condition.Message

			if len(msg) > 1024 {
				msg = msg[:1024] + "..."
			}

			row := CheckResultTableRow{
				Status:      status,
				Group:       string(exec.Check.Group()),
				Kind:        exec.Result.Metadata.Kind,
				Check:       exec.Result.Metadata.Name,
				Severity:    severity,
				Message:     msg,
				Description: exec.Result.Spec.Description,
			}

			if err := renderer.Append(row); err != nil {
				return fmt.Errorf("appending table row: %w", err)
			}
		}
	}

	if err := renderer.Render(); err != nil {
		return fmt.Errorf("rendering table: %w", err)
	}

	_, _ = fmt.Fprintln(out)
	_, _ = fmt.Fprintln(out, "Summary:")
	_, _ = fmt.Fprintf(out, "  Total: %d | Passed: %d | Failed: %d\n", totalChecks, totalPassed, totalFailed)

	return nil
}

// OutputJSON outputs diagnostic results in List format.
func OutputJSON(out io.Writer, results []check.CheckExecution, clusterVersion *string, targetVersion *string) error {
	// Create the list
	list := result.NewDiagnosticResultList(clusterVersion, targetVersion)

	// Add all results in execution order
	for _, exec := range results {
		list.Items = append(list.Items, exec.Result)
	}

	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")

	if err := encoder.Encode(list); err != nil {
		return fmt.Errorf("encoding JSON: %w", err)
	}

	return nil
}

// OutputYAML outputs diagnostic results in List format.
func OutputYAML(out io.Writer, results []check.CheckExecution, clusterVersion *string, targetVersion *string) error {
	// Create the list
	list := result.NewDiagnosticResultList(clusterVersion, targetVersion)

	// Add all results in execution order
	for _, exec := range results {
		list.Items = append(list.Items, exec.Result)
	}

	yamlBytes, err := yaml.Marshal(list)
	if err != nil {
		return fmt.Errorf("encoding YAML: %w", err)
	}

	_, _ = fmt.Fprint(out, string(yamlBytes))

	return nil
}
