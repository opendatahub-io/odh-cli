package lint

import (
	"errors"
	"fmt"
	"io"
	"path"
	"sort"
	"time"

	"github.com/fatih/color"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	printerjson "github.com/opendatahub-io/odh-cli/pkg/printer/json"
	"github.com/opendatahub-io/odh-cli/pkg/printer/table"
	printeryaml "github.com/opendatahub-io/odh-cli/pkg/printer/yaml"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
	"github.com/opendatahub-io/odh-cli/pkg/util/iostreams"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/genericiooptions"
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
	tableHeaders = []string{"STATUS", "GROUP", "KIND", "CHECK", "IMPACT", "MESSAGE"}
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

	// FailOnCritical exits with non-zero code if critical findings detected
	FailOnCritical bool

	// FailOnWarning exits with non-zero code if warning findings detected
	FailOnWarning bool

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
		FailOnCritical: true,           // Exit with error on critical findings (default)
		FailOnWarning:  false,          // Don't exit on warnings by default
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
	Group       string
	Kind        string
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

// TableOutputOptions configures the behavior of OutputTable.
type TableOutputOptions struct {
	// ShowImpactedObjects enables listing impacted objects after the summary.
	ShowImpactedObjects bool

	// NamespaceRequesters maps namespace names to their openshift.io/requester annotation value.
	// Used when ShowImpactedObjects is true to display the requester for each namespace group.
	NamespaceRequesters map[string]string
}

// OutputTable is a shared function for outputting check results in table format.
// When opts.ShowImpactedObjects is true, impacted objects are listed after the summary.
func OutputTable(out io.Writer, results []check.CheckExecution, opts TableOutputOptions) error {
	totalChecks := 0
	totalPassed := 0
	totalWarnings := 0
	totalFailed := 0

	// Create single table renderer for all results
	renderer := table.NewRenderer[CheckResultTableRow](
		table.WithWriter[CheckResultTableRow](out),
		table.WithHeaders[CheckResultTableRow](tableHeaders...),
		table.WithTableOptions[CheckResultTableRow](table.DefaultTableOptions...),
	)

	// Append all results to single table - one row per condition
	for _, exec := range results {
		// Each diagnostic result can have multiple conditions
		// Create one table row per condition
		for _, condition := range exec.Result.Status.Conditions {
			totalChecks++

			// Determine impact display string from condition.
			impact := getImpactString(&condition, severityCrit, severityWarn, severityInfo)

			// Determine status symbol and count based on impact.
			var status string

			switch impact {
			case severityCrit:
				// Blocking impact = failed check
				status = statusFail
				totalFailed++
			case severityWarn:
				// Advisory impact = warning (not counted as failure)
				status = statusWarn
				totalWarnings++
			default:
				// No impact/success
				status = statusPass
				totalPassed++
			}

			row := CheckResultTableRow{
				Status:      status,
				Group:       exec.Result.Group,
				Kind:        exec.Result.Kind,
				Check:       exec.Result.Name,
				Impact:      impact,
				Message:     condition.Message,
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
	_, _ = fmt.Fprintf(out, "  Total: %d | Passed: %d | Warnings: %d | Failed: %d\n", totalChecks, totalPassed, totalWarnings, totalFailed)

	if opts.ShowImpactedObjects {
		outputImpactedObjects(out, results, opts.NamespaceRequesters)
	}

	return nil
}

// impactedGroup holds aggregated impacted objects for a specific check.
type impactedGroup struct {
	group     check.CheckGroup
	kind      string
	checkType check.CheckType
	objects   []metav1.PartialObjectMetadata
}

// namespaceGroup holds objects within a single namespace for display.
type namespaceGroup struct {
	namespace string
	objects   []metav1.PartialObjectMetadata
}

// outputImpactedObjects prints impacted objects grouped by group/kind/checkType and namespace.
// Within each group/kind/checkType, objects are sub-grouped by namespace (sorted alphabetically).
// Each namespace header includes the openshift.io/requester annotation if available.
func outputImpactedObjects(
	out io.Writer,
	results []check.CheckExecution,
	namespaceRequesters map[string]string,
) {
	// Aggregate objects by group/kind/checkType, preserving insertion order.
	var groups []impactedGroup

	type groupKey struct {
		group     check.CheckGroup
		kind      string
		checkType check.CheckType
	}

	seen := make(map[groupKey]int) // key -> index in groups slice

	for _, exec := range results {
		if len(exec.Result.ImpactedObjects) == 0 {
			continue
		}

		key := groupKey{
			group:     check.CheckGroup(exec.Result.Group),
			kind:      exec.Result.Kind,
			checkType: check.CheckType(exec.Result.Name),
		}

		if idx, ok := seen[key]; ok {
			groups[idx].objects = append(groups[idx].objects, exec.Result.ImpactedObjects...)
		} else {
			seen[key] = len(groups)
			groups = append(groups, impactedGroup{
				group:     key.group,
				kind:      key.kind,
				checkType: key.checkType,
				objects:   append([]metav1.PartialObjectMetadata{}, exec.Result.ImpactedObjects...),
			})
		}
	}

	if len(groups) == 0 {
		return
	}

	_, _ = fmt.Fprintln(out)
	_, _ = fmt.Fprintln(out, "Impacted Objects:")

	for i, g := range groups {
		if i > 0 {
			_, _ = fmt.Fprintln(out)
		}

		_, _ = fmt.Fprintf(out, "  %s / %s / %s:\n", g.group, g.kind, g.checkType)

		// Check for group renderer first (takes precedence).
		if groupRenderer := check.GetImpactedGroupRenderer(g.group, g.kind, g.checkType); groupRenderer != nil {
			// Pass total count as maxDisplay since upstream removed truncation
			groupRenderer(out, g.objects, len(g.objects))

			continue
		}

		// Fall back to namespace-grouped rendering.
		// Sub-group objects by namespace.
		nsGroups := groupByNamespace(g.objects)

		for _, nsg := range nsGroups {
			if nsg.namespace == "" {
				// Cluster-scoped objects: list directly without namespace header.
				for _, obj := range nsg.objects {
					_, _ = fmt.Fprintf(out, "    - %s\n", formatImpactedObject(obj))
				}
			} else {
				// Print namespace header with requester annotation if available.
				nsHeader := nsg.namespace
				if requester, ok := namespaceRequesters[nsg.namespace]; ok && requester != "" {
					nsHeader = fmt.Sprintf("%s (requester: %s)", nsg.namespace, requester)
				}

				_, _ = fmt.Fprintf(out, "    %s:\n", nsHeader)

				for _, obj := range nsg.objects {
					_, _ = fmt.Fprintf(out, "      - %s\n", formatImpactedObject(obj))
				}
			}
		}
	}
}

// formatImpactedObject returns the display string for an impacted object.
// Includes the Kind from TypeMeta when available to help identify the resource type.
func formatImpactedObject(obj metav1.PartialObjectMetadata) string {
	if obj.Kind != "" {
		return fmt.Sprintf("%s (%s)", obj.Name, obj.Kind)
	}

	return obj.Name
}

// groupByNamespace sub-groups objects by namespace, sorted alphabetically.
// Cluster-scoped objects (empty namespace) are placed first.
func groupByNamespace(objects []metav1.PartialObjectMetadata) []namespaceGroup {
	nsMap := make(map[string][]metav1.PartialObjectMetadata)

	for _, obj := range objects {
		nsMap[obj.Namespace] = append(nsMap[obj.Namespace], obj)
	}

	// Collect and sort namespace keys.
	namespaces := make([]string, 0, len(nsMap))
	for ns := range nsMap {
		namespaces = append(namespaces, ns)
	}

	sort.Strings(namespaces)

	groups := make([]namespaceGroup, 0, len(namespaces))
	for _, ns := range namespaces {
		groups = append(groups, namespaceGroup{
			namespace: ns,
			objects:   nsMap[ns],
		})
	}

	return groups
}

// OutputJSON outputs diagnostic results in List format.
func OutputJSON(out io.Writer, results []check.CheckExecution, clusterVersion *string, targetVersion *string) error {
	// Create the list
	list := result.NewDiagnosticResultList(clusterVersion, targetVersion)

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
func OutputYAML(out io.Writer, results []check.CheckExecution, clusterVersion *string, targetVersion *string) error {
	// Create the list
	list := result.NewDiagnosticResultList(clusterVersion, targetVersion)

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
