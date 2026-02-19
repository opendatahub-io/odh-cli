package lint

import (
	"errors"
	"fmt"
	"io"
	"path"
	"sort"
	"time"

	"github.com/fatih/color"

	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/genericiooptions"

	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	printerjson "github.com/opendatahub-io/odh-cli/pkg/printer/json"
	"github.com/opendatahub-io/odh-cli/pkg/printer/table"
	printeryaml "github.com/opendatahub-io/odh-cli/pkg/printer/yaml"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
	"github.com/opendatahub-io/odh-cli/pkg/util/iostreams"
)

// OutputFormat represents the output format for lint commands.
type OutputFormat string

const (
	OutputFormatTable OutputFormat = "table"
	OutputFormatJSON  OutputFormat = "json"
	OutputFormatYAML  OutputFormat = "yaml"

	// DefaultTimeout is the default timeout for lint commands.
	DefaultTimeout = 5 * time.Minute
)

//nolint:gochecknoglobals
var (
	// Table output symbols.
	statusPass = color.New(color.FgGreen).Sprint("✓")
	statusWarn = color.New(color.FgYellow).Sprint("⚠")
	statusFail = color.New(color.FgRed).Sprint("✗")

	// Severity level formatting.
	severityCrit = color.New(color.FgRed).Sprint("critical")
	severityWarn = color.New(color.FgYellow).Add(color.Bold).Sprint("warning") // Bold yellow (orange-ish)
	severityInfo = color.New(color.FgCyan).Sprint("info")

	// Table headers.
	tableHeaders = []string{"STATUS", "KIND", "GROUP", "CHECK", "IMPACT", "MESSAGE"}
)

// printVerdict prints the Result section after the summary.
func printVerdict(out io.Writer, hasBlocking bool, hasAdvisory bool) {
	_, _ = fmt.Fprintln(out)
	_, _ = fmt.Fprintln(out, "Result:")

	switch {
	case hasBlocking:
		verdict := color.New(color.FgRed, color.Bold).Sprint("FAIL")
		_, _ = fmt.Fprintf(out, "  %s - blocking findings detected\n", verdict)
	case hasAdvisory:
		verdict := color.New(color.FgYellow, color.Bold).Sprint("WARNING")
		_, _ = fmt.Fprintf(out, "  %s - advisory findings detected\n", verdict)
	default:
		verdict := color.New(color.FgGreen, color.Bold).Sprint("PASS")
		_, _ = fmt.Fprintf(out, "  %s - all checks passed\n", verdict)
	}
}

// Validate checks if the output format is valid.
func (o OutputFormat) Validate() error {
	switch o {
	case OutputFormatTable, OutputFormatJSON, OutputFormatYAML:
		return nil
	default:
		return fmt.Errorf("invalid output format: %s (must be one of: table, json, yaml)", o)
	}
}

// SharedOptions contains options common to all lint subcommands.
type SharedOptions struct {
	// IO provides structured access to stdin, stdout, stderr with convenience methods
	IO iostreams.Interface

	// ConfigFlags provides access to kubeconfig and context
	ConfigFlags *genericclioptions.ConfigFlags

	// OutputFormat specifies the output format (table, json, yaml)
	OutputFormat OutputFormat

	// CheckSelectors filters which checks to run (glob patterns, repeatable)
	CheckSelectors []string

	// Verbose enables progress messages (default: false, quiet by default)
	Verbose bool

	// Debug enables detailed diagnostic logging for troubleshooting (default: false)
	Debug bool

	// Timeout is the maximum duration for command execution
	Timeout time.Duration

	// Client is the Kubernetes client (populated during Complete)
	Client client.Client

	// Throttling settings for Kubernetes API client
	QPS   float32
	Burst int
}

// NewSharedOptions creates a new SharedOptions with defaults.
// ConfigFlags must be provided by the caller to ensure CLI auth flags are properly propagated.
func NewSharedOptions(
	streams genericiooptions.IOStreams,
	configFlags *genericclioptions.ConfigFlags,
) *SharedOptions {
	return &SharedOptions{
		ConfigFlags:    configFlags,
		OutputFormat:   OutputFormatTable,
		CheckSelectors: []string{"*"},  // Run all checks by default
		Timeout:        DefaultTimeout, // Default timeout to prevent hanging on slow clusters
		IO:             iostreams.NewIOStreams(streams.In, streams.Out, streams.ErrOut),
		QPS:            client.DefaultQPS,
		Burst:          client.DefaultBurst,
	}
}

