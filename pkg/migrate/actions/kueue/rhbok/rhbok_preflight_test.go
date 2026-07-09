package rhbok_test

import (
	"errors"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	k8stesting "k8s.io/client-go/testing"

	"github.com/opendatahub-io/odh-cli/pkg/migrate/action"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/action/result"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/actions/kueue/rhbok"
	"github.com/opendatahub-io/odh-cli/pkg/util/kube/rbac"

	. "github.com/onsi/gomega"
)

func TestCheckCurrentKueueState(t *testing.T) {
	a := &rhbok.RHBOKMigrationAction{}

	t.Run("DSC with Kueue Managed", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()
		dsc := makeDSCV1("default-dsc", withComponent("kueue", "Managed"))
		target := newTarget(t, []*unstructured.Unstructured{dsc}, targetOpts{rbacAllowed: true})

		rhbok.ExportCheckCurrentKueueState(a, ctx, target)

		res := target.Recorder.(action.RootRecorder).Build()
		g.Expect(res.Status.Steps).To(HaveLen(1))
		g.Expect(res.Status.Steps[0].Status).To(Equal(result.StepCompleted))
		g.Expect(res.Status.Steps[0].Message).To(ContainSubstring("Managed"))
	})

	t.Run("DSC with Kueue Unmanaged", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()
		dsc := makeDSCV1("default-dsc", withComponent("kueue", "Unmanaged"))
		target := newTarget(t, []*unstructured.Unstructured{dsc}, targetOpts{rbacAllowed: true})

		rhbok.ExportCheckCurrentKueueState(a, ctx, target)

		res := target.Recorder.(action.RootRecorder).Build()
		g.Expect(res.Status.Steps).To(HaveLen(1))
		g.Expect(res.Status.Steps[0].Status).To(Equal(result.StepCompleted))
		g.Expect(res.Status.Steps[0].Message).To(ContainSubstring("Unmanaged"))
	})

	t.Run("DSC not found", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()
		target := newTarget(t, nil, targetOpts{rbacAllowed: true})

		rhbok.ExportCheckCurrentKueueState(a, ctx, target)

		res := target.Recorder.(action.RootRecorder).Build()
		g.Expect(res.Status.Steps).To(HaveLen(1))
		g.Expect(res.Status.Steps[0].Status).To(Equal(result.StepFailed))
		g.Expect(res.Status.Steps[0].Message).To(ContainSubstring("not found"))
	})

	t.Run("Kueue component missing in DSC", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()
		dsc := makeDSCV1("default-dsc", withComponent("codeflare", "Managed"))
		target := newTarget(t, []*unstructured.Unstructured{dsc}, targetOpts{rbacAllowed: true})

		rhbok.ExportCheckCurrentKueueState(a, ctx, target)

		res := target.Recorder.(action.RootRecorder).Build()
		g.Expect(res.Status.Steps).To(HaveLen(1))
		g.Expect(res.Status.Steps[0].Status).To(Equal(result.StepFailed))
	})

	t.Run("empty managementState", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()
		dsc := makeDSCV1("default-dsc", withComponent("kueue", ""))
		target := newTarget(t, []*unstructured.Unstructured{dsc}, targetOpts{rbacAllowed: true})

		rhbok.ExportCheckCurrentKueueState(a, ctx, target)

		res := target.Recorder.(action.RootRecorder).Build()
		g.Expect(res.Status.Steps).To(HaveLen(1))
		g.Expect(res.Status.Steps[0].Status).To(Equal(result.StepFailed))
		g.Expect(res.Status.Steps[0].Message).To(ContainSubstring("not configured"))
	})
}

