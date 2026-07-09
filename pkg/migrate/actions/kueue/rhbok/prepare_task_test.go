package rhbok_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8stesting "k8s.io/client-go/testing"

	"github.com/opendatahub-io/odh-cli/pkg/migrate/action/result"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/actions/kueue/rhbok"

	. "github.com/onsi/gomega"
)

func TestPrepareTask_Validate(t *testing.T) {
	t.Run("runs preflight checks and builds result", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		dsc := makeDSCV1("default-dsc", withComponent("kueue", "Managed"))
		cq := makeClusterQueue("test-cq")
		target := newTarget(t, []*unstructured.Unstructured{dsc, cq}, targetOpts{rbacAllowed: true})

		a := &rhbok.RHBOKMigrationAction{}
		task := a.Prepare()

		res, err := task.Validate(ctx, target)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(res).ToNot(BeNil())
		g.Expect(len(res.Status.Steps)).To(BeNumerically(">=", 5))
	})

	t.Run("reports failure when DSC not found", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		target := newTarget(t, nil, targetOpts{rbacAllowed: true})

		a := &rhbok.RHBOKMigrationAction{}
		task := a.Prepare()

		res, err := task.Validate(ctx, target)
		g.Expect(err).ToNot(HaveOccurred())

		dscStep := findStep(res.Status.Steps, "check-kueue-state")
		g.Expect(dscStep).ToNot(BeNil())
		g.Expect(dscStep.Status).To(Equal(result.StepFailed))
	})

	t.Run("fails execute when validation fails", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		target := newTarget(t, nil, targetOpts{rbacAllowed: true})

		a := &rhbok.RHBOKMigrationAction{}
		task := a.Prepare()

		res, err := task.Execute(ctx, target)
		g.Expect(err).To(MatchError("preflight checks failed"))
		g.Expect(res).ToNot(BeNil())

		dscStep := findStep(res.Status.Steps, "check-kueue-state")
		g.Expect(dscStep).ToNot(BeNil())
		g.Expect(dscStep.Status).To(Equal(result.StepFailed))
		g.Expect(findStep(res.Status.Steps, "backup-kueue-resources")).To(BeNil())
		g.Expect(findStep(res.Status.Steps, "backup-skipped")).To(BeNil())
	})
}

func TestPrepareTask_Execute(t *testing.T) {
	t.Run("backs up ClusterQueues and ConfigMap when Kueue is Managed", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		outputDir := t.TempDir()
		dsc := makeDSCV1("default-dsc", withComponent("kueue", "Managed"))
		cq := makeClusterQueue("test-cq")
		cm := makeConfigMap(rhbok.ExportConfigMapName, inNamespace(rhbok.ExportApplicationsNamespace))
		target := newTarget(t, []*unstructured.Unstructured{dsc, cq, cm}, targetOpts{
			rbacAllowed: true,
			outputDir:   outputDir,
		})

		a := &rhbok.RHBOKMigrationAction{}
		task := a.Prepare()

		res, err := task.Execute(ctx, target)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(res).ToNot(BeNil())

		cqFiles, err := filepath.Glob(filepath.Join(outputDir, "cluster-scoped", "clusterqueues*"))
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(cqFiles).To(HaveLen(1))

		cmFiles, err := filepath.Glob(filepath.Join(outputDir, rhbok.ExportApplicationsNamespace, "configmaps*"))
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(cmFiles).To(HaveLen(1))
	})

	t.Run("skips backup when Kueue is not managed", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		outputDir := t.TempDir()
		dsc := makeDSCV1("default-dsc", withComponent("kueue", "Unmanaged"))
		target := newTarget(t, []*unstructured.Unstructured{dsc}, targetOpts{
			rbacAllowed: true,
			outputDir:   outputDir,
		})

		a := &rhbok.RHBOKMigrationAction{}
		task := a.Prepare()

		res, err := task.Execute(ctx, target)
		g.Expect(err).ToNot(HaveOccurred())

		// Find the backup-skipped step
		found := false
		for _, step := range res.Status.Steps {
			if step.Name == "backup-skipped" {
				found = true
				g.Expect(step.Status).To(Equal(result.StepSkipped))
			}
		}
		g.Expect(found).To(BeTrue())

		entries, err := os.ReadDir(outputDir)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(entries).To(BeEmpty())
	})

	t.Run("dry-run skips backup writes", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		outputDir := t.TempDir()
		dsc := makeDSCV1("default-dsc", withComponent("kueue", "Managed"))
		cq := makeClusterQueue("test-cq")
		target := newTarget(t, []*unstructured.Unstructured{dsc, cq}, targetOpts{
			dryRun:      true,
			rbacAllowed: true,
			outputDir:   outputDir,
		})

		a := &rhbok.RHBOKMigrationAction{}
		task := a.Prepare()

		res, err := task.Execute(ctx, target)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(res).ToNot(BeNil())

		entries, err := os.ReadDir(outputDir)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(entries).To(BeEmpty())
	})

	t.Run("no ClusterQueues skips ClusterQueue backup", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		outputDir := t.TempDir()
		dsc := makeDSCV1("default-dsc", withComponent("kueue", "Managed"))
		cm := makeConfigMap(rhbok.ExportConfigMapName, inNamespace(rhbok.ExportApplicationsNamespace))
		target := newTarget(t, []*unstructured.Unstructured{dsc, cm}, targetOpts{
			rbacAllowed: true,
			outputDir:   outputDir,
		})

		a := &rhbok.RHBOKMigrationAction{}
		task := a.Prepare()

		res, err := task.Execute(ctx, target)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(res).ToNot(BeNil())

		cqFiles, err := filepath.Glob(filepath.Join(outputDir, "cluster-scoped", "clusterqueues*"))
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(cqFiles).To(BeEmpty())
	})

	t.Run("ConfigMap not found skips ConfigMap backup", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		outputDir := t.TempDir()
		dsc := makeDSCV1("default-dsc", withComponent("kueue", "Managed"))
		cq := makeClusterQueue("test-cq")
		target := newTarget(t, []*unstructured.Unstructured{dsc, cq}, targetOpts{
			rbacAllowed: true,
			outputDir:   outputDir,
		})

		a := &rhbok.RHBOKMigrationAction{}
		task := a.Prepare()

		res, err := task.Execute(ctx, target)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(res).ToNot(BeNil())

		cmFiles, err := filepath.Glob(filepath.Join(outputDir, rhbok.ExportApplicationsNamespace, "configmaps*"))
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(cmFiles).To(BeEmpty())
	})
}

