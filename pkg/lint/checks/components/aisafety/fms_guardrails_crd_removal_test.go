package aisafety_test

import (
	"testing"

	"github.com/blang/semver/v4"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	resultpkg "github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/testutil"
	"github.com/opendatahub-io/odh-cli/pkg/lint/checks/components/aisafety"
	"github.com/opendatahub-io/odh-cli/pkg/resources"

	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

const guardrailsCRDName = "guardrailsorchestrators.trustyai.opendatahub.io"

func listKinds() map[schema.GroupVersionResource]string {
	return map[schema.GroupVersionResource]string{
		resources.CustomResourceDefinition.GVR(): resources.CustomResourceDefinition.ListKind(),
		resources.GuardrailsOrchestrator.GVR():   resources.GuardrailsOrchestrator.ListKind(),
	}
}

func newGuardrailsCRD() *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.CustomResourceDefinition.APIVersion(),
			"kind":       resources.CustomResourceDefinition.Kind,
			"metadata": map[string]any{
				"name": guardrailsCRDName,
			},
		},
	}
}

func newGuardrailsOrchestratorCR(namespace, name string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.GuardrailsOrchestrator.APIVersion(),
			"kind":       resources.GuardrailsOrchestrator.Kind,
			"metadata": map[string]any{
				"name":      name,
				"namespace": namespace,
			},
		},
	}
}

func TestFMSGuardrailsCRDRemoval_CRDNotFound(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds(),
		CurrentVersion: "3.5.0",
		TargetVersion:  "3.6.0",
	})

	chk := aisafety.NewFMSGuardrailsCRDRemovalCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(check.ConditionTypeCompatible),
		"Status": Equal(metav1.ConditionTrue),
		"Reason": Equal(check.ReasonRequirementsMet),
	}))
}

func TestFMSGuardrailsCRDRemoval_CRDFoundNoCRs(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds(),
		Objects:        []*unstructured.Unstructured{newGuardrailsCRD()},
		CurrentVersion: "3.5.0",
		TargetVersion:  "3.6.0",
	})

	chk := aisafety.NewFMSGuardrailsCRDRemovalCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(check.ConditionTypeCompatible),
		"Status": Equal(metav1.ConditionFalse),
		"Reason": Equal(check.ReasonFeatureRemoved),
	}))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactBlocking))
	g.Expect(result.Status.Conditions[0].Message).To(ContainSubstring(guardrailsCRDName))
	g.Expect(result.Status.Conditions[0].Remediation).To(ContainSubstring("oc delete crd"))
	g.Expect(result.Status.Conditions[0].Remediation).ToNot(ContainSubstring("--all"))
	g.Expect(result.ImpactedObjects).To(BeEmpty())
}

func TestFMSGuardrailsCRDRemoval_CRDFoundWithCRs(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	cr1 := newGuardrailsOrchestratorCR("ns-a", "guardrails-1")
	cr2 := newGuardrailsOrchestratorCR("ns-b", "guardrails-2")

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds(),
		Objects:        []*unstructured.Unstructured{newGuardrailsCRD(), cr1, cr2},
		CurrentVersion: "3.5.0",
		TargetVersion:  "3.6.0",
	})

	chk := aisafety.NewFMSGuardrailsCRDRemovalCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(check.ConditionTypeCompatible),
		"Status": Equal(metav1.ConditionFalse),
		"Reason": Equal(check.ReasonFeatureRemoved),
	}))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactBlocking))
	g.Expect(result.Status.Conditions[0].Message).To(ContainSubstring("2 GuardrailsOrchestrator CR(s)"))
	g.Expect(result.Status.Conditions[0].Remediation).To(ContainSubstring("--all -A"))
	g.Expect(result.Status.Conditions[0].Remediation).To(ContainSubstring("oc delete crd"))
	g.Expect(result.ImpactedObjects).To(HaveLen(2))
}

func TestFMSGuardrailsCRDRemoval_CRDFoundWithCRs_2xTo36(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	cr := newGuardrailsOrchestratorCR("default", "my-guardrails")

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds(),
		Objects:        []*unstructured.Unstructured{newGuardrailsCRD(), cr},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.6.0",
	})

	chk := aisafety.NewFMSGuardrailsCRDRemovalCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(check.ConditionTypeCompatible),
		"Status": Equal(metav1.ConditionFalse),
		"Reason": Equal(check.ReasonFeatureRemoved),
	}))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactBlocking))
	g.Expect(result.ImpactedObjects).To(HaveLen(1))
}

func TestFMSGuardrailsCRDRemoval_CanApply(t *testing.T) {
	tests := []struct {
		name     string
		from     string
		to       string
		expected bool
	}{
		{
			name:     "3.5 to 3.6 applies",
			from:     "3.5.0",
			to:       "3.6.0",
			expected: true,
		},
		{
			name:     "3.4 to 3.6 applies",
			from:     "3.4.0",
			to:       "3.6.0",
			expected: true,
		},
		{
			name:     "2.17 to 3.6 applies",
			from:     "2.17.0",
			to:       "3.6.0",
			expected: true,
		},
		{
			name:     "3.5 to 3.7 applies",
			from:     "3.5.0",
			to:       "3.7.0",
			expected: true,
		},
		{
			name:     "3.6 to 3.7 does not apply",
			from:     "3.6.0",
			to:       "3.7.0",
			expected: false,
		},
		{
			name:     "3.5 to 3.5 does not apply",
			from:     "3.5.0",
			to:       "3.5.0",
			expected: false,
		},
		{
			name:     "3.3 to 3.5 does not apply",
			from:     "3.3.0",
			to:       "3.5.0",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			chk := aisafety.NewFMSGuardrailsCRDRemovalCheck()

			from := semver.MustParse(tt.from)
			to := semver.MustParse(tt.to)
			target := check.Target{
				CurrentVersion: &from,
				TargetVersion:  &to,
			}

			canApply, err := chk.CanApply(t.Context(), target)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(canApply).To(Equal(tt.expected))
		})
	}
}

func TestFMSGuardrailsCRDRemoval_CanApply_NilVersions(t *testing.T) {
	g := NewWithT(t)

	chk := aisafety.NewFMSGuardrailsCRDRemovalCheck()

	canApply, err := chk.CanApply(t.Context(), check.Target{})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeFalse())
}

func TestFMSGuardrailsCRDRemoval_Metadata(t *testing.T) {
	g := NewWithT(t)

	chk := aisafety.NewFMSGuardrailsCRDRemovalCheck()

	g.Expect(chk.ID()).To(Equal("components.aisafety.fms-guardrails-crd-removal"))
	g.Expect(chk.Name()).To(Equal("Components :: AI Safety :: FMS Guardrails CRD Removal (3.6)"))
	g.Expect(chk.Group()).To(Equal(check.GroupComponent))
	g.Expect(chk.Description()).ToNot(BeEmpty())
}