func TestCheckNoRHBOKConflicts(t *testing.T) {
	a := &rhbok.RHBOKMigrationAction{}

	t.Run("no subscription exists", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()
		dsc := makeDSCV1("default-dsc", withComponent("kueue", "Managed"))
		target := newTarget(t, []*unstructured.Unstructured{dsc}, targetOpts{rbacAllowed: true})

		rhbok.ExportCheckNoRHBOKConflicts(a, ctx, target)

		res := target.Recorder.(action.RootRecorder).Build()
		g.Expect(res.Status.Steps).To(HaveLen(1))
		g.Expect(res.Status.Steps[0].Status).To(Equal(result.StepCompleted))
		g.Expect(res.Status.Steps[0].Message).To(ContainSubstring("No"))
	})

	t.Run("subscription already exists", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()
		dsc := makeDSCV1("default-dsc", withComponent("kueue", "Removed"))
		sub := makeSubscription("kueue-operator", inNamespace("openshift-kueue-operator"))
		target := newTarget(t, []*unstructured.Unstructured{dsc, sub}, targetOpts{
			rbacAllowed: true,
			olmObjects: []runtime.Object{
				newOLMSubscription("kueue-operator", "openshift-kueue-operator"),
			},
		})

		rhbok.ExportCheckNoRHBOKConflicts(a, ctx, target)

		res := target.Recorder.(action.RootRecorder).Build()
		g.Expect(res.Status.Steps).To(HaveLen(1))
		g.Expect(res.Status.Steps[0].Status).To(Equal(result.StepCompleted))
		g.Expect(res.Status.Steps[0].Message).To(ContainSubstring("already installed"))
	})

	t.Run("conflict when managed and operator installed", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()
		dsc := makeDSCV1("default-dsc", withComponent("kueue", "Managed"))
		sub := makeSubscription("kueue-operator", inNamespace("openshift-kueue-operator"))
		target := newTarget(t, []*unstructured.Unstructured{dsc, sub}, targetOpts{
			rbacAllowed: true,
			olmObjects: []runtime.Object{
				newOLMSubscription("kueue-operator", "openshift-kueue-operator"),
			},
		})

		rhbok.ExportCheckNoRHBOKConflicts(a, ctx, target)

		res := target.Recorder.(action.RootRecorder).Build()
		g.Expect(res.Status.Steps).To(HaveLen(1))
		g.Expect(res.Status.Steps[0].Status).To(Equal(result.StepFailed))
		g.Expect(res.Status.Steps[0].Message).To(ContainSubstring("Conflict"))
	})
}

func TestVerifyKueueResources(t *testing.T) {
	a := &rhbok.RHBOKMigrationAction{}

	t.Run("ClusterQueues and LocalQueues exist", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()
		cq := makeClusterQueue("test-cq")
		lq := makeLocalQueue("test-lq", inNamespace("default"))
		target := newTarget(t, []*unstructured.Unstructured{cq, lq}, targetOpts{rbacAllowed: true})

		rhbok.ExportVerifyKueueResources(a, ctx, target)

		res := target.Recorder.(action.RootRecorder).Build()
		g.Expect(res.Status.Steps).To(HaveLen(1))
		g.Expect(res.Status.Steps[0].Status).To(Equal(result.StepCompleted))
		g.Expect(res.Status.Steps[0].Message).To(ContainSubstring("1 ClusterQueues"))
		g.Expect(res.Status.Steps[0].Message).To(ContainSubstring("1 LocalQueues"))
	})

	t.Run("no resources", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()
		target := newTarget(t, nil, targetOpts{rbacAllowed: true})

		rhbok.ExportVerifyKueueResources(a, ctx, target)

		res := target.Recorder.(action.RootRecorder).Build()
		g.Expect(res.Status.Steps).To(HaveLen(1))
		g.Expect(res.Status.Steps[0].Status).To(Equal(result.StepCompleted))
		g.Expect(res.Status.Steps[0].Message).To(ContainSubstring("0 ClusterQueues"))
	})
}

