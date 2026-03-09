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
var kueueLabelsLLMListKinds = map[schema.GroupVersionResource]string{
	resources.LLMInferenceService.GVR(): resources.LLMInferenceService.ListKind(),
	resources.Namespace.GVR():           resources.Namespace.ListKind(),
	resources.DataScienceCluster.GVR():  resources.DataScienceCluster.ListKind(),
}

func newLLMISVC(name string, namespace string, labels map[string]any) *unstructured.Unstructured {
	metadata := map[string]any{
		"name":      name,
		"namespace": namespace,
	}

	if len(labels) > 0 {
		metadata["labels"] = labels
	}

	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.LLMInferenceService.APIVersion(),
			"kind":       resources.LLMInferenceService.Kind,
			"metadata":   metadata,
		},
	}
}

func TestKueueLabelsLLMCheck_Metadata(t *testing.T) {
	g := NewWithT(t)

	chk := kueue.NewKueueLabelsLLMCheck()

	g.Expect(chk.ID()).To(Equal("workloads.kueue.llminferenceservice-labels"))
	g.Expect(chk.Name()).To(Equal("Workloads :: Kueue :: LLMInferenceService Labels"))
	g.Expect(chk.Group()).To(Equal(check.GroupWorkload))
	g.Expect(chk.CheckKind()).To(Equal("kueue"))
	g.Expect(chk.CheckType()).To(Equal(string(check.CheckTypeDataIntegrity)))
	g.Expect(chk.Description()).ToNot(BeEmpty())
	g.Expect(chk.Remediation()).To(ContainSubstring("kueue.x-k8s.io/queue-name"))
}

func TestKueueLabelsLLMCheck_CanApply_KueueManaged(t *testing.T) {
	g := NewWithT(t)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      kueueLabelsLLMListKinds,
		Objects:        []*unstructured.Unstructured{testutil.NewDSC(map[string]string{"kueue": "Managed"})},
		CurrentVersion: "3.0.0",
		TargetVersion:  "3.0.0",
	})

	chk := kueue.NewKueueLabelsLLMCheck()
	canApply, err := chk.CanApply(t.Context(), target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeTrue())
}

func TestKueueLabelsLLMCheck_CanApply_KueueRemoved(t *testing.T) {
	g := NewWithT(t)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      kueueLabelsLLMListKinds,
		Objects:        []*unstructured.Unstructured{testutil.NewDSC(map[string]string{"kueue": "Removed"})},
		CurrentVersion: "3.0.0",
		TargetVersion:  "3.0.0",
	})

	chk := kueue.NewKueueLabelsLLMCheck()
	canApply, err := chk.CanApply(t.Context(), target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeFalse())
}

func TestKueueLabelsLLMCheck_CanApply_KueueUnmanaged(t *testing.T) {
	g := NewWithT(t)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      kueueLabelsLLMListKinds,
		Objects:        []*unstructured.Unstructured{testutil.NewDSC(map[string]string{"kueue": "Unmanaged"})},
		CurrentVersion: "3.0.0",
		TargetVersion:  "3.0.0",
	})

	chk := kueue.NewKueueLabelsLLMCheck()
	canApply, err := chk.CanApply(t.Context(), target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeTrue())
}

func TestKueueLabelsLLMCheck_NoLLMInferenceServices(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      kueueLabelsLLMListKinds,
		CurrentVersion: "3.0.0",
		TargetVersion:  "3.0.0",
	})

	chk := kueue.NewKueueLabelsLLMCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(2))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(kueue.ConditionTypeLLMISVCKueueLabels),
		"Status":  Equal(metav1.ConditionTrue),
		"Reason":  Equal(check.ReasonRequirementsMet),
		"Message": Equal(fmt.Sprintf(kueue.MsgNoWorkloads, "LLMInferenceService")),
	}))
	g.Expect(result.Status.Conditions[1].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(kueue.ConditionTypeLLMISVCKueueMissingLabels),
		"Status":  Equal(metav1.ConditionTrue),
		"Message": Equal(fmt.Sprintf(kueue.MsgNoWorkloadsInKueueNs, "LLMInferenceService")),
	}))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactNone))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "0"))
	g.Expect(result.ImpactedObjects).To(BeEmpty())
}

func TestKueueLabelsLLMCheck_WithoutQueueLabelInKueueNamespace(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	ns := newKueueNamespace("kueue-ns", map[string]any{
		constants.LabelKueueManaged: "true",
	})

	llm := newLLMISVC("unlabeled-llm", "kueue-ns", nil)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      kueueLabelsLLMListKinds,
		Objects:        []*unstructured.Unstructured{ns, llm},
		CurrentVersion: "3.0.0",
		TargetVersion:  "3.0.0",
	})

	chk := kueue.NewKueueLabelsLLMCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(2))
	g.Expect(result.Status.Conditions[0].Status).To(Equal(metav1.ConditionTrue))
	g.Expect(result.Status.Conditions[0].Message).To(Equal(fmt.Sprintf(kueue.MsgNoLabeledWorkloads, "LLMInferenceService")))
	g.Expect(result.Status.Conditions[1].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(kueue.ConditionTypeLLMISVCKueueMissingLabels),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonConfigurationInvalid),
		"Message": Equal(fmt.Sprintf(kueue.MsgMissingLabelInKueueNs, 1, "LLMInferenceService")),
	}))
	g.Expect(result.Status.Conditions[1].Impact).To(Equal(resultpkg.ImpactBlocking))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "1"))
	g.Expect(result.ImpactedObjects).To(HaveLen(1))
	g.Expect(result.ImpactedObjects[0].Name).To(Equal("unlabeled-llm"))
	g.Expect(result.ImpactedObjects[0].Namespace).To(Equal("kueue-ns"))
}

