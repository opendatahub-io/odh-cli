package migrate

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/blang/semver/v4"

	"k8s.io/cli-runtime/pkg/genericiooptions"

	"github.com/opendatahub-io/odh-cli/pkg/migrate/action"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/action/result"

	. "github.com/onsi/gomega"
)

type stubTask struct {
	result    *result.ActionResult
	err       error
	execCount int
}

func (s *stubTask) Validate(_ context.Context, _ action.Target) (*result.ActionResult, error) {
	return s.result, s.err
}

func (s *stubTask) Execute(_ context.Context, _ action.Target) (*result.ActionResult, error) {
	s.execCount++

	return s.result, s.err
}

func newSuccessResult() *result.ActionResult {
	r := result.New("migration", "test", "Test", "")
	r.Status.Completed = true

	return r
}

func newResultWithSkippedSteps() *result.ActionResult {
	r := result.New("migration", "test", "Test", "")
	r.Status.Completed = true
	r.Status.Steps = []result.ActionStep{
		result.NewStep("step1", "Step 1", result.StepCompleted, "done"),
		result.NewStep("step2", "Step 2", result.StepSkipped, "user cancelled"),
	}

	return r
}

func newIncompleteResult() *result.ActionResult {
	r := result.New("migration", "test", "Test", "")
	r.Status.Completed = false

	return r
}

func newResultWithFailedSteps() *result.ActionResult {
	r := result.New("migration", "test", "Test", "")
	r.Status.Completed = true
	r.Status.Steps = []result.ActionStep{
		result.NewStep("step1", "Step 1", result.StepCompleted, "done"),
		result.NewStep("step2", "Step 2", result.StepFailed, "auth model not patched"),
	}

	return r
}

func newResultWithFailedAndSkippedSteps() *result.ActionResult {
	r := result.New("migration", "test", "Test", "")
	r.Status.Completed = true
	r.Status.Steps = []result.ActionStep{
		result.NewStep("step1", "Step 1", result.StepCompleted, "done"),
		result.NewStep("step2", "Step 2", result.StepSkipped, "user cancelled"),
		result.NewStep("step3", "Step 3", result.StepFailed, "auth model not patched"),
	}

	return r
}

type runTestStreams struct {
	cmd    *RunCommand
	outBuf *bytes.Buffer
	errBuf *bytes.Buffer
}

func newTestRunCommand() (*RunCommand, *bytes.Buffer) {
	s := newTestRunCommandWithStdout()

	return s.cmd, s.errBuf
}

func newTestRunCommandWithStdout() runTestStreams {
	outBuf := &bytes.Buffer{}
	errBuf := &bytes.Buffer{}
	streams := genericiooptions.IOStreams{
		In:     &bytes.Buffer{},
		Out:    outBuf,
		ErrOut: errBuf,
	}

	cmd := &RunCommand{
		SharedOptions: NewSharedOptions(streams),
		registry:      action.NewActionRegistry(),
	}

	return runTestStreams{cmd: cmd, outBuf: outBuf, errBuf: errBuf}
}

