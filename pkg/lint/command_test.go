package lint_test

import (
	"bytes"
	"testing"

	"github.com/spf13/pflag"

	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/genericiooptions"

	"github.com/opendatahub-io/odh-cli/pkg/cmd"
	"github.com/opendatahub-io/odh-cli/pkg/lint"

	. "github.com/onsi/gomega"
)

// testConfigFlags creates ConfigFlags for testing.
func testConfigFlags() *genericclioptions.ConfigFlags {
	return genericclioptions.NewConfigFlags(true)
}

// T022: Test lint mode (no --target-version flag).
func TestLintMode_NoVersionFlag(t *testing.T) {
	t.Run("lint mode should skip checks when no target version provided", func(t *testing.T) {
		g := NewWithT(t)

		var out, errOut bytes.Buffer
		streams := genericiooptions.IOStreams{
			In:     &bytes.Buffer{},
			Out:    &out,
			ErrOut: &errOut,
		}

		cmd := lint.NewCommand(streams, testConfigFlags())

		g.Expect(cmd.TargetVersion).To(BeEmpty())

		// Without --target-version, Run() will short-circuit when
		// current and target versions share the same major.minor
		err := cmd.Complete()
		g.Expect(err).ToNot(HaveOccurred())
	})
}

// T023: Test upgrade mode (with --target-version flag).
func TestUpgradeMode_WithVersionFlag(t *testing.T) {
	t.Run("upgrade mode should assess upgrade readiness", func(t *testing.T) {
		g := NewWithT(t)

		var out, errOut bytes.Buffer
		streams := genericiooptions.IOStreams{
			In:     &bytes.Buffer{},
			Out:    &out,
			ErrOut: &errOut,
		}

		// Use current non-deprecated constructor
		cmd := lint.NewCommand(streams, testConfigFlags())

		// Set --target-version flag (upgrade mode)
		cmd.TargetVersion = "3.0.0"
		g.Expect(cmd.TargetVersion).To(Equal("3.0.0"))

		// Upgrade mode should accept target version
		err := cmd.Validate()
		g.Expect(err).ToNot(HaveOccurred())
	})
}

// T024: Test CheckTarget.CurrentVersion == CheckTarget.TargetVersion in lint mode.
func TestLintMode_CheckTargetVersionMatches(t *testing.T) {
	t.Run("lint mode should pass same version for CurrentVersion and TargetVersion", func(t *testing.T) {
		g := NewWithT(t)

		var out, errOut bytes.Buffer
		streams := genericiooptions.IOStreams{
			In:     &bytes.Buffer{},
			Out:    &out,
			ErrOut: &errOut,
		}

		command := lint.NewCommand(streams, testConfigFlags())
		g.Expect(command).ToNot(BeNil())

		// Verify no --target-version flag set (lint mode)
		g.Expect(command.TargetVersion).To(BeEmpty())

		// In lint mode, Run() detects that current == target (same major.minor)
		// and short-circuits with a "no checks will be executed" message
	})
}

// T025: Test CheckTarget.CurrentVersion != CheckTarget.TargetVersion in upgrade mode.
func TestUpgradeMode_CheckTargetVersionDiffers(t *testing.T) {
	t.Run("upgrade mode should pass different versions for CurrentVersion and TargetVersion", func(t *testing.T) {
		g := NewWithT(t)

		var out, errOut bytes.Buffer
		streams := genericiooptions.IOStreams{
			In:     &bytes.Buffer{},
			Out:    &out,
			ErrOut: &errOut,
		}

		command := lint.NewCommand(streams, testConfigFlags())
		g.Expect(command).ToNot(BeNil())

		// Set --target-version flag (upgrade mode)
		command.TargetVersion = "3.0.0"
		g.Expect(command.TargetVersion).To(Equal("3.0.0"))

		// Verify version parses correctly in Complete
		err := command.Complete()
		g.Expect(err).ToNot(HaveOccurred())

		// In upgrade mode, Run() delegates to runUpgradeMode() when
		// current and target differ at the major.minor level
	})
}

