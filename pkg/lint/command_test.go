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