func TestRunMigrationMode(t *testing.T) {
	current := semver.MustParse("2.25.0")
	target := semver.MustParse("3.0.0")

	t.Run("should execute a single migration successfully", func(t *testing.T) {
		g := NewWithT(t)
		cmd, _ := newTestRunCommand()

		task := &stubTask{result: newSuccessResult()}
		cmd.registry.MustRegister(&stubAction{
			id: "test.migrate", phase: action.PhasePreUpgrade, canApply: true,
			runTask: task,
		})
		cmd.MigrationIDs = []string{"test.migrate"}

		err := cmd.runMigrationMode(context.Background(), &current, &target, action.PhasePreUpgrade)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(task.execCount).To(Equal(1))
	})

	t.Run("should return error for unknown migration ID", func(t *testing.T) {
		g := NewWithT(t)
		cmd, _ := newTestRunCommand()
		cmd.MigrationIDs = []string{"nonexistent"}

		err := cmd.runMigrationMode(context.Background(), &current, &target, action.PhasePreUpgrade)
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("not found"))
	})

	t.Run("should warn and proceed when phase does not match explicit migration", func(t *testing.T) {
		g := NewWithT(t)
		cmd, errBuf := newTestRunCommand()

		task := &stubTask{result: newSuccessResult()}
		cmd.registry.MustRegister(&stubAction{
			id: "pre.action", phase: action.PhasePreUpgrade, canApply: true,
			runTask: task,
		})
		cmd.MigrationIDs = []string{"pre.action"}

		err := cmd.runMigrationMode(context.Background(), &current, &target, action.PhasePostUpgrade)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(task.execCount).To(Equal(1))
		g.Expect(errBuf.String()).To(ContainSubstring("WARNING"))
		g.Expect(errBuf.String()).To(ContainSubstring("proceeding because --migration was explicit"))
	})

	t.Run("should return error when task execution fails", func(t *testing.T) {
		g := NewWithT(t)
		cmd, _ := newTestRunCommand()

		cmd.registry.MustRegister(&stubAction{
			id: "fail.action", phase: action.PhasePreUpgrade, canApply: true,
			runTask: &stubTask{err: errors.New("task failed")},
		})
		cmd.MigrationIDs = []string{"fail.action"}

		err := cmd.runMigrationMode(context.Background(), &current, &target, action.PhasePreUpgrade)
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("migration failed"))
	})

	t.Run("should return error when migration is incomplete", func(t *testing.T) {
		g := NewWithT(t)
		cmd, _ := newTestRunCommand()

		cmd.registry.MustRegister(&stubAction{
			id: "incomplete.action", phase: action.PhasePreUpgrade, canApply: true,
			runTask: &stubTask{result: newIncompleteResult()},
		})
		cmd.MigrationIDs = []string{"incomplete.action"}

		err := cmd.runMigrationMode(context.Background(), &current, &target, action.PhasePreUpgrade)
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("migration halted"))
	})

	t.Run("should return error when action has nil run task", func(t *testing.T) {
		g := NewWithT(t)
		cmd, _ := newTestRunCommand()

		cmd.registry.MustRegister(&stubAction{
			id: "nil.task", phase: action.PhasePreUpgrade, canApply: true,
			runTask: nil,
		})
		cmd.MigrationIDs = []string{"nil.task"}

		err := cmd.runMigrationMode(context.Background(), &current, &target, action.PhasePreUpgrade)
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("no run task"))
	})

	t.Run("should execute multiple migrations", func(t *testing.T) {
		g := NewWithT(t)
		cmd, _ := newTestRunCommand()

		task1 := &stubTask{result: newSuccessResult()}
		task2 := &stubTask{result: newSuccessResult()}
		cmd.registry.MustRegister(&stubAction{
			id: "first.migrate", phase: action.PhasePreUpgrade, canApply: true,
			runTask: task1,
		})
		cmd.registry.MustRegister(&stubAction{
			id: "second.migrate", phase: action.PhasePreUpgrade, canApply: true,
			runTask: task2,
		})
		cmd.MigrationIDs = []string{"first.migrate", "second.migrate"}

		err := cmd.runMigrationMode(context.Background(), &current, &target, action.PhasePreUpgrade)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(task1.execCount).To(Equal(1))
		g.Expect(task2.execCount).To(Equal(1))
	})

	t.Run("should report skipped steps in output", func(t *testing.T) {
		g := NewWithT(t)
		cmd, errBuf := newTestRunCommand()

		cmd.registry.MustRegister(&stubAction{
			id: "skip.action", phase: action.PhasePreUpgrade, canApply: true,
			runTask: &stubTask{result: newResultWithSkippedSteps()},
		})
		cmd.MigrationIDs = []string{"skip.action"}

		err := cmd.runMigrationMode(context.Background(), &current, &target, action.PhasePreUpgrade)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(errBuf.String()).To(ContainSubstring("completed with skipped steps"))
		g.Expect(errBuf.String()).To(ContainSubstring("some steps were skipped"))
		g.Expect(errBuf.String()).ToNot(ContainSubstring("completed successfully"))
	})

	t.Run("should report failed steps and return error", func(t *testing.T) {
		g := NewWithT(t)
		cmd, errBuf := newTestRunCommand()

		cmd.registry.MustRegister(&stubAction{
			id: "fail.action", phase: action.PhasePreUpgrade, canApply: true,
			runTask: &stubTask{result: newResultWithFailedSteps()},
		})
		cmd.MigrationIDs = []string{"fail.action"}

		err := cmd.runMigrationMode(context.Background(), &current, &target, action.PhasePreUpgrade)
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("completed with failures"))
		g.Expect(errBuf.String()).To(ContainSubstring("completed with failures"))
		g.Expect(errBuf.String()).To(ContainSubstring("some steps failed"))
		g.Expect(errBuf.String()).ToNot(ContainSubstring("completed successfully"))
	})

	t.Run("should report failures over skips when both present across migrations", func(t *testing.T) {
		g := NewWithT(t)
		cmd, errBuf := newTestRunCommand()

		cmd.registry.MustRegister(&stubAction{
			id: "fail.action", phase: action.PhasePreUpgrade, canApply: true,
			runTask: &stubTask{result: newResultWithFailedSteps()},
		})
		cmd.registry.MustRegister(&stubAction{
			id: "skip.action", phase: action.PhasePreUpgrade, canApply: true,
			runTask: &stubTask{result: newResultWithSkippedSteps()},
		})
		cmd.MigrationIDs = []string{"fail.action", "skip.action"}

		err := cmd.runMigrationMode(context.Background(), &current, &target, action.PhasePreUpgrade)
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("completed with failures"))
		g.Expect(errBuf.String()).To(ContainSubstring("some steps failed, some were skipped"))
		g.Expect(errBuf.String()).ToNot(ContainSubstring("completed successfully"))
	})

	t.Run("should track skips when single migration has both failed and skipped steps", func(t *testing.T) {
		g := NewWithT(t)
		cmd, errBuf := newTestRunCommand()

		cmd.registry.MustRegister(&stubAction{
			id: "mixed.action", phase: action.PhasePreUpgrade, canApply: true,
			runTask: &stubTask{result: newResultWithFailedAndSkippedSteps()},
		})
		cmd.MigrationIDs = []string{"mixed.action"}

		err := cmd.runMigrationMode(context.Background(), &current, &target, action.PhasePreUpgrade)
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("completed with failures"))
		g.Expect(errBuf.String()).To(ContainSubstring("some steps failed, some were skipped"))
		g.Expect(errBuf.String()).ToNot(ContainSubstring("completed successfully"))
	})

	t.Run("should execute migration when explicit --migration matches --phase", func(t *testing.T) {
		g := NewWithT(t)
		cmd, _ := newTestRunCommand()

		task := &stubTask{result: newSuccessResult()}
		cmd.registry.MustRegister(&stubAction{
			id: "pre.action", phase: action.PhasePreUpgrade, canApply: true,
			runTask: task,
		})
		cmd.MigrationIDs = []string{"pre.action"}

		err := cmd.runMigrationMode(context.Background(), &current, &target, action.PhasePreUpgrade)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(task.execCount).To(Equal(1))
	})

	t.Run("should emit JSON structured output when format is json", func(t *testing.T) {
		g := NewWithT(t)
		s := newTestRunCommandWithStdout()
		s.cmd.OutputFormat = OutputFormatJSON

		task := &stubTask{result: newSuccessResult()}
		s.cmd.registry.MustRegister(&stubAction{
			id: "test.migrate", phase: action.PhasePreUpgrade, canApply: true,
			runTask: task,
		})
		s.cmd.MigrationIDs = []string{"test.migrate"}

		err := s.cmd.runMigrationMode(context.Background(), &current, &target, action.PhasePreUpgrade)
		g.Expect(err).ToNot(HaveOccurred())

		var result RunResult
		g.Expect(json.Unmarshal(s.outBuf.Bytes(), &result)).To(Succeed())
		g.Expect(result.Kind).To(Equal("MigrateRunResult"))
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
		s := newTestRunCommandWithStdout()
		s.cmd.OutputFormat = OutputFormatYAML

		task := &stubTask{result: newSuccessResult()}
		s.cmd.registry.MustRegister(&stubAction{
			id: "test.migrate", phase: action.PhasePreUpgrade, canApply: true,
			runTask: task,
		})
		s.cmd.MigrationIDs = []string{"test.migrate"}

		err := s.cmd.runMigrationMode(context.Background(), &current, &target, action.PhasePreUpgrade)
		g.Expect(err).ToNot(HaveOccurred())

		output := s.outBuf.String()
		g.Expect(output).To(ContainSubstring("kind: MigrateRunResult"))
		g.Expect(output).To(ContainSubstring("currentVersion: 2.25.0"))
		g.Expect(output).To(ContainSubstring("targetVersion: 3.0.0"))
	})

	t.Run("should include skipped steps in JSON structured output", func(t *testing.T) {
		g := NewWithT(t)
		s := newTestRunCommandWithStdout()
		s.cmd.OutputFormat = OutputFormatJSON

		s.cmd.registry.MustRegister(&stubAction{
			id: "skip.action", phase: action.PhasePreUpgrade, canApply: true,
			runTask: &stubTask{result: newResultWithSkippedSteps()},
		})
		s.cmd.MigrationIDs = []string{"skip.action"}

		err := s.cmd.runMigrationMode(context.Background(), &current, &target, action.PhasePreUpgrade)
		g.Expect(err).ToNot(HaveOccurred())

		var result RunResult
		g.Expect(json.Unmarshal(s.outBuf.Bytes(), &result)).To(Succeed())
		g.Expect(result.Migrations).To(HaveLen(1))
		g.Expect(result.Migrations[0].HasSkippedSteps).To(BeTrue())
	})
}