func TestVerifyRBAC(t *testing.T) {
	a := &rhbok.RHBOKMigrationAction{}

	t.Run("all permissions allowed", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()
		target := newTarget(t, nil, targetOpts{rbacAllowed: true})

		checks := []rbac.PermissionCheck{
			{Verb: "get", Group: "apps", Resource: "deployments"},
		}
		rhbok.ExportVerifyRBAC(a, ctx, target, checks)

		res := target.Recorder.(action.RootRecorder).Build()
		g.Expect(res.Status.Steps).To(HaveLen(1))
		g.Expect(res.Status.Steps[0].Status).To(Equal(result.StepCompleted))
		g.Expect(res.Status.Steps[0].Message).To(ContainSubstring("1 required permissions verified"))
	})

	t.Run("some permissions denied", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()
		target := newTarget(t, nil, targetOpts{rbacAllowed: false})

		checks := []rbac.PermissionCheck{
			{Verb: "get", Group: "apps", Resource: "deployments"},
			{Verb: "create", Group: "apps", Resource: "deployments"},
		}
		rhbok.ExportVerifyRBAC(a, ctx, target, checks)

		res := target.Recorder.(action.RootRecorder).Build()
		g.Expect(res.Status.Steps).To(HaveLen(1))
		g.Expect(res.Status.Steps[0].Status).To(Equal(result.StepFailed))
		g.Expect(res.Status.Steps[0].Message).To(ContainSubstring("2 required permission(s) denied"))
		g.Expect(res.Status.Steps[0].Children).To(HaveLen(2))
	})
}

func TestCheckCurrentKueueState_APIError(t *testing.T) {
	a := &rhbok.RHBOKMigrationAction{}

	t.Run("non-NotFound error", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		target := newTarget(t, nil, targetOpts{
			rbacAllowed: true,
			dynamicReactor: func(act k8stesting.Action) (bool, runtime.Object, error) {
				if act.GetResource().Resource == "datascienceclusters" && act.GetVerb() == "list" {
					return true, nil, errors.New("server timeout")
				}

				return false, nil, nil
			},
		})

		rhbok.ExportCheckCurrentKueueState(a, ctx, target)

		res := target.Recorder.(action.RootRecorder).Build()
		g.Expect(res.Status.Steps).To(HaveLen(1))
		g.Expect(res.Status.Steps[0].Status).To(Equal(result.StepFailed))
		g.Expect(res.Status.Steps[0].Message).To(ContainSubstring("Failed to get"))
	})
}

func TestCheckNoRHBOKConflicts_APIError(t *testing.T) {
	a := &rhbok.RHBOKMigrationAction{}

	t.Run("non-NotFound error", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()
		dsc := makeDSCV1("default-dsc", withComponent("kueue", "Managed"))

		target := newTarget(t, []*unstructured.Unstructured{dsc}, targetOpts{
			rbacAllowed: true,
			olmReactor: func(act k8stesting.Action) (bool, runtime.Object, error) {
				if act.GetResource().Resource == "subscriptions" && act.GetVerb() == "list" {
					return true, nil, errors.New("server unavailable")
				}

				return false, nil, nil
			},
		})

		rhbok.ExportCheckNoRHBOKConflicts(a, ctx, target)

		res := target.Recorder.(action.RootRecorder).Build()
		g.Expect(res.Status.Steps).To(HaveLen(1))
		g.Expect(res.Status.Steps[0].Status).To(Equal(result.StepFailed))
		g.Expect(res.Status.Steps[0].Message).To(ContainSubstring("Failed to check"))
	})
}

func TestVerifyKueueResources_ListErrors(t *testing.T) {
	a := &rhbok.RHBOKMigrationAction{}

	t.Run("ClusterQueue list error", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		target := newTarget(t, nil, targetOpts{
			rbacAllowed: true,
			dynamicReactor: func(act k8stesting.Action) (bool, runtime.Object, error) {
				if act.GetResource().Resource == "clusterqueues" && act.GetVerb() == "list" {
					return true, nil, errors.New("forbidden: cluster admin required")
				}

				return false, nil, nil
			},
		})

		rhbok.ExportVerifyKueueResources(a, ctx, target)

		res := target.Recorder.(action.RootRecorder).Build()
		g.Expect(res.Status.Steps).To(HaveLen(1))
		g.Expect(res.Status.Steps[0].Status).To(Equal(result.StepFailed))
		g.Expect(res.Status.Steps[0].Message).To(ContainSubstring("Failed to list ClusterQueues"))
	})

	t.Run("LocalQueue list error", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		cq := makeClusterQueue("test-cq")
		target := newTarget(t, []*unstructured.Unstructured{cq}, targetOpts{
			rbacAllowed: true,
			dynamicReactor: func(act k8stesting.Action) (bool, runtime.Object, error) {
				if act.GetResource().Resource == "localqueues" && act.GetVerb() == "list" {
					return true, nil, errors.New("forbidden: cluster admin required")
				}

				return false, nil, nil
			},
		})

		rhbok.ExportVerifyKueueResources(a, ctx, target)

		res := target.Recorder.(action.RootRecorder).Build()
		g.Expect(res.Status.Steps).To(HaveLen(1))
		g.Expect(res.Status.Steps[0].Status).To(Equal(result.StepFailed))
		g.Expect(res.Status.Steps[0].Message).To(ContainSubstring("Failed to list LocalQueues"))
	})
}