// Complete populates the client and performs pre-validation setup.
func (o *SharedOptions) Complete() error {
	// Create REST config with user-specified throttling
	restConfig, err := client.NewRESTConfig(o.ConfigFlags, o.QPS, o.Burst)
	if err != nil {
		return fmt.Errorf("failed to create REST config: %w", err)
	}

	// Create client with configured throttling
	c, err := client.NewClientWithConfig(restConfig)
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

	// Validate check selectors
	if err := ValidateCheckSelectors(o.CheckSelectors); err != nil {
		return err
	}

	// Validate timeout
	if o.Timeout <= 0 {
		return errors.New("timeout must be greater than 0")
	}

	return nil
}

// ValidateCheckSelectors validates all check selector patterns.
func ValidateCheckSelectors(selectors []string) error {
	if len(selectors) == 0 {
		return errors.New("at least one check selector is required")
	}

	for _, s := range selectors {
		if err := ValidateCheckSelector(s); err != nil {
			return err
		}
	}

	return nil
}

// ValidateCheckSelector validates a single check selector pattern.
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

// CommandOption is a functional option for configuring a Command.
// Use with NewCommand for optional configuration like target version.
//
// Example:
//
//	cmd := lint.NewCommand(streams, configFlags,
//	    lint.WithTargetVersion("3.0"),
//	)
type CommandOption func(*Command)

// WithTargetVersion returns a CommandOption that sets the target version.
func WithTargetVersion(version string) CommandOption {
	return func(c *Command) {
		c.TargetVersion = version
	}
}

// CheckResultOutput represents a check result for JSON/YAML output.
type CheckResultOutput struct {
	CheckID     string         `json:"checkId"               yaml:"checkId"`
	CheckName   string         `json:"checkName"             yaml:"checkName"`
	Group       string         `json:"group"                 yaml:"group"`
	Status      string         `json:"status"                yaml:"status"`
	Impact      *string        `json:"impact,omitempty"      yaml:"impact,omitempty"`
	Message     string         `json:"message"               yaml:"message"`
	Remediation string         `json:"remediation,omitempty" yaml:"remediation,omitempty"`
	Details     map[string]any `json:"details,omitempty"     yaml:"details,omitempty"`
}

