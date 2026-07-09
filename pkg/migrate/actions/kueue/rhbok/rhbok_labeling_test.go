package rhbok_test

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/opendatahub-io/odh-cli/pkg/constants"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/action"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/action/result"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/actions/kueue/rhbok"

	. "github.com/onsi/gomega"
)

func TestDiscoverLabelingPlan(t *testing.T) {
	a := &rhbok.RHBOKMigrationAction{}

	t.Run("union of kueue-enabled and local queue namespaces", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		teamA := makeNamespace("team-a", map[string]string{constants.LabelKueueManaged: "true"})
		teamB := makeNamespace("team-b", nil)
		lq := makeLocalQueue("team-lq", inNamespace("team-b"))
		objects := []*unstructured.Unstructured{teamA, teamB, lq}

		target := newTarget(t, objects, targetOpts{
			rbacAllowed: true,
			kubeObjects: []runtime.Object{
				makeKubeNamespace("team-a", map[string]string{constants.LabelKueueManaged: "true"}),
				makeKubeNamespace("team-b", nil),
			},
		})

		plan, err := rhbok.ExportDiscoverLabelingPlan(a, ctx, target)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(rhbok.ExportPlanNamespaces(plan)).To(ConsistOf("team-a", "team-b"))
	})

	t.Run("excludes namespaces already openshift-managed", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		managed := makeNamespace("team-managed", map[string]string{
			constants.LabelKueueManaged:          "true",
			constants.LabelKueueOpenshiftManaged: "true",
		})
		target := newTarget(t, []*unstructured.Unstructured{managed}, targetOpts{
			rbacAllowed: true,
			kubeObjects: []runtime.Object{
				makeKubeNamespace("team-managed", map[string]string{
					constants.LabelKueueManaged:          "true",
					constants.LabelKueueOpenshiftManaged: "true",
				}),
			},
		})

		plan, err := rhbok.ExportDiscoverLabelingPlan(a, ctx, target)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(rhbok.ExportPlanNamespaces(plan)).To(BeEmpty())
	})

	t.Run("selects workloads missing queue-name label", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		ns := makeNamespace("team-a", map[string]string{constants.LabelKueueManaged: "true"})
		nb := makeNotebook("nb-1", inNamespace("team-a"))
		labeled := makeNotebook("nb-2", inNamespace("team-a"),
			withLabel(constants.LabelKueueQueueName, "existing"))

		target := newTarget(t, []*unstructured.Unstructured{ns, nb, labeled}, targetOpts{
			rbacAllowed: true,
			kubeObjects: []runtime.Object{
				makeKubeNamespace("team-a", map[string]string{constants.LabelKueueManaged: "true"}),
			},
		})

		plan, err := rhbok.ExportDiscoverLabelingPlan(a, ctx, target)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(rhbok.ExportPlanWorkloads(plan)).To(HaveLen(1))
		g.Expect(rhbok.ExportWorkloadQueueName(rhbok.ExportPlanWorkloads(plan)[0])).To(Equal("default"))
	})

	t.Run("uses single local queue name when unambiguous", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		ns := makeNamespace("team-a", map[string]string{constants.LabelKueueManaged: "true"})
		lq := makeLocalQueue("my-queue", inNamespace("team-a"))
		nb := makeNotebook("nb-1", inNamespace("team-a"))

		target := newTarget(t, []*unstructured.Unstructured{ns, lq, nb}, targetOpts{
			rbacAllowed: true,
			kubeObjects: []runtime.Object{
				makeKubeNamespace("team-a", map[string]string{constants.LabelKueueManaged: "true"}),
			},
		})

		plan, err := rhbok.ExportDiscoverLabelingPlan(a, ctx, target)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(rhbok.ExportPlanWorkloads(plan)).To(HaveLen(1))
		g.Expect(rhbok.ExportWorkloadQueueName(rhbok.ExportPlanWorkloads(plan)[0])).To(Equal("my-queue"))
	})
}

func TestLabelKueueNamespaces(t *testing.T) {
	a := &rhbok.RHBOKMigrationAction{}

	t.Run("applies openshift managed label", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		ns := makeNamespace("team-a", map[string]string{constants.LabelKueueManaged: "true"})
		target := newTarget(t, []*unstructured.Unstructured{ns}, targetOpts{
			skipConfirm: true,
			rbacAllowed: true,
			kubeObjects: []runtime.Object{
				makeKubeNamespace("team-a", map[string]string{constants.LabelKueueManaged: "true"}),
			},
		})

		rhbok.ExportLabelKueueNamespaces(a, ctx, target)

		updated, err := target.Client.CoreV1().Namespaces().Get(ctx, "team-a", metav1.GetOptions{})
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(updated.Labels[constants.LabelKueueOpenshiftManaged]).To(Equal("true"))
		g.Expect(updated.Labels[constants.LabelKueueManaged]).To(Equal("true"))

		res := target.Recorder.(action.RootRecorder).Build()
		step := res.Status.Steps[0]
		g.Expect(step.Status).To(Equal(result.StepCompleted))
		g.Expect(step.Message).To(ContainSubstring("Labeled 1/1"))
	})
}
