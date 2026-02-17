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

func newAuthorino(
	tlsEnabled bool,
	certSecretName string,
	readyStatus string,
) *unstructured.Unstructured {
	tls := map[string]any{
		"enabled": tlsEnabled,
	}
	if certSecretName != "" {
		tls["certSecretRef"] = map[string]any{
			"name": certSecretName,
		}
	}

	conditions := []any{}
	if readyStatus != "" {
		conditions = append(conditions, map[string]any{
			"type":   "Ready",
			"status": readyStatus,
			"reason": "Provisioned",
		})
	}

	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.Authorino.APIVersion(),
			"kind":       resources.Authorino.Kind,
			"metadata": map[string]any{
				"name":      "authorino",
				"namespace": "kuadrant-system",
			},
			"spec": map[string]any{
				"clusterWide": true,
				"listener": map[string]any{
					"tls": tls,
				},
			},
			"status": map[string]any{
				"conditions": conditions,
			},
		},
	}
}

func authorinoListKinds() map[schema.GroupVersionResource]string {
	return map[schema.GroupVersionResource]string{
		resources.LLMInferenceService.GVR(): resources.LLMInferenceService.ListKind(),
		resources.Authorino.GVR():           resources.Authorino.ListKind(),
	}
}

func TestAuthorinoTLSReadinessCheck_FullyConfiguredAndReady(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds: authorinoListKinds(),
		Objects: []*unstructured.Unstructured{
			newLLMInferenceService(),
			newAuthorino(true, "authorino-server-cert", "True"),
		},
	})

	c := kserve.NewAuthorinoTLSReadinessCheck()
	result, err := c.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(2))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeConfigured),
		"Status":  Equal(metav1.ConditionTrue),
		"Reason":  Equal(check.ReasonConfigurationValid),
		"Message": ContainSubstring("authorino-server-cert"),
	}))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactNone))
	g.Expect(result.Status.Conditions[1].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeReady),
		"Status":  Equal(metav1.ConditionTrue),
		"Reason":  Equal(check.ReasonResourceAvailable),
		"Message": ContainSubstring("Authorino is installed and ready"),
	}))
	g.Expect(result.Status.Conditions[1].Impact).To(Equal(resultpkg.ImpactNone))
}

func TestAuthorinoTLSReadinessCheck_NotFound(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds: authorinoListKinds(),
		Objects: []*unstructured.Unstructured{
			newLLMInferenceService(),
		},
	})

	c := kserve.NewAuthorinoTLSReadinessCheck()
	result, err := c.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeReady),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonResourceNotFound),
		"Message": ContainSubstring("Authorino resource not found"),
	}))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactBlocking))
}

func TestAuthorinoTLSReadinessCheck_TLSDisabled(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds: authorinoListKinds(),
		Objects: []*unstructured.Unstructured{
			newLLMInferenceService(),
			newAuthorino(false, "", "True"),
		},
	})

	c := kserve.NewAuthorinoTLSReadinessCheck()
	result, err := c.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(2))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeConfigured),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonConfigurationInvalid),
		"Message": ContainSubstring("TLS is not enabled"),
	}))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactBlocking))
}

func TestAuthorinoTLSReadinessCheck_TLSEnabledNoCertSecret(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds: authorinoListKinds(),
		Objects: []*unstructured.Unstructured{
			newLLMInferenceService(),
			newAuthorino(true, "", "True"),
		},
	})

	c := kserve.NewAuthorinoTLSReadinessCheck()
	result, err := c.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(2))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeConfigured),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonConfigurationInvalid),
		"Message": ContainSubstring("certSecretRef is not configured"),
	}))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactBlocking))
}

