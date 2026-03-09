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
var kueueLabelsPyTorchJobListKinds = map[schema.GroupVersionResource]string{
	resources.PyTorchJob.GVR():         resources.PyTorchJob.ListKind(),
	resources.Namespace.GVR():          resources.Namespace.ListKind(),
	resources.DataScienceCluster.GVR(): resources.DataScienceCluster.ListKind(),
}

func newPyTorchJobNamespace(name string, labels map[string]any) *unstructured.Unstructured {
	metadata := map[string]any{"name": name}

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

func newPyTorchJob(name string, namespace string, labels map[string]any) *unstructured.Unstructured {
	metadata := map[string]any{
		"name":      name,
		"namespace": namespace,
	}

	if len(labels) > 0 {
		metadata["labels"] = labels
	}

	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.PyTorchJob.APIVersion(),
			"kind":       resources.PyTorchJob.Kind,
			"metadata":   metadata,
		},
	}
}

func TestKueueLabelsPyTorchJobCheck_Metadata(t *testing.T) {
	g := NewWithT(t)

	chk := kueue.NewKueueLabelsPyTorchJobCheck()

	g.Expect(chk.ID()).To(Equal("workloads.kueue.pytorchjob-labels"))
	g.Expect(chk.Name()).To(Equal("Workloads :: Kueue :: PyTorchJob Labels"))
	g.Expect(chk.Group()).To(Equal(check.GroupWorkload))
	g.Expect(chk.CheckKind()).To(Equal("kueue"))
	g.Expect(chk.CheckType()).To(Equal(string(check.CheckTypeDataIntegrity)))
	g.Expect(chk.Remediation()).To(ContainSubstring("kueue.x-k8s.io/queue-name"))
}

func TestKueueLabelsPyTorchJobCheck_CanApply_KueueManaged(t *testing.T) {
	g := NewWithT(t)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      kueueLabelsPyTorchJobListKinds,
		Objects:        []*unstructured.Unstructured{testutil.NewDSC(map[string]string{"kueue": "Managed"})},
		CurrentVersion: "3.0.0",
		TargetVersion:  "3.0.0",
	})

	chk := kueue.NewKueueLabelsPyTorchJobCheck()
	canApply, err := chk.CanApply(t.Context(), target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeTrue())
}

func TestKueueLabelsPyTorchJobCheck_CanApply_KueueRemoved(t *testing.T) {
	g := NewWithT(t)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      kueueLabelsPyTorchJobListKinds,
		Objects:        []*unstructured.Unstructured{testutil.NewDSC(map[string]string{"kueue": "Removed"})},
		CurrentVersion: "3.0.0",
		TargetVersion:  "3.0.0",
	})

	chk := kueue.NewKueueLabelsPyTorchJobCheck()
	canApply, err := chk.CanApply(t.Context(), target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeFalse())
}

func TestKueueLabelsPyTorchJobCheck_CanApply_KueueUnmanaged(t *testing.T) {
	g := NewWithT(t)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      kueueLabelsPyTorchJobListKinds,
		Objects:        []*unstructured.Unstructured{testutil.NewDSC(map[string]string{"kueue": "Unmanaged"})},
		CurrentVersion: "3.0.0",
		TargetVersion:  "3.0.0",
	})

	chk := kueue.NewKueueLabelsPyTorchJobCheck()
	canApply, err := chk.CanApply(t.Context(), target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeTrue())
}

func TestKueueLabelsPyTorchJobCheck_NoPyTorchJobs(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      kueueLabelsPyTorchJobListKinds,
		CurrentVersion: "3.0.0",
		TargetVersion:  "3.0.0",
	})

	chk := kueue.NewKueueLabelsPyTorchJobCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(2))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(kueue.ConditionTypePyTorchJobKueueLabels),
		"Status":  Equal(metav1.ConditionTrue),
		"Reason":  Equal(check.ReasonRequirementsMet),
		"Message": Equal(fmt.Sprintf(kueue.MsgNoWorkloads, "PyTorchJob")),
	}))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactNone))
	g.Expect(result.Status.Conditions[1].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(kueue.ConditionTypePyTorchJobKueueMissingLabels),
		"Status":  Equal(metav1.ConditionTrue),
		"Message": Equal(fmt.Sprintf(kueue.MsgNoWorkloadsInKueueNs, "PyTorchJob")),
	}))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "0"))
	g.Expect(result.ImpactedObjects).To(BeEmpty())
}

