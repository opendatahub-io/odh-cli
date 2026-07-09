//nolint:testpackage // Tests internal tool definitions and applier functions
package mcp

import (
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/genericiooptions"

	"github.com/opendatahub-io/odh-cli/pkg/backup"
	"github.com/opendatahub-io/odh-cli/pkg/components"
	"github.com/opendatahub-io/odh-cli/pkg/deps"
	"github.com/opendatahub-io/odh-cli/pkg/events"
	"github.com/opendatahub-io/odh-cli/pkg/get"
	"github.com/opendatahub-io/odh-cli/pkg/lint"
	"github.com/opendatahub-io/odh-cli/pkg/logs"
	"github.com/opendatahub-io/odh-cli/pkg/migrate"
	"github.com/opendatahub-io/odh-cli/pkg/status"

	. "github.com/onsi/gomega"
)

func TestAllTools(t *testing.T) {
	flags := genericclioptions.NewConfigFlags(true)

	t.Run("should return exactly 12 tools", func(t *testing.T) {
		g := NewWithT(t)

		tools := allTools(flags)

		g.Expect(tools).To(HaveLen(12))
	})

	t.Run("should have expected tool names", func(t *testing.T) {
		g := NewWithT(t)

		tools := allTools(flags)
		names := make([]string, len(tools))
		for i, def := range tools {
			names[i] = def.tool.Name
		}

		g.Expect(names).To(ConsistOf(
			"odh_status",
			"odh_lint",
			"odh_get",
			"odh_deps",
			"odh_deps_install",
			"odh_backup",
			"odh_logs",
			"odh_events",
			"odh_components_list",
			"odh_components_describe",
			"odh_migrate_list",
			"odh_migrate_run",
		))
	})

	t.Run("should mark read-only tools correctly", func(t *testing.T) {
		g := NewWithT(t)

		readOnlyTools := []string{"odh_status", "odh_lint", "odh_get", "odh_deps", "odh_logs", "odh_events", "odh_components_list", "odh_components_describe", "odh_migrate_list"}
		tools := allTools(flags)

		for _, def := range tools {
			for _, name := range readOnlyTools {
				if def.tool.Name == name {
					g.Expect(*def.tool.Annotations.ReadOnlyHint).To(BeTrue(), "%s should be read-only", name)
					g.Expect(*def.tool.Annotations.DestructiveHint).To(BeFalse(), "%s should not be destructive", name)
				}
			}
		}
	})

	t.Run("should mark destructive tools correctly", func(t *testing.T) {
		g := NewWithT(t)

		destructiveTools := []string{"odh_migrate_run", "odh_deps_install"}
		tools := allTools(flags)

		for _, def := range tools {
			for _, name := range destructiveTools {
				if def.tool.Name == name {
					g.Expect(*def.tool.Annotations.DestructiveHint).To(BeTrue(), "%s should be destructive", name)
					g.Expect(*def.tool.Annotations.ReadOnlyHint).To(BeFalse(), "%s should not be read-only", name)
				}
			}
		}
	})

	t.Run("should require resource for odh_get", func(t *testing.T) {
		g := NewWithT(t)

		tools := allTools(flags)

		for _, def := range tools {
			if def.tool.Name == "odh_get" {
				g.Expect(def.tool.InputSchema.Required).To(ContainElement("resource"))
			}
		}
	})

	t.Run("should require migrations and target_version for odh_migrate_run", func(t *testing.T) {
		g := NewWithT(t)

		tools := allTools(flags)

		for _, def := range tools {
			if def.tool.Name == "odh_migrate_run" {
				g.Expect(def.tool.InputSchema.Required).To(ContainElement("migrations"))
				g.Expect(def.tool.InputSchema.Required).To(ContainElement("target_version"))
			}
		}
	})

	t.Run("should have non-nil handlers for all tools", func(t *testing.T) {
		g := NewWithT(t)

		tools := allTools(flags)

		for _, def := range tools {
			g.Expect(def.handler).ToNot(BeNil(), "handler for %s should not be nil", def.tool.Name)
		}
	})
}

