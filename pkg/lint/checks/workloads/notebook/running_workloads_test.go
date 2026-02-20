package notebook_test

import (
	"fmt"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	resultpkg "github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/testutil"
	"github.com/opendatahub-io/odh-cli/pkg/lint/checks/workloads/notebook"
	"github.com/opendatahub-io/odh-cli/pkg/resources"

	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

//nolint:gochecknoglobals
var runningWorkloadsListKinds = map[schema.GroupVersionResource]string{
	resources.Notebook.GVR():           resources.Notebook.ListKind(),
	resources.DSCInitialization.GVR():  resources.DSCInitialization.ListKind(),
	resources.DataScienceCluster.GVR(): resources.DataScienceCluster.ListKind(),
}

func TestRunningWorkloadsCheck_Metadata(t *testing.T) {
	g := NewWithT(t)

	chk := notebook.NewRunningWorkloadsCheck()

	g.Expect(chk.ID()).To(Equal("workloads.notebook.running-workloads"))
	g.Expect(chk.Name()).To(Equal("Workloads :: Notebook :: Running Workloads"))
	g.Expect(chk.Group()).To(Equal(check.GroupWorkload))
	g.Expect(chk.CheckKind()).To(Equal("notebook"))
	g.Expect(chk.CheckType()).To(Equal(string(check.CheckTypeWorkloadState)))
	g.Expect(chk.Description()).ToNot(BeEmpty())
	g.Expect(chk.Remediation()).To(ContainSubstring("Save all pending work"))
}

func TestRunningWorkloadsCheck_CanApply_NilVersions(t *testing.T) {
	g := NewWithT(t)

	chk := notebook.NewRunningWorkloadsCheck()
	canApply, err := chk.CanApply(t.Context(), check.Target{})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeFalse())
}

func TestRunningWorkloadsCheck_CanApply_LintMode2x(t *testing.T) {
	g := NewWithT(t)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      runningWorkloadsListKinds,
		Objects:        []*unstructured.Unstructured{testutil.NewDSC(map[string]string{"workbenches": "Managed"})},
		CurrentVersion: "2.17.0",
		TargetVersion:  "2.17.0",
	})

	chk := notebook.NewRunningWorkloadsCheck()
	canApply, err := chk.CanApply(t.Context(), target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeFalse())
}

func TestRunningWorkloadsCheck_CanApply_UpgradeTo3x_Managed(t *testing.T) {
	g := NewWithT(t)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      runningWorkloadsListKinds,
		Objects:        []*unstructured.Unstructured{testutil.NewDSC(map[string]string{"workbenches": "Managed"})},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := notebook.NewRunningWorkloadsCheck()
	canApply, err := chk.CanApply(t.Context(), target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeTrue())
}

func TestRunningWorkloadsCheck_CanApply_UpgradeTo3x_Removed(t *testing.T) {
	g := NewWithT(t)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      runningWorkloadsListKinds,
		Objects:        []*unstructured.Unstructured{testutil.NewDSC(map[string]string{"workbenches": "Removed"})},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := notebook.NewRunningWorkloadsCheck()
	canApply, err := chk.CanApply(t.Context(), target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeFalse())
}

func TestRunningWorkloadsCheck_CanApply_LintMode3x(t *testing.T) {
	g := NewWithT(t)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      runningWorkloadsListKinds,
		Objects:        []*unstructured.Unstructured{testutil.NewDSC(map[string]string{"workbenches": "Managed"})},
		CurrentVersion: "3.0.0",
		TargetVersion:  "3.0.0",
	})

	chk := notebook.NewRunningWorkloadsCheck()
	canApply, err := chk.CanApply(t.Context(), target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeFalse())
}

func TestRunningWorkloadsCheck_NoNotebooks(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      runningWorkloadsListKinds,
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := notebook.NewRunningWorkloadsCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(notebook.ConditionTypeRunningWorkloads),
		"Status":  Equal(metav1.ConditionTrue),
		"Reason":  Equal(check.ReasonRequirementsMet),
		"Message": Equal(notebook.MsgAllNotebooksStopped),
	}))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactNone))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "0"))
	g.Expect(result.ImpactedObjects).To(BeEmpty())
}

