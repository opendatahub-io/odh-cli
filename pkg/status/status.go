package status

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/blang/semver/v4"
	"github.com/fatih/color"
	"github.com/opendatahub-io/opendatahub-operator/pkg/clusterhealth"
	"github.com/spf13/pflag"
	"golang.org/x/sync/errgroup"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	"k8s.io/client-go/rest"

	"github.com/opendatahub-io/odh-cli/pkg/api"
	"github.com/opendatahub-io/odh-cli/pkg/cmd"
	"github.com/opendatahub-io/odh-cli/pkg/deps"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
	"github.com/opendatahub-io/odh-cli/pkg/util/iostreams"
	"github.com/opendatahub-io/odh-cli/pkg/util/version"
)

var _ cmd.Command = (*Command)(nil)

// OutputFormat represents the output format for the status command.
type OutputFormat string

const (
	OutputFormatTable OutputFormat = "table"
	OutputFormatJSON  OutputFormat = "json"
	OutputFormatYAML  OutputFormat = "yaml"

	DefaultTimeout = 30 * time.Second

	WaitConditionHealthy = "healthy"

	// Default operator deployment name for RHOAI.
	defaultRHOAIOperatorName = "rhods-operator"

	// Output format strings.
	fmtPlatformStatus   = "PLATFORM STATUS: %s\n"
	fmtEnvironmentHdr   = "\nEnvironment:\n"
	fmtPlatformVersion  = "  RHOAI Version:      %s\n"
	fmtOpenShiftVersion = "  OpenShift Version:  %s\n"
)

const (
	flagDescOutput      = `Output format: "table", "json", or "yaml"`
	flagDescVerbose     = "Show per-item details for each section"
	flagDescSection     = "Limit output to specific sections (repeatable): nodes, deployments, pods, events, quotas, operator, dsci, dsc"
	flagDescLayer       = "Limit output to a layer (repeatable): infrastructure (nodes), workload (deployments, pods, events, quotas), operator (operator, dsci, dsc)"
	flagDescNoColor     = "Disable color output"
	flagDescTimeout     = "Maximum time to wait for health checks to complete"
	flagDescAppsNS      = "Override the applications namespace (auto-detected from DSCI)"
	flagDescOperNS      = "Override the operator namespace (auto-detected from OLM/CSV)"
	flagDescOperName    = "Override the operator deployment name (auto-detected from CSV)"
	flagDescInfra       = "Also scan kube-system for core infrastructure health"
	flagDescIncludeDeps = "Include dependency operator status in output"
	flagDescQPS         = "Kubernetes API queries per second"
	flagDescBurst       = "Kubernetes API burst capacity"
)

// Command contains the status command configuration.
type Command struct {
	cmd.WaitOptions
	cmd.WatchOptions

	IO          iostreams.Interface
	ConfigFlags *genericclioptions.ConfigFlags

	OutputFormat OutputFormat
	Verbose      bool
	NoColor      bool
	Timeout      time.Duration
	Sections     []string
	Layers       []string

	AppsNamespace     string
	OperatorNamespace string
	OperatorName      string
	IncludeInfra      bool
	IncludeDeps       bool

	QPS   float32
	Burst int

	// TimeoutExplicit is true when --timeout was explicitly passed by the user.
	// When false and --watch is active, timeout defaults to 0 (indefinite).
	TimeoutExplicit bool

	// Populated during Complete
	restConfig *rest.Config
	client     client.Client

	// healthConfig, when non-nil, skips buildHealthConfig and uses this
	// config directly. Allows tests to inject a fake controller-runtime client.
	healthConfig *clusterhealth.Config
}

// NewCommand creates a new status Command with defaults.
func NewCommand(
	streams genericiooptions.IOStreams,
	configFlags *genericclioptions.ConfigFlags,
) *Command {
	return &Command{
		IO:           iostreams.NewIOStreams(streams.In, streams.Out, streams.ErrOut),
		ConfigFlags:  configFlags,
		OutputFormat: OutputFormatTable,
		Timeout:      DefaultTimeout,
		WaitOptions: cmd.WaitOptions{
			PollInterval: cmd.DefaultPollInterval,
		},
		IncludeDeps: true,
		QPS:         client.DefaultQPS,
		Burst:       client.DefaultBurst,
	}
}