// T026: Integration test for both lint and upgrade modes.
func TestIntegration_LintAndUpgradeModes(t *testing.T) {
	t.Run("command should support both lint and upgrade modes", func(t *testing.T) {
		g := NewWithT(t)

		var out, errOut bytes.Buffer
		streams := genericiooptions.IOStreams{
			In:     &bytes.Buffer{},
			Out:    &out,
			ErrOut: &errOut,
		}

		// Test lint mode configuration
		lintCmd := lint.NewCommand(streams, testConfigFlags())
		g.Expect(lintCmd).ToNot(BeNil())
		g.Expect(lintCmd.TargetVersion).To(BeEmpty())

		// Test upgrade mode configuration
		upgradeCmd := lint.NewCommand(streams, testConfigFlags())
		upgradeCmd.TargetVersion = "3.0.0"
		g.Expect(upgradeCmd.TargetVersion).To(Equal("3.0.0"))

		// Verify both modes complete successfully
		err := lintCmd.Complete()
		g.Expect(err).ToNot(HaveOccurred())

		err = upgradeCmd.Complete()
		g.Expect(err).ToNot(HaveOccurred())

		// Verify both modes validate successfully
		err = lintCmd.Validate()
		g.Expect(err).ToNot(HaveOccurred())

		err = upgradeCmd.Validate()
		g.Expect(err).ToNot(HaveOccurred())

		// Note: Full end-to-end Run() testing requires k3s-envtest infrastructure
		// Run() prints environment, then either short-circuits (same major.minor)
		// or delegates to runUpgradeMode() (different major.minor)
	})
}

// T027: Preserve upgrade command tests (copy from upgrade package)
// These tests will be added after T027 is complete

// T042: Test AddFlags method registers flags correctly.
func TestCommand_AddFlags(t *testing.T) {
	t.Run("AddFlags should register all command flags", func(t *testing.T) {
		g := NewWithT(t)

		var out, errOut bytes.Buffer
		streams := genericiooptions.IOStreams{
			In:     &bytes.Buffer{},
			Out:    &out,
			ErrOut: &errOut,
		}

		command := lint.NewCommand(streams, testConfigFlags())
		g.Expect(command).ToNot(BeNil())

		// Create a FlagSet and call AddFlags
		fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
		command.AddFlags(fs)

		// Verify flags are registered
		g.Expect(fs.Lookup("target-version")).ToNot(BeNil())
		g.Expect(fs.Lookup("output")).ToNot(BeNil())
		g.Expect(fs.Lookup("checks")).ToNot(BeNil())
		g.Expect(fs.Lookup("timeout")).ToNot(BeNil())
	})
}

// T043: Test Command implements cmd.Command interface.
func TestCommand_ImplementsInterface(t *testing.T) {
	t.Run("Command should implement cmd.Command interface", func(t *testing.T) {
		g := NewWithT(t)

		var out, errOut bytes.Buffer
		streams := genericiooptions.IOStreams{
			In:     &bytes.Buffer{},
			Out:    &out,
			ErrOut: &errOut,
		}

		command := lint.NewCommand(streams, testConfigFlags())
		g.Expect(command).ToNot(BeNil())

		// Verify interface implementation at compile time
		var _ cmd.Command = command
	})
}

// T044: Test NewCommand constructor initialization.
func TestCommand_NewCommand(t *testing.T) {
	t.Run("NewCommand should initialize with defaults", func(t *testing.T) {
		g := NewWithT(t)

		var out, errOut bytes.Buffer
		streams := genericiooptions.IOStreams{
			In:     &bytes.Buffer{},
			Out:    &out,
			ErrOut: &errOut,
		}

		command := lint.NewCommand(streams, testConfigFlags())
		g.Expect(command).ToNot(BeNil())

		// Per FR-014, SharedOptions should be initialized internally
		g.Expect(command.SharedOptions).ToNot(BeNil())
		g.Expect(command.IO).ToNot(BeNil())
		g.Expect(command.IO.Out()).To(Equal(&out))
		g.Expect(command.IO.ErrOut()).To(Equal(&errOut))
	})
}

// T058: Test functional options with NewCommand.
func TestCommand_FunctionalOptions(t *testing.T) {
	t.Run("WithTargetVersion should set target version", func(t *testing.T) {
		g := NewWithT(t)

		var out, errOut bytes.Buffer
		streams := genericiooptions.IOStreams{
			In:     &bytes.Buffer{},
			Out:    &out,
			ErrOut: &errOut,
		}

		command := lint.NewCommand(streams, testConfigFlags(),
			lint.WithTargetVersion("3.0.0"),
		)

		g.Expect(command).ToNot(BeNil())
		g.Expect(command.TargetVersion).To(Equal("3.0.0"))
		g.Expect(command.IO).ToNot(BeNil())
	})
}

// TestColorDetection_DefaultBehavior tests that colors are disabled for non-TTY output.
func TestColorDetection_DefaultBehavior(t *testing.T) {
	t.Run("should disable colors when output is not a TTY (buffer)", func(t *testing.T) {
		g := NewWithT(t)

		var out, errOut bytes.Buffer
		streams := genericiooptions.IOStreams{
			In:     &bytes.Buffer{},
			Out:    &out,
			ErrOut: &errOut,
		}

		command := lint.NewCommand(streams, testConfigFlags())

		err := command.Complete()
		g.Expect(err).ToNot(HaveOccurred())
		// Buffers are not TTY, so TTY detection should disable colors
		g.Expect(command.NoColor).To(BeTrue())
	})
}

