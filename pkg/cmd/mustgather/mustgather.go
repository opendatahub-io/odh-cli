package mustgather

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/spf13/pflag"

	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/genericiooptions"

	"github.com/opendatahub-io/odh-cli/pkg/cmd"
)

// Verify Command implements cmd.Command interface at compile time.
var _ cmd.Command = (*Command)(nil)

const (
	// Default path where must-gather scripts are bundled in the container.
	defaultScriptsPath = "/opt/must-gather"
	// Default output directory for must-gather collection.
	defaultOutputDir = "/tmp/must-gather"
)

// supportedComponents returns the list of currently available components.
func supportedComponents() []string {
	return []string{
		"llm-d",
	}
}

// plannedComponents returns the list of components that are planned but not yet implemented.
func plannedComponents() []string {
	return []string{
		"all",
		"dashboard",
		"dsp",
		"kserve",
		"kuberay",
		"kueue",
		"kfto",
		"mr",
		"notebooks",
		"trustyai",
	}
}

// Command holds the must-gather command configuration.
type Command struct {
	streams     genericiooptions.IOStreams
	configFlags *genericclioptions.ConfigFlags

	// Flags
	Component      string
	Since          string
	ScriptsPath    string
	ListComponents bool
}

// NewCommand creates a new must-gather command.
func NewCommand(
	streams genericiooptions.IOStreams,
	configFlags *genericclioptions.ConfigFlags,
) *Command {
	return &Command{
		streams:     streams,
		configFlags: configFlags,
		ScriptsPath: defaultScriptsPath,
	}
}

// AddFlags adds flags to the command.
// Flags take precedence over environment variables (COMPONENT, MUST_GATHER_SINCE).
func (c *Command) AddFlags(fs *pflag.FlagSet) {
	fs.StringVar(&c.Component, "component", "",
		"Collect for specific component (e.g., llm-d). Currently only llm-d is supported. Overrides COMPONENT environment variable")
	fs.StringVar(&c.Since, "since", "",
		"Only collect logs newer than duration (e.g., 1h, 30m). Overrides MUST_GATHER_SINCE environment variable")
	fs.StringVar(&c.ScriptsPath, "scripts-path", defaultScriptsPath,
		"Path to must-gather collection scripts")
	fs.BoolVar(&c.ListComponents, "list-components", false,
		"List available components and exit")
}

// Complete initializes command state.
func (c *Command) Complete() error {
	return nil
}

// Validate checks command configuration.
func (c *Command) Validate() error {
	// Skip validation for --list-components (doesn't need scripts or component validation)
	if c.ListComponents {
		return nil
	}

	// Check if scripts path exists (fundamental requirement - fail fast)
	gatherScript := filepath.Join(c.ScriptsPath, "gather")
	if _, err := os.Stat(gatherScript); os.IsNotExist(err) {
		return fmt.Errorf("gather script not found at %s - are you running in the container?", gatherScript)
	}

	// Validate '--component' value
	if c.Component != "" {
		comp := strings.TrimSpace(c.Component)

		// Check if component is supported
		if slices.Contains(supportedComponents(), comp) {
			return nil
		}

		// Check if component is planned (not yet implemented)
		if slices.Contains(plannedComponents(), comp) {
			return fmt.Errorf("component '%s' is planned but not yet supported. Use --list-components to see currently available components", comp)
		}

		// Unknown component
		return fmt.Errorf("unknown component '%s'. Use --list-components to see supported and planned components", comp)
	}

	return nil
}

// Run executes the must-gather collection.
func (c *Command) Run(ctx context.Context) error {
	if c.ListComponents {
		return c.listComponents()
	}

	_, _ = fmt.Fprintf(c.streams.Out, "Collecting diagnostic information to %s\n", defaultOutputDir)

	// Ignore env variable if already set but only take values from flags
	env := make([]string, 0, len(os.Environ()))
	for _, e := range os.Environ() {
		if !strings.HasPrefix(e, "COMPONENT=") && !strings.HasPrefix(e, "MUST_GATHER_SINCE=") {
			env = append(env, e)
		}
	}
	if c.Component != "" {
		env = append(env, "COMPONENT="+c.Component)
		_, _ = fmt.Fprintf(c.streams.Out, "Component filter: %s\n", c.Component)
	}

	if c.Since != "" {
		env = append(env, "MUST_GATHER_SINCE="+c.Since)
		_, _ = fmt.Fprintf(c.streams.Out, "Log time range: %s\n", c.Since)
	}

	// Execute the gather script by running from /tmp then the output will be /tmp/must-gather
	gatherScript := filepath.Join(c.ScriptsPath, "gather")
	//nolint:gosec // G204: gatherScript path is validated in Validate() to exist at expected location.
	execCmd := exec.CommandContext(ctx, "/bin/bash", gatherScript)
	execCmd.Dir = "/tmp"
	execCmd.Env = env
	execCmd.Stdout = c.streams.Out
	execCmd.Stderr = c.streams.ErrOut

	_, _ = fmt.Fprint(c.streams.Out, "\nStarting collection...\n\n")

	if err := execCmd.Run(); err != nil {
		return fmt.Errorf("running gather script: %w", err)
	}

	_, _ = fmt.Fprintf(c.streams.Out, "\nCollection complete. Output written to: %s\n", defaultOutputDir)

	return nil
}

func (c *Command) listComponents() error {
	_, _ = fmt.Fprint(c.streams.Out, "Currently supported components:\n\n")
	_, _ = fmt.Fprint(c.streams.Out, "  llm-d       - LLM-D components (xKS environments: OCP, AKS, CKS)\n")

	_, _ = fmt.Fprint(c.streams.Out, "\nPlanned components (not yet available):\n\n")

	// Component descriptions matching the plannedComponents constant
	descriptions := map[string]string{
		"all":       "Collect all components",
		"dashboard": "OpenShift AI Dashboard",
		"dsp":       "Data Science Pipelines",
		"kserve":    "KServe model serving",
		"kuberay":   "KubeRay for distributed computing",
		"kueue":     "Kueue for job queuing",
		"kfto":      "Kubeflow Training Operator",
		"mr":        "Model Registry",
		"notebooks": "Workbenches/Notebooks",
		"trustyai":  "TrustyAI explainability",
	}

	for _, comp := range plannedComponents() {
		desc := descriptions[comp]
		_, _ = fmt.Fprintf(c.streams.Out, "  %-12s - %s\n", comp, desc)
	}

	_, _ = fmt.Fprint(c.streams.Out, "\nUsage: odh-cli must-gather --component llm-d\n")

	return nil
}