func newRequest(args map[string]any) mcp.CallToolRequest {
	return mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: args,
		},
	}
}

func TestParseDuration(t *testing.T) {
	t.Run("should return zero for empty string with no fallback", func(t *testing.T) {
		g := NewWithT(t)

		d, err := parseDuration(newRequest(nil), "timeout", "")

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(d).To(BeZero())
	})

	t.Run("should use fallback when key is missing", func(t *testing.T) {
		g := NewWithT(t)

		d, err := parseDuration(newRequest(nil), "timeout", "30s")

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(d).To(Equal(30 * time.Second))
	})

	t.Run("should parse valid duration", func(t *testing.T) {
		g := NewWithT(t)

		d, err := parseDuration(newRequest(map[string]any{"timeout": "45s"}), "timeout", "30s")

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(d).To(Equal(45 * time.Second))
	})

	t.Run("should return error for invalid duration", func(t *testing.T) {
		g := NewWithT(t)

		_, err := parseDuration(newRequest(map[string]any{"timeout": "bad"}), "timeout", "30s")

		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("invalid timeout duration"))
		g.Expect(err.Error()).To(ContainSubstring("bad"))
	})

	t.Run("should return error for duration missing unit", func(t *testing.T) {
		g := NewWithT(t)

		_, err := parseDuration(newRequest(map[string]any{"since": "30"}), "since", "1h")

		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("invalid since duration"))
	})
}

func TestApplyStatusArgs(t *testing.T) {
	streams := genericiooptions.IOStreams{}
	flags := genericclioptions.NewConfigFlags(true)

	t.Run("should force JSON output and no color", func(t *testing.T) {
		g := NewWithT(t)
		cmd := status.NewCommand(streams, flags)

		err := applyStatusArgs(cmd, newRequest(nil))

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(cmd.OutputFormat).To(Equal(status.OutputFormatJSON))
		g.Expect(cmd.NoColor).To(BeTrue())
	})

	t.Run("should map all arguments", func(t *testing.T) {
		g := NewWithT(t)
		cmd := status.NewCommand(streams, flags)

		err := applyStatusArgs(cmd, newRequest(map[string]any{
			"verbose":            true,
			"sections":           []any{"nodes", "pods"},
			"layers":             []any{"operator"},
			"timeout":            "45s",
			"apps_namespace":     "my-apps",
			"operator_namespace": "my-operator",
			"include_deps":       false,
		}))

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(cmd.Verbose).To(BeTrue())
		g.Expect(cmd.Sections).To(Equal([]string{"nodes", "pods"}))
		g.Expect(cmd.Layers).To(Equal([]string{"operator"}))
		g.Expect(cmd.Timeout).To(Equal(45 * time.Second))
		g.Expect(cmd.AppsNamespace).To(Equal("my-apps"))
		g.Expect(cmd.OperatorNamespace).To(Equal("my-operator"))
		g.Expect(cmd.IncludeDeps).To(BeFalse())
	})
}

func TestApplyLintArgs(t *testing.T) {
	streams := genericiooptions.IOStreams{}
	flags := genericclioptions.NewConfigFlags(true)

	t.Run("should force JSON output", func(t *testing.T) {
		g := NewWithT(t)
		cmd := lint.NewCommand(streams, flags)

		err := applyLintArgs(cmd, newRequest(nil))

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(cmd.OutputFormat).To(Equal(lint.OutputFormatJSON))
		g.Expect(cmd.NoColor).To(BeTrue())
	})

	t.Run("should map all arguments", func(t *testing.T) {
		g := NewWithT(t)
		cmd := lint.NewCommand(streams, flags)

		err := applyLintArgs(cmd, newRequest(map[string]any{
			"target_version":       "3.0",
			"severity":             "warning",
			"checks":               []any{"*notebook*"},
			"isvc_deployment_mode": "serverless",
			"timeout":              "2m",
		}))

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(cmd.TargetVersion).To(Equal("3.0"))
		g.Expect(cmd.SeverityLevel).To(Equal(lint.SeverityLevel("warning")))
		g.Expect(cmd.CheckSelectors).To(Equal([]string{"*notebook*"}))
		g.Expect(cmd.ISVCDeploymentMode).To(Equal("serverless"))
		g.Expect(cmd.Timeout).To(Equal(2 * time.Minute))
	})
}

