package rhbok_test

import (
	"errors"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	k8stesting "k8s.io/client-go/testing"

	"github.com/opendatahub-io/odh-cli/pkg/constants"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/action"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/action/result"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/actions/kueue/rhbok"

	. "github.com/onsi/gomega"
)

type verifiedMigrationFixture struct {
	objects []*unstructured.Unstructured
	opts    targetOpts
	dsc     *unstructured.Unstructured
	ns      *unstructured.Unstructured
	nb      *unstructured.Unstructured
}

func verifiedMigrationSetup() verifiedMigrationFixture {
	dsc := makeDSCV1("default-dsc",
		withComponent("kueue", "Unmanaged"),
		withDSCCondition("KueueReady", "True", "Ready"),
	)
	ns := makeNamespace("team-a", map[string]string{
		constants.LabelKueueManaged:          "true",
		constants.LabelKueueOpenshiftManaged: "true",
	})
	nb := makeNotebook("nb-1", inNamespace("team-a"),
		withLabel(constants.LabelKueueQueueName, "default"))

	opts := targetOpts{
		skipConfirm: true,
		rbacAllowed: true,
		olmObjects: []runtime.Object{
			newOLMSubscription(rhbok.ExportSubscriptionName, rhbok.ExportOperatorNamespace),
		},
		kubeObjects: []runtime.Object{
			makeKubeNamespace("team-a", map[string]string{
				constants.LabelKueueManaged:          "true",
				constants.LabelKueueOpenshiftManaged: "true",
			}),
		},
	}

	return verifiedMigrationFixture{
		objects: []*unstructured.Unstructured{dsc, ns, nb},
		opts:    opts,
		dsc:     dsc,
		ns:      ns,
		nb:      nb,
	}
}

func TestVerifyMigrationComplete(t *testing.T) {
	a := &rhbok.RHBOKMigrationAction{}

	t.Run("dry-run skips", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		target := newTarget(t, nil, targetOpts{dryRun: true, rbacAllowed: true})

		rhbok.ExportVerifyMigration(a, ctx, target)

		res := target.Recorder.(action.RootRecorder).Build()
		step := findStep(res.Status.Steps, "verify-migration-complete")
		g.Expect(step).ToNot(BeNil())
		g.Expect(step.Status).To(Equal(result.StepSkipped))
	})

	t.Run("passes when all criteria are met", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		fixture := verifiedMigrationSetup()
		target := newTarget(t, fixture.objects, fixture.opts)

		rhbok.ExportVerifyMigration(a, ctx, target)

		res := target.Recorder.(action.RootRecorder).Build()
		step := findStep(res.Status.Steps, "verify-migration-complete")
		g.Expect(step).ToNot(BeNil())
		g.Expect(step.Status).To(Equal(result.StepCompleted))
		g.Expect(step.Message).To(ContainSubstring("passed"))
	})

	t.Run("fails when embedded deployment still exists", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		fixture := verifiedMigrationSetup()
		embedded := makeDeployment(rhbok.ExportEmbeddedDeployment,
			inNamespace(rhbok.ExportApplicationsNamespace))
		objects := append(fixture.objects, embedded)
		target := newTarget(t, objects, fixture.opts)

		rhbok.ExportVerifyMigration(a, ctx, target)

		res := target.Recorder.(action.RootRecorder).Build()
		step := findStep(res.Status.Steps, "verify-migration-complete")
		g.Expect(step.Status).To(Equal(result.StepFailed))
		g.Expect(step.Message).To(ContainSubstring("embedded deployment"))
	})

	t.Run("fails when KueueReady is not True", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		fixture := verifiedMigrationSetup()
		fixture.dsc = makeDSCV1("default-dsc",
			withComponent("kueue", "Unmanaged"),
			withDSCCondition("KueueReady", "False", "Removed"),
		)
		fixture.objects[0] = fixture.dsc
		target := newTarget(t, fixture.objects, fixture.opts)

		rhbok.ExportVerifyMigration(a, ctx, target)

		res := target.Recorder.(action.RootRecorder).Build()
		step := findStep(res.Status.Steps, "verify-migration-complete")
		g.Expect(step.Status).To(Equal(result.StepFailed))
		g.Expect(step.Message).To(ContainSubstring("KueueReady"))
	})

	t.Run("fails when operator subscription is missing", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		fixture := verifiedMigrationSetup()
		fixture.opts.olmObjects = nil
		target := newTarget(t, fixture.objects, fixture.opts)

		rhbok.ExportVerifyMigration(a, ctx, target)

		res := target.Recorder.(action.RootRecorder).Build()
		step := findStep(res.Status.Steps, "verify-migration-complete")
		g.Expect(step.Status).To(Equal(result.StepFailed))
		g.Expect(step.Message).To(ContainSubstring("subscription not found"))
	})

	t.Run("fails when namespaces are missing openshift managed label", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		fixture := verifiedMigrationSetup()
		fixture.ns = makeNamespace("team-a", map[string]string{constants.LabelKueueManaged: "true"})
		fixture.objects[1] = fixture.ns
		fixture.opts.kubeObjects = []runtime.Object{
			makeKubeNamespace("team-a", map[string]string{constants.LabelKueueManaged: "true"}),
		}
		target := newTarget(t, fixture.objects, fixture.opts)

		rhbok.ExportVerifyMigration(a, ctx, target)

		res := target.Recorder.(action.RootRecorder).Build()
		step := findStep(res.Status.Steps, "verify-migration-complete")
		g.Expect(step.Status).To(Equal(result.StepFailed))
		g.Expect(step.Message).To(ContainSubstring("missing"))
		g.Expect(step.Message).To(ContainSubstring(constants.LabelKueueOpenshiftManaged))
	})

	t.Run("fails when workloads are missing queue-name label", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		fixture := verifiedMigrationSetup()
		fixture.nb = makeNotebook("nb-1", inNamespace("team-a"))
		fixture.objects[2] = fixture.nb
		target := newTarget(t, fixture.objects, fixture.opts)

		rhbok.ExportVerifyMigration(a, ctx, target)

		res := target.Recorder.(action.RootRecorder).Build()
		step := findStep(res.Status.Steps, "verify-migration-complete")
		g.Expect(step.Status).To(Equal(result.StepFailed))
		g.Expect(step.Message).To(ContainSubstring("missing queue-name label"))
	})
}