func TestRunningWorkloadsCheck_AllStopped(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	nb1 := newNotebook("stopped-notebook-1", "ns1", notebookOptions{
		Annotations: map[string]any{
			notebook.AnnotationKubeflowResourceStopped: "2026-02-20T10:30:00Z",
		},
	})

	nb2 := newNotebook("stopped-notebook-2", "ns2", notebookOptions{
		Annotations: map[string]any{
			notebook.AnnotationKubeflowResourceStopped: "2026-01-15T08:00:00Z",
		},
	})

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      runningWorkloadsListKinds,
		Objects:        []*unstructured.Unstructured{nb1, nb2},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := notebook.NewRunningWorkloadsCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(notebook.ConditionTypeRunningWorkloads),
		"Status": Equal(metav1.ConditionTrue),
		"Reason": Equal(check.ReasonRequirementsMet),
	}))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactNone))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "0"))
	g.Expect(result.ImpactedObjects).To(BeEmpty())
}

func TestRunningWorkloadsCheck_OneRunning(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	// Running notebook (no kubeflow-resource-stopped annotation)
	nbRunning := newNotebook("running-notebook", "user-ns", notebookOptions{})

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      runningWorkloadsListKinds,
		Objects:        []*unstructured.Unstructured{nbRunning},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := notebook.NewRunningWorkloadsCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(notebook.ConditionTypeRunningWorkloads),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonWorkloadsImpacted),
		"Message": Equal(fmt.Sprintf(notebook.MsgRunningNotebooksFound, 1)),
	}))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactAdvisory))
	g.Expect(result.Status.Conditions[0].Remediation).To(ContainSubstring("Save all pending work"))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "1"))
	g.Expect(result.ImpactedObjects).To(HaveLen(1))
	g.Expect(result.ImpactedObjects[0].Name).To(Equal("running-notebook"))
	g.Expect(result.ImpactedObjects[0].Namespace).To(Equal("user-ns"))
}

func TestRunningWorkloadsCheck_MixedRunningAndStopped(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	// Stopped notebook
	nbStopped := newNotebook("stopped-notebook", "ns1", notebookOptions{
		Annotations: map[string]any{
			notebook.AnnotationKubeflowResourceStopped: "2026-02-20T10:30:00Z",
		},
	})

	// Running notebook (no annotations at all)
	nbRunning1 := newNotebook("running-notebook-1", "ns2", notebookOptions{})

	// Running notebook (has other annotations but not kubeflow-resource-stopped)
	nbRunning2 := newNotebook("running-notebook-2", "ns3", notebookOptions{
		Annotations: map[string]any{
			"opendatahub.io/some-other-annotation": "value",
		},
	})

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      runningWorkloadsListKinds,
		Objects:        []*unstructured.Unstructured{nbStopped, nbRunning1, nbRunning2},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := notebook.NewRunningWorkloadsCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(notebook.ConditionTypeRunningWorkloads),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonWorkloadsImpacted),
		"Message": Equal(fmt.Sprintf(notebook.MsgRunningNotebooksFound, 2)),
	}))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactAdvisory))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "2"))
	g.Expect(result.ImpactedObjects).To(HaveLen(2))
}

func TestRunningWorkloadsCheck_AllRunning(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	nb1 := newNotebook("notebook-1", "ns1", notebookOptions{})
	nb2 := newNotebook("notebook-2", "ns2", notebookOptions{})
	nb3 := newNotebook("notebook-3", "ns3", notebookOptions{})

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      runningWorkloadsListKinds,
		Objects:        []*unstructured.Unstructured{nb1, nb2, nb3},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := notebook.NewRunningWorkloadsCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(notebook.ConditionTypeRunningWorkloads),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonWorkloadsImpacted),
		"Message": Equal(fmt.Sprintf(notebook.MsgRunningNotebooksFound, 3)),
	}))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactAdvisory))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "3"))
	g.Expect(result.ImpactedObjects).To(HaveLen(3))
}
