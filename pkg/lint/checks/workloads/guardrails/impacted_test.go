package guardrails_test

import (
	"testing"

	"github.com/blang/semver/v4"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/lburgazzoli/odh-cli/pkg/lint/check"
	resultpkg "github.com/lburgazzoli/odh-cli/pkg/lint/check/result"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/shared/testutil"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/workloads/guardrails"
	"github.com/lburgazzoli/odh-cli/pkg/resources"

	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

const (
	validConfigYAML = `chat_generation:
  service:
    hostname: "my-service.example.com"
    port: 8080
detectors:
  - name: "detector-1"
    type: "text_contents"
`

	missingAllFieldsConfigYAML = `some_other_key: value
`
)

//nolint:gochecknoglobals // Test fixture - shared across test functions.
var impactedListKinds = map[schema.GroupVersionResource]string{
	resources.GuardrailsOrchestrator.GVR(): resources.GuardrailsOrchestrator.ListKind(),
	resources.ConfigMap.GVR():              resources.ConfigMap.ListKind(),
}

func newTestOrchestrator(
	name string,
	namespace string,
	spec map[string]any,
) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.GuardrailsOrchestrator.APIVersion(),
			"kind":       resources.GuardrailsOrchestrator.Kind,
			"metadata": map[string]any{
				"name":      name,
				"namespace": namespace,
			},
			"spec": spec,
		},
	}
}

func newTestConfigMap(
	name string,
	namespace string,
	data map[string]any,
) *unstructured.Unstructured {
	obj := map[string]any{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
		"metadata": map[string]any{
			"name":      name,
			"namespace": namespace,
		},
	}
	if data != nil {
		obj["data"] = data
	}

	return &unstructured.Unstructured{Object: obj}
}

func TestImpactedWorkloadsCheck_NoResources(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      impactedListKinds,
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := guardrails.NewImpactedWorkloadsCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(guardrails.ConditionTypeConfigurationValid),
		"Status": Equal(metav1.ConditionTrue),
		"Reason": Equal(check.ReasonVersionCompatible),
	}))
	g.Expect(result.Status.Conditions[0].Condition.Message).To(ContainSubstring("No GuardrailsOrchestrators found"))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "0"))
	g.Expect(result.ImpactedObjects).To(BeEmpty())
}

func TestImpactedWorkloadsCheck_ValidCR(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	orch := newTestOrchestrator("test-orch", "test-ns", map[string]any{
		"orchestratorConfig":      "orch-config",
		"replicas":                int64(1),
		"enableGuardrailsGateway": true,
		"enableBuiltInDetectors":  true,
		"guardrailsGatewayConfig": "gateway-config",
	})

	orchCM := newTestConfigMap("orch-config", "test-ns", map[string]any{
		"config.yaml": validConfigYAML,
	})

	gatewayCM := newTestConfigMap("gateway-config", "test-ns", map[string]any{
		"some-key": "some-value",
	})

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      impactedListKinds,
		Objects:        []*unstructured.Unstructured{orch, orchCM, gatewayCM},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := guardrails.NewImpactedWorkloadsCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(guardrails.ConditionTypeConfigurationValid),
		"Status": Equal(metav1.ConditionTrue),
		"Reason": Equal(check.ReasonConfigurationValid),
	}))
	g.Expect(result.Status.Conditions[0].Condition.Message).To(ContainSubstring("configured correctly"))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "0"))
	g.Expect(result.ImpactedObjects).To(BeEmpty())
}

func TestImpactedWorkloadsCheck_InvalidCRSpec(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	// CR with missing/invalid spec fields, no ConfigMap refs.
	orch := newTestOrchestrator("bad-orch", "test-ns", map[string]any{
		"replicas":                int64(0),
		"enableGuardrailsGateway": false,
		"enableBuiltInDetectors":  false,
	})

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      impactedListKinds,
		Objects:        []*unstructured.Unstructured{orch},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := guardrails.NewImpactedWorkloadsCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(guardrails.ConditionTypeConfigurationValid),
		"Status": Equal(metav1.ConditionFalse),
		"Reason": Equal(check.ReasonConfigurationInvalid),
	}))
	g.Expect(result.Status.Conditions[0].Condition.Message).To(ContainSubstring("misconfigured"))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactAdvisory))

	// Only the GuardrailsOrchestrator CR should be impacted (no ConfigMap refs).
	g.Expect(result.ImpactedObjects).To(HaveLen(1))
	g.Expect(result.ImpactedObjects[0]).To(MatchFields(IgnoreExtras, Fields{
		"ObjectMeta": MatchFields(IgnoreExtras, Fields{
			"Name":      Equal("bad-orch"),
			"Namespace": Equal("test-ns"),
		}),
	}))

	// Impacted object annotations describe the specific issues.
	g.Expect(result.ImpactedObjects[0].Annotations).To(And(
		HaveKeyWithValue("guardrails.opendatahub.io/orchestrator-config", "not set"),
		HaveKeyWithValue("guardrails.opendatahub.io/replicas", "less than 1"),
		HaveKeyWithValue("guardrails.opendatahub.io/gateway-config",
			"enableGuardrailsGateway not enabled; guardrailsGatewayConfig not set"),
		HaveKeyWithValue("guardrails.opendatahub.io/builtin-detectors", "not enabled"),
	))
}