func TestPrepareTask_Execute_BackupNotFound(t *testing.T) {
	t.Run("ClusterQueue CRD not found skips backup", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		dsc := makeDSCV1("default-dsc", withComponent("kueue", "Managed"))
		target := newTarget(t, []*unstructured.Unstructured{dsc}, targetOpts{
			rbacAllowed: true,
			dynamicReactor: func(act k8stesting.Action) (bool, runtime.Object, error) {
				if act.GetResource().Resource == "clusterqueues" {
					return true, nil, apierrors.NewNotFound(
						schema.GroupResource{Group: "kueue.x-k8s.io", Resource: "clusterqueues"}, "")
				}

				return false, nil, nil
			},
		})

		a := &rhbok.RHBOKMigrationAction{}
		task := a.Prepare()

		res, err := task.Execute(ctx, target)
		g.Expect(err).ToNot(HaveOccurred())

		backupStep := findStepRecursive(res.Status.Steps, "backup-clusterqueues")
		g.Expect(backupStep).ToNot(BeNil())
		g.Expect(backupStep.Status).To(Equal(result.StepSkipped))
		g.Expect(backupStep.Message).To(ContainSubstring("No ClusterQueue CRD found"))
	})

	t.Run("ConfigMap not found skips backup", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		dsc := makeDSCV1("default-dsc", withComponent("kueue", "Managed"))
		cq := makeClusterQueue("cq-1")
		target := newTarget(t, []*unstructured.Unstructured{dsc, cq}, targetOpts{
			rbacAllowed: true,
			dynamicReactor: func(act k8stesting.Action) (bool, runtime.Object, error) {
				if act.GetResource().Resource == "configmaps" && act.GetVerb() == "get" {
					return true, nil, apierrors.NewNotFound(
						schema.GroupResource{Resource: "configmaps"}, rhbok.ExportConfigMapName)
				}

				return false, nil, nil
			},
		})

		a := &rhbok.RHBOKMigrationAction{}
		task := a.Prepare()

		res, err := task.Execute(ctx, target)
		g.Expect(err).ToNot(HaveOccurred())

		cmStep := findStepRecursive(res.Status.Steps, "backup-configmap")
		g.Expect(cmStep).ToNot(BeNil())
		g.Expect(cmStep.Status).To(Equal(result.StepSkipped))
	})
}

func TestPrepareTask_Execute_BackupErrors(t *testing.T) {
	t.Run("ClusterQueue list error fails validation", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		dsc := makeDSCV1("default-dsc", withComponent("kueue", "Managed"))
		target := newTarget(t, []*unstructured.Unstructured{dsc}, targetOpts{
			rbacAllowed: true,
			dynamicReactor: func(act k8stesting.Action) (bool, runtime.Object, error) {
				if act.GetResource().Resource == "clusterqueues" {
					return true, nil, errors.New("forbidden")
				}

				return false, nil, nil
			},
		})

		a := &rhbok.RHBOKMigrationAction{}
		task := a.Prepare()

		res, err := task.Execute(ctx, target)
		g.Expect(err).To(MatchError("preflight checks failed"))

		verifyStep := findStep(res.Status.Steps, "verify-kueue-resources")
		g.Expect(verifyStep).ToNot(BeNil())
		g.Expect(verifyStep.Status).To(Equal(result.StepFailed))
		g.Expect(findStep(res.Status.Steps, "backup-kueue-resources")).To(BeNil())
	})

	t.Run("ConfigMap get error fails backup", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		dsc := makeDSCV1("default-dsc", withComponent("kueue", "Managed"))
		target := newTarget(t, []*unstructured.Unstructured{dsc}, targetOpts{
			rbacAllowed: true,
			dynamicReactor: func(act k8stesting.Action) (bool, runtime.Object, error) {
				if act.GetResource().Resource == "configmaps" && act.GetVerb() == "get" {
					return true, nil, errors.New("forbidden")
				}

				return false, nil, nil
			},
		})

		a := &rhbok.RHBOKMigrationAction{}
		task := a.Prepare()

		res, err := task.Execute(ctx, target)
		g.Expect(err).ToNot(HaveOccurred())

		cmStep := findStepRecursive(res.Status.Steps, "backup-configmap")
		g.Expect(cmStep).ToNot(BeNil())
		g.Expect(cmStep.Status).To(Equal(result.StepFailed))
	})
}

func findStep(steps []result.ActionStep, name string) *result.ActionStep {
	for i := range steps {
		if steps[i].Name == name {
			return &steps[i]
		}
	}

	return nil
}

func findStepRecursive(steps []result.ActionStep, name string) *result.ActionStep {
	for i := range steps {
		if steps[i].Name == name {
			return &steps[i]
		}

		if found := findStepRecursive(steps[i].Children, name); found != nil {
			return found
		}
	}

	return nil
}
