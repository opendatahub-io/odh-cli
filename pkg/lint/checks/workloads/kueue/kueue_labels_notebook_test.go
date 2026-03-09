package kueue_test

import (
	"fmt"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/opendatahub-io/odh-cli/pkg/constants"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	resultpkg "github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/testutil"
	"github.com/opendatahub-io/odh-cli/pkg/lint/checks/workloads/kueue"
	"github.com/opendatahub-io/odh-cli/pkg/resources"

	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

//nolint:gochecknoglobals
var kueueLabelsNotebookListKinds = map[schema.GroupVersionResource]string{
	resources.Notebook.GVR():           resources.Notebook.ListKind(),
	resources.Namespace.GVR():          resources.Namespace.ListKind(),
	resources.DataScienceCluster.GVR(): resources.DataScienceCluster.ListKind(),
}

func newNamespace(name string, labels map[string]any) *unstructured.Unstructured {
	metadata := map[string]any{
		"name": name,
	}

	if len(labels) > 0 {
		metadata["labels"] = labels
	}

	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.Namespace.APIVersion(),
			"kind":       resources.Namespace.Kind,
			"metadata":   metadata,
		},
	}
}

func newNotebook(name string, namespace string, labels map[string]any) *unstructured.Unstructured {
	metadata := map[string]any{
		"name":      name,
		"namespace": namespace,
	}

	if len(labels) > 0 {
		metadata["labels"] = labels
	}

	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.Notebook.APIVersion(),
			"kind":       resources.Notebook.Kind,
			"metadata":   metadata,
		},
	}
}

func TestKueueLabelsNotebookCheck_Metadata(t *testing.T) {
	g := NewWithT(t)

	chk := kueue.NewKueueLabelsNotebookCheck()

	g.Expect(chk.ID()).To(Equal("workloads.kueue.notebook-labels"))
	g.Expect(chk.Name()).To(Equal("Workloads :: Kueue :: Notebook Labels"))
	g.Expect(chk.Group()).To(Equal(check.GroupWorkload))
	g.Expect(chk.CheckKind()).To(Equal("kueue"))
	g.Expect(chk.CheckType()).To(Equal(string(check.CheckTypeDataIntegrity)))
	g.Expect(chk.Description()).ToNot(BeEmpty())
	g.Expect(chk.Remediation()).To(ContainSubstring("kueue.x-k8s.io/queue-name"))
}

func TestKueueLabelsNotebookCheck_CanApply_KueueManaged(t *testing.T) {
	g := NewWithT(t)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      kueueLabelsNotebookListKinds,
		Objects:        []*unstructured.Unstructured{testutil.NewDSC(map[string]string{"kueue": "Managed"})},
		CurrentVersion: "3.0.0",
		TargetVersion:  "3.0.0",
	})

	chk := kueue.NewKueueLabelsNotebookCheck()
	canApply, err := chk.CanApply(t.Context(), target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeTrue())
}

func TestKueueLabelsNotebookCheck_CanApply_KueueUnmanaged(t *testing.T) {
	g := NewWithT(t)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      kueueLabelsNotebookListKinds,
		Objects:        []*unstructured.Unstructured{testutil.NewDSC(map[string]string{"kueue": "Unmanaged"})},
		CurrentVersion: "3.0.0",
		TargetVersion:  "3.0.0",
	})

	chk := kueue.NewKueueLabelsNotebookCheck()
	canApply, err := chk.CanApply(t.Context(), target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeTrue())
}

func TestKueueLabelsNotebookCheck_CanApply_KueueRemoved(t *testing.T) {
	g := NewWithT(t)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      kueueLabelsNotebookListKinds,
		Objects:        []*unstructured.Unstructured{testutil.NewDSC(map[string]string{"kueue": "Removed"})},
		CurrentVersion: "3.0.0",
		TargetVersion:  "3.0.0",
	})

	chk := kueue.NewKueueLabelsNotebookCheck()
	canApply, err := chk.CanApply(t.Context(), target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeFalse())
}

func TestKueueLabelsNotebookCheck_NoNotebooks(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      kueueLabelsNotebookListKinds,
		Objects:        []*unstructured.Unstructured{testutil.NewDSC(map[string]string{"kueue": "Managed"})},
		CurrentVersion: "3.0.0",
		TargetVersion:  "3.0.0",
	})

	chk := kueue.NewKueueLabelsNotebookCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(2))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(kueue.ConditionTypeNotebookKueueLabels),
		"Status":  Equal(metav1.ConditionTrue),
		"Reason":  Equal(check.ReasonRequirementsMet),
		"Message": Equal(fmt.Sprintf(kueue.MsgNoWorkloads, "Notebook")),
	}))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactNone))
	g.Expect(result.Status.Conditions[1].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(kueue.ConditionTypeNotebookKueueMissingLabels),
		"Status":  Equal(metav1.ConditionTrue),
		"Message": Equal(fmt.Sprintf(kueue.MsgNoWorkloadsInKueueNs, "Notebook")),
	}))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "0"))
	g.Expect(result.ImpactedObjects).To(BeEmpty())
}

