package action_test

import (
	"testing"

	"github.com/opendatahub-io/odh-cli/pkg/migrate/action"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/action/result"

	. "github.com/onsi/gomega"
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
	g.Expect(actionResult.Status.Steps).To(HaveLen(1))
	g.Expect(actionResult.Status.Steps[0].Name).To(Equal("step1"))
	g.Expect(actionResult.Status.Steps[0].Description).To(Equal("First Step"))
	g.Expect(actionResult.Status.Steps[0].Status).To(Equal(result.StepCompleted))
	g.Expect(actionResult.Status.Steps[0].Message).To(Equal("Step 1 completed"))
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
	g.Expect(actionResult.Status.Steps).To(HaveLen(1))

	parentStep := actionResult.Status.Steps[0]
	g.Expect(parentStep.Name).To(Equal("parent"))
	g.Expect(parentStep.Children).To(HaveLen(2))
	g.Expect(parentStep.Children[0].Name).To(Equal("child1"))
	g.Expect(parentStep.Children[0].Status).To(Equal(result.StepCompleted))
	g.Expect(parentStep.Children[1].Name).To(Equal("child2"))
	g.Expect(parentStep.Children[1].Status).To(Equal(result.StepFailed))
}

func TestRecorder_AddDetail(t *testing.T) {
	g := NewWithT(t)

	recorder := action.NewRootRecorder()
	step := recorder.Child("test", "Test Step")

	step.AddDetail("key1", "value1")
	step.AddDetail("key2", 42)
	step.Complete(result.StepCompleted, "Done")

	actionResult := recorder.Build()
	g.Expect(actionResult.Status.Steps).To(HaveLen(1))
	g.Expect(actionResult.Status.Steps[0].Details).To(HaveKey("key1"))
	g.Expect(actionResult.Status.Steps[0].Details["key1"]).To(Equal("value1"))
	g.Expect(actionResult.Status.Steps[0].Details).To(HaveKey("key2"))
	g.Expect(actionResult.Status.Steps[0].Details["key2"]).To(Equal(42))
}

func TestRecorder_Record(t *testing.T) {
	g := NewWithT(t)

	recorder := action.NewRootRecorder()
	recorder.Record("quick-step", "Quick step message", result.StepCompleted)

	actionResult := recorder.Build()
	g.Expect(actionResult.Status.Steps).To(HaveLen(1))
	g.Expect(actionResult.Status.Steps[0].Name).To(Equal("quick-step"))
	g.Expect(actionResult.Status.Steps[0].Message).To(Equal("Quick step message"))
	g.Expect(actionResult.Status.Steps[0].Status).To(Equal(result.StepCompleted))
}
