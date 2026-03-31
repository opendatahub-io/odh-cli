package notebook_test

import (
	"bytes"
	"fmt"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/opendatahub-io/odh-cli/pkg/constants"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	resultpkg "github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/testutil"
	"github.com/opendatahub-io/odh-cli/pkg/lint/checks/workloads/notebook"
	"github.com/opendatahub-io/odh-cli/pkg/resources"

	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

//nolint:gochecknoglobals
var nonStoppedWorkloadsListKinds = map[schema.GroupVersionResource]string{
	resources.Notebook.GVR():           resources.Notebook.ListKind(),
	resources.DSCInitialization.GVR():  resources.DSCInitialization.ListKind(),
	resources.DataScienceCluster.GVR(): resources.DataScienceCluster.ListKind(),
}

func TestNonStoppedWorkloadsCheck_Metadata(t *testing.T) {
	g := NewWithT(t)

	chk := notebook.NewNonStoppedWorkloadsCheck()

	g.Expect(chk.ID()).To(Equal("workloads.notebook.non-stopped-workloads"))
	g.Expect(chk.Name()).To(Equal("Workloads :: Notebook :: Non-Stopped Workloads"))
	g.Expect(chk.Group()).To(Equal(check.GroupWorkload))
	g.Expect(chk.CheckKind()).To(Equal("notebook"))
	g.Expect(chk.CheckType()).To(Equal(string(check.CheckTypeWorkloadState)))
	g.Expect(chk.Description()).ToNot(BeEmpty())
	g.Expect(chk.Remediation()).To(ContainSubstring("Save all pending work"))
}

func TestNonStoppedWorkloadsCheck_CanApply_NilVersions(t *testing.T) {
	g := NewWithT(t)

	chk := notebook.NewNonStoppedWorkloadsCheck()
	canApply, err := chk.CanApply(t.Context(), check.Target{})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeFalse())
}

func TestNonStoppedWorkloadsCheck_CanApply_SameVersion(t *testing.T) {
	g := NewWithT(t)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      nonStoppedWorkloadsListKinds,
		CurrentVersion: "2.17.0",
		TargetVersion:  "2.17.0",
	})

	chk := notebook.NewNonStoppedWorkloadsCheck()
	canApply, err := chk.CanApply(t.Context(), target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeFalse())
}

func TestNonStoppedWorkloadsCheck_CanApply_UpgradeTo3x(t *testing.T) {
	g := NewWithT(t)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      nonStoppedWorkloadsListKinds,
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := notebook.NewNonStoppedWorkloadsCheck()
	canApply, err := chk.CanApply(t.Context(), target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeTrue())
}

func TestNonStoppedWorkloadsCheck_Validate_SkipWhenDSCMissing(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      nonStoppedWorkloadsListKinds,
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := notebook.NewNonStoppedWorkloadsCheck()
	result, err := chk.Validate(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).To(BeNil())
}

func TestNonStoppedWorkloadsCheck_Validate_SkipWhenWorkbenchesRemoved(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      nonStoppedWorkloadsListKinds,
		Objects:        []*unstructured.Unstructured{workbenchesDSC(constants.ManagementStateRemoved)},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := notebook.NewNonStoppedWorkloadsCheck()
	result, err := chk.Validate(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).To(BeNil())
}

func TestNonStoppedWorkloadsCheck_NoNotebooks(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      nonStoppedWorkloadsListKinds,
		Objects:        []*unstructured.Unstructured{workbenchesDSC(constants.ManagementStateManaged)},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := notebook.NewNonStoppedWorkloadsCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(notebook.ConditionTypeNonStoppedWorkloads),
		"Status":  Equal(metav1.ConditionTrue),
		"Reason":  Equal(check.ReasonRequirementsMet),
		"Message": Equal(notebook.MsgAllNotebooksStopped),
	}))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactNone))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "0"))
	g.Expect(result.ImpactedObjects).To(BeEmpty())
}

func TestNonStoppedWorkloadsCheck_AllStopped(t *testing.T) {
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
		ListKinds:      nonStoppedWorkloadsListKinds,
		Objects:        []*unstructured.Unstructured{workbenchesDSC(constants.ManagementStateManaged), nb1, nb2},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := notebook.NewNonStoppedWorkloadsCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(notebook.ConditionTypeNonStoppedWorkloads),
		"Status": Equal(metav1.ConditionTrue),
		"Reason": Equal(check.ReasonRequirementsMet),
	}))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactNone))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "0"))
	g.Expect(result.ImpactedObjects).To(BeEmpty())
}

