package migrate

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/blang/semver/v4"

	"k8s.io/cli-runtime/pkg/genericiooptions"

	"github.com/opendatahub-io/odh-cli/pkg/migrate/action"

	. "github.com/onsi/gomega"
)

type prepareTestStreams struct {
	cmd    *PrepareCommand
	outBuf *bytes.Buffer
	errBuf *bytes.Buffer
}

func newTestPrepareCommand(t *testing.T) (*PrepareCommand, *bytes.Buffer) {
	t.Helper()

	s := newTestPrepareCommandWithStdout(t)

	return s.cmd, s.errBuf
}

func newTestPrepareCommandWithStdout(t *testing.T) prepareTestStreams {
	t.Helper()

	outBuf := &bytes.Buffer{}
	errBuf := &bytes.Buffer{}
	streams := genericiooptions.IOStreams{
		In:     &bytes.Buffer{},
		Out:    outBuf,
		ErrOut: errBuf,
	}

	cmd := &PrepareCommand{
		SharedOptions: NewSharedOptions(streams),
		registry:      action.NewActionRegistry(),
		OutputDir:     filepath.Join(t.TempDir(), "backup-test"),
	}

	return prepareTestStreams{cmd: cmd, outBuf: outBuf, errBuf: errBuf}
}

func TestRunPrepareMode(t *testing.T) {
	current := semver.MustParse("2.25.0")
	target := semver.MustParse("3.0.0")

	t.Run("should execute prepare task successfully", func(t *testing.T) {
		g := NewWithT(t)
		cmd, _ := newTestPrepareCommand(t)

		task := &stubTask{result: newSuccessResult()}
		cmd.registry.MustRegister(&stubAction{
			id: "test.migrate", phase: action.PhasePreUpgrade, canApply: true,
			prepareTask: task,
		})
		cmd.MigrationIDs = []string{"test.migrate"}

		err := cmd.runPrepareMode(context.Background(), &current, &target, action.PhasePreUpgrade)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(task.execCount).To(Equal(1))
	})

	t.Run("should skip action with nil prepare task", func(t *testing.T) {
		g := NewWithT(t)
		cmd, errBuf := newTestPrepareCommand(t)

		cmd.registry.MustRegister(&stubAction{
			id: "no.prepare", phase: action.PhasePreUpgrade, canApply: true,
			prepareTask: nil,
		})
		cmd.MigrationIDs = []string{"no.prepare"}

		err := cmd.runPrepareMode(context.Background(), &current, &target, action.PhasePreUpgrade)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(errBuf.String()).To(ContainSubstring("no prepare phase"))
	})

	t.Run("should warn and proceed when phase does not match explicit migration", func(t *testing.T) {
		g := NewWithT(t)
		cmd, errBuf := newTestPrepareCommand(t)

		task := &stubTask{result: newSuccessResult()}
		cmd.registry.MustRegister(&stubAction{
			id: "pre.action", phase: action.PhasePreUpgrade, canApply: true,
			prepareTask: task,
		})
		cmd.MigrationIDs = []string{"pre.action"}

		err := cmd.runPrepareMode(context.Background(), &current, &target, action.PhasePostUpgrade)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(task.execCount).To(Equal(1))
		g.Expect(errBuf.String()).To(ContainSubstring("WARNING"))
		g.Expect(errBuf.String()).To(ContainSubstring("proceeding because --migration was explicit"))
	})

	t.Run("should return error for unknown migration ID", func(t *testing.T) {
		g := NewWithT(t)
		cmd, _ := newTestPrepareCommand(t)
		cmd.MigrationIDs = []string{"nonexistent"}

		err := cmd.runPrepareMode(context.Background(), &current, &target, action.PhasePreUpgrade)
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("not found"))
	})

	t.Run("should return error when task execution fails", func(t *testing.T) {
		g := NewWithT(t)
		cmd, _ := newTestPrepareCommand(t)

		cmd.registry.MustRegister(&stubAction{
			id: "fail.action", phase: action.PhasePreUpgrade, canApply: true,
			prepareTask: &stubTask{err: errors.New("prepare failed")},
		})
		cmd.MigrationIDs = []string{"fail.action"}

		err := cmd.runPrepareMode(context.Background(), &current, &target, action.PhasePreUpgrade)
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("preparation failed"))
	})

	t.Run("should show no backups message when output dir does not exist", func(t *testing.T) {
		g := NewWithT(t)
		cmd, errBuf := newTestPrepareCommand(t)

		task := &stubTask{result: newSuccessResult()}
		cmd.registry.MustRegister(&stubAction{
			id: "test.migrate", phase: action.PhasePreUpgrade, canApply: true,
			prepareTask: task,
		})
		cmd.MigrationIDs = []string{"test.migrate"}

		err := cmd.runPrepareMode(context.Background(), &current, &target, action.PhasePreUpgrade)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(errBuf.String()).To(ContainSubstring("No backups were created"))
	})

	t.Run("should show backups saved message when output dir has files", func(t *testing.T) {
		g := NewWithT(t)
		cmd, errBuf := newTestPrepareCommand(t)

		g.Expect(os.MkdirAll(cmd.OutputDir, 0o750)).To(Succeed())
		g.Expect(os.WriteFile(filepath.Join(cmd.OutputDir, "backup.yaml"), []byte("data"), 0o600)).To(Succeed())

		task := &stubTask{result: newSuccessResult()}
		cmd.registry.MustRegister(&stubAction{
			id: "test.migrate", phase: action.PhasePreUpgrade, canApply: true,
			prepareTask: task,
		})
		cmd.MigrationIDs = []string{"test.migrate"}

		err := cmd.runPrepareMode(context.Background(), &current, &target, action.PhasePreUpgrade)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(errBuf.String()).To(ContainSubstring("Backups saved to"))
	})

	t.Run("should return error when preparation is incomplete", func(t *testing.T) {
		g := NewWithT(t)
		cmd, _ := newTestPrepareCommand(t)

		cmd.registry.MustRegister(&stubAction{
			id: "incomplete.action", phase: action.PhasePreUpgrade, canApply: true,
			prepareTask: &stubTask{result: newIncompleteResult()},
		})
		cmd.MigrationIDs = []string{"incomplete.action"}

		err := cmd.runPrepareMode(context.Background(), &current, &target, action.PhasePreUpgrade)
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("preparation halted"))
	})

	t.Run("should execute multiple preparations", func(t *testing.T) {
		g := NewWithT(t)
		cmd, _ := newTestPrepareCommand(t)

		task1 := &stubTask{result: newSuccessResult()}
		task2 := &stubTask{result: newSuccessResult()}
		cmd.registry.MustRegister(&stubAction{
			id: "first.migrate", phase: action.PhasePreUpgrade, canApply: true,
			prepareTask: task1,
		})
		cmd.registry.MustRegister(&stubAction{
			id: "second.migrate", phase: action.PhasePreUpgrade, canApply: true,
			prepareTask: task2,
		})
		cmd.MigrationIDs = []string{"first.migrate", "second.migrate"}

		err := cmd.runPrepareMode(context.Background(), &current, &target, action.PhasePreUpgrade)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(task1.execCount).To(Equal(1))
		g.Expect(task2.execCount).To(Equal(1))
	})

	t.Run("should emit JSON structured output when format is json", func(t *testing.T) {
		g := NewWithT(t)
		s := newTestPrepareCommandWithStdout(t)
		s.cmd.OutputFormat = OutputFormatJSON

		task := &stubTask{result: newSuccessResult()}
		s.cmd.registry.MustRegister(&stubAction{
			id: "test.migrate", phase: action.PhasePreUpgrade, canApply: true,
			prepareTask: task,
		})
		s.cmd.MigrationIDs = []string{"test.migrate"}

		err := s.cmd.runPrepareMode(context.Background(), &current, &target, action.PhasePreUpgrade)
		g.Expect(err).ToNot(HaveOccurred())

		var result PrepareResult
		g.Expect(json.Unmarshal(s.outBuf.Bytes(), &result)).To(Succeed())
		g.Expect(result.Kind).To(Equal("MigratePrepareResult"))
		g.Expect(result.APIVersion).To(Equal("cli.opendatahub.io/v1"))
		g.Expect(result.CurrentVersion).To(Equal("2.25.0"))
		g.Expect(result.TargetVersion).To(Equal("3.0.0"))
		g.Expect(result.Phase).To(Equal("pre-upgrade"))
		g.Expect(result.Migrations).To(HaveLen(1))
		g.Expect(result.Migrations[0].ID).To(Equal("test.migrate"))
		g.Expect(result.Migrations[0].Completed).To(BeTrue())
		g.Expect(result.Status).ToNot(BeNil())
		g.Expect(result.Status.Result).To(Equal("success"))
	})

	t.Run("should emit YAML structured output when format is yaml", func(t *testing.T) {
		g := NewWithT(t)
		s := newTestPrepareCommandWithStdout(t)
		s.cmd.OutputFormat = OutputFormatYAML

		task := &stubTask{result: newSuccessResult()}
		s.cmd.registry.MustRegister(&stubAction{
			id: "test.migrate", phase: action.PhasePreUpgrade, canApply: true,
			prepareTask: task,
		})
		s.cmd.MigrationIDs = []string{"test.migrate"}

		err := s.cmd.runPrepareMode(context.Background(), &current, &target, action.PhasePreUpgrade)
		g.Expect(err).ToNot(HaveOccurred())

		output := s.outBuf.String()
		g.Expect(output).To(ContainSubstring("kind: MigratePrepareResult"))
		g.Expect(output).To(ContainSubstring("currentVersion: 2.25.0"))
		g.Expect(output).To(ContainSubstring("targetVersion: 3.0.0"))
	})

	t.Run("should include skipped migrations in structured output", func(t *testing.T) {
		g := NewWithT(t)
		s := newTestPrepareCommandWithStdout(t)
		s.cmd.OutputFormat = OutputFormatJSON

		s.cmd.registry.MustRegister(&stubAction{
			id: "no.prepare", phase: action.PhasePreUpgrade, canApply: true,
			prepareTask: nil,
		})
		s.cmd.MigrationIDs = []string{"no.prepare"}

		err := s.cmd.runPrepareMode(context.Background(), &current, &target, action.PhasePreUpgrade)
		g.Expect(err).ToNot(HaveOccurred())

		var result PrepareResult
		g.Expect(json.Unmarshal(s.outBuf.Bytes(), &result)).To(Succeed())
		g.Expect(result.Migrations).To(HaveLen(1))
		g.Expect(result.Migrations[0].ID).To(Equal("no.prepare"))
		g.Expect(result.Migrations[0].Skipped).To(BeTrue())
	})
}