func TestKueueLabelsNotebookCheck_NotebookWithoutQueueLabel(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	ns := newNamespace("user-ns", nil)
	nb := newNotebook("my-notebook", "user-ns", nil)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      kueueLabelsNotebookListKinds,
		Objects:        []*unstructured.Unstructured{testutil.NewDSC(map[string]string{"kueue": "Managed"}), ns, nb},
		CurrentVersion: "3.0.0",
		TargetVersion:  "3.0.0",
	})

	chk := kueue.NewKueueLabelsNotebookCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions[0].Status).To(Equal(metav1.ConditionTrue))
	g.Expect(result.Status.Conditions[0].Message).To(Equal(fmt.Sprintf(kueue.MsgNoLabeledWorkloads, "Notebook")))
	g.Expect(result.Status.Conditions[1].Status).To(Equal(metav1.ConditionTrue))
	g.Expect(result.Status.Conditions[1].Message).To(Equal(fmt.Sprintf(kueue.MsgNoWorkloadsInKueueNs, "Notebook")))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "0"))
	g.Expect(result.ImpactedObjects).To(BeEmpty())
}

func TestKueueLabelsNotebookCheck_NotebookWithQueueLabelInKueueNamespace(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	ns := newNamespace("kueue-ns", map[string]any{
		constants.LabelKueueManaged: "true",
	})

	nb := newNotebook("good-notebook", "kueue-ns", map[string]any{
		constants.LabelKueueQueueName: "default",
	})

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      kueueLabelsNotebookListKinds,
		Objects:        []*unstructured.Unstructured{testutil.NewDSC(map[string]string{"kueue": "Managed"}), ns, nb},
		CurrentVersion: "3.0.0",
		TargetVersion:  "3.0.0",
	})

	chk := kueue.NewKueueLabelsNotebookCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions[0].Status).To(Equal(metav1.ConditionTrue))
	g.Expect(result.Status.Conditions[0].Message).To(Equal(fmt.Sprintf(kueue.MsgAllValid, 1, "Notebook")))
	g.Expect(result.Status.Conditions[1].Status).To(Equal(metav1.ConditionTrue))
	g.Expect(result.Status.Conditions[1].Message).To(Equal(fmt.Sprintf(kueue.MsgAllInKueueNsLabeled, 1, "Notebook")))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "0"))
	g.Expect(result.ImpactedObjects).To(BeEmpty())
}

func TestKueueLabelsNotebookCheck_NotebookWithoutQueueLabelInKueueNamespace(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	ns := newNamespace("kueue-ns", map[string]any{
		constants.LabelKueueManaged: "true",
	})

	nb := newNotebook("unlabeled-notebook", "kueue-ns", nil)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      kueueLabelsNotebookListKinds,
		Objects:        []*unstructured.Unstructured{testutil.NewDSC(map[string]string{"kueue": "Managed"}), ns, nb},
		CurrentVersion: "3.0.0",
		TargetVersion:  "3.0.0",
	})

	chk := kueue.NewKueueLabelsNotebookCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(2))
	g.Expect(result.Status.Conditions[0].Status).To(Equal(metav1.ConditionTrue))
	g.Expect(result.Status.Conditions[0].Message).To(Equal(fmt.Sprintf(kueue.MsgNoLabeledWorkloads, "Notebook")))
	g.Expect(result.Status.Conditions[1].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(kueue.ConditionTypeNotebookKueueMissingLabels),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonConfigurationInvalid),
		"Message": Equal(fmt.Sprintf(kueue.MsgMissingLabelInKueueNs, 1, "Notebook")),
	}))
	g.Expect(result.Status.Conditions[1].Impact).To(Equal(resultpkg.ImpactBlocking))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "1"))
	g.Expect(result.ImpactedObjects).To(HaveLen(1))
	g.Expect(result.ImpactedObjects[0].Name).To(Equal("unlabeled-notebook"))
	g.Expect(result.ImpactedObjects[0].Namespace).To(Equal("kueue-ns"))
}

func TestKueueLabelsNotebookCheck_LabeledNotebookInNonKueueNamespace(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	ns := newNamespace("plain-ns", nil)

	nb := newNotebook("bad-notebook", "plain-ns", map[string]any{
		constants.LabelKueueQueueName: "default",
	})

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      kueueLabelsNotebookListKinds,
		Objects:        []*unstructured.Unstructured{ns, nb},
		CurrentVersion: "3.0.0",
		TargetVersion:  "3.0.0",
	})

	chk := kueue.NewKueueLabelsNotebookCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(2))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(kueue.ConditionTypeNotebookKueueLabels),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonConfigurationInvalid),
		"Message": Equal(fmt.Sprintf(kueue.MsgNsNotKueueEnabled, 1, "Notebook")),
	}))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactBlocking))
	g.Expect(result.Status.Conditions[0].Remediation).To(ContainSubstring("kueue.x-k8s.io/queue-name"))
	g.Expect(result.Status.Conditions[1].Status).To(Equal(metav1.ConditionTrue))
	g.Expect(result.Status.Conditions[1].Message).To(Equal(fmt.Sprintf(kueue.MsgNoWorkloadsInKueueNs, "Notebook")))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "1"))
	g.Expect(result.ImpactedObjects).To(HaveLen(1))
	g.Expect(result.ImpactedObjects[0].Name).To(Equal("bad-notebook"))
	g.Expect(result.ImpactedObjects[0].Namespace).To(Equal("plain-ns"))
}

