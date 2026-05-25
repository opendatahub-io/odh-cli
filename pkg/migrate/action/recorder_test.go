package action_test

import (
	"testing"

	"github.com/opendatahub-io/odh-cli/pkg/migrate/action"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/action/result"

	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

func TestRecorder_Child(t *testing.T) {
	g := NewWithT(t)

	recorder := action.NewRootRecorder()
	g.Expect(recorder).ToNot(BeNil())

	step1 := recorder.Child("step1", "First Step")
	g.Expect(step1).ToNot(BeNil())

	step1.Complete(result.StepCompleted, "Step 1 completed")

	actionResult := recorder.Build()
	g.Expect(actionResult).ToNot(BeNil())
	g.Expect(actionResult).To(HaveField("Status.Steps", HaveLen(1)))
	g.Expect(actionResult.Status.Steps[0]).To(MatchFields(IgnoreExtras, Fields{
		"Name":        Equal("step1"),
		"Description": Equal("First Step"),
		"Status":      Equal(result.StepCompleted),
		"Message":     Equal("Step 1 completed"),
	}))
}

func TestRecorder_NestedChildren(t *testing.T) {
	g := NewWithT(t)

	recorder := action.NewRootRecorder()

	parent := recorder.Child("parent", "Parent Step")
	child1 := parent.Child("child1", "Child 1")
	child2 := parent.Child("child2", "Child 2")

	child1.Complete(result.StepCompleted, "Child 1 done")
	child2.Complete(result.StepFailed, "Child 2 failed")
	parent.Complete(result.StepFailed, "Parent failed due to child")

	actionResult := recorder.Build()
	g.Expect(actionResult).To(HaveField("Status.Steps", HaveLen(1)))
	g.Expect(actionResult).To(HaveField("Status.Completed", BeFalse()))

	parentStep := actionResult.Status.Steps[0]
	g.Expect(parentStep).To(HaveField("Name", Equal("parent")))
	g.Expect(parentStep).To(HaveField("Children", HaveLen(2)))
	g.Expect(parentStep.Children[0]).To(MatchFields(IgnoreExtras, Fields{
		"Name":   Equal("child1"),
		"Status": Equal(result.StepCompleted),
	}))
	g.Expect(parentStep.Children[1]).To(MatchFields(IgnoreExtras, Fields{
		"Name":   Equal("child2"),
		"Status": Equal(result.StepFailed),
	}))
}

func TestRecorder_AddDetail(t *testing.T) {
	g := NewWithT(t)

	recorder := action.NewRootRecorder()
	step := recorder.Child("test", "Test Step")

	step.AddDetail("key1", "value1")
	step.AddDetail("key2", 42)
	step.Complete(result.StepCompleted, "Done")

	actionResult := recorder.Build()
	g.Expect(actionResult).To(HaveField("Status.Steps", HaveLen(1)))
	g.Expect(actionResult.Status.Steps[0]).To(HaveField("Details", And(
		HaveKeyWithValue("key1", "value1"),
		HaveKeyWithValue("key2", 42),
	)))
}

func TestRecorder_Record(t *testing.T) {
	g := NewWithT(t)

	recorder := action.NewRootRecorder()
	recorder.Record("quick-step", "Quick step message", result.StepCompleted)

	actionResult := recorder.Build()
	g.Expect(actionResult).To(HaveField("Status.Steps", HaveLen(1)))
	g.Expect(actionResult.Status.Steps[0]).To(MatchFields(IgnoreExtras, Fields{
		"Name":    Equal("quick-step"),
		"Message": Equal("Quick step message"),
		"Status":  Equal(result.StepCompleted),
	}))
}

func TestRecorder_NonTerminalSteps(t *testing.T) {
	t.Run("running child prevents completion", func(t *testing.T) {
		g := NewWithT(t)

		recorder := action.NewRootRecorder()

		parent := recorder.Child("parent", "Parent Step")
		// Leave child in running state (default state when created)
		parent.Child("child1", "Child 1")
		parent.Complete(result.StepCompleted, "Parent completed but child running")

		actionResult := recorder.Build()
		g.Expect(actionResult).To(HaveField("Status.Steps", HaveLen(1)))
		g.Expect(actionResult).To(HaveField("Status.Completed", BeFalse()))
	})

	t.Run("pending step prevents completion", func(t *testing.T) {
		g := NewWithT(t)

		recorder := action.NewRootRecorder()
		pendingChild := recorder.Child("pending", "Pending Step")
		pendingChild.Complete(result.StepPending, "Explicitly pending")

		actionResult := recorder.Build()
		g.Expect(actionResult).To(HaveField("Status.Completed", BeFalse()))
	})

	t.Run("all terminal steps allows completion", func(t *testing.T) {
		g := NewWithT(t)

		recorder := action.NewRootRecorder()

		parent := recorder.Child("parent", "Parent Step")
		child1 := parent.Child("child1", "Child 1")
		child2 := parent.Child("child2", "Child 2")

		child1.Complete(result.StepCompleted, "Child 1 done")
		child2.Complete(result.StepSkipped, "Child 2 skipped")
		parent.Complete(result.StepCompleted, "Parent completed")

		actionResult := recorder.Build()
		g.Expect(actionResult).To(HaveField("Status.Completed", BeTrue()))
	})
}
