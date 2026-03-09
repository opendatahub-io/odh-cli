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
var kueueLabelsISVCListKinds = map[schema.GroupVersionResource]string{
	resources.InferenceService.GVR():   resources.InferenceService.ListKind(),
	resources.Namespace.GVR():          resources.Namespace.ListKind(),
	resources.DataScienceCluster.GVR(): resources.DataScienceCluster.ListKind(),
}

func newKueueNamespace(name string, labels map[string]any) *unstructured.Unstructured {
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

func newISVC(name string, namespace string, labels map[string]any) *unstructured.Unstructured {
	metadata := map[string]any{
		"name":      name,
		"namespace": namespace,
	}

	if len(labels) > 0 {
		metadata["labels"] = labels
	}

	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.InferenceService.APIVersion(),
			"kind":       resources.InferenceService.Kind,
			"metadata":   metadata,
		},
	}
}

func TestKueueLabelsISVCCheck_Metadata(t *testing.T) {
	g := NewWithT(t)

	chk := kueue.NewKueueLabelsISVCCheck()

	g.Expect(chk.ID()).To(Equal("workloads.kueue.inferenceservice-labels"))
	g.Expect(chk.Name()).To(Equal("Workloads :: Kueue :: InferenceService Labels"))
	g.Expect(chk.Group()).To(Equal(check.GroupWorkload))
	g.Expect(chk.CheckKind()).To(Equal("kueue"))
	g.Expect(chk.CheckType()).To(Equal(string(check.CheckTypeDataIntegrity)))
	g.Expect(chk.Description()).ToNot(BeEmpty())
	g.Expect(chk.Remediation()).To(ContainSubstring("kueue.x-k8s.io/queue-name"))
}

func TestKueueLabelsISVCCheck_CanApply_KueueManaged(t *testing.T) {
	g := NewWithT(t)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      kueueLabelsISVCListKinds,
		Objects:        []*unstructured.Unstructured{testutil.NewDSC(map[string]string{"kueue": "Managed"})},
		CurrentVersion: "3.0.0",
		TargetVersion:  "3.0.0",
	})

	chk := kueue.NewKueueLabelsISVCCheck()
	canApply, err := chk.CanApply(t.Context(), target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeTrue())
}

func TestKueueLabelsISVCCheck_CanApply_KueueUnmanaged(t *testing.T) {
	g := NewWithT(t)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      kueueLabelsISVCListKinds,
		Objects:        []*unstructured.Unstructured{testutil.NewDSC(map[string]string{"kueue": "Unmanaged"})},
		CurrentVersion: "3.0.0",
		TargetVersion:  "3.0.0",
	})

	chk := kueue.NewKueueLabelsISVCCheck()
	canApply, err := chk.CanApply(t.Context(), target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeTrue())
}

func TestKueueLabelsISVCCheck_CanApply_KueueRemoved(t *testing.T) {
	g := NewWithT(t)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      kueueLabelsISVCListKinds,
		Objects:        []*unstructured.Unstructured{testutil.NewDSC(map[string]string{"kueue": "Removed"})},
		CurrentVersion: "3.0.0",
		TargetVersion:  "3.0.0",
	})

	chk := kueue.NewKueueLabelsISVCCheck()
	canApply, err := chk.CanApply(t.Context(), target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeFalse())
}

func TestKueueLabelsISVCCheck_NoInferenceServices(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      kueueLabelsISVCListKinds,
		CurrentVersion: "3.0.0",
		TargetVersion:  "3.0.0",
	})

	chk := kueue.NewKueueLabelsISVCCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(2))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(kueue.ConditionTypeISVCKueueLabels),
		"Status":  Equal(metav1.ConditionTrue),
		"Reason":  Equal(check.ReasonRequirementsMet),
		"Message": Equal(fmt.Sprintf(kueue.MsgNoWorkloads, "InferenceService")),
	}))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactNone))
	g.Expect(result.Status.Conditions[1].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(kueue.ConditionTypeISVCKueueMissingLabels),
		"Status":  Equal(metav1.ConditionTrue),
		"Message": Equal(fmt.Sprintf(kueue.MsgNoWorkloadsInKueueNs, "InferenceService")),
	}))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "0"))
	g.Expect(result.ImpactedObjects).To(BeEmpty())
}

