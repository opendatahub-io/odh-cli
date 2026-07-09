package rhbok_test

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/opendatahub-io/odh-cli/pkg/migrate/action"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/action/result"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/actions/kueue/rhbok"

	. "github.com/onsi/gomega"
)

func TestWaitForEmbeddedRemoval(t *testing.T) {
	a := &rhbok.RHBOKMigrationAction{}

	t.Run("completes when KueueReady is Removed", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		dsc := makeDSCV1("default-dsc",
			withComponent("kueue", "Removed"),
			withDSCCondition("KueueReady", "False", "Removed"),
		)
		target := newTarget(t, []*unstructured.Unstructured{dsc}, targetOpts{
			skipConfirm: true,
			rbacAllowed: true,
		})

		rhbok.ExportWaitEmbeddedRemoval(a, ctx, target)

		res := target.Recorder.(action.RootRecorder).Build()
		g.Expect(res.Status.Steps).To(HaveLen(1))
		g.Expect(res.Status.Steps[0].Status).To(Equal(result.StepCompleted))
		g.Expect(res.Status.Steps[0].Message).To(ContainSubstring("Embedded Kueue removed"))
	})

	t.Run("skips when skip-remove-embedded is set", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		a := &rhbok.RHBOKMigrationAction{SkipRemoveEmbedded: true}
		target := newTarget(t, nil, targetOpts{rbacAllowed: true})

		rhbok.ExportWaitEmbeddedRemoval(a, ctx, target)

		res := target.Recorder.(action.RootRecorder).Build()
		g.Expect(res.Status.Steps[0].Status).To(Equal(result.StepSkipped))
		g.Expect(res.Status.Steps[0].Message).To(ContainSubstring("skip-remove-embedded"))
	})
}

func TestWaitForKueueReady(t *testing.T) {
	a := &rhbok.RHBOKMigrationAction{}

	t.Run("completes when KueueReady is True and pods are running", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		dsc := makeDSCV1("default-dsc",
			withComponent("kueue", "Unmanaged"),
			withDSCCondition("KueueReady", "True", "Ready"),
		)
		target := newTarget(t, []*unstructured.Unstructured{dsc}, targetOpts{
			skipConfirm: true,
			rbacAllowed: true,
		})

		rhbok.ExportWaitKueueReady(a, ctx, target)

		res := target.Recorder.(action.RootRecorder).Build()
		g.Expect(res.Status.Steps).To(HaveLen(1))
		g.Expect(res.Status.Steps[0].Status).To(Equal(result.StepCompleted))
		g.Expect(res.Status.Steps[0].Message).To(ContainSubstring("KueueReady=True"))
	})
}
