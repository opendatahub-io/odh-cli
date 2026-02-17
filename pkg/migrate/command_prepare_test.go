package migrate_test

import (
	"testing"

	"k8s.io/cli-runtime/pkg/genericiooptions"

	"github.com/opendatahub-io/odh-cli/pkg/migrate"

	. "github.com/onsi/gomega"
)

func TestPrepareCommand_Validate(t *testing.T) {
	g := NewWithT(t)

	t.Run("should require migration ID", func(t *testing.T) {
		cmd := migrate.NewPrepareCommand(genericiooptions.IOStreams{})
		cmd.MigrationIDs = []string{}
		cmd.TargetVersion = "3.0.0"

		err := cmd.Validate()
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("migration"))
	})

	t.Run("should require target version", func(t *testing.T) {
		cmd := migrate.NewPrepareCommand(genericiooptions.IOStreams{})
		cmd.MigrationIDs = []string{"test.migration"}
		cmd.TargetVersion = ""

		err := cmd.Validate()
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("target-version"))
	})

	t.Run("should validate successfully with required fields", func(t *testing.T) {
		cmd := migrate.NewPrepareCommand(genericiooptions.IOStreams{})
		cmd.MigrationIDs = []string{"test.migration"}
		cmd.TargetVersion = "3.0.0"

		err := cmd.Complete()
		g.Expect(err).ToNot(HaveOccurred())

		err = cmd.Validate()
		g.Expect(err).ToNot(HaveOccurred())
	})

	t.Run("should accept multiple migration IDs", func(t *testing.T) {
		cmd := migrate.NewPrepareCommand(genericiooptions.IOStreams{})
		cmd.MigrationIDs = []string{"migration1", "migration2", "migration3"}
		cmd.TargetVersion = "3.0.0"

		err := cmd.Complete()
		g.Expect(err).ToNot(HaveOccurred())

		err = cmd.Validate()
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(cmd.MigrationIDs).To(HaveLen(3))
	})
}

func TestPrepareCommand_Complete(t *testing.T) {
	g := NewWithT(t)

	t.Run("should parse valid target version", func(t *testing.T) {
		cmd := migrate.NewPrepareCommand(genericiooptions.IOStreams{})
		cmd.TargetVersion = "3.0.0"

		err := cmd.Complete()
		g.Expect(err).ToNot(HaveOccurred())
	})

	t.Run("should reject invalid target version", func(t *testing.T) {
		cmd := migrate.NewPrepareCommand(genericiooptions.IOStreams{})
		cmd.TargetVersion = "invalid"

		err := cmd.Complete()
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("invalid target version"))
	})

	t.Run("should accept partial version format", func(t *testing.T) {
		cmd := migrate.NewPrepareCommand(genericiooptions.IOStreams{})
		cmd.TargetVersion = "3.0"

		err := cmd.Complete()
		g.Expect(err).ToNot(HaveOccurred())
	})

	t.Run("should set default output directory with timestamp", func(t *testing.T) {
		cmd := migrate.NewPrepareCommand(genericiooptions.IOStreams{})
		cmd.TargetVersion = "3.0.0"
		cmd.OutputDir = ""

		err := cmd.Complete()
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(cmd.OutputDir).ToNot(BeEmpty())
		g.Expect(cmd.OutputDir).To(ContainSubstring("backup-migrate-"))
	})

	t.Run("should preserve custom output directory", func(t *testing.T) {
		cmd := migrate.NewPrepareCommand(genericiooptions.IOStreams{})
		cmd.TargetVersion = "3.0.0"
		cmd.OutputDir = "/custom/path"

		err := cmd.Complete()
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(cmd.OutputDir).To(Equal("/custom/path"))
	})

	t.Run("should always enable verbose mode", func(t *testing.T) {
		cmd := migrate.NewPrepareCommand(genericiooptions.IOStreams{})
		cmd.TargetVersion = "3.0.0"
		cmd.Verbose = false

		err := cmd.Complete()
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(cmd.Verbose).To(BeTrue())
	})
}