func TestKueueLabelsISVCCheck_WithoutQueueLabel(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	ns := newKueueNamespace("user-ns", nil)
	isvc := newISVC("my-isvc", "user-ns", nil)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      kueueLabelsISVCListKinds,
		Objects:        []*unstructured.Unstructured{ns, isvc},
		CurrentVersion: "3.0.0",
		TargetVersion:  "3.0.0",
	})

	chk := kueue.NewKueueLabelsISVCCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions[0].Status).To(Equal(metav1.ConditionTrue))
	g.Expect(result.Status.Conditions[0].Message).To(Equal(fmt.Sprintf(kueue.MsgNoLabeledWorkloads, "InferenceService")))
	g.Expect(result.Status.Conditions[1].Status).To(Equal(metav1.ConditionTrue))
	g.Expect(result.Status.Conditions[1].Message).To(Equal(fmt.Sprintf(kueue.MsgNoWorkloadsInKueueNs, "InferenceService")))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "0"))
	g.Expect(result.ImpactedObjects).To(BeEmpty())
}

func TestKueueLabelsISVCCheck_WithQueueLabelInKueueNamespace(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	ns := newKueueNamespace("kueue-ns", map[string]any{
		constants.LabelKueueManaged: "true",
	})

	isvc := newISVC("good-isvc", "kueue-ns", map[string]any{
		constants.LabelKueueQueueName: "default",
	})

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      kueueLabelsISVCListKinds,
		Objects:        []*unstructured.Unstructured{ns, isvc},
		CurrentVersion: "3.0.0",
		TargetVersion:  "3.0.0",
	})

	chk := kueue.NewKueueLabelsISVCCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions[0].Status).To(Equal(metav1.ConditionTrue))
	g.Expect(result.Status.Conditions[0].Message).To(Equal(fmt.Sprintf(kueue.MsgAllValid, 1, "InferenceService")))
	g.Expect(result.Status.Conditions[1].Status).To(Equal(metav1.ConditionTrue))
	g.Expect(result.Status.Conditions[1].Message).To(Equal(fmt.Sprintf(kueue.MsgAllInKueueNsLabeled, 1, "InferenceService")))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "0"))
	g.Expect(result.ImpactedObjects).To(BeEmpty())
}

func TestKueueLabelsISVCCheck_WithoutQueueLabelInKueueNamespace(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	ns := newKueueNamespace("kueue-ns", map[string]any{
		constants.LabelKueueManaged: "true",
	})

	isvc := newISVC("unlabeled-isvc", "kueue-ns", nil)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      kueueLabelsISVCListKinds,
		Objects:        []*unstructured.Unstructured{ns, isvc},
		CurrentVersion: "3.0.0",
		TargetVersion:  "3.0.0",
	})

	chk := kueue.NewKueueLabelsISVCCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(2))
	g.Expect(result.Status.Conditions[0].Status).To(Equal(metav1.ConditionTrue))
	g.Expect(result.Status.Conditions[0].Message).To(Equal(fmt.Sprintf(kueue.MsgNoLabeledWorkloads, "InferenceService")))
	g.Expect(result.Status.Conditions[1].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(kueue.ConditionTypeISVCKueueMissingLabels),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonConfigurationInvalid),
		"Message": Equal(fmt.Sprintf(kueue.MsgMissingLabelInKueueNs, 1, "InferenceService")),
	}))
	g.Expect(result.Status.Conditions[1].Impact).To(Equal(resultpkg.ImpactBlocking))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "1"))
	g.Expect(result.ImpactedObjects).To(HaveLen(1))
	g.Expect(result.ImpactedObjects[0].Name).To(Equal("unlabeled-isvc"))
	g.Expect(result.ImpactedObjects[0].Namespace).To(Equal("kueue-ns"))
}

func TestKueueLabelsISVCCheck_LabeledInNonKueueNamespace(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	ns := newKueueNamespace("plain-ns", nil)

	isvc := newISVC("bad-isvc", "plain-ns", map[string]any{
		constants.LabelKueueQueueName: "default",
	})

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      kueueLabelsISVCListKinds,
		Objects:        []*unstructured.Unstructured{ns, isvc},
		CurrentVersion: "3.0.0",
		TargetVersion:  "3.0.0",
	})

	chk := kueue.NewKueueLabelsISVCCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(2))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(kueue.ConditionTypeISVCKueueLabels),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonConfigurationInvalid),
		"Message": Equal(fmt.Sprintf(kueue.MsgNsNotKueueEnabled, 1, "InferenceService")),
	}))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactBlocking))
	g.Expect(result.Status.Conditions[0].Remediation).To(ContainSubstring("kueue.x-k8s.io/queue-name"))
	g.Expect(result.Status.Conditions[1].Status).To(Equal(metav1.ConditionTrue))
	g.Expect(result.Status.Conditions[1].Message).To(Equal(fmt.Sprintf(kueue.MsgNoWorkloadsInKueueNs, "InferenceService")))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "1"))
	g.Expect(result.ImpactedObjects).To(HaveLen(1))
	g.Expect(result.ImpactedObjects[0].Name).To(Equal("bad-isvc"))
	g.Expect(result.ImpactedObjects[0].Namespace).To(Equal("plain-ns"))
}