func TestNonStoppedWorkloadsCheck_OneRunning(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	nbRunning := newNotebook("running-notebook", "user-ns", notebookOptions{
		Status: map[string]any{
			"containerState": map[string]any{
				"running": map[string]any{
					"startedAt": "2026-03-25T17:40:38Z",
				},
			},
			"readyReplicas": int64(1),
		},
	})

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      nonStoppedWorkloadsListKinds,
		Objects:        []*unstructured.Unstructured{workbenchesDSC(constants.ManagementStateManaged), nbRunning},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := notebook.NewNonStoppedWorkloadsCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(notebook.ConditionTypeNonStoppedWorkloads),
		"Status": Equal(metav1.ConditionFalse),
		"Reason": Equal(check.ReasonWorkloadsImpacted),
	}))
	g.Expect(result.Status.Conditions[0].Message).To(ContainSubstring("1 non-stopped"))
	g.Expect(result.Status.Conditions[0].Message).To(ContainSubstring("1 running"))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactAdvisory))
	g.Expect(result.Status.Conditions[0].Remediation).To(ContainSubstring("Save all pending work"))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "1"))
	g.Expect(result.ImpactedObjects).To(HaveLen(1))
	g.Expect(result.ImpactedObjects[0].Name).To(Equal("running-notebook"))
	g.Expect(result.ImpactedObjects[0].Namespace).To(Equal("user-ns"))
	g.Expect(result.ImpactedObjects[0].Annotations).To(HaveKeyWithValue(
		notebook.AnnotationCheckContainerState, notebook.ContainerStateRunning))
}

func TestNonStoppedWorkloadsCheck_WaitingCrashLoopBackOff(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	nbCrash := newNotebook("crash-notebook", "user-ns", notebookOptions{
		Status: map[string]any{
			"containerState": map[string]any{
				"waiting": map[string]any{
					"reason":  "CrashLoopBackOff",
					"message": "back-off 10s restarting failed container",
				},
			},
			"readyReplicas": int64(0),
		},
	})

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      nonStoppedWorkloadsListKinds,
		Objects:        []*unstructured.Unstructured{workbenchesDSC(constants.ManagementStateManaged), nbCrash},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := notebook.NewNonStoppedWorkloadsCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Message).To(ContainSubstring("1 non-stopped"))
	g.Expect(result.Status.Conditions[0].Message).To(ContainSubstring("1 waiting"))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactAdvisory))
	g.Expect(result.ImpactedObjects).To(HaveLen(1))
	g.Expect(result.ImpactedObjects[0].Annotations).To(HaveKeyWithValue(
		notebook.AnnotationCheckContainerState, notebook.ContainerStateWaiting))
	g.Expect(result.ImpactedObjects[0].Annotations).To(HaveKeyWithValue(
		notebook.AnnotationCheckContainerWaitReason, "CrashLoopBackOff"))
}

func TestNonStoppedWorkloadsCheck_WaitingImagePullBackOff(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	nbBadImage := newNotebook("bad-image-notebook", "user-ns", notebookOptions{
		Status: map[string]any{
			"containerState": map[string]any{
				"waiting": map[string]any{
					"reason":  "ImagePullBackOff",
					"message": "Back-off pulling image",
				},
			},
			"readyReplicas": int64(0),
		},
	})

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      nonStoppedWorkloadsListKinds,
		Objects:        []*unstructured.Unstructured{workbenchesDSC(constants.ManagementStateManaged), nbBadImage},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := notebook.NewNonStoppedWorkloadsCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions[0].Message).To(ContainSubstring("1 waiting"))
	g.Expect(result.ImpactedObjects[0].Annotations).To(HaveKeyWithValue(
		notebook.AnnotationCheckContainerWaitReason, "ImagePullBackOff"))
}

func TestNonStoppedWorkloadsCheck_MixedRunningAndWaiting(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	nbStopped := newNotebook("stopped-notebook", "ns1", notebookOptions{
		Annotations: map[string]any{
			notebook.AnnotationKubeflowResourceStopped: "2026-02-20T10:30:00Z",
		},
	})

	nbRunning := newNotebook("running-notebook", "ns2", notebookOptions{
		Status: map[string]any{
			"containerState": map[string]any{
				"running": map[string]any{
					"startedAt": "2026-03-25T17:40:38Z",
				},
			},
			"readyReplicas": int64(1),
		},
	})

	nbCrash := newNotebook("crash-notebook", "ns3", notebookOptions{
		Status: map[string]any{
			"containerState": map[string]any{
				"waiting": map[string]any{
					"reason":  "CrashLoopBackOff",
					"message": "back-off restarting failed container",
				},
			},
			"readyReplicas": int64(0),
		},
	})

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      nonStoppedWorkloadsListKinds,
		Objects:        []*unstructured.Unstructured{workbenchesDSC(constants.ManagementStateManaged), nbStopped, nbRunning, nbCrash},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := notebook.NewNonStoppedWorkloadsCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(notebook.ConditionTypeNonStoppedWorkloads),
		"Status": Equal(metav1.ConditionFalse),
		"Reason": Equal(check.ReasonWorkloadsImpacted),
	}))
	g.Expect(result.Status.Conditions[0].Message).To(ContainSubstring("2 non-stopped"))
	g.Expect(result.Status.Conditions[0].Message).To(ContainSubstring("1 running"))
	g.Expect(result.Status.Conditions[0].Message).To(ContainSubstring("1 waiting"))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactAdvisory))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "2"))
	g.Expect(result.ImpactedObjects).To(HaveLen(2))
}

