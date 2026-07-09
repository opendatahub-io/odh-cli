package mcp

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/genericiooptions"

	"github.com/opendatahub-io/odh-cli/pkg/backup"
	pkgcmd "github.com/opendatahub-io/odh-cli/pkg/cmd"
	"github.com/opendatahub-io/odh-cli/pkg/components"
	"github.com/opendatahub-io/odh-cli/pkg/deps"
	"github.com/opendatahub-io/odh-cli/pkg/events"
	"github.com/opendatahub-io/odh-cli/pkg/get"
	"github.com/opendatahub-io/odh-cli/pkg/lint"
	"github.com/opendatahub-io/odh-cli/pkg/logs"
	"github.com/opendatahub-io/odh-cli/pkg/migrate"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/status"
)

const outputJSON = "json"

func parseDuration(request mcp.CallToolRequest, key, fallback string) (time.Duration, error) {
	raw := request.GetString(key, fallback)
	if raw == "" {
		return 0, nil
	}

	d, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid %s duration %q: %w", key, raw, err)
	}

	return d, nil
}

// allTools returns all MCP tool definitions wired to the given ConfigFlags.
func allTools(configFlags *genericclioptions.ConfigFlags) []toolDefinition {
	return []toolDefinition{
		statusTool(configFlags),
		lintTool(configFlags),
		getTool(configFlags),
		depsTool(configFlags),
		depsInstallTool(configFlags),
		backupTool(configFlags),
		logsTool(configFlags),
		eventsTool(configFlags),
		componentsListTool(configFlags),
		componentsDescribeTool(configFlags),
		migrateListTool(configFlags),
		migrateRunTool(configFlags),
	}
}

// --- odh_status ---