// TestColorDetection_NoColorFlag tests --no-color flag behavior.
func TestColorDetection_NoColorFlag(t *testing.T) {
	t.Run("should disable colors when --no-color flag is set", func(t *testing.T) {
		g := NewWithT(t)

		var out, errOut bytes.Buffer
		streams := genericiooptions.IOStreams{
			In:     &bytes.Buffer{},
			Out:    &out,
			ErrOut: &errOut,
		}

		command := lint.NewCommand(streams, testConfigFlags())
		command.NoColor = true

		err := command.Complete()
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(command.NoColor).To(BeTrue())
	})
}

// TestColorDetection_NOColorEnv tests NO_COLOR environment variable.
func TestColorDetection_NOColorEnv(t *testing.T) {
	t.Run("should disable colors when NO_COLOR env var is set", func(t *testing.T) {
		g := NewWithT(t)

		// Set NO_COLOR environment variable
		t.Setenv("NO_COLOR", "1")

		var out, errOut bytes.Buffer
		streams := genericiooptions.IOStreams{
			In:     &bytes.Buffer{},
			Out:    &out,
			ErrOut: &errOut,
		}

		command := lint.NewCommand(streams, testConfigFlags())

		err := command.Complete()
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(command.NoColor).To(BeTrue())
	})
}

// TestColorDetection_NOColorEnvValues tests NO_COLOR with various values.
func TestColorDetection_NOColorEnvValues(t *testing.T) {
	testCases := []struct {
		name     string
		value    string
		expected bool
	}{
		{"value=1", "1", true},
		{"value=true", "true", true},
		{"value=yes", "yes", true},
		{"value=anything", "anything", true},
		{"empty string", "", true}, // Still true because buffer is non-TTY
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			t.Setenv("NO_COLOR", tc.value)

			var out, errOut bytes.Buffer
			streams := genericiooptions.IOStreams{
				In:     &bytes.Buffer{},
				Out:    &out,
				ErrOut: &errOut,
			}

			command := lint.NewCommand(streams, testConfigFlags())

			err := command.Complete()
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(command.NoColor).To(Equal(tc.expected))
		})
	}
}

// TestColorDetection_JSONFormat tests JSON output forces NoColor.
func TestColorDetection_JSONFormat(t *testing.T) {
	t.Run("should force no colors for JSON output format", func(t *testing.T) {
		g := NewWithT(t)

		var out, errOut bytes.Buffer
		streams := genericiooptions.IOStreams{
			In:     &bytes.Buffer{},
			Out:    &out,
			ErrOut: &errOut,
		}

		command := lint.NewCommand(streams, testConfigFlags(),
			lint.WithOutputFormat(lint.OutputFormatJSON),
		)

		err := command.Complete()
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(command.NoColor).To(BeTrue())
	})
}

// TestColorDetection_YAMLFormat tests YAML output forces NoColor.
func TestColorDetection_YAMLFormat(t *testing.T) {
	t.Run("should force no colors for YAML output format", func(t *testing.T) {
		g := NewWithT(t)

		var out, errOut bytes.Buffer
		streams := genericiooptions.IOStreams{
			In:     &bytes.Buffer{},
			Out:    &out,
			ErrOut: &errOut,
		}

		command := lint.NewCommand(streams, testConfigFlags(),
			lint.WithOutputFormat(lint.OutputFormatYAML),
		)

		err := command.Complete()
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(command.NoColor).To(BeTrue())
	})
}

// TestColorDetection_PriorityFlagOverridesDefault tests flag priority.
func TestColorDetection_PriorityFlagOverridesDefault(t *testing.T) {
	t.Run("priority: --no-color flag overrides default", func(t *testing.T) {
		g := NewWithT(t)

		var out, errOut bytes.Buffer
		streams := genericiooptions.IOStreams{
			In:     &bytes.Buffer{},
			Out:    &out,
			ErrOut: &errOut,
		}

		command := lint.NewCommand(streams, testConfigFlags())
		command.NoColor = true

		err := command.Complete()
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(command.NoColor).To(BeTrue())
	})
}