func TestApplyGetArgs(t *testing.T) {
	streams := genericiooptions.IOStreams{}
	flags := genericclioptions.NewConfigFlags(true)

	t.Run("should force JSON output and map resource", func(t *testing.T) {
		g := NewWithT(t)
		cmd := get.NewCommand(streams, flags)

		err := applyGetArgs(cmd, newRequest(map[string]any{
			"resource": "nb",
		}))

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(cmd.OutputFormat).To(Equal("json"))
		g.Expect(cmd.ResourceName).To(Equal("nb"))
	})

	t.Run("should map all arguments", func(t *testing.T) {
		g := NewWithT(t)
		cmd := get.NewCommand(streams, flags)

		err := applyGetArgs(cmd, newRequest(map[string]any{
			"resource":       "isvc",
			"name":           "my-model",
			"namespace":      "prod",
			"all_namespaces": true,
			"selector":       "app=serving",
			"verbose":        true,
		}))

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(cmd.ResourceName).To(Equal("isvc"))
		g.Expect(cmd.ItemName).To(Equal("my-model"))
		g.Expect(cmd.Namespace).To(Equal("prod"))
		g.Expect(cmd.AllNamespaces).To(BeTrue())
		g.Expect(cmd.LabelSelector).To(Equal("app=serving"))
		g.Expect(cmd.Verbose).To(BeTrue())
	})
}

func TestApplyDepsArgs(t *testing.T) {
	streams := genericiooptions.IOStreams{}
	flags := genericclioptions.NewConfigFlags(true)

	t.Run("should force JSON output", func(t *testing.T) {
		g := NewWithT(t)
		cmd := deps.NewCommand(streams, flags)

		err := applyDepsArgs(cmd, newRequest(nil))

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(cmd.Output).To(Equal("json"))
	})

	t.Run("should map all arguments", func(t *testing.T) {
		g := NewWithT(t)
		cmd := deps.NewCommand(streams, flags)

		err := applyDepsArgs(cmd, newRequest(map[string]any{
			"dry_run": true,
			"version": "2.17.0",
			"refresh": true,
			"verbose": true,
		}))

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(cmd.DryRun).To(BeTrue())
		g.Expect(cmd.Version).To(Equal("2.17.0"))
		g.Expect(cmd.Refresh).To(BeTrue())
		g.Expect(cmd.Verbose).To(BeTrue())
	})
}

func TestApplyDepsInstallArgs(t *testing.T) {
	streams := genericiooptions.IOStreams{}
	flags := genericclioptions.NewConfigFlags(true)

	t.Run("should default dry_run to true", func(t *testing.T) {
		g := NewWithT(t)
		cmd := deps.NewInstallCommand(streams, flags)

		err := applyDepsInstallArgs(cmd, newRequest(nil))

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(cmd.DryRun).To(BeTrue())
	})

	t.Run("should map all arguments", func(t *testing.T) {
		g := NewWithT(t)
		cmd := deps.NewInstallCommand(streams, flags)

		err := applyDepsInstallArgs(cmd, newRequest(map[string]any{
			"target":           "servicemeshoperator",
			"dry_run":          true,
			"include_optional": true,
			"version":          "2.17.0",
			"refresh":          true,
			"timeout":          "8m",
		}))

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(cmd.TargetDep).To(Equal("servicemeshoperator"))
		g.Expect(cmd.DryRun).To(BeTrue())
		g.Expect(cmd.IncludeOptional).To(BeTrue())
		g.Expect(cmd.Version).To(Equal("2.17.0"))
		g.Expect(cmd.Refresh).To(BeTrue())
		g.Expect(cmd.Timeout).To(Equal(8 * time.Minute))
	})
}