func TestVerifyKueueResources_MultipleResources(t *testing.T) {
	a := &rhbok.RHBOKMigrationAction{}

	t.Run("multiple ClusterQueues and LocalQueues", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()
		cq1 := makeClusterQueue("cq-1")
		cq2 := makeClusterQueue("cq-2")
		lq1 := makeLocalQueue("lq-1", inNamespace("default"))
		lq2 := makeLocalQueue("lq-2", inNamespace("team-a"))
		lq3 := makeLocalQueue("lq-3", inNamespace("team-b"))
		target := newTarget(t, []*unstructured.Unstructured{cq1, cq2, lq1, lq2, lq3}, targetOpts{rbacAllowed: true})

		rhbok.ExportVerifyKueueResources(a, ctx, target)

		res := target.Recorder.(action.RootRecorder).Build()
		g.Expect(res.Status.Steps).To(HaveLen(1))
		g.Expect(res.Status.Steps[0].Status).To(Equal(result.StepCompleted))
		g.Expect(res.Status.Steps[0].Message).To(ContainSubstring("2 ClusterQueues"))
		g.Expect(res.Status.Steps[0].Message).To(ContainSubstring("3 LocalQueues"))
	})

	t.Run("only ClusterQueues no LocalQueues", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()
		cq := makeClusterQueue("cq-1")
		target := newTarget(t, []*unstructured.Unstructured{cq}, targetOpts{rbacAllowed: true})

		rhbok.ExportVerifyKueueResources(a, ctx, target)

		res := target.Recorder.(action.RootRecorder).Build()
		g.Expect(res.Status.Steps).To(HaveLen(1))
		g.Expect(res.Status.Steps[0].Status).To(Equal(result.StepCompleted))
		g.Expect(res.Status.Steps[0].Message).To(ContainSubstring("1 ClusterQueues"))
		g.Expect(res.Status.Steps[0].Message).To(ContainSubstring("0 LocalQueues"))
	})
}

func TestCheckKueueManaged(t *testing.T) {
	a := &rhbok.RHBOKMigrationAction{}

	t.Run("Kueue is Managed", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()
		dsc := makeDSCV1("default-dsc", withComponent("kueue", "Managed"))
		target := newTarget(t, []*unstructured.Unstructured{dsc}, targetOpts{rbacAllowed: true})

		result := rhbok.ExportCheckKueueManaged(a, ctx, target)
		g.Expect(result).To(BeTrue())
	})

	t.Run("Kueue is Unmanaged", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()
		dsc := makeDSCV1("default-dsc", withComponent("kueue", "Unmanaged"))
		target := newTarget(t, []*unstructured.Unstructured{dsc}, targetOpts{rbacAllowed: true})

		result := rhbok.ExportCheckKueueManaged(a, ctx, target)
		g.Expect(result).To(BeFalse())
	})

	t.Run("DSC not found", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()
		target := newTarget(t, nil, targetOpts{rbacAllowed: true})

		result := rhbok.ExportCheckKueueManaged(a, ctx, target)
		g.Expect(result).To(BeFalse())
	})

	t.Run("Kueue component not in DSC", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()
		dsc := makeDSCV1("default-dsc", withComponent("codeflare", "Managed"))
		target := newTarget(t, []*unstructured.Unstructured{dsc}, targetOpts{rbacAllowed: true})

		result := rhbok.ExportCheckKueueManaged(a, ctx, target)
		g.Expect(result).To(BeFalse())
	})
}
