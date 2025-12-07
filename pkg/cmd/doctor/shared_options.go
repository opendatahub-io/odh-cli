package doctor

import (
	"errors"
	"fmt"
	"path"
	"time"

	"k8s.io/cli-runtime/pkg/genericclioptions"

	"github.com/lburgazzoli/odh-cli/pkg/doctor/check"
	"github.com/lburgazzoli/odh-cli/pkg/util/client"
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
func (m MinimumSeverity) ShouldInclude(severity *check.Severity) bool {
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
		return *severity == check.SeverityCritical
	}

	// For warning filter, show critical and warning
	if m == MinimumSeverityWarning {
		return *severity == check.SeverityCritical || *severity == check.SeverityWarning
	}

	// Default: show all
	return true
}

// SharedOptions contains options common to all doctor subcommands.
type SharedOptions struct {
	// IOStreams provides access to stdin, stdout, stderr
	genericclioptions.IOStreams

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

	// Timeout is the maximum duration for command execution
	Timeout time.Duration

	// Client is the Kubernetes client (populated during Complete)
	Client *client.Client
}

// NewSharedOptions creates a new SharedOptions with defaults.
func NewSharedOptions(streams genericclioptions.IOStreams) *SharedOptions {
	return &SharedOptions{
		ConfigFlags:    genericclioptions.NewConfigFlags(true),
		OutputFormat:   OutputFormatTable,
		CheckSelector:  "*",                // Run all checks by default
		MinSeverity:    MinimumSeverityAll, // Show all severity levels by default
		FailOnCritical: true,               // Exit with error on critical findings (default)
		FailOnWarning:  false,              // Don't exit on warnings by default
		Timeout:        DefaultTimeout,     // Default timeout to prevent hanging on slow clusters
		IOStreams:      streams,
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