func TestApplyLogsArgs(t *testing.T) {
	streams := genericiooptions.IOStreams{}
	flags := genericclioptions.NewConfigFlags(true)

	t.Run("should disable Follow and map all arguments", func(t *testing.T) {
		g := NewWithT(t)
		cmd := logs.NewCommand(streams, flags)

		err := applyLogsArgs(cmd, newRequest(map[string]any{
			"target":    "dashboard",
			"tail":      100,
			"since":     "5m",
			"previous":  true,
			"container": "oauth-proxy",
		}))

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(cmd.Follow).To(BeFalse())
		g.Expect(cmd.Target).To(Equal("dashboard"))
		g.Expect(cmd.Tail).To(Equal(int64(100)))
		g.Expect(cmd.Since).To(Equal(5 * time.Minute))
		g.Expect(cmd.Previous).To(BeTrue())
		g.Expect(cmd.Container).To(Equal("oauth-proxy"))
	})
}

func TestApplyEventsArgs(t *testing.T) {
	streams := genericiooptions.IOStreams{}
	flags := genericclioptions.NewConfigFlags(true)

	t.Run("should force JSON, disable Follow, and map all arguments", func(t *testing.T) {
		g := NewWithT(t)
		cmd := events.NewCommand(streams, flags)

		err := applyEventsArgs(cmd, newRequest(map[string]any{
			"type":               "Warning",
			"since":              "30m",
			"all_namespaces":     true,
			"component":          "kserve",
			"operator_namespace": "redhat-ods-operator",
		}))

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(cmd.OutputFormat).To(Equal("json"))
		g.Expect(cmd.Follow).To(BeFalse())
		g.Expect(cmd.EventType).To(Equal("Warning"))
		g.Expect(cmd.Since).To(Equal(30 * time.Minute))
		g.Expect(cmd.AllNamespaces).To(BeTrue())
		g.Expect(cmd.Component).To(Equal("kserve"))
		g.Expect(cmd.OperatorNSOverride).To(Equal("redhat-ods-operator"))
	})
}

func TestApplyComponentsListArgs(t *testing.T) {
	streams := genericiooptions.IOStreams{}
	flags := genericclioptions.NewConfigFlags(true)

	t.Run("should force JSON output and map arguments", func(t *testing.T) {
		g := NewWithT(t)
		cmd := components.NewListCommand(streams, flags)

		err := applyComponentsListArgs(cmd, newRequest(map[string]any{
			"verbose": true,
		}))

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(cmd.OutputFormat).To(Equal("json"))
		g.Expect(cmd.Verbose).To(BeTrue())
	})
}

func TestApplyComponentsDescribeArgs(t *testing.T) {
	streams := genericiooptions.IOStreams{}
	flags := genericclioptions.NewConfigFlags(true)

	t.Run("should force JSON output and map arguments", func(t *testing.T) {
		g := NewWithT(t)
		cmd := components.NewDescribeCommand(streams, flags)

		err := applyComponentsDescribeArgs(cmd, newRequest(map[string]any{
			"component": "kserve",
			"verbose":   true,
		}))

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(cmd.OutputFormat).To(Equal("json"))
		g.Expect(cmd.ComponentName).To(Equal("kserve"))
		g.Expect(cmd.Verbose).To(BeTrue())
	})
}

