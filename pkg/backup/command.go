package backup

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/spf13/pflag"
	"golang.org/x/sync/errgroup"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/cli-runtime/pkg/genericiooptions"

	"github.com/lburgazzoli/odh-cli/pkg/backup/dependencies"
	"github.com/lburgazzoli/odh-cli/pkg/backup/dependencies/notebooks"
	"github.com/lburgazzoli/odh-cli/pkg/backup/pipeline"
	"github.com/lburgazzoli/odh-cli/pkg/util/kube"
)

const (
	// Directory permissions for backup output directories.
	dirPermissions = 0o755
	// File permissions for backup YAML files.
	filePermissions = 0o644
)

// Command handles the backup operation.
type Command struct {
	*SharedOptions

	OutputDir    string
	StripFields  []string
	Includes     []string
	Excludes     []string
	MaxWorkers   int
	Dependencies bool
	DryRun       bool

	depRegistry *dependencies.Registry
}

// NewCommand creates a new backup Command.
func NewCommand(streams genericiooptions.IOStreams) *Command {
	return &Command{
		SharedOptions: NewSharedOptions(streams),
		Dependencies:  true,
	}
}

// AddFlags adds flags to the command.
func (c *Command) AddFlags(fs *pflag.FlagSet) {
	fs.StringVar(&c.OutputDir, "output-dir", "", "Output directory for backups (if not specified, dumps to stdout)")
	fs.StringArrayVar(&c.StripFields, "strip", nil, "Field paths to strip (repeatable, e.g., --strip .status)")
	fs.StringArrayVar(&c.Includes, "includes", nil, "Workload types to include (repeatable, e.g., --includes notebooks.kubeflow.org)")
	fs.StringArrayVar(&c.Excludes, "exclude", nil, "Workload types to exclude (repeatable)")
	fs.IntVar(&c.MaxWorkers, "max-workers", 0, "Maximum concurrent workers (0 = auto-detect based on CPU count)")
	fs.BoolVarP(&c.Verbose, "verbose", "v", false, "Enable verbose output")
	fs.BoolVar(&c.DryRun, "dry-run", false, "Preview backup without writing files (automatically enables verbose)")
	fs.DurationVar(&c.Timeout, "timeout", c.Timeout, "Timeout for backup operation")

	// Throttling settings
	fs.Float32Var(&c.QPS, "qps", c.QPS, "Kubernetes API QPS limit (queries per second)")
	fs.IntVar(&c.Burst, "burst", c.Burst, "Kubernetes API burst capacity")

	// Dependency resolution
	fs.BoolVar(&c.Dependencies, "dependencies", true, "Resolve and backup workload dependencies (ConfigMaps, PVCs, Secrets)")
}

// Complete populates derived values and performs setup.
func (c *Command) Complete() error {
	if err := c.SharedOptions.Complete(); err != nil {
		return err
	}

	// Auto-enable verbose mode for dry-run
	if c.DryRun {
		c.Verbose = true
	}

	if len(c.Includes) == 0 {
		c.Includes = DefaultWorkloadTypes
	}

	c.StripFields = append(DefaultStripFields, c.StripFields...)

	// Auto-detect worker count if not specified
	if c.MaxWorkers == 0 {
		c.MaxWorkers = runtime.NumCPU()
		const maxWorkersLimit = 20
		if c.MaxWorkers > maxWorkersLimit {
			c.MaxWorkers = maxWorkersLimit
		}
	}

	// Create registry - always needed even if empty
	c.depRegistry = dependencies.NewRegistry()

	// Only register resolvers if dependency resolution is enabled
	if c.Dependencies {
		c.depRegistry.MustRegister(notebooks.NewResolver())
	}

	return nil
}

// Validate checks that all options are valid.
func (c *Command) Validate() error {
	if err := c.SharedOptions.Validate(); err != nil {
		return err
	}

	return nil
}