func TestImpactedWorkloadsCheck_MissingOrchestratorConfigMap(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	// CR references a ConfigMap that does not exist.
	orch := newTestOrchestrator("test-orch", "test-ns", map[string]any{
		"orchestratorConfig":      "missing-config",
		"replicas":                int64(1),
		"enableGuardrailsGateway": true,
		"enableBuiltInDetectors":  true,
		"guardrailsGatewayConfig": "gateway-config",
	})

	gatewayCM := newTestConfigMap("gateway-config", "test-ns", map[string]any{
		"some-key": "some-value",
	})

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      impactedListKinds,
		Objects:        []*unstructured.Unstructured{orch, gatewayCM},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := guardrails.NewImpactedWorkloadsCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(guardrails.ConditionTypeConfigurationValid),
		"Status": Equal(metav1.ConditionFalse),
		"Reason": Equal(check.ReasonConfigurationInvalid),
	}))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactAdvisory))

	// GuardrailsOrchestrator CR + missing orchestrator ConfigMap.
	g.Expect(result.ImpactedObjects).To(HaveLen(2))

	// First: the GuardrailsOrchestrator CR.
	g.Expect(result.ImpactedObjects[0]).To(MatchFields(IgnoreExtras, Fields{
		"TypeMeta": MatchFields(IgnoreExtras, Fields{
			"Kind": Equal("GuardrailsOrchestrator"),
		}),
		"ObjectMeta": MatchFields(IgnoreExtras, Fields{
			"Name":      Equal("test-orch"),
			"Namespace": Equal("test-ns"),
		}),
	}))
	g.Expect(result.ImpactedObjects[0].Annotations).To(
		HaveKeyWithValue("guardrails.opendatahub.io/orchestrator-configmap", "orchestrator ConfigMap not found"),
	)

	// Second: the missing ConfigMap reference.
	g.Expect(result.ImpactedObjects[1]).To(MatchFields(IgnoreExtras, Fields{
		"TypeMeta": MatchFields(IgnoreExtras, Fields{
			"Kind": Equal("ConfigMap"),
		}),
		"ObjectMeta": MatchFields(IgnoreExtras, Fields{
			"Name":      Equal("missing-config"),
			"Namespace": Equal("test-ns"),
		}),
	}))
	g.Expect(result.ImpactedObjects[1].Annotations).To(
		HaveKeyWithValue("guardrails.opendatahub.io/issues", "orchestrator ConfigMap not found"),
	)
}

func TestImpactedWorkloadsCheck_InvalidOrchestratorConfigMap(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	orch := newTestOrchestrator("test-orch", "test-ns", map[string]any{
		"orchestratorConfig":      "orch-config",
		"replicas":                int64(1),
		"enableGuardrailsGateway": true,
		"enableBuiltInDetectors":  true,
		"guardrailsGatewayConfig": "gateway-config",
	})

	// ConfigMap exists but missing required YAML fields.
	orchCM := newTestConfigMap("orch-config", "test-ns", map[string]any{
		"config.yaml": missingAllFieldsConfigYAML,
	})

	gatewayCM := newTestConfigMap("gateway-config", "test-ns", map[string]any{
		"some-key": "some-value",
	})

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      impactedListKinds,
		Objects:        []*unstructured.Unstructured{orch, orchCM, gatewayCM},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := guardrails.NewImpactedWorkloadsCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(guardrails.ConditionTypeConfigurationValid),
		"Status": Equal(metav1.ConditionFalse),
		"Reason": Equal(check.ReasonConfigurationInvalid),
	}))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactAdvisory))

	// GuardrailsOrchestrator CR + invalid orchestrator ConfigMap.
	g.Expect(result.ImpactedObjects).To(HaveLen(2))

	// First: the GuardrailsOrchestrator CR with ConfigMap issue annotation.
	g.Expect(result.ImpactedObjects[0].Annotations).To(HaveKeyWithValue(
		"guardrails.opendatahub.io/orchestrator-configmap",
		"chat_generation.service misconfiguration; detectors misconfiguration",
	))

	// Second: the ConfigMap with issues annotation.
	g.Expect(result.ImpactedObjects[1]).To(MatchFields(IgnoreExtras, Fields{
		"TypeMeta": MatchFields(IgnoreExtras, Fields{
			"Kind": Equal("ConfigMap"),
		}),
		"ObjectMeta": MatchFields(IgnoreExtras, Fields{
			"Name":      Equal("orch-config"),
			"Namespace": Equal("test-ns"),
		}),
	}))
	g.Expect(result.ImpactedObjects[1].Annotations).To(
		HaveKeyWithValue("guardrails.opendatahub.io/issues",
			"chat_generation.service misconfiguration; detectors misconfiguration"),
	)
}