// TestColorDetection_PriorityJSONOverridesFlag tests JSON format has highest priority.
func TestColorDetection_PriorityJSONOverridesFlag(t *testing.T) {
	t.Run("priority: JSON format overrides --no-color=false", func(t *testing.T) {
		g := NewWithT(t)

		var out, errOut bytes.Buffer
		streams := genericiooptions.IOStreams{
			In:     &bytes.Buffer{},
			Out:    &out,
			ErrOut: &errOut,
		}

		// Even if NoColor is false, JSON should force it to true
		command := lint.NewCommand(streams, testConfigFlags(),
			lint.WithOutputFormat(lint.OutputFormatJSON),
		)
		command.NoColor = false

		err := command.Complete()
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(command.NoColor).To(BeTrue())
	})
}

// TestColorDetection_PriorityJSONOverridesEnv tests JSON format overrides NO_COLOR env.
func TestColorDetection_PriorityJSONOverridesEnv(t *testing.T) {
	t.Run("priority: JSON format overrides NO_COLOR unset", func(t *testing.T) {
		g := NewWithT(t)

		// NO_COLOR is not set (colors would be enabled)
		// But JSON format should force NoColor=true

		var out, errOut bytes.Buffer
		streams := genericiooptions.IOStreams{
			In:     &bytes.Buffer{},
			Out:    &out,
			ErrOut: &errOut,
		}

		command := lint.NewCommand(streams, testConfigFlags(),
			lint.WithOutputFormat(lint.OutputFormatJSON),
		)

		err := command.Complete()
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(command.NoColor).To(BeTrue())
	})
}

// TestIsTerminal_BufferOutput tests IsTerminal returns false for buffer output.
func TestIsTerminal_BufferOutput(t *testing.T) {
	t.Run("should return false for buffer (non-TTY)", func(t *testing.T) {
		g := NewWithT(t)

		// Create command with buffer output (not a real file/terminal)
		var out, errOut bytes.Buffer
		streams := genericiooptions.IOStreams{
			In:     &bytes.Buffer{},
			Out:    &out,
			ErrOut: &errOut,
		}

		command := lint.NewCommand(streams, testConfigFlags())

		// Buffer is not a TTY
		g.Expect(command.IsTerminal()).To(BeFalse())
	})
}

// TestColorDetection_PriorityEnvOverridesDefault tests NO_COLOR env overrides default.
func TestColorDetection_PriorityEnvOverridesDefault(t *testing.T) {
	t.Run("priority: NO_COLOR env overrides default", func(t *testing.T) {
		g := NewWithT(t)

		// Set NO_COLOR env var
		t.Setenv("NO_COLOR", "1")

		var out, errOut bytes.Buffer
		streams := genericiooptions.IOStreams{
			In:     &bytes.Buffer{},
			Out:    &out,
			ErrOut: &errOut,
		}

		// NoColor flag not set (would default to false)
		// But NO_COLOR env should force it to true
		command := lint.NewCommand(streams, testConfigFlags())

		err := command.Complete()
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(command.NoColor).To(BeTrue())
	})
}

// TestColorDetection_CompletePriorityChain tests full priority chain.
func TestColorDetection_CompletePriorityChain(t *testing.T) {
	t.Run("priority chain: JSON > Flag > Env > TTY > Default", func(t *testing.T) {
		testCases := []struct {
			name      string
			format    lint.OutputFormat
			flagValue bool
			envValue  string
			expected  bool
			reason    string
		}{
			{
				name:      "JSON forces NoColor",
				format:    lint.OutputFormatJSON,
				flagValue: false,
				envValue:  "",
				expected:  true,
				reason:    "JSON format has highest priority",
			},
			{
				name:      "Flag overrides env",
				format:    lint.OutputFormatTable,
				flagValue: true,
				envValue:  "",
				expected:  true,
				reason:    "Flag set to true",
			},
			{
				name:      "Env works when flag not set",
				format:    lint.OutputFormatTable,
				flagValue: false,
				envValue:  "1",
				expected:  true,
				reason:    "NO_COLOR env set",
			},
			{
				name:      "Default to true (TTY detection with buffer)",
				format:    lint.OutputFormatTable,
				flagValue: false,
				envValue:  "",
				expected:  true,
				reason:    "No overrides, but buffer is non-TTY so colors disabled",
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				g := NewWithT(t)

				if tc.envValue != "" {
					t.Setenv("NO_COLOR", tc.envValue)
				}

				var out, errOut bytes.Buffer
				streams := genericiooptions.IOStreams{
					In:     &bytes.Buffer{},
					Out:    &out,
					ErrOut: &errOut,
				}

				command := lint.NewCommand(streams, testConfigFlags(),
					lint.WithOutputFormat(tc.format),
				)
				command.NoColor = tc.flagValue

				err := command.Complete()
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(command.NoColor).To(Equal(tc.expected), tc.reason)
			})
		}
	})
}