func TestVerifyResourcesPreserved(t *testing.T) {
	a := &rhbok.RHBOKMigrationAction{}

	t.Run("reports preserved queue counts", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		cq1 := makeClusterQueue("cq-1")
		cq2 := makeClusterQueue("cq-2")
		lq := makeLocalQueue("lq-1", inNamespace("default"))
		target := newTarget(t, []*unstructured.Unstructured{cq1, cq2, lq}, targetOpts{rbacAllowed: true})

		rhbok.ExportVerifyResources(a, ctx, target)

		res := target.Recorder.(action.RootRecorder).Build()
		step := findStep(res.Status.Steps, "verify-resources-preserved")
		g.Expect(step).ToNot(BeNil())
		g.Expect(step.Status).To(Equal(result.StepCompleted))
		g.Expect(step.Message).To(ContainSubstring("2 ClusterQueues"))
		g.Expect(step.Message).To(ContainSubstring("1 LocalQueues"))
	})

	t.Run("dry-run skips", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		target := newTarget(t, nil, targetOpts{dryRun: true, rbacAllowed: true})

		rhbok.ExportVerifyResources(a, ctx, target)

		res := target.Recorder.(action.RootRecorder).Build()
		step := findStep(res.Status.Steps, "verify-resources-preserved")
		g.Expect(step.Status).To(Equal(result.StepSkipped))
	})

	t.Run("ClusterQueue list error fails", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		target := newTarget(t, nil, targetOpts{
			rbacAllowed: true,
			dynamicReactor: func(act k8stesting.Action) (bool, runtime.Object, error) {
				if act.GetResource().Resource == "clusterqueues" && act.GetVerb() == "list" {
					return true, nil, errors.New("forbidden")
				}

				return false, nil, nil
			},
		})

		rhbok.ExportVerifyResources(a, ctx, target)

		res := target.Recorder.(action.RootRecorder).Build()
		step := findStep(res.Status.Steps, "verify-resources-preserved")
		g.Expect(step.Status).To(Equal(result.StepFailed))
		g.Expect(step.Message).To(ContainSubstring("Failed to list ClusterQueues"))
	})
}