func statusTool(configFlags *genericclioptions.ConfigFlags) toolDefinition {
	tool := mcp.NewTool("odh_status",
		mcp.WithDescription("Show platform health and version information for OpenShift AI / RHOAI"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithBoolean("verbose", mcp.Description("Show per-item details for each section")),
		mcp.WithArray("sections", mcp.Description("Limit output to specific sections"),
			mcp.WithStringEnumItems([]string{"nodes", "deployments", "pods", "events", "quotas", "operator", "dsci", "dsc"})),
		mcp.WithArray("layers", mcp.Description("Limit output to specific layers"),
			mcp.WithStringEnumItems([]string{"infrastructure", "workload", "operator"})),
		mcp.WithString("timeout", mcp.Description("Maximum time to wait (Go duration, e.g. '30s')"), mcp.DefaultString("30s")),
		mcp.WithString("apps_namespace", mcp.Description("Override the applications namespace")),
		mcp.WithString("operator_namespace", mcp.Description("Override the operator namespace")),
		mcp.WithBoolean("include_deps", mcp.Description("Include dependency operator status"), mcp.DefaultBool(true)),
	)

	adapter := &toolAdapter{
		configFlags: configFlags,
		factory: func(streams genericiooptions.IOStreams, flags *genericclioptions.ConfigFlags) pkgcmd.Command {
			return status.NewCommand(streams, flags)
		},
		applier: applyStatusArgs,
	}

	return toolDefinition{tool: tool, handler: adapter.handle}
}

func applyStatusArgs(command pkgcmd.Command, request mcp.CallToolRequest) error {
	cmd, ok := command.(*status.Command)
	if !ok {
		return fmt.Errorf("unexpected command type: %T", command)
	}

	cmd.OutputFormat = status.OutputFormatJSON
	cmd.NoColor = true
	cmd.Verbose = request.GetBool("verbose", false)
	cmd.Sections = request.GetStringSlice("sections", nil)
	cmd.Layers = request.GetStringSlice("layers", nil)
	cmd.AppsNamespace = request.GetString("apps_namespace", "")
	cmd.OperatorNamespace = request.GetString("operator_namespace", "")
	cmd.IncludeDeps = request.GetBool("include_deps", true)

	d, err := parseDuration(request, "timeout", "30s")
	if err != nil {
		return err
	}

	if d > 0 {
		cmd.Timeout = d
	}

	return nil
}

// --- odh_lint ---

func lintTool(configFlags *genericclioptions.ConfigFlags) toolDefinition {
	tool := mcp.NewTool("odh_lint",
		mcp.WithDescription("Validate current OpenShift AI installation or assess upgrade readiness"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithString("target_version", mcp.Description("Target version for upgrade assessment (e.g. '3.0')")),
		mcp.WithString("severity", mcp.Description("Minimum severity threshold"),
			mcp.Enum("prohibited", "critical", "warning", "info"), mcp.DefaultString("info")),
		mcp.WithArray("checks", mcp.Description("Check selector patterns (glob)"), mcp.WithStringItems()),
		mcp.WithString("isvc_deployment_mode", mcp.Description("Filter InferenceService by deployment mode"),
			mcp.Enum("all", "serverless", "modelmesh"), mcp.DefaultString("all")),
		mcp.WithString("timeout", mcp.Description("Maximum execution time"), mcp.DefaultString("5m")),
	)

	adapter := &toolAdapter{
		configFlags: configFlags,
		factory: func(streams genericiooptions.IOStreams, flags *genericclioptions.ConfigFlags) pkgcmd.Command {
			return lint.NewCommand(streams, flags)
		},
		applier: applyLintArgs,
	}

	return toolDefinition{tool: tool, handler: adapter.handle}
}

func applyLintArgs(command pkgcmd.Command, request mcp.CallToolRequest) error {
	cmd, ok := command.(*lint.Command)
	if !ok {
		return fmt.Errorf("unexpected command type: %T", command)
	}

	cmd.OutputFormat = lint.OutputFormatJSON
	cmd.Quiet = false
	cmd.NoColor = true
	cmd.TargetVersion = request.GetString("target_version", "")
	cmd.ISVCDeploymentMode = request.GetString("isvc_deployment_mode", "all")

	if severity := request.GetString("severity", ""); severity != "" {
		cmd.SeverityLevel = lint.SeverityLevel(severity)
	}

	if checks := request.GetStringSlice("checks", nil); len(checks) > 0 {
		cmd.CheckSelectors = checks
	}

	d, err := parseDuration(request, "timeout", "5m")
	if err != nil {
		return err
	}

	if d > 0 {
		cmd.Timeout = d
	}

	return nil
}

// --- odh_get ---

func getTool(configFlags *genericclioptions.ConfigFlags) toolDefinition {
	tool := mcp.NewTool("odh_get",
		mcp.WithDescription("Get ODH/RHOAI resources (notebooks, inference services, serving runtimes, pipelines)"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithString("resource", mcp.Required(),
			mcp.Description("Resource type"),
			mcp.Enum("notebooks", "nb", "inferenceservices", "isvc", "servingruntimes", "sr", "datasciencepipelinesapplications", "pipeline")),
		mcp.WithString("name", mcp.Description("Specific resource name")),
		mcp.WithString("namespace", mcp.Description("Target namespace")),
		mcp.WithBoolean("all_namespaces", mcp.Description("List across all namespaces")),
		mcp.WithString("selector", mcp.Description("Label selector to filter resources")),
		mcp.WithBoolean("verbose", mcp.Description("Show additional details")),
	)

	adapter := &toolAdapter{
		configFlags: configFlags,
		factory: func(streams genericiooptions.IOStreams, flags *genericclioptions.ConfigFlags) pkgcmd.Command {
			return get.NewCommand(streams, flags)
		},
		applier: applyGetArgs,
	}

	return toolDefinition{tool: tool, handler: adapter.handle}
}

func applyGetArgs(command pkgcmd.Command, request mcp.CallToolRequest) error {
	cmd, ok := command.(*get.Command)
	if !ok {
		return fmt.Errorf("unexpected command type: %T", command)
	}

	cmd.OutputFormat = outputJSON
	cmd.ResourceName = request.GetString("resource", "")
	cmd.ItemName = request.GetString("name", "")
	cmd.AllNamespaces = request.GetBool("all_namespaces", false)
	cmd.LabelSelector = request.GetString("selector", "")
	cmd.Verbose = request.GetBool("verbose", false)

	if ns := request.GetString("namespace", ""); ns != "" {
		cmd.Namespace = ns
	}

	return nil
}

// --- odh_deps ---

func depsTool(configFlags *genericclioptions.ConfigFlags) toolDefinition {
	tool := mcp.NewTool("odh_deps",
		mcp.WithDescription("Show operator dependency status for OpenShift AI / RHOAI"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithBoolean("dry_run", mcp.Description("Show manifest data without querying cluster")),
		mcp.WithString("version", mcp.Description("ODH/RHOAI version to show dependencies for")),
		mcp.WithBoolean("refresh", mcp.Description("Fetch latest manifest from odh-gitops")),
		mcp.WithBoolean("verbose", mcp.Description("Enable verbose output")),
	)

	adapter := &toolAdapter{
		configFlags: configFlags,
		factory: func(streams genericiooptions.IOStreams, flags *genericclioptions.ConfigFlags) pkgcmd.Command {
			return deps.NewCommand(streams, flags)
		},
		applier: applyDepsArgs,
	}

	return toolDefinition{tool: tool, handler: adapter.handle}
}

func applyDepsArgs(command pkgcmd.Command, request mcp.CallToolRequest) error {
	cmd, ok := command.(*deps.Command)
	if !ok {
		return fmt.Errorf("unexpected command type: %T", command)
	}

	cmd.Output = outputJSON
	cmd.DryRun = request.GetBool("dry_run", false)
	cmd.Version = request.GetString("version", "")
	cmd.Refresh = request.GetBool("refresh", false)
	cmd.Verbose = request.GetBool("verbose", false)

	return nil
}

// --- odh_deps_install ---

func depsInstallTool(configFlags *genericclioptions.ConfigFlags) toolDefinition {
	tool := mcp.NewTool("odh_deps_install",
		mcp.WithDescription("Install missing operator dependencies for OpenShift AI / RHOAI via OLM"),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(true),
		mcp.WithString("target", mcp.Description("Specific dependency name to install (empty = all missing required)")),
		mcp.WithBoolean("dry_run", mcp.Description("Show what would be installed without executing"), mcp.DefaultBool(true)),
		mcp.WithBoolean("include_optional", mcp.Description("Install optional dependencies in addition to required")),
		mcp.WithString("version", mcp.Description("ODH/RHOAI version to install dependencies for")),
		mcp.WithBoolean("refresh", mcp.Description("Fetch latest manifest from odh-gitops")),
		mcp.WithString("timeout", mcp.Description("Timeout for waiting on each operator CSV"), mcp.DefaultString("5m")),
	)

	adapter := &toolAdapter{
		configFlags: configFlags,
		factory: func(streams genericiooptions.IOStreams, flags *genericclioptions.ConfigFlags) pkgcmd.Command {
			return deps.NewInstallCommand(streams, flags)
		},
		applier: applyDepsInstallArgs,
	}

	return toolDefinition{tool: tool, handler: adapter.handle}
}

func applyDepsInstallArgs(command pkgcmd.Command, request mcp.CallToolRequest) error {
	cmd, ok := command.(*deps.InstallCommand)
	if !ok {
		return fmt.Errorf("unexpected command type: %T", command)
	}

	cmd.TargetDep = request.GetString("target", "")
	cmd.DryRun = request.GetBool("dry_run", true)
	cmd.IncludeOptional = request.GetBool("include_optional", false)
	cmd.Version = request.GetString("version", "")
	cmd.Refresh = request.GetBool("refresh", false)

	d, err := parseDuration(request, "timeout", "5m")
	if err != nil {
		return err
	}

	if d > 0 {
		cmd.Timeout = d
	}

	return nil
}

// --- odh_backup ---

func backupTool(configFlags *genericclioptions.ConfigFlags) toolDefinition {
	tool := mcp.NewTool("odh_backup",
		mcp.WithDescription("Backup OpenShift AI workloads and dependencies as YAML"),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithString("output_dir", mcp.Description("Output directory for backups (empty = return YAML in response)")),
		mcp.WithArray("includes", mcp.Description("Workload types to include (e.g. notebooks.kubeflow.org)"), mcp.WithStringItems()),
		mcp.WithArray("excludes", mcp.Description("Workload types to exclude"), mcp.WithStringItems()),
		mcp.WithBoolean("dependencies", mcp.Description("Resolve and backup workload dependencies"), mcp.DefaultBool(true)),
		mcp.WithBoolean("dry_run", mcp.Description("Preview backup without writing files")),
		mcp.WithString("timeout", mcp.Description("Timeout for backup operation"), mcp.DefaultString("10m")),
	)

	adapter := &toolAdapter{
		configFlags: configFlags,
		factory: func(streams genericiooptions.IOStreams, flags *genericclioptions.ConfigFlags) pkgcmd.Command {
			cmd := backup.NewCommand(streams)
			cmd.ConfigFlags = flags

			return cmd
		},
		applier: applyBackupArgs,
	}

	return toolDefinition{tool: tool, handler: adapter.handle}
}

func applyBackupArgs(command pkgcmd.Command, request mcp.CallToolRequest) error {
	cmd, ok := command.(*backup.Command)
	if !ok {
		return fmt.Errorf("unexpected command type: %T", command)
	}

	outputDir := request.GetString("output_dir", "")
	if outputDir != "" {
		outputDir = filepath.Clean(outputDir)
		if filepath.IsAbs(outputDir) {
			return fmt.Errorf("output_dir %q must be a relative path", outputDir)
		}
		if strings.HasPrefix(outputDir, "..") {
			return fmt.Errorf("output_dir %q must not traverse above working directory", outputDir)
		}
	}

	cmd.OutputDir = outputDir
	cmd.Includes = request.GetStringSlice("includes", nil)
	cmd.Excludes = request.GetStringSlice("excludes", nil)
	cmd.Dependencies = request.GetBool("dependencies", true)
	cmd.DryRun = request.GetBool("dry_run", false)

	d, err := parseDuration(request, "timeout", "10m")
	if err != nil {
		return err
	}

	if d > 0 {
		cmd.Timeout = d
	}

	return nil
}

// --- odh_logs ---

func logsTool(configFlags *genericclioptions.ConfigFlags) toolDefinition {
	tool := mcp.NewTool("odh_logs",
		mcp.WithDescription("Get logs from OpenShift AI operator or component pods"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithString("target", mcp.Required(),
			mcp.Description("Log target: 'operator' or a component name (e.g. dashboard, kserve, ray)"),
			mcp.Enum(append([]string{"operator"}, resources.ComponentNames()...)...)),
		mcp.WithInteger("tail", mcp.Description("Number of recent log lines to display (default: all)"), mcp.DefaultNumber(-1)),
		mcp.WithString("since", mcp.Description("Only return logs newer than a relative duration (e.g. 5s, 2m, 3h)")),
		mcp.WithBoolean("previous", mcp.Description("Show logs from previous terminated container")),
		mcp.WithString("container", mcp.Description("Container name to get logs from")),
	)

	adapter := &toolAdapter{
		configFlags: configFlags,
		factory: func(streams genericiooptions.IOStreams, flags *genericclioptions.ConfigFlags) pkgcmd.Command {
			return logs.NewCommand(streams, flags)
		},
		applier: applyLogsArgs,
	}

	return toolDefinition{tool: tool, handler: adapter.handle}
}

func applyLogsArgs(command pkgcmd.Command, request mcp.CallToolRequest) error {
	cmd, ok := command.(*logs.Command)
	if !ok {
		return fmt.Errorf("unexpected command type: %T", command)
	}

	cmd.Follow = false // MCP cannot stream
	cmd.Target = request.GetString("target", "")
	cmd.Tail = int64(request.GetInt("tail", -1))
	cmd.Previous = request.GetBool("previous", false)
	cmd.Container = request.GetString("container", "")

	d, err := parseDuration(request, "since", "")
	if err != nil {
		return err
	}

	if d > 0 {
		cmd.Since = d
	}

	return nil
}

// --- odh_events ---

func eventsTool(configFlags *genericclioptions.ConfigFlags) toolDefinition {
	tool := mcp.NewTool("odh_events",
		mcp.WithDescription("List Kubernetes events for OpenShift AI components"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithString("type", mcp.Description("Filter events by type: Warning or Normal"), mcp.Enum("Warning", "Normal")),
		mcp.WithString("since", mcp.Description("Only show events newer than this duration (e.g. 5m, 1h)"), mcp.DefaultString("1h")),
		mcp.WithBoolean("all_namespaces", mcp.Description("List events across all ODH namespaces")),
		mcp.WithString("component", mcp.Description("Filter events by ODH component (dashboard, kserve, ray, etc.)"),
			mcp.Enum(resources.ComponentNames()...)),
		mcp.WithString("operator_namespace", mcp.Description("Override the operator namespace")),
	)

	adapter := &toolAdapter{
		configFlags: configFlags,
		factory: func(streams genericiooptions.IOStreams, flags *genericclioptions.ConfigFlags) pkgcmd.Command {
			return events.NewCommand(streams, flags)
		},
		applier: applyEventsArgs,
	}

	return toolDefinition{tool: tool, handler: adapter.handle}
}

func applyEventsArgs(command pkgcmd.Command, request mcp.CallToolRequest) error {
	cmd, ok := command.(*events.Command)
	if !ok {
		return fmt.Errorf("unexpected command type: %T", command)
	}

	cmd.OutputFormat = outputJSON
	cmd.Follow = false // MCP cannot stream
	cmd.EventType = request.GetString("type", "")
	cmd.AllNamespaces = request.GetBool("all_namespaces", false)
	cmd.Component = request.GetString("component", "")
	cmd.OperatorNSOverride = request.GetString("operator_namespace", "")

	d, err := parseDuration(request, "since", "1h")
	if err != nil {
		return err
	}

	if d > 0 {
		cmd.Since = d
	}

	return nil
}

// --- odh_components_list ---

func componentsListTool(configFlags *genericclioptions.ConfigFlags) toolDefinition {
	tool := mcp.NewTool("odh_components_list",
		mcp.WithDescription("List all OpenShift AI components and their status"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithBoolean("verbose", mcp.Description("Enable verbose output")),
	)

	adapter := &toolAdapter{
		configFlags: configFlags,
		factory: func(streams genericiooptions.IOStreams, flags *genericclioptions.ConfigFlags) pkgcmd.Command {
			return components.NewListCommand(streams, flags)
		},
		applier: applyComponentsListArgs,
	}

	return toolDefinition{tool: tool, handler: adapter.handle}
}

func applyComponentsListArgs(command pkgcmd.Command, request mcp.CallToolRequest) error {
	cmd, ok := command.(*components.ListCommand)
	if !ok {
		return fmt.Errorf("unexpected command type: %T", command)
	}

	cmd.OutputFormat = outputJSON
	cmd.Verbose = request.GetBool("verbose", false)

	return nil
}

// --- odh_components_describe ---

func componentsDescribeTool(configFlags *genericclioptions.ConfigFlags) toolDefinition {
	tool := mcp.NewTool("odh_components_describe",
		mcp.WithDescription("Show detailed information about a specific OpenShift AI component"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithString("component", mcp.Required(),
			mcp.Description("Component name (e.g. dashboard, kserve, ray)"),
			mcp.Enum(resources.ComponentNames()...)),
		mcp.WithBoolean("verbose", mcp.Description("Enable verbose output")),
	)

	adapter := &toolAdapter{
		configFlags: configFlags,
		factory: func(streams genericiooptions.IOStreams, flags *genericclioptions.ConfigFlags) pkgcmd.Command {
			return components.NewDescribeCommand(streams, flags)
		},
		applier: applyComponentsDescribeArgs,
	}

	return toolDefinition{tool: tool, handler: adapter.handle}
}

func applyComponentsDescribeArgs(command pkgcmd.Command, request mcp.CallToolRequest) error {
	cmd, ok := command.(*components.DescribeCommand)
	if !ok {
		return fmt.Errorf("unexpected command type: %T", command)
	}

	cmd.OutputFormat = outputJSON
	cmd.ComponentName = request.GetString("component", "")
	cmd.Verbose = request.GetBool("verbose", false)

	return nil
}

// --- odh_migrate_list ---

func migrateListTool(configFlags *genericclioptions.ConfigFlags) toolDefinition {
	tool := mcp.NewTool("odh_migrate_list",
		mcp.WithDescription("List available migrations for OpenShift AI"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithString("target_version", mcp.Description("Target version to filter migrations")),
		mcp.WithBoolean("all", mcp.Description("Show all migrations regardless of applicability")),
		mcp.WithString("phase", mcp.Description("Filter by lifecycle phase"),
			mcp.Enum("pre-upgrade", "post-upgrade", "pre-enablement")),
	)

	adapter := &toolAdapter{
		configFlags: configFlags,
		factory: func(streams genericiooptions.IOStreams, flags *genericclioptions.ConfigFlags) pkgcmd.Command {
			cmd := migrate.NewListCommand(streams)
			cmd.ConfigFlags = flags

			return cmd
		},
		applier: applyMigrateListArgs,
	}

	return toolDefinition{tool: tool, handler: adapter.handle}
}

func applyMigrateListArgs(command pkgcmd.Command, request mcp.CallToolRequest) error {
	cmd, ok := command.(*migrate.ListCommand)
	if !ok {
		return fmt.Errorf("unexpected command type: %T", command)
	}

	cmd.OutputFormat = migrate.OutputFormatJSON
	cmd.TargetVersion = request.GetString("target_version", "")
	cmd.ShowAll = request.GetBool("all", false)
	cmd.Phase = request.GetString("phase", "")

	return nil
}

// --- odh_migrate_run ---

func migrateRunTool(configFlags *genericclioptions.ConfigFlags) toolDefinition {
	tool := mcp.NewTool("odh_migrate_run",
		mcp.WithDescription("Execute OpenShift AI migrations (destructive — changes cluster state)"),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(true),
		mcp.WithArray("migrations", mcp.Required(),
			mcp.Description("Migration IDs to execute"), mcp.WithStringItems()),
		mcp.WithString("target_version", mcp.Required(),
			mcp.Description("Target version")),
		mcp.WithBoolean("dry_run", mcp.Description("Preview changes without applying"), mcp.DefaultBool(true)),
		mcp.WithString("phase", mcp.Description("Lifecycle phase"),
			mcp.Enum("pre-upgrade", "post-upgrade", "pre-enablement")),
		mcp.WithString("timeout", mcp.Description("Maximum execution time"), mcp.DefaultString("10m")),
	)

	adapter := &toolAdapter{
		configFlags: configFlags,
		factory: func(streams genericiooptions.IOStreams, flags *genericclioptions.ConfigFlags) pkgcmd.Command {
			cmd := migrate.NewRunCommand(streams)
			cmd.ConfigFlags = flags

			return cmd
		},
		applier: applyMigrateRunArgs,
	}

	return toolDefinition{tool: tool, handler: adapter.handle}
}

func applyMigrateRunArgs(command pkgcmd.Command, request mcp.CallToolRequest) error {
	cmd, ok := command.(*migrate.RunCommand)
	if !ok {
		return fmt.Errorf("unexpected command type: %T", command)
	}

	cmd.Yes = true // MCP cannot prompt interactively
	cmd.MigrationIDs = request.GetStringSlice("migrations", nil)
	cmd.TargetVersion = request.GetString("target_version", "")
	cmd.DryRun = request.GetBool("dry_run", true)
	cmd.Phase = request.GetString("phase", "")

	d, err := parseDuration(request, "timeout", "10m")
	if err != nil {
		return err
	}

	if d > 0 {
		cmd.Timeout = d
	}

	return nil
}