func TestKueueLabelsNotebookCheck_MixedLabeledNotebooks(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	nsKueue := newNamespace("kueue-ns", map[string]any{
		constants.LabelKueueManaged: "true",
	})
	nsPlain := newNamespace("plain-ns", nil)

	nbGood := newNotebook("good-notebook", "kueue-ns", map[string]any{
		constants.LabelKueueQueueName: "default",
	})
	nbBad := newNotebook("bad-notebook", "plain-ns", map[string]any{
		constants.LabelKueueQueueName: "default",
	})
	nbPlain := newNotebook("plain-notebook", "plain-ns", nil)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      kueueLabelsNotebookListKinds,
		Objects:        []*unstructured.Unstructured{nsKueue, nsPlain, nbGood, nbBad, nbPlain},
		CurrentVersion: "3.0.0",
		TargetVersion:  "3.0.0",
	})

	chk := kueue.NewKueueLabelsNotebookCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions[0].Status).To(Equal(metav1.ConditionFalse))
	g.Expect(result.Status.Conditions[0].Message).To(Equal(fmt.Sprintf(kueue.MsgNsNotKueueEnabled, 1, "Notebook")))
	g.Expect(result.Status.Conditions[1].Status).To(Equal(metav1.ConditionTrue))
	g.Expect(result.Status.Conditions[1].Message).To(Equal(fmt.Sprintf(kueue.MsgAllInKueueNsLabeled, 1, "Notebook")))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "1"))
	g.Expect(result.ImpactedObjects).To(HaveLen(1))
	g.Expect(result.ImpactedObjects[0].Name).To(Equal("bad-notebook"))
}

func TestKueueLabelsNotebookCheck_OpenshiftKueueNamespaceLabel(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	ns := newNamespace("ocp-kueue-ns", map[string]any{
		constants.LabelKueueOpenshiftManaged: "true",
	})

	nb := newNotebook("good-notebook", "ocp-kueue-ns", map[string]any{
		constants.LabelKueueQueueName: "default",
	})

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      kueueLabelsNotebookListKinds,
		Objects:        []*unstructured.Unstructured{testutil.NewDSC(map[string]string{"kueue": "Managed"}), ns, nb},
		CurrentVersion: "3.0.0",
		TargetVersion:  "3.0.0",
	})

	chk := kueue.NewKueueLabelsNotebookCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions[0].Status).To(Equal(metav1.ConditionTrue))
	g.Expect(result.Status.Conditions[0].Message).To(Equal(fmt.Sprintf(kueue.MsgAllValid, 1, "Notebook")))
	g.Expect(result.Status.Conditions[1].Status).To(Equal(metav1.ConditionTrue))
	g.Expect(result.Status.Conditions[1].Message).To(Equal(fmt.Sprintf(kueue.MsgAllInKueueNsLabeled, 1, "Notebook")))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "0"))
	g.Expect(result.ImpactedObjects).To(BeEmpty())
}

func TestKueueLabelsNotebookCheck_NotebookWithCustomQueueName(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	ns := newNamespace("kueue-ns", map[string]any{
		constants.LabelKueueManaged: "true",
	})

	nb := newNotebook("custom-queue-notebook", "kueue-ns", map[string]any{
		constants.LabelKueueQueueName: "team-queue",
	})

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      kueueLabelsNotebookListKinds,
		Objects:        []*unstructured.Unstructured{testutil.NewDSC(map[string]string{"kueue": "Managed"}), ns, nb},
		CurrentVersion: "3.0.0",
		TargetVersion:  "3.0.0",
	})

	chk := kueue.NewKueueLabelsNotebookCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions[0].Status).To(Equal(metav1.ConditionTrue))
	g.Expect(result.Status.Conditions[0].Message).To(Equal(fmt.Sprintf(kueue.MsgAllValid, 1, "Notebook")))
	g.Expect(result.Status.Conditions[1].Status).To(Equal(metav1.ConditionTrue))
	g.Expect(result.Status.Conditions[1].Message).To(Equal(fmt.Sprintf(kueue.MsgAllInKueueNsLabeled, 1, "Notebook")))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "0"))
	g.Expect(result.ImpactedObjects).To(BeEmpty())
}

func TestKueueLabelsNotebookCheck_AnnotationTargetVersion(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      kueueLabelsNotebookListKinds,
		Objects:        []*unstructured.Unstructured{testutil.NewDSC(map[string]string{"kueue": "Managed"})},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := kueue.NewKueueLabelsNotebookCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationCheckTargetVersion, "3.0.0"))
}