func TestApplyBackupArgs(t *testing.T) {
	streams := genericiooptions.IOStreams{}

	t.Run("should reject path traversal in output_dir", func(t *testing.T) {
		g := NewWithT(t)
		cmd := backup.NewCommand(streams)

		err := applyBackupArgs(cmd, newRequest(map[string]any{
			"output_dir": "../../etc/evil",
		}))

		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("must not traverse"))
	})

	t.Run("should reject intermediate path traversal in output_dir", func(t *testing.T) {
		g := NewWithT(t)
		cmd := backup.NewCommand(streams)

		err := applyBackupArgs(cmd, newRequest(map[string]any{
			"output_dir": "foo/../../bar",
		}))

		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("must not traverse"))
	})

	t.Run("should reject absolute paths in output_dir", func(t *testing.T) {
		g := NewWithT(t)
		cmd := backup.NewCommand(streams)

		err := applyBackupArgs(cmd, newRequest(map[string]any{
			"output_dir": "/tmp/backup",
		}))

		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("must be a relative path"))
	})

	t.Run("should allow relative paths in output_dir", func(t *testing.T) {
		g := NewWithT(t)
		cmd := backup.NewCommand(streams)

		err := applyBackupArgs(cmd, newRequest(map[string]any{
			"output_dir": "backups/my-backup",
		}))

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(cmd.OutputDir).To(Equal("backups/my-backup"))
	})

	t.Run("should map all arguments", func(t *testing.T) {
		g := NewWithT(t)
		cmd := backup.NewCommand(streams)

		err := applyBackupArgs(cmd, newRequest(map[string]any{
			"output_dir":   "backups/cluster-1",
			"includes":     []any{"notebooks.kubeflow.org"},
			"excludes":     []any{"pipelines"},
			"dependencies": false,
			"dry_run":      true,
			"timeout":      "15m",
		}))

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(cmd.OutputDir).To(Equal("backups/cluster-1"))
		g.Expect(cmd.Includes).To(Equal([]string{"notebooks.kubeflow.org"}))
		g.Expect(cmd.Excludes).To(Equal([]string{"pipelines"}))
		g.Expect(cmd.Dependencies).To(BeFalse())
		g.Expect(cmd.DryRun).To(BeTrue())
		g.Expect(cmd.Timeout).To(Equal(15 * time.Minute))
	})
}

func TestApplyMigrateListArgs(t *testing.T) {
	streams := genericiooptions.IOStreams{}

	t.Run("should force JSON output", func(t *testing.T) {
		g := NewWithT(t)
		cmd := migrate.NewListCommand(streams)

		err := applyMigrateListArgs(cmd, newRequest(nil))

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(cmd.OutputFormat).To(Equal(migrate.OutputFormatJSON))
	})

	t.Run("should map all arguments", func(t *testing.T) {
		g := NewWithT(t)
		cmd := migrate.NewListCommand(streams)

		err := applyMigrateListArgs(cmd, newRequest(map[string]any{
			"target_version": "3.0",
			"all":            true,
			"phase":          "pre-upgrade",
		}))

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(cmd.TargetVersion).To(Equal("3.0"))
		g.Expect(cmd.ShowAll).To(BeTrue())
		g.Expect(cmd.Phase).To(Equal("pre-upgrade"))
	})
}

func TestApplyMigrateRunArgs(t *testing.T) {
	streams := genericiooptions.IOStreams{}

	t.Run("should auto-set Yes and default dry_run to true", func(t *testing.T) {
		g := NewWithT(t)
		cmd := migrate.NewRunCommand(streams)

		err := applyMigrateRunArgs(cmd, newRequest(nil))

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(cmd.Yes).To(BeTrue())
		g.Expect(cmd.DryRun).To(BeTrue())
	})

	t.Run("should map all arguments", func(t *testing.T) {
		g := NewWithT(t)
		cmd := migrate.NewRunCommand(streams)

		err := applyMigrateRunArgs(cmd, newRequest(map[string]any{
			"migrations":     []any{"dspa-v2", "kserve-v3"},
			"target_version": "3.0",
			"dry_run":        true,
			"phase":          "post-upgrade",
			"timeout":        "20m",
		}))

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(cmd.MigrationIDs).To(Equal([]string{"dspa-v2", "kserve-v3"}))
		g.Expect(cmd.TargetVersion).To(Equal("3.0"))
		g.Expect(cmd.DryRun).To(BeTrue())
		g.Expect(cmd.Phase).To(Equal("post-upgrade"))
		g.Expect(cmd.Timeout).To(Equal(20 * time.Minute))
		g.Expect(cmd.Yes).To(BeTrue())
	})
}
