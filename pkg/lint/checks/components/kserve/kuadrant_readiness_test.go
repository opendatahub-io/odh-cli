package kserve_test

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	resultpkg "github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/testutil"
	"github.com/opendatahub-io/odh-cli/pkg/lint/checks/components/kserve"
	"github.com/opendatahub-io/odh-cli/pkg/resources"

	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

func newKuadrant(readyStatus string) *unstructured.Unstructured {
	conditions := []any{}
	if readyStatus != "" {
		conditions = append(conditions, map[string]any{
			"type":   "Ready",
			"status": readyStatus,
			"reason": "Ready",
		})
	}

	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.Kuadrant.APIVersion(),
			"kind":       resources.Kuadrant.Kind,
			"metadata": map[string]any{
				"name":      "kuadrant",
				"namespace": "kuadrant-system",
			},
			"spec": map[string]any{},
			"status": map[string]any{
				"conditions": conditions,
			},
		},
	}
}

func newLLMInferenceService() *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.LLMInferenceService.APIVersion(),
			"kind":       resources.LLMInferenceService.Kind,
			"metadata": map[string]any{
				"name":      "test-llm",
				"namespace": "test-ns",
			},
		},
	}
}

func llmdListKinds() map[schema.GroupVersionResource]string {
	return map[schema.GroupVersionResource]string{
		resources.LLMInferenceService.GVR(): resources.LLMInferenceService.ListKind(),
		resources.Kuadrant.GVR():            resources.Kuadrant.ListKind(),
	}
}

func TestKuadrantReadinessCheck_Ready(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds: llmdListKinds(),
		Objects: []*unstructured.Unstructured{
			newLLMInferenceService(),
			newKuadrant("True"),
		},
	})

	c := kserve.NewKuadrantReadinessCheck()
	result, err := c.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeReady),
		"Status":  Equal(metav1.ConditionTrue),
		"Reason":  Equal(check.ReasonResourceAvailable),
		"Message": ContainSubstring("Kuadrant is installed and ready"),
	}))
}

func TestKuadrantReadinessCheck_NotReady(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds: llmdListKinds(),
		Objects: []*unstructured.Unstructured{
			newLLMInferenceService(),
			newKuadrant("False"),
		},
	})

	c := kserve.NewKuadrantReadinessCheck()
	result, err := c.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeReady),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonResourceUnavailable),
		"Message": ContainSubstring("not ready"),
	}))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactBlocking))
}

func TestKuadrantReadinessCheck_NotFound(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds: llmdListKinds(),
		Objects: []*unstructured.Unstructured{
			newLLMInferenceService(),
		},
	})

	c := kserve.NewKuadrantReadinessCheck()
	result, err := c.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeReady),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonResourceNotFound),
		"Message": ContainSubstring("Kuadrant resource not found"),
	}))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactBlocking))
}

func TestKuadrantReadinessCheck_MissingReadyCondition(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds: llmdListKinds(),
		Objects: []*unstructured.Unstructured{
			newLLMInferenceService(),
			newKuadrant(""),
		},
	})

	c := kserve.NewKuadrantReadinessCheck()
	result, err := c.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeReady),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonInsufficientData),
		"Message": ContainSubstring("Ready condition status is empty"),
	}))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactBlocking))
}

func TestKuadrantReadinessCheck_CanApply_WithLLMD(t *testing.T) {
	g := NewWithT(t)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		CurrentVersion: "2.25.0",
		TargetVersion:  "3.0.0",
		ListKinds: map[schema.GroupVersionResource]string{
			resources.LLMInferenceService.GVR(): resources.LLMInferenceService.ListKind(),
		},
		Objects: []*unstructured.Unstructured{
			newLLMInferenceService(),
		},
	})

	c := kserve.NewKuadrantReadinessCheck()
	canApply, err := c.CanApply(t.Context(), target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeTrue())
}

func TestKuadrantReadinessCheck_CanApply_WithoutLLMD(t *testing.T) {
	g := NewWithT(t)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		CurrentVersion: "2.25.0",
		TargetVersion:  "3.0.0",
		ListKinds: map[schema.GroupVersionResource]string{
			resources.LLMInferenceService.GVR(): resources.LLMInferenceService.ListKind(),
		},
		Objects: nil,
	})

	c := kserve.NewKuadrantReadinessCheck()
	canApply, err := c.CanApply(t.Context(), target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeFalse())
}

func TestKuadrantReadinessCheck_CanApply_NotUpgrade2xTo3x(t *testing.T) {
	g := NewWithT(t)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		CurrentVersion: "2.24.0",
		TargetVersion:  "2.25.0",
		ListKinds: map[schema.GroupVersionResource]string{
			resources.LLMInferenceService.GVR(): resources.LLMInferenceService.ListKind(),
		},
		Objects: []*unstructured.Unstructured{
			newLLMInferenceService(),
		},
	})

	c := kserve.NewKuadrantReadinessCheck()
	canApply, err := c.CanApply(t.Context(), target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeFalse())
}

func TestKuadrantReadinessCheck_CanApply_NotUpgrade3xTo4x(t *testing.T) {
	g := NewWithT(t)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		CurrentVersion: "3.0.0",
		TargetVersion:  "4.0.0",
		ListKinds: map[schema.GroupVersionResource]string{
			resources.LLMInferenceService.GVR(): resources.LLMInferenceService.ListKind(),
		},
		Objects: []*unstructured.Unstructured{
			newLLMInferenceService(),
		},
	})

	c := kserve.NewKuadrantReadinessCheck()
	canApply, err := c.CanApply(t.Context(), target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeFalse())
}

func TestKuadrantReadinessCheck_Metadata(t *testing.T) {
	g := NewWithT(t)

	c := kserve.NewKuadrantReadinessCheck()

	g.Expect(c.ID()).To(Equal("components.kserve.kuadrant-readiness"))
	g.Expect(c.Name()).To(Equal("Components :: KServe :: Kuadrant Readiness"))
	g.Expect(c.Group()).To(Equal(check.GroupComponent))
	g.Expect(c.Description()).ToNot(BeEmpty())
}