func TestKueueLabelsISVCCheck_MixedLabeledInferenceServices(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	nsKueue := newKueueNamespace("kueue-ns", map[string]any{
		constants.LabelKueueManaged: "true",
	})
	nsPlain := newKueueNamespace("plain-ns", nil)

	isvcGood := newISVC("good-isvc", "kueue-ns", map[string]any{
		constants.LabelKueueQueueName: "default",
	})
	isvcBad := newISVC("bad-isvc", "plain-ns", map[string]any{
		constants.LabelKueueQueueName: "default",
	})
	isvcPlain := newISVC("plain-isvc", "plain-ns", nil)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      kueueLabelsISVCListKinds,
		Objects:        []*unstructured.Unstructured{nsKueue, nsPlain, isvcGood, isvcBad, isvcPlain},
		CurrentVersion: "3.0.0",
		TargetVersion:  "3.0.0",
	})

	chk := kueue.NewKueueLabelsISVCCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions[0].Status).To(Equal(metav1.ConditionFalse))
	g.Expect(result.Status.Conditions[0].Message).To(Equal(fmt.Sprintf(kueue.MsgNsNotKueueEnabled, 1, "InferenceService")))
	g.Expect(result.Status.Conditions[1].Status).To(Equal(metav1.ConditionTrue))
	g.Expect(result.Status.Conditions[1].Message).To(Equal(fmt.Sprintf(kueue.MsgAllInKueueNsLabeled, 1, "InferenceService")))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "1"))
	g.Expect(result.ImpactedObjects).To(HaveLen(1))
	g.Expect(result.ImpactedObjects[0].Name).To(Equal("bad-isvc"))
}

func TestKueueLabelsISVCCheck_OpenshiftKueueLabel(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	ns := newKueueNamespace("ocp-kueue-ns", map[string]any{
		constants.LabelKueueOpenshiftManaged: "true",
	})

	isvc := newISVC("good-isvc", "ocp-kueue-ns", map[string]any{
		constants.LabelKueueQueueName: "default",
	})

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      kueueLabelsISVCListKinds,
		Objects:        []*unstructured.Unstructured{ns, isvc},
		CurrentVersion: "3.0.0",
		TargetVersion:  "3.0.0",
	})

	chk := kueue.NewKueueLabelsISVCCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions[0].Status).To(Equal(metav1.ConditionTrue))
	g.Expect(result.Status.Conditions[0].Message).To(Equal(fmt.Sprintf(kueue.MsgAllValid, 1, "InferenceService")))
	g.Expect(result.Status.Conditions[1].Status).To(Equal(metav1.ConditionTrue))
	g.Expect(result.Status.Conditions[1].Message).To(Equal(fmt.Sprintf(kueue.MsgAllInKueueNsLabeled, 1, "InferenceService")))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "0"))
	g.Expect(result.ImpactedObjects).To(BeEmpty())
}

func TestKueueLabelsISVCCheck_CustomQueueName(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	ns := newKueueNamespace("kueue-ns", map[string]any{
		constants.LabelKueueManaged: "true",
	})

	isvc := newISVC("custom-queue-isvc", "kueue-ns", map[string]any{
		constants.LabelKueueQueueName: "team-queue",
	})

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      kueueLabelsISVCListKinds,
		Objects:        []*unstructured.Unstructured{ns, isvc},
		CurrentVersion: "3.0.0",
		TargetVersion:  "3.0.0",
	})

	chk := kueue.NewKueueLabelsISVCCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions[0].Status).To(Equal(metav1.ConditionTrue))
	g.Expect(result.Status.Conditions[0].Message).To(Equal(fmt.Sprintf(kueue.MsgAllValid, 1, "InferenceService")))
	g.Expect(result.Status.Conditions[1].Status).To(Equal(metav1.ConditionTrue))
	g.Expect(result.Status.Conditions[1].Message).To(Equal(fmt.Sprintf(kueue.MsgAllInKueueNsLabeled, 1, "InferenceService")))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "0"))
	g.Expect(result.ImpactedObjects).To(BeEmpty())
}

func TestKueueLabelsISVCCheck_AnnotationTargetVersion(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      kueueLabelsISVCListKinds,
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := kueue.NewKueueLabelsISVCCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationCheckTargetVersion, "3.0.0"))
}
