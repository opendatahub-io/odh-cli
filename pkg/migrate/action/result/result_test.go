package result_test

import (
	"testing"

	"github.com/opendatahub-io/odh-cli/pkg/migrate/action/result"

	. "github.com/onsi/gomega"
)

func TestHasSkippedSteps(t *testing.T) {
	t.Run("should return false when no steps", func(t *testing.T) {
		g := NewWithT(t)
		r := result.New("migration", "test", "Test", "")
		g.Expect(r.HasSkippedSteps()).To(BeFalse())
	})

	t.Run("should return false when all steps completed", func(t *testing.T) {
		g := NewWithT(t)
		r := result.New("migration", "test", "Test", "")
		r.Status.Steps = []result.ActionStep{
			result.NewStep("step1", "Step 1", result.StepCompleted, "done"),
			result.NewStep("step2", "Step 2", result.StepCompleted, "done"),
		}
		g.Expect(r.HasSkippedSteps()).To(BeFalse())
	})

	t.Run("should return true when top-level step is skipped", func(t *testing.T) {
		g := NewWithT(t)
		r := result.New("migration", "test", "Test", "")
		r.Status.Steps = []result.ActionStep{
			result.NewStep("step1", "Step 1", result.StepCompleted, "done"),
			result.NewStep("step2", "Step 2", result.StepSkipped, "user cancelled"),
		}
		g.Expect(r.HasSkippedSteps()).To(BeTrue())
	})

	t.Run("should return true when nested child step is skipped", func(t *testing.T) {
		g := NewWithT(t)
		r := result.New("migration", "test", "Test", "")

		parent := result.NewStep("parent", "Parent", result.StepCompleted, "done")
		parent.Children = []result.ActionStep{
			result.NewStep("child", "Child", result.StepSkipped, "skipped"),
		}

		r.Status.Steps = []result.ActionStep{parent}
		g.Expect(r.HasSkippedSteps()).To(BeTrue())
	})

	t.Run("should return false when children are all completed", func(t *testing.T) {
		g := NewWithT(t)
		r := result.New("migration", "test", "Test", "")

		parent := result.NewStep("parent", "Parent", result.StepCompleted, "done")
		parent.Children = []result.ActionStep{
			result.NewStep("child", "Child", result.StepCompleted, "done"),
		}

		r.Status.Steps = []result.ActionStep{parent}
		g.Expect(r.HasSkippedSteps()).To(BeFalse())
	})
}

func TestHasFailedSteps(t *testing.T) {
	t.Run("should return false when no steps", func(t *testing.T) {
		g := NewWithT(t)
		r := result.New("migration", "test", "Test", "")
		g.Expect(r.HasFailedSteps()).To(BeFalse())
	})

	t.Run("should return false when all steps completed", func(t *testing.T) {
		g := NewWithT(t)
		r := result.New("migration", "test", "Test", "")
		r.Status.Steps = []result.ActionStep{
			result.NewStep("step1", "Step 1", result.StepCompleted, "done"),
			result.NewStep("step2", "Step 2", result.StepCompleted, "done"),
		}
		g.Expect(r.HasFailedSteps()).To(BeFalse())
	})

	t.Run("should return true when top-level step failed", func(t *testing.T) {
		g := NewWithT(t)
		r := result.New("migration", "test", "Test", "")
		r.Status.Steps = []result.ActionStep{
			result.NewStep("step1", "Step 1", result.StepCompleted, "done"),
			result.NewStep("step2", "Step 2", result.StepFailed, "something broke"),
		}
		g.Expect(r.HasFailedSteps()).To(BeTrue())
	})

	t.Run("should return true when nested child step failed", func(t *testing.T) {
		g := NewWithT(t)
		r := result.New("migration", "test", "Test", "")

		parent := result.NewStep("parent", "Parent", result.StepCompleted, "done")
		parent.Children = []result.ActionStep{
			result.NewStep("child", "Child", result.StepFailed, "failed"),
		}

		r.Status.Steps = []result.ActionStep{parent}
		g.Expect(r.HasFailedSteps()).To(BeTrue())
	})

	t.Run("should return false when only skipped steps", func(t *testing.T) {
		g := NewWithT(t)
		r := result.New("migration", "test", "Test", "")
		r.Status.Steps = []result.ActionStep{
			result.NewStep("step1", "Step 1", result.StepCompleted, "done"),
			result.NewStep("step2", "Step 2", result.StepSkipped, "skipped"),
		}
		g.Expect(r.HasFailedSteps()).To(BeFalse())
	})
}