// Run executes the backup command.
func (c *Command) Run(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()

	if c.OutputDir != "" {
		if err := os.MkdirAll(c.OutputDir, dirPermissions); err != nil {
			return fmt.Errorf("creating output directory: %w", err)
		}
	}

	gvrsToBackup := c.resolveWorkloadGVRs()

	if c.Verbose {
		mode := "with dependencies"
		if !c.Dependencies {
			mode = "without dependencies"
		}

		action := "Backing up"
		if c.DryRun {
			action = "Dry-run:"
		}

		c.IO.Errorf("%s %d workload types %s (%d workers)...",
			action, len(gvrsToBackup), mode, c.MaxWorkers)
	}

	// Create pipeline stages
	discovery := &pipeline.DiscoveryStage{
		Client:  c.Client,
		Verbose: c.Verbose,
		IO:      c.IO,
	}

	resolver := &pipeline.ResolverStage{
		Client:      c.Client,
		DepRegistry: c.depRegistry,
		Verbose:     c.Verbose,
		IO:          c.IO,
	}

	writer := &pipeline.WriterStage{
		WriteResource: c.writeResource,
		IO:            c.IO,
		DryRun:        c.DryRun,
		OutputDir:     c.OutputDir,
	}

	// Process each workload type
	for _, gvr := range gvrsToBackup {
		if err := c.runPipeline(ctx, gvr, discovery, resolver, writer); err != nil {
			c.IO.Errorf("Warning: Failed to backup %s: %v\n", gvr.Resource, err)
		}
	}

	if c.DryRun {
		c.IO.Errorf("Dry-run complete (no files written)")
	} else if c.OutputDir == "" && c.Verbose {
		c.IO.Errorf("Backup complete (stdout)")
	} else {
		c.IO.Errorf("Backup complete: %s", c.OutputDir)
	}

	return nil
}

// runPipeline executes the three-stage pipeline for a workload type.
func (c *Command) runPipeline(
	ctx context.Context,
	gvr schema.GroupVersionResource,
	discovery *pipeline.DiscoveryStage,
	resolver *pipeline.ResolverStage,
	writer *pipeline.WriterStage,
) error {
	// Create channels
	workloadCh := make(chan pipeline.WorkloadItem, c.MaxWorkers)
	resolvedCh := make(chan pipeline.WorkloadWithDeps, c.MaxWorkers)

	// Launch pipeline stages
	g, ctx := errgroup.WithContext(ctx)

	// Stage 1: Discovery
	g.Go(func() error {
		defer close(workloadCh)

		return discovery.Run(ctx, gvr, workloadCh)
	})

	// Stage 2: Dependency Resolver (N workers)
	g.Go(func() error {
		defer close(resolvedCh)

		return resolver.Run(ctx, c.MaxWorkers, workloadCh, resolvedCh)
	})

	// Stage 3: Writer
	g.Go(func() error {
		return writer.Run(ctx, resolvedCh)
	})

	if err := g.Wait(); err != nil {
		return fmt.Errorf("pipeline execution failed: %w", err)
	}

	return nil
}

// resolveWorkloadGVRs converts include/exclude strings to GVRs.
func (c *Command) resolveWorkloadGVRs() []schema.GroupVersionResource {
	//nolint:prealloc // Size unknown until references are extracted
	var includeGVRs []schema.GroupVersionResource
	for _, include := range c.Includes {
		gvr := parseGVRString(include)
		includeGVRs = append(includeGVRs, gvr)
	}

	excludeSet := make(map[string]bool)
	for _, exclude := range c.Excludes {
		gvr := parseGVRString(exclude)
		excludeSet[gvr.GroupResource().String()] = true
	}

	var result []schema.GroupVersionResource
	for _, gvr := range includeGVRs {
		if !excludeSet[gvr.GroupResource().String()] {
			result = append(result, gvr)
		}
	}

	return result
}

// writeResource strips fields and writes a resource to the output directory or stdout.
func (c *Command) writeResource(
	gvr schema.GroupVersionResource,
	obj *unstructured.Unstructured,
) error {
	stripped, err := kube.StripFields(obj, c.StripFields)
	if err != nil {
		return fmt.Errorf("stripping fields: %w", err)
	}

	// Dry-run mode: Log what would be written without actually writing
	if c.DryRun {
		return c.logDryRunResource(gvr, stripped)
	}

	if c.OutputDir == "" {
		return WriteResourceToStdout(c.IO.Out(), gvr, stripped)
	}

	return WriteResourceToFile(c.OutputDir, gvr, stripped)
}

// logDryRunResource logs the file path that would be created in dry-run mode.
func (c *Command) logDryRunResource(
	gvr schema.GroupVersionResource,
	obj *unstructured.Unstructured,
) error {
	namespace := obj.GetNamespace()
	name := obj.GetName()

	if c.OutputDir == "" {
		// Stdout mode: Show resource that would be written
		c.IO.Errorf("Would write to stdout: %s/%s (%s)",
			namespace, name, gvr.Resource)

		return nil
	}

	// File mode: Show file path that would be created
	if namespace == "" {
		namespace = "cluster-scoped"
	}

	gvrStr := gvr.Resource
	if gvr.Group != "" {
		gvrStr = gvr.Resource + "." + gvr.Group
	}

	filename := fmt.Sprintf("%s-%s.yaml", gvrStr, name)
	filePath := filepath.Join(c.OutputDir, namespace, filename)

	c.IO.Errorf("Would create: %s", filePath)

	return nil
}