func TestImpactedWorkloadsCheck_MissingGatewayConfigMap(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	orch := newTestOrchestrator("test-orch", "test-ns", map[string]any{
		"orchestratorConfig":      "orch-config",
		"replicas":                int64(1),
		"enableGuardrailsGateway": true,
		"enableBuiltInDetectors":  true,
		"guardrailsGatewayConfig": "missing-gateway",
	})

	orchCM := newTestConfigMap("orch-config", "test-ns", map[string]any{
		"config.yaml": validConfigYAML,
	})

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      impactedListKinds,
		Objects:        []*unstructured.Unstructured{orch, orchCM},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := guardrails.NewImpactedWorkloadsCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(guardrails.ConditionTypeConfigurationValid),
		"Status": Equal(metav1.ConditionFalse),
		"Reason": Equal(check.ReasonConfigurationInvalid),
	}))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactAdvisory))

	// GuardrailsOrchestrator CR + missing gateway ConfigMap.
	g.Expect(result.ImpactedObjects).To(HaveLen(2))

	// First: the GuardrailsOrchestrator CR.
	g.Expect(result.ImpactedObjects[0].Annotations).To(
		HaveKeyWithValue("guardrails.opendatahub.io/gateway-configmap", "gateway ConfigMap not found"),
	)

	// Second: the missing gateway ConfigMap.
	g.Expect(result.ImpactedObjects[1]).To(MatchFields(IgnoreExtras, Fields{
		"TypeMeta": MatchFields(IgnoreExtras, Fields{
			"Kind": Equal("ConfigMap"),
		}),
		"ObjectMeta": MatchFields(IgnoreExtras, Fields{
			"Name":      Equal("missing-gateway"),
			"Namespace": Equal("test-ns"),
		}),
	}))
	g.Expect(result.ImpactedObjects[1].Annotations).To(
		HaveKeyWithValue("guardrails.opendatahub.io/issues", "gateway ConfigMap not found"),
	)
}

func TestImpactedWorkloadsCheck_Metadata(t *testing.T) {
	g := NewWithT(t)

	chk := guardrails.NewImpactedWorkloadsCheck()

	g.Expect(chk.ID()).To(Equal("workloads.guardrails.impacted-workloads"))
	g.Expect(chk.Name()).To(Equal("Workloads :: Guardrails :: Impacted Workloads (3.x)"))
	g.Expect(chk.Group()).To(Equal(check.GroupWorkload))
	g.Expect(chk.Description()).ToNot(BeEmpty())
}

func TestImpactedWorkloadsCheck_CanApply(t *testing.T) {
	t.Run("nil versions", func(t *testing.T) {
		g := NewWithT(t)

		target := check.Target{
			CurrentVersion: nil,
			TargetVersion:  nil,
		}

		chk := guardrails.NewImpactedWorkloadsCheck()
		canApply, err := chk.CanApply(t.Context(), target)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(canApply).To(BeFalse())
	})

	t.Run("2x to 2x", func(t *testing.T) {
		g := NewWithT(t)

		currentVer := semver.MustParse("2.16.0")
		targetVer := semver.MustParse("2.17.0")
		target := check.Target{
			CurrentVersion: &currentVer,
			TargetVersion:  &targetVer,
		}

		chk := guardrails.NewImpactedWorkloadsCheck()
		canApply, err := chk.CanApply(t.Context(), target)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(canApply).To(BeFalse())
	})

	t.Run("2x to 3x", func(t *testing.T) {
		g := NewWithT(t)

		currentVer := semver.MustParse("2.17.0")
		targetVer := semver.MustParse("3.0.0")
		target := check.Target{
			CurrentVersion: &currentVer,
			TargetVersion:  &targetVer,
		}

		chk := guardrails.NewImpactedWorkloadsCheck()
		canApply, err := chk.CanApply(t.Context(), target)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(canApply).To(BeTrue())
	})

	t.Run("3x to 3x", func(t *testing.T) {
		g := NewWithT(t)

		currentVer := semver.MustParse("3.0.0")
		targetVer := semver.MustParse("3.3.0")
		target := check.Target{
			CurrentVersion: &currentVer,
			TargetVersion:  &targetVer,
		}

		chk := guardrails.NewImpactedWorkloadsCheck()
		canApply, err := chk.CanApply(t.Context(), target)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(canApply).To(BeFalse())
	})
}

func TestImpactedWorkloadsCheck_AnnotationTargetVersion(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      impactedListKinds,
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := guardrails.NewImpactedWorkloadsCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationCheckTargetVersion, "3.0.0"))
}