func TestKueueLabelsPyTorchJobCheck_WithoutQueueLabelInKueueNamespace(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	ns := newPyTorchJobNamespace("kueue-ns", map[string]any{constants.LabelKueueManaged: "true"})
	job := newPyTorchJob("unlabeled-job", "kueue-ns", nil)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      kueueLabelsPyTorchJobListKinds,
		Objects:        []*unstructured.Unstructured{ns, job},
		CurrentVersion: "3.0.0",
		TargetVersion:  "3.0.0",
	})

	chk := kueue.NewKueueLabelsPyTorchJobCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(2))
	g.Expect(result.Status.Conditions[0].Status).To(Equal(metav1.ConditionTrue))
	g.Expect(result.Status.Conditions[0].Message).To(Equal(fmt.Sprintf(kueue.MsgNoLabeledWorkloads, "PyTorchJob")))
	g.Expect(result.Status.Conditions[1].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(kueue.ConditionTypePyTorchJobKueueMissingLabels),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonConfigurationInvalid),
		"Message": Equal(fmt.Sprintf(kueue.MsgMissingLabelInKueueNs, 1, "PyTorchJob")),
	}))
	g.Expect(result.Status.Conditions[1].Impact).To(Equal(resultpkg.ImpactBlocking))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "1"))
	g.Expect(result.ImpactedObjects).To(HaveLen(1))
	g.Expect(result.ImpactedObjects[0].Name).To(Equal("unlabeled-job"))
	g.Expect(result.ImpactedObjects[0].Namespace).To(Equal("kueue-ns"))
}

func TestKueueLabelsPyTorchJobCheck_LabeledInNonKueueNamespace(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	ns := newPyTorchJobNamespace("plain-ns", nil)
	job := newPyTorchJob("bad-job", "plain-ns", map[string]any{constants.LabelKueueQueueName: "default"})

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      kueueLabelsPyTorchJobListKinds,
		Objects:        []*unstructured.Unstructured{ns, job},
		CurrentVersion: "3.0.0",
		TargetVersion:  "3.0.0",
	})

	chk := kueue.NewKueueLabelsPyTorchJobCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions[0].Status).To(Equal(metav1.ConditionFalse))
	g.Expect(result.Status.Conditions[0].Message).To(Equal(fmt.Sprintf(kueue.MsgNsNotKueueEnabled, 1, "PyTorchJob")))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactBlocking))
	g.Expect(result.Status.Conditions[1].Status).To(Equal(metav1.ConditionTrue))
	g.Expect(result.Status.Conditions[1].Message).To(Equal(fmt.Sprintf(kueue.MsgNoWorkloadsInKueueNs, "PyTorchJob")))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "1"))
	g.Expect(result.ImpactedObjects).To(HaveLen(1))
	g.Expect(result.ImpactedObjects[0].Name).To(Equal("bad-job"))
}

func TestKueueLabelsPyTorchJobCheck_WithQueueLabelInKueueNamespace(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	ns := newPyTorchJobNamespace("kueue-ns", map[string]any{constants.LabelKueueManaged: "true"})
	job := newPyTorchJob("good-job", "kueue-ns", map[string]any{constants.LabelKueueQueueName: "default"})

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      kueueLabelsPyTorchJobListKinds,
		Objects:        []*unstructured.Unstructured{ns, job},
		CurrentVersion: "3.0.0",
		TargetVersion:  "3.0.0",
	})

	chk := kueue.NewKueueLabelsPyTorchJobCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions[0].Status).To(Equal(metav1.ConditionTrue))
	g.Expect(result.Status.Conditions[0].Message).To(Equal(fmt.Sprintf(kueue.MsgAllValid, 1, "PyTorchJob")))
	g.Expect(result.Status.Conditions[1].Status).To(Equal(metav1.ConditionTrue))
	g.Expect(result.Status.Conditions[1].Message).To(Equal(fmt.Sprintf(kueue.MsgAllInKueueNsLabeled, 1, "PyTorchJob")))
	g.Expect(result.ImpactedObjects).To(BeEmpty())
}

func TestKueueLabelsPyTorchJobCheck_WithoutQueueLabel(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	ns := newPyTorchJobNamespace("plain-ns", nil)
	job := newPyTorchJob("my-job", "plain-ns", nil)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      kueueLabelsPyTorchJobListKinds,
		Objects:        []*unstructured.Unstructured{ns, job},
		CurrentVersion: "3.0.0",
		TargetVersion:  "3.0.0",
	})

	chk := kueue.NewKueueLabelsPyTorchJobCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions[0].Status).To(Equal(metav1.ConditionTrue))
	g.Expect(result.Status.Conditions[0].Message).To(Equal(fmt.Sprintf(kueue.MsgNoLabeledWorkloads, "PyTorchJob")))
	g.Expect(result.Status.Conditions[1].Status).To(Equal(metav1.ConditionTrue))
	g.Expect(result.Status.Conditions[1].Message).To(Equal(fmt.Sprintf(kueue.MsgNoWorkloadsInKueueNs, "PyTorchJob")))
	g.Expect(result.ImpactedObjects).To(BeEmpty())
}