func TestKueueLabelsLLMCheck_LabeledInNonKueueNamespace(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	ns := newKueueNamespace("plain-ns", nil)

	llm := newLLMISVC("bad-llm", "plain-ns", map[string]any{
		constants.LabelKueueQueueName: "default",
	})

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      kueueLabelsLLMListKinds,
		Objects:        []*unstructured.Unstructured{ns, llm},
		CurrentVersion: "3.0.0",
		TargetVersion:  "3.0.0",
	})

	chk := kueue.NewKueueLabelsLLMCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(2))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(kueue.ConditionTypeLLMISVCKueueLabels),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonConfigurationInvalid),
		"Message": Equal(fmt.Sprintf(kueue.MsgNsNotKueueEnabled, 1, "LLMInferenceService")),
	}))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactBlocking))
	g.Expect(result.Status.Conditions[0].Remediation).To(ContainSubstring("kueue.x-k8s.io/queue-name"))
	g.Expect(result.Status.Conditions[1].Status).To(Equal(metav1.ConditionTrue))
	g.Expect(result.Status.Conditions[1].Message).To(Equal(fmt.Sprintf(kueue.MsgNoWorkloadsInKueueNs, "LLMInferenceService")))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "1"))
	g.Expect(result.ImpactedObjects).To(HaveLen(1))
	g.Expect(result.ImpactedObjects[0].Name).To(Equal("bad-llm"))
	g.Expect(result.ImpactedObjects[0].Namespace).To(Equal("plain-ns"))
}

func TestKueueLabelsLLMCheck_WithQueueLabelInKueueNamespace(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	ns := newKueueNamespace("kueue-ns", map[string]any{
		constants.LabelKueueManaged: "true",
	})

	llm := newLLMISVC("good-llm", "kueue-ns", map[string]any{
		constants.LabelKueueQueueName: "default",
	})

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      kueueLabelsLLMListKinds,
		Objects:        []*unstructured.Unstructured{ns, llm},
		CurrentVersion: "3.0.0",
		TargetVersion:  "3.0.0",
	})

	chk := kueue.NewKueueLabelsLLMCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions[0].Status).To(Equal(metav1.ConditionTrue))
	g.Expect(result.Status.Conditions[0].Message).To(Equal(fmt.Sprintf(kueue.MsgAllValid, 1, "LLMInferenceService")))
	g.Expect(result.Status.Conditions[1].Status).To(Equal(metav1.ConditionTrue))
	g.Expect(result.Status.Conditions[1].Message).To(Equal(fmt.Sprintf(kueue.MsgAllInKueueNsLabeled, 1, "LLMInferenceService")))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "0"))
	g.Expect(result.ImpactedObjects).To(BeEmpty())
}

func TestKueueLabelsLLMCheck_MixedLabeledLLMInferenceServices(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	nsKueue := newKueueNamespace("kueue-ns", map[string]any{
		constants.LabelKueueManaged: "true",
	})
	nsPlain := newKueueNamespace("plain-ns", nil)

	llmGood := newLLMISVC("good-llm", "kueue-ns", map[string]any{
		constants.LabelKueueQueueName: "default",
	})
	llmBad := newLLMISVC("bad-llm", "plain-ns", map[string]any{
		constants.LabelKueueQueueName: "default",
	})
	llmPlain := newLLMISVC("plain-llm", "plain-ns", nil)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      kueueLabelsLLMListKinds,
		Objects:        []*unstructured.Unstructured{nsKueue, nsPlain, llmGood, llmBad, llmPlain},
		CurrentVersion: "3.0.0",
		TargetVersion:  "3.0.0",
	})

	chk := kueue.NewKueueLabelsLLMCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions[0].Status).To(Equal(metav1.ConditionFalse))
	g.Expect(result.Status.Conditions[0].Message).To(Equal(fmt.Sprintf(kueue.MsgNsNotKueueEnabled, 1, "LLMInferenceService")))
	g.Expect(result.Status.Conditions[1].Status).To(Equal(metav1.ConditionTrue))
	g.Expect(result.Status.Conditions[1].Message).To(Equal(fmt.Sprintf(kueue.MsgAllInKueueNsLabeled, 1, "LLMInferenceService")))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "1"))
	g.Expect(result.ImpactedObjects).To(HaveLen(1))
	g.Expect(result.ImpactedObjects[0].Name).To(Equal("bad-llm"))
}

func TestKueueLabelsLLMCheck_OpenshiftKueueLabel(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	ns := newKueueNamespace("ocp-kueue-ns", map[string]any{
		constants.LabelKueueOpenshiftManaged: "true",
	})

	llm := newLLMISVC("good-llm", "ocp-kueue-ns", map[string]any{
		constants.LabelKueueQueueName: "default",
	})

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      kueueLabelsLLMListKinds,
		Objects:        []*unstructured.Unstructured{ns, llm},
		CurrentVersion: "3.0.0",
		TargetVersion:  "3.0.0",
	})

	chk := kueue.NewKueueLabelsLLMCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions[0].Status).To(Equal(metav1.ConditionTrue))
	g.Expect(result.Status.Conditions[0].Message).To(Equal(fmt.Sprintf(kueue.MsgAllValid, 1, "LLMInferenceService")))
	g.Expect(result.Status.Conditions[1].Status).To(Equal(metav1.ConditionTrue))
	g.Expect(result.Status.Conditions[1].Message).To(Equal(fmt.Sprintf(kueue.MsgAllInKueueNsLabeled, 1, "LLMInferenceService")))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "0"))
	g.Expect(result.ImpactedObjects).To(BeEmpty())
}