// AddFlags registers command-specific flags with the provided FlagSet.
func (c *Command) AddFlags(fs *pflag.FlagSet) {
	fs.StringVarP((*string)(&c.OutputFormat), "output", "o", string(OutputFormatTable), flagDescOutput)
	_ = fs.SetAnnotation("output", api.AnnotationValidValues, []string{"table", "json", "yaml"})
	fs.BoolVarP(&c.Verbose, "verbose", "v", false, flagDescVerbose)
	fs.StringArrayVar(&c.Sections, "section", nil, flagDescSection)
	fs.StringArrayVar(&c.Layers, "layer", nil, flagDescLayer)
	fs.BoolVar(&c.NoColor, "no-color", false, flagDescNoColor)
	fs.DurationVar(&c.Timeout, "timeout", c.Timeout, flagDescTimeout)
	fs.StringVar(&c.AppsNamespace, "apps-namespace", "", flagDescAppsNS)
	fs.StringVar(&c.OperatorNamespace, "operator-namespace", "", flagDescOperNS)
	fs.StringVar(&c.OperatorName, "operator-name", "", flagDescOperName)
	fs.BoolVar(&c.IncludeInfra, "include-infra", false, flagDescInfra)
	fs.BoolVar(&c.IncludeDeps, "include-deps", c.IncludeDeps, flagDescIncludeDeps)
	c.AddWaitFlags(fs, []string{WaitConditionHealthy})
	c.AddWatchFlag(fs)
	fs.Float32Var(&c.QPS, "qps", c.QPS, flagDescQPS)
	fs.IntVar(&c.Burst, "burst", c.Burst, flagDescBurst)
}

// Complete populates the client and performs pre-validation setup.
func (c *Command) Complete() error {
	restConfig, err := client.NewRESTConfig(c.ConfigFlags, c.QPS, c.Burst)
	if err != nil {
		return fmt.Errorf("failed to create REST config: %w", err)
	}

	c.restConfig = restConfig

	k8sClient, err := client.NewClientWithConfig(restConfig)
	if err != nil {
		return fmt.Errorf("failed to create Kubernetes client: %w", err)
	}

	c.client = k8sClient

	if c.OutputFormat == OutputFormatJSON || c.OutputFormat == OutputFormatYAML {
		c.NoColor = true
	}

	color.NoColor = c.NoColor

	return nil
}

// Validate checks that all required options are valid.
func (c *Command) Validate() error {
	if err := c.OutputFormat.Validate(); err != nil {
		return err
	}

	if err := validateSections(c.Sections); err != nil {
		return err
	}

	if err := validateLayers(c.Layers); err != nil {
		return err
	}

	if err := c.ValidateWatch(c.HasWaitMode(), string(c.OutputFormat), []string{"json", "yaml"}, c.Timeout, c.PollInterval); err != nil {
		return err //nolint:wrapcheck // structured validation errors propagate as-is
	}

	if err := c.ValidateWait(c.Timeout); err != nil {
		return err //nolint:wrapcheck // structured validation errors propagate as-is
	}

	if !c.HasWaitMode() && !c.Watch && c.Timeout <= 0 {
		return ErrInvalidTimeout()
	}

	return nil
}

// Run executes the status command.
func (c *Command) Run(ctx context.Context) error {
	if c.Watch {
		return c.runWatch(ctx)
	}

	if c.HasWaitMode() {
		return c.runWaitFor(ctx)
	}

	ctx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()

	hc, err := c.resolveHealthConfig(ctx)
	if err != nil {
		return err
	}

	report, depStatuses, err := c.runHealthCheck(ctx, hc)
	if err != nil {
		return err
	}

	return c.output(ctx, report, depStatuses)
}

// checkDependencies checks operator dependency status.
// Returns nil slice on error (non-fatal).
func (c *Command) checkDependencies(ctx context.Context) []deps.DependencyStatus {
	result, err := deps.GetManifest(ctx, false)
	if err != nil {
		if c.Verbose || c.IncludeDeps {
			_, _ = fmt.Fprintf(c.IO.ErrOut(), "Dependencies: skipped (manifest unavailable: %v)\n", err)
		}

		return nil
	}

	statuses, err := deps.CheckDependencies(ctx, c.client, result.Manifest)
	if err != nil {
		if c.Verbose || c.IncludeDeps {
			if errors.Is(err, deps.ErrOLMNotAvailable) {
				_, _ = fmt.Fprintln(c.IO.ErrOut(), "Dependencies: skipped (OLM not available)")
			} else {
				_, _ = fmt.Fprintf(c.IO.ErrOut(), "Dependencies: skipped (check failed: %v)\n", err)
			}
		}

		return nil
	}

	return statuses
}

// output renders the report in the requested format.
func (c *Command) output(ctx context.Context, report *clusterhealth.Report, depStatuses []deps.DependencyStatus) error {
	switch c.OutputFormat {
	case OutputFormatTable:
		return c.outputTable(ctx, report, depStatuses)
	case OutputFormatJSON:
		return c.outputJSON(report, depStatuses)
	case OutputFormatYAML:
		return c.outputYAML(report, depStatuses)
	default:
		return fmt.Errorf("unsupported output format: %s", c.OutputFormat)
	}
}