func TestNonStoppedWorkloadsCheck_EmptyContainerState(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	// Notebook with no containerState (pod not yet created)
	nbNoState := newNotebook("no-state-notebook", "user-ns", notebookOptions{
		Status: map[string]any{
			"containerState": map[string]any{},
			"readyReplicas":  int64(0),
		},
	})

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      nonStoppedWorkloadsListKinds,
		Objects:        []*unstructured.Unstructured{workbenchesDSC(constants.ManagementStateManaged), nbNoState},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := notebook.NewNonStoppedWorkloadsCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions[0].Message).To(ContainSubstring("1 waiting"))
	g.Expect(result.ImpactedObjects[0].Annotations).To(HaveKeyWithValue(
		notebook.AnnotationCheckContainerState, notebook.ContainerStateWaiting))
}

func TestNonStoppedWorkloadsCheck_AllRunning(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	nb1 := newNotebook("notebook-1", "ns1", notebookOptions{
		Status: map[string]any{
			"containerState": map[string]any{
				"running": map[string]any{"startedAt": "2026-03-25T17:40:38Z"},
			},
			"readyReplicas": int64(1),
		},
	})
	nb2 := newNotebook("notebook-2", "ns2", notebookOptions{
		Status: map[string]any{
			"containerState": map[string]any{
				"running": map[string]any{"startedAt": "2026-03-25T18:00:00Z"},
			},
			"readyReplicas": int64(1),
		},
	})
	nb3 := newNotebook("notebook-3", "ns3", notebookOptions{
		Status: map[string]any{
			"containerState": map[string]any{
				"running": map[string]any{"startedAt": "2026-03-25T19:00:00Z"},
			},
			"readyReplicas": int64(1),
		},
	})

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      nonStoppedWorkloadsListKinds,
		Objects:        []*unstructured.Unstructured{workbenchesDSC(constants.ManagementStateManaged), nb1, nb2, nb3},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := notebook.NewNonStoppedWorkloadsCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(notebook.ConditionTypeNonStoppedWorkloads),
		"Status": Equal(metav1.ConditionFalse),
		"Reason": Equal(check.ReasonWorkloadsImpacted),
	}))
	g.Expect(result.Status.Conditions[0].Message).To(ContainSubstring(fmt.Sprintf(notebook.MsgNonStoppedNotebooksFound, 3)))
	g.Expect(result.Status.Conditions[0].Message).To(ContainSubstring("3 running"))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactAdvisory))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "3"))
	g.Expect(result.ImpactedObjects).To(HaveLen(3))
}

func TestNonStoppedWorkloadsCheck_FormatVerboseOutput_MixedStates(t *testing.T) {
	g := NewWithT(t)

	dr := &resultpkg.DiagnosticResult{
		Annotations: map[string]string{
			resultpkg.AnnotationResourceCRDName: "notebooks.kubeflow.org",
		},
		ImpactedObjects: []metav1.PartialObjectMetadata{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: "nb-running-1", Namespace: "ns1",
					Annotations: map[string]string{
						notebook.AnnotationCheckContainerState: notebook.ContainerStateRunning,
					},
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: "nb-running-2", Namespace: "ns1",
					Annotations: map[string]string{
						notebook.AnnotationCheckContainerState: notebook.ContainerStateRunning,
					},
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: "nb-crash", Namespace: "ns2",
					Annotations: map[string]string{
						notebook.AnnotationCheckContainerState:      notebook.ContainerStateWaiting,
						notebook.AnnotationCheckContainerWaitReason: "CrashLoopBackOff",
					},
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: "nb-bad-image", Namespace: "ns3",
					Annotations: map[string]string{
						notebook.AnnotationCheckContainerState:      notebook.ContainerStateWaiting,
						notebook.AnnotationCheckContainerWaitReason: "ImagePullBackOff",
					},
				},
			},
		},
	}

	var buf bytes.Buffer
	chk := notebook.NewNonStoppedWorkloadsCheck()
	chk.FormatVerboseOutput(&buf, dr)
	output := buf.String()

	g.Expect(output).To(ContainSubstring("running (2 notebooks):"))
	g.Expect(output).To(ContainSubstring("namespace: ns1"))
	g.Expect(output).To(ContainSubstring("notebooks.kubeflow.org/nb-running-1"))
	g.Expect(output).To(ContainSubstring("notebooks.kubeflow.org/nb-running-2"))
	g.Expect(output).To(ContainSubstring("waiting (2 notebooks):"))
	g.Expect(output).To(ContainSubstring("CrashLoopBackOff (1):"))
	g.Expect(output).To(ContainSubstring("notebooks.kubeflow.org/nb-crash"))
	g.Expect(output).To(ContainSubstring("ImagePullBackOff (1):"))
	g.Expect(output).To(ContainSubstring("notebooks.kubeflow.org/nb-bad-image"))
}