func TestAuthorinoTLSReadinessCheck_TLSEnabledEmptyCertSecretName(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	// Create Authorino with certSecretRef present but name empty
	obj := newAuthorino(true, "placeholder", "True")
	err := unstructured.SetNestedField(obj.Object, "", "spec", "listener", "tls", "certSecretRef", "name")
	g.Expect(err).ToNot(HaveOccurred())

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds: authorinoListKinds(),
		Objects: []*unstructured.Unstructured{
			newLLMInferenceService(),
			obj,
		},
	})

	c := kserve.NewAuthorinoTLSReadinessCheck()
	result, err := c.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(2))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeConfigured),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonConfigurationInvalid),
		"Message": ContainSubstring("certSecretRef.name is empty"),
	}))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactBlocking))
}

func TestAuthorinoTLSReadinessCheck_TLSConfiguredButNotReady(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds: authorinoListKinds(),
		Objects: []*unstructured.Unstructured{
			newLLMInferenceService(),
			newAuthorino(true, "authorino-server-cert", "False"),
		},
	})

	c := kserve.NewAuthorinoTLSReadinessCheck()
	result, err := c.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(2))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(check.ConditionTypeConfigured),
		"Status": Equal(metav1.ConditionTrue),
	}))
	g.Expect(result.Status.Conditions[1].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeReady),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonResourceUnavailable),
		"Message": ContainSubstring("not ready"),
	}))
	g.Expect(result.Status.Conditions[1].Impact).To(Equal(resultpkg.ImpactBlocking))
}

func TestAuthorinoTLSReadinessCheck_MissingReadyCondition(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds: authorinoListKinds(),
		Objects: []*unstructured.Unstructured{
			newLLMInferenceService(),
			newAuthorino(true, "authorino-server-cert", ""),
		},
	})

	c := kserve.NewAuthorinoTLSReadinessCheck()
	result, err := c.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(2))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(check.ConditionTypeConfigured),
		"Status": Equal(metav1.ConditionTrue),
	}))
	g.Expect(result.Status.Conditions[1].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeReady),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonInsufficientData),
		"Message": ContainSubstring("Ready condition status is empty"),
	}))
	g.Expect(result.Status.Conditions[1].Impact).To(Equal(resultpkg.ImpactBlocking))
}

func TestAuthorinoTLSReadinessCheck_CanApply_WithLLMD(t *testing.T) {
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

	c := kserve.NewAuthorinoTLSReadinessCheck()
	canApply, err := c.CanApply(t.Context(), target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeTrue())
}

func TestAuthorinoTLSReadinessCheck_CanApply_WithoutLLMD(t *testing.T) {
	g := NewWithT(t)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		CurrentVersion: "2.25.0",
		TargetVersion:  "3.0.0",
		ListKinds: map[schema.GroupVersionResource]string{
			resources.LLMInferenceService.GVR(): resources.LLMInferenceService.ListKind(),
		},
		Objects: nil,
	})

	c := kserve.NewAuthorinoTLSReadinessCheck()
	canApply, err := c.CanApply(t.Context(), target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeFalse())
}

func TestAuthorinoTLSReadinessCheck_CanApply_NotUpgrade2xTo3x(t *testing.T) {
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

	c := kserve.NewAuthorinoTLSReadinessCheck()
	canApply, err := c.CanApply(t.Context(), target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeFalse())
}

func TestAuthorinoTLSReadinessCheck_CanApply_NotUpgrade3xTo4x(t *testing.T) {
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

	c := kserve.NewAuthorinoTLSReadinessCheck()
	canApply, err := c.CanApply(t.Context(), target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeFalse())
}

func TestAuthorinoTLSReadinessCheck_Metadata(t *testing.T) {
	g := NewWithT(t)

	c := kserve.NewAuthorinoTLSReadinessCheck()

	g.Expect(c.ID()).To(Equal("components.kserve.authorino-tls-readiness"))
	g.Expect(c.Name()).To(Equal("Components :: KServe :: Authorino TLS Readiness"))
	g.Expect(c.Group()).To(Equal(check.GroupComponent))
	g.Expect(c.Description()).ToNot(BeEmpty())
}