// outputTable renders the report as a human-readable table.
func (c *Command) outputTable(ctx context.Context, report *clusterhealth.Report, depStatuses []deps.DependencyStatus) error {
	w := c.IO.Out()

	// Detect versions concurrently
	var ver, ocpVer *semver.Version

	g, gctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		v, err := version.Detect(gctx, c.client)
		if err == nil {
			ver = v
		}

		return nil
	})

	g.Go(func() error {
		v, err := version.DetectOpenShiftVersion(gctx, c.client)
		if err == nil {
			ocpVer = v
		}

		return nil
	})

	_ = g.Wait()

	// Print PLATFORM STATUS header
	if _, err := fmt.Fprintf(w, fmtPlatformStatus, formatPlatformStatus(report)); err != nil {
		return fmt.Errorf("writing platform status: %w", err)
	}

	// Print Environment section
	if err := c.writeEnvironmentSection(w, ver, ocpVer); err != nil {
		return err
	}

	if _, err := fmt.Fprintln(w); err != nil {
		return fmt.Errorf("writing newline: %w", err)
	}

	return c.renderTableOutput(report, depStatuses)
}

// formatPlatformStatus returns colored "Healthy" or "Unhealthy" based on report state.
func formatPlatformStatus(report *clusterhealth.Report) string {
	if report.Healthy() {
		return color.GreenString("Healthy")
	}

	return color.RedString("Unhealthy")
}

// writeEnvironmentSection writes the Environment header and version info if available.
func (c *Command) writeEnvironmentSection(w io.Writer, ver, ocpVer *semver.Version) error {
	if ver == nil && ocpVer == nil {
		return nil
	}

	if _, err := fmt.Fprint(w, fmtEnvironmentHdr); err != nil {
		return fmt.Errorf("writing environment header: %w", err)
	}

	if ver != nil {
		if _, err := fmt.Fprintf(w, fmtPlatformVersion, ver.String()); err != nil {
			return fmt.Errorf("writing platform version: %w", err)
		}
	}

	if ocpVer != nil {
		if _, err := fmt.Fprintf(w, fmtOpenShiftVersion, ocpVer.String()); err != nil {
			return fmt.Errorf("writing OpenShift version: %w", err)
		}
	}

	return nil
}

// outputJSON renders the report as JSON with envelope.
func (c *Command) outputJSON(report *clusterhealth.Report, depStatuses []deps.DependencyStatus) error {
	return renderJSON(c.IO.Out(), report, depStatuses)
}

// outputYAML renders the report as YAML with envelope.
func (c *Command) outputYAML(report *clusterhealth.Report, depStatuses []deps.DependencyStatus) error {
	return renderYAML(c.IO.Out(), report, depStatuses)
}

// Validate checks if the output format is valid.
func (o OutputFormat) Validate() error {
	switch o {
	case OutputFormatTable, OutputFormatJSON, OutputFormatYAML:
		return nil
	default:
		return ErrInvalidOutputFormat(string(o))
	}
}

func isValidSection(section string) bool {
	switch section {
	case clusterhealth.SectionNodes,
		clusterhealth.SectionDeployments,
		clusterhealth.SectionPods,
		clusterhealth.SectionEvents,
		clusterhealth.SectionQuotas,
		clusterhealth.SectionOperator,
		clusterhealth.SectionDSCI,
		clusterhealth.SectionDSC:
		return true
	default:
		return false
	}
}

func validateSections(sections []string) error {
	for _, s := range sections {
		if !isValidSection(s) {
			return ErrInvalidSection(s)
		}
	}

	return nil
}

func isValidLayer(layer string) bool {
	switch layer {
	case clusterhealth.LayerInfrastructure,
		clusterhealth.LayerWorkload,
		clusterhealth.LayerOperator:
		return true
	default:
		return false
	}
}

func validateLayers(layers []string) error {
	for _, l := range layers {
		if !isValidLayer(l) {
			return ErrInvalidLayer(l)
		}
	}

	return nil
}

// discoverCRNames returns NamespacedNames for DSCI and DSC singletons.
// Uses the pre-fetched DSCI and fetches DSC separately.
// Returns zero-value names if either CR is not found (non-fatal).
// Non-NotFound errors are propagated.
//
//nolint:nonamedreturns // named returns improve readability for multiple NamespacedName values
func discoverCRNames(
	ctx context.Context,
	c client.Reader,
	dsci *unstructured.Unstructured,
) (dsciName, dscName types.NamespacedName, err error) {
	if dsci != nil {
		dsciName = types.NamespacedName{
			Namespace: dsci.GetNamespace(),
			Name:      dsci.GetName(),
		}
	}

	dsc, err := client.GetDataScienceCluster(ctx, c)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return dsciName, dscName, nil
		}

		return dsciName, dscName, fmt.Errorf("getting DataScienceCluster: %w", err)
	}

	if dsc != nil {
		dscName = types.NamespacedName{
			Namespace: dsc.GetNamespace(),
			Name:      dsc.GetName(),
		}
	}

	return dsciName, dscName, nil
}