// CheckResultTableRow represents a single condition row for table output.
// Each row represents one condition from a diagnostic result.
type CheckResultTableRow struct {
	Status      string
	Kind        string
	Group       string
	Check       string
	Impact      string
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

// FlattenResults converts a map of results by group to a flat sorted array.
// Results are sorted by:
// 1. Group (canonical order: Dependency, Service, Component, Workload)
// 2. Kind (alphabetically within each group)
// 3. Name (alphabetically within each kind).
func FlattenResults(resultsByGroup map[check.CheckGroup][]check.CheckExecution) []check.CheckExecution {
	//nolint:prealloc // Small result set; extra iteration to calculate capacity isn't worth the complexity.
	flattened := make([]check.CheckExecution, 0)

	// Iterate through groups in canonical order
	for _, group := range check.CanonicalGroupOrder {
		groupResults := resultsByGroup[group]

		// Sort within group by Kind, then by Name
		sort.Slice(groupResults, func(i, j int) bool {
			// First compare by Kind
			if groupResults[i].Result.Kind != groupResults[j].Result.Kind {
				return groupResults[i].Result.Kind < groupResults[j].Result.Kind
			}
			// If Kind is the same, compare by Name
			return groupResults[i].Result.Name < groupResults[j].Result.Name
		})

		flattened = append(flattened, groupResults...)
	}

	return flattened
}

// getImpactString determines the display string from a condition's impact.
func getImpactString(
	condition *result.Condition,
	blockingStr string,
	advisoryStr string,
	noneStr string,
) string {
	// Use Impact field directly (always set by NewCondition).
	switch condition.Impact {
	case result.ImpactBlocking:
		return blockingStr
	case result.ImpactAdvisory:
		return advisoryStr
	case result.ImpactNone:
		return noneStr
	}
	// Unreachable - all Impact values handled above
	return noneStr
}

// sortableRow pairs a table row with the raw impact for sort comparisons.
type sortableRow struct {
	row    CheckResultTableRow
	impact result.Impact
}

// Impact sort priorities (lower = higher severity).
const (
	impactPriorityBlocking = iota
	impactPriorityAdvisory
	impactPriorityNone
)

// impactSortPriority returns a numeric priority so blocking (critical) sorts
// before advisory (warning) which sorts before none (info).
func impactSortPriority(impact result.Impact) int {
	switch impact {
	case result.ImpactBlocking:
		return impactPriorityBlocking
	case result.ImpactAdvisory:
		return impactPriorityAdvisory
	case result.ImpactNone:
		return impactPriorityNone
	}

	return impactPriorityNone
}

// groupSortPriority returns a numeric priority that follows the canonical
// group order: dependency -> service -> component -> workload.
func groupSortPriority(group string) int {
	for i, g := range check.CanonicalGroupOrder {
		if string(g) == group {
			return i
		}
	}

	return len(check.CanonicalGroupOrder)
}

// VersionInfo holds version data for display in the status report.
type VersionInfo struct {
	RHOAICurrentVersion string
	RHOAITargetVersion  string // empty in lint mode
	OpenShiftVersion    string
}

// TableOutputOptions configures the behavior of OutputTable.
type TableOutputOptions struct {
	// VersionInfo contains version data to display in the Environment section.
	VersionInfo *VersionInfo

	// ShowImpactedObjects enables listing impacted objects after the summary.
	ShowImpactedObjects bool

	// NamespaceRequesters maps namespace names to their openshift.io/requester annotation value.
	// Used when ShowImpactedObjects is true to display the requester for each namespace group.
	NamespaceRequesters map[string]string
}

// collectSortedRows builds table rows from check executions and sorts them
// by Group (canonical) -> Kind -> Impact (critical, warning, info) -> Check.
func collectSortedRows(results []check.CheckExecution) []sortableRow {
	totalConditions := 0
	for _, exec := range results {
		totalConditions += len(exec.Result.Status.Conditions)
	}

	rows := make([]sortableRow, 0, totalConditions)

	for _, exec := range results {
		for _, condition := range exec.Result.Status.Conditions {
			rows = append(rows, sortableRow{
				row: CheckResultTableRow{
					Status:      statusSymbol(condition.Impact),
					Kind:        exec.Result.Kind,
					Group:       exec.Result.Group,
					Check:       exec.Result.Name,
					Impact:      getImpactString(&condition, severityCrit, severityWarn, severityInfo),
					Message:     condition.Message,
					Description: exec.Result.Spec.Description,
				},
				impact: condition.Impact,
			})
		}
	}

	sort.Slice(rows, func(i, j int) bool {
		gi, gj := groupSortPriority(rows[i].row.Group), groupSortPriority(rows[j].row.Group)
		if gi != gj {
			return gi < gj
		}

		if rows[i].row.Kind != rows[j].row.Kind {
			return rows[i].row.Kind < rows[j].row.Kind
		}

		pi, pj := impactSortPriority(rows[i].impact), impactSortPriority(rows[j].impact)
		if pi != pj {
			return pi < pj
		}

		return rows[i].row.Check < rows[j].row.Check
	})

	return rows
}

// statusSymbol returns the colored status symbol for the given impact level.
func statusSymbol(impact result.Impact) string {
	switch impact {
	case result.ImpactBlocking:
		return statusFail
	case result.ImpactAdvisory:
		return statusWarn
	case result.ImpactNone:
		return statusPass
	}

	return statusPass
}

// OutputTable is a shared function for outputting check results in table format.
// When opts.ShowImpactedObjects is true, impacted objects are listed after the summary.
func OutputTable(out io.Writer, results []check.CheckExecution, opts TableOutputOptions) error {
	rows := collectSortedRows(results)

	renderer := table.NewRenderer[CheckResultTableRow](
		table.WithWriter[CheckResultTableRow](out),
		table.WithHeaders[CheckResultTableRow](tableHeaders...),
		table.WithTableOptions[CheckResultTableRow](table.DefaultTableOptions...),
	)

	totalChecks := 0
	totalPassed := 0
	totalWarnings := 0
	totalFailed := 0

	for _, sr := range rows {
		totalChecks++

		switch sr.impact {
		case result.ImpactBlocking:
			totalFailed++
		case result.ImpactAdvisory:
			totalWarnings++
		case result.ImpactNone:
			totalPassed++
		}

		if err := renderer.Append(sr.row); err != nil {
			return fmt.Errorf("appending table row: %w", err)
		}
	}

	if err := renderer.Render(); err != nil {
		return fmt.Errorf("rendering table: %w", err)
	}

	if opts.VersionInfo != nil {
		_, _ = fmt.Fprintln(out)
		outputVersionInfo(out, opts.VersionInfo)
	}

	_, _ = fmt.Fprintln(out)
	_, _ = fmt.Fprintln(out, "Summary:")
	_, _ = fmt.Fprintf(out, "  Total: %d | Passed: %d | Warnings: %d | Failed: %d\n", totalChecks, totalPassed, totalWarnings, totalFailed)

	if opts.ShowImpactedObjects {
		outputImpactedObjects(out, results, opts.NamespaceRequesters)
	}

	return nil
}

// outputVersionInfo prints the Environment section with version details.
func outputVersionInfo(out io.Writer, info *VersionInfo) {
	_, _ = fmt.Fprintln(out, "Environment:")

	if info.RHOAITargetVersion != "" {
		_, _ = fmt.Fprintf(out, "  OpenShift AI version: %s -> %s\n", info.RHOAICurrentVersion, info.RHOAITargetVersion)
	} else {
		_, _ = fmt.Fprintf(out, "  OpenShift AI version: %s\n", info.RHOAICurrentVersion)
	}

	if info.OpenShiftVersion != "" {
		_, _ = fmt.Fprintf(out, "  OpenShift version:    %s\n", info.OpenShiftVersion)
	}
}

// outputImpactedObjects prints impacted objects for each check execution.
// Each check's objects are rendered by the check's VerboseOutputFormatter if implemented,
// or by the default namespace-grouped renderer otherwise.
func outputImpactedObjects(
	out io.Writer,
	results []check.CheckExecution,
	namespaceRequesters map[string]string,
) {
	defaultFmt := &check.DefaultVerboseFormatter{
		NamespaceRequesters: namespaceRequesters,
	}

	printed := false

	for _, exec := range results {
		if len(exec.Result.ImpactedObjects) == 0 {
			continue
		}

		if !printed {
			_, _ = fmt.Fprintln(out)
			_, _ = fmt.Fprintln(out, "Impacted Objects:")

			printed = true
		} else {
			_, _ = fmt.Fprintln(out)
		}

		_, _ = fmt.Fprintf(out, "  %s / %s / %s:\n", exec.Result.Group, exec.Result.Kind, exec.Result.Name)

		if f, ok := exec.Check.(check.VerboseOutputFormatter); ok {
			f.FormatVerboseOutput(out, exec.Result)
		} else {
			defaultFmt.FormatVerboseOutput(out, exec.Result)
		}
	}
}

// OutputJSON outputs diagnostic results in List format.
func OutputJSON(
	out io.Writer,
	results []check.CheckExecution,
	clusterVersion *string,
	targetVersion *string,
	openShiftVersion *string,
) error {
	// Create the list
	list := result.NewDiagnosticResultList(clusterVersion, targetVersion, openShiftVersion)

	// Add all results in execution order
	for _, exec := range results {
		list.Results = append(list.Results, exec.Result)
	}

	renderer := printerjson.NewRenderer[*result.DiagnosticResultList](
		printerjson.WithWriter[*result.DiagnosticResultList](out),
	)

	if err := renderer.Render(list); err != nil {
		return fmt.Errorf("rendering JSON output: %w", err)
	}

	return nil
}

// OutputYAML outputs diagnostic results in List format.
func OutputYAML(
	out io.Writer,
	results []check.CheckExecution,
	clusterVersion *string,
	targetVersion *string,
	openShiftVersion *string,
) error {
	// Create the list
	list := result.NewDiagnosticResultList(clusterVersion, targetVersion, openShiftVersion)

	// Add all results in execution order
	for _, exec := range results {
		list.Results = append(list.Results, exec.Result)
	}

	renderer := printeryaml.NewRenderer[*result.DiagnosticResultList](
		printeryaml.WithWriter[*result.DiagnosticResultList](out),
	)

	if err := renderer.Render(list); err != nil {
		return fmt.Errorf("rendering YAML output: %w", err)
	}

	return nil
}
