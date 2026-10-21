package cobra_test

import (
	"testing"

	"github.com/spf13/cobra"

	utilcobra "github.com/opendatahub-io/odh-cli/pkg/util/cobra"

	. "github.com/onsi/gomega"
)

// Verify both option forms satisfy the shared option interface.
var (
	_ utilcobra.CommandOption = utilcobra.CommandOptions{}
	_ utilcobra.CommandOption = (*utilcobra.CommandOptions)(nil)
	_ utilcobra.CommandOption = utilcobra.WithHidden(true)
)

// Test constants for command names and flags used across option tests.
const (
	useChild      = "child"
	useFirst      = "first"
	useGrandchild = "grandchild"
	useHidden     = "hidden"
	useParent     = "parent"
	useSecond     = "second"
	useVisible    = "visible"

	flagDryRun = "dry-run"
)

func TestApplyOptionsWithoutOptions(t *testing.T) {
	t.Run("should keep an initially visible command visible", func(t *testing.T) {
		g := NewWithT(t)

		cmd := &cobra.Command{Use: useVisible}

		utilcobra.ApplyOptions(cmd)

		g.Expect(cmd).To(HaveField("Hidden", BeFalse()))
	})

	t.Run("should keep an initially hidden command hidden", func(t *testing.T) {
		g := NewWithT(t)

		cmd := &cobra.Command{Use: useHidden, Hidden: true}

		utilcobra.ApplyOptions(cmd)

		g.Expect(cmd).To(HaveField("Hidden", BeTrue()))
	})
}

func TestApplyOptionsWithHiddenFunction(t *testing.T) {
	t.Run("should hide a visible command", func(t *testing.T) {
		g := NewWithT(t)

		cmd := &cobra.Command{Use: useVisible}

		utilcobra.ApplyOptions(cmd, utilcobra.WithHidden(true))

		g.Expect(cmd).To(HaveField("Hidden", BeTrue()))
	})

	t.Run("should unhide a hidden command", func(t *testing.T) {
		g := NewWithT(t)

		cmd := &cobra.Command{Use: useHidden, Hidden: true}

		utilcobra.ApplyOptions(cmd, utilcobra.WithHidden(false))

		g.Expect(cmd).To(HaveField("Hidden", BeFalse()))
	})
}

func TestApplyOptionsWithCommandOptionsStruct(t *testing.T) {
	t.Run("should hide through a struct value", func(t *testing.T) {
		g := NewWithT(t)

		cmd := &cobra.Command{Use: useVisible}

		utilcobra.ApplyOptions(cmd, utilcobra.CommandOptions{Hidden: true})

		g.Expect(cmd).To(HaveField("Hidden", BeTrue()))
	})

	t.Run("should hide through a struct pointer", func(t *testing.T) {
		g := NewWithT(t)

		cmd := &cobra.Command{Use: useVisible}

		utilcobra.ApplyOptions(cmd, &utilcobra.CommandOptions{Hidden: true})

		g.Expect(cmd).To(HaveField("Hidden", BeTrue()))
	})

	t.Run("should unhide a hidden command through a struct value", func(t *testing.T) {
		g := NewWithT(t)

		cmd := &cobra.Command{Use: useHidden, Hidden: true}

		utilcobra.ApplyOptions(cmd, utilcobra.CommandOptions{Hidden: false})

		g.Expect(cmd).To(HaveField("Hidden", BeFalse()))
	})

	t.Run("should unhide through a zero-valued struct", func(t *testing.T) {
		g := NewWithT(t)

		cmd := &cobra.Command{Use: useHidden, Hidden: true}

		utilcobra.ApplyOptions(cmd, utilcobra.CommandOptions{})

		g.Expect(cmd).To(HaveField("Hidden", BeFalse()))
	})
}

func TestApplyOptionsLastOptionWins(t *testing.T) {
	t.Run("should let the last function option win when hiding", func(t *testing.T) {
		g := NewWithT(t)

		cmd := &cobra.Command{Use: useVisible}

		utilcobra.ApplyOptions(cmd, utilcobra.WithHidden(false), utilcobra.WithHidden(true))

		g.Expect(cmd).To(HaveField("Hidden", BeTrue()))
	})

	t.Run("should let the last function option win when unhiding", func(t *testing.T) {
		g := NewWithT(t)

		cmd := &cobra.Command{Use: useVisible}

		utilcobra.ApplyOptions(cmd, utilcobra.WithHidden(true), utilcobra.WithHidden(false))

		g.Expect(cmd).To(HaveField("Hidden", BeFalse()))
	})

	t.Run("should let the last struct option win", func(t *testing.T) {
		g := NewWithT(t)

		cmd := &cobra.Command{Use: useVisible}

		utilcobra.ApplyOptions(cmd,
			utilcobra.CommandOptions{Hidden: true},
			utilcobra.CommandOptions{Hidden: false},
		)

		g.Expect(cmd).To(HaveField("Hidden", BeFalse()))
	})

	t.Run("should let a struct option override a preceding function option", func(t *testing.T) {
		g := NewWithT(t)

		cmd := &cobra.Command{Use: useVisible}

		utilcobra.ApplyOptions(cmd, utilcobra.WithHidden(true), utilcobra.CommandOptions{Hidden: false})

		g.Expect(cmd).To(HaveField("Hidden", BeFalse()))
	})

	t.Run("should let a function option override a preceding struct option", func(t *testing.T) {
		g := NewWithT(t)

		cmd := &cobra.Command{Use: useVisible}

		utilcobra.ApplyOptions(cmd, utilcobra.CommandOptions{Hidden: false}, utilcobra.WithHidden(true))

		g.Expect(cmd).To(HaveField("Hidden", BeTrue()))
	})
}

func TestApplyOptionsIsIndependentPerTarget(t *testing.T) {
	g := NewWithT(t)

	first := &cobra.Command{Use: useFirst}
	second := &cobra.Command{Use: useSecond}

	utilcobra.ApplyOptions(first, utilcobra.WithHidden(true))

	g.Expect(first).To(HaveField("Hidden", BeTrue()))
	g.Expect(second).To(HaveField("Hidden", BeFalse()))
}

func TestApplyOptionsOnlyAffectsTargetCommand(t *testing.T) {
	g := NewWithT(t)

	parent := &cobra.Command{Use: useParent}
	child := &cobra.Command{Use: useChild, Hidden: true}
	grandchild := &cobra.Command{Use: useGrandchild}

	parent.AddCommand(child)
	child.AddCommand(grandchild)
	child.Flags().Bool(flagDryRun, false, "run without applying changes")

	utilcobra.ApplyOptions(child, utilcobra.WithHidden(false))

	g.Expect(child).To(HaveField("Hidden", BeFalse()))
	g.Expect(parent).To(HaveField("Hidden", BeFalse()))
	g.Expect(grandchild).To(HaveField("Hidden", BeFalse()))

	// Applying options must be limited to visibility: unrelated command
	// metadata and registered flags stay intact.
	g.Expect(child).To(HaveField("Use", Equal(useChild)))
	g.Expect(child.Flags().Lookup(flagDryRun)).ToNot(BeNil())
}
