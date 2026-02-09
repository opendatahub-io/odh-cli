package guardrails_test

import (
	"testing"

	"github.com/blang/semver/v4"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	metadatafake "k8s.io/client-go/metadata/fake"

	"github.com/lburgazzoli/odh-cli/pkg/lint/check"
	resultpkg "github.com/lburgazzoli/odh-cli/pkg/lint/check/result"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/workloads/guardrails"
	"github.com/lburgazzoli/odh-cli/pkg/resources"
	"github.com/lburgazzoli/odh-cli/pkg/util/client"

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

func newTestTarget(
	t *testing.T,
	objects ...runtime.Object,
) check.Target {
	t.Helper()

	scheme := runtime.NewScheme()
	_ = metav1.AddMetaToScheme(scheme)
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, impactedListKinds, objects...)
	metadataClient := metadatafake.NewSimpleMetadataClient(scheme)

	c := client.NewForTesting(client.TestClientConfig{
		Dynamic:  dynamicClient,
		Metadata: metadataClient,
	})

	currentVer := semver.MustParse("2.17.0")
	targetVer := semver.MustParse("3.0.0")

	return check.Target{
		Client:         c,
		CurrentVersion: &currentVer,
		TargetVersion:  &targetVer,
	}
}

func TestImpactedWorkloadsCheck_NoResources(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	target := newTestTarget(t)

	chk := guardrails.NewImpactedWorkloadsCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(3))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(guardrails.ConditionTypeOrchestratorCRConfigured),
		"Status": Equal(metav1.ConditionTrue),
		"Reason": Equal(check.ReasonVersionCompatible),
	}))
	g.Expect(result.Status.Conditions[1].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(guardrails.ConditionTypeOrchestratorConfigMapValid),
		"Status": Equal(metav1.ConditionTrue),
		"Reason": Equal(check.ReasonVersionCompatible),
	}))
	g.Expect(result.Status.Conditions[2].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(guardrails.ConditionTypeGatewayConfigMapValid),
		"Status": Equal(metav1.ConditionTrue),
		"Reason": Equal(check.ReasonVersionCompatible),
	}))
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

	target := newTestTarget(t, orch, orchCM, gatewayCM)

	chk := guardrails.NewImpactedWorkloadsCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(3))

	// All three conditions should pass
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(guardrails.ConditionTypeOrchestratorCRConfigured),
		"Status": Equal(metav1.ConditionTrue),
		"Reason": Equal(check.ReasonConfigurationValid),
	}))
	g.Expect(result.Status.Conditions[1].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(guardrails.ConditionTypeOrchestratorConfigMapValid),
		"Status": Equal(metav1.ConditionTrue),
		"Reason": Equal(check.ReasonConfigurationValid),
	}))
	g.Expect(result.Status.Conditions[2].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(guardrails.ConditionTypeGatewayConfigMapValid),
		"Status": Equal(metav1.ConditionTrue),
		"Reason": Equal(check.ReasonConfigurationValid),
	}))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "0"))
	g.Expect(result.ImpactedObjects).To(BeEmpty())
}

func TestImpactedWorkloadsCheck_InvalidCRSpec(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	// CR with missing/invalid spec fields
	orch := newTestOrchestrator("bad-orch", "test-ns", map[string]any{
		"replicas":                int64(0),
		"enableGuardrailsGateway": false,
		"enableBuiltInDetectors":  false,
	})

	target := newTestTarget(t, orch)

	chk := guardrails.NewImpactedWorkloadsCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(3))

	// CR config condition should fail with advisory impact and list specific issues
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(guardrails.ConditionTypeOrchestratorCRConfigured),
		"Status": Equal(metav1.ConditionFalse),
		"Reason": Equal(check.ReasonConfigurationInvalid),
		"Message": And(
			ContainSubstring("1 of 1"),
			ContainSubstring("CR spec issues"),
			ContainSubstring("orchestratorConfig is not set"),
			ContainSubstring("enableGuardrailsGateway is false"),
		),
	}))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactAdvisory))

	g.Expect(result.ImpactedObjects).To(HaveLen(1))
	g.Expect(result.ImpactedObjects[0].Name).To(Equal("bad-orch"))
	g.Expect(result.ImpactedObjects[0].Namespace).To(Equal("test-ns"))
}

func TestImpactedWorkloadsCheck_MissingOrchestratorConfigMap(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	// CR references a ConfigMap that does not exist
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

	target := newTestTarget(t, orch, gatewayCM)

	chk := guardrails.NewImpactedWorkloadsCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(3))

	// CR config passes
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(guardrails.ConditionTypeOrchestratorCRConfigured),
		"Status": Equal(metav1.ConditionTrue),
	}))

	// Orchestrator ConfigMap fails with specific issue detail
	g.Expect(result.Status.Conditions[1].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(guardrails.ConditionTypeOrchestratorConfigMapValid),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonConfigurationInvalid),
		"Message": And(ContainSubstring("orchestrator ConfigMap issues"), ContainSubstring("missing-config")),
	}))
	g.Expect(result.Status.Conditions[1].Impact).To(Equal(resultpkg.ImpactAdvisory))

	// Gateway ConfigMap passes
	g.Expect(result.Status.Conditions[2].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(guardrails.ConditionTypeGatewayConfigMapValid),
		"Status": Equal(metav1.ConditionTrue),
	}))

	g.Expect(result.ImpactedObjects).To(HaveLen(1))
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

	// ConfigMap exists but missing required YAML fields
	orchCM := newTestConfigMap("orch-config", "test-ns", map[string]any{
		"config.yaml": missingAllFieldsConfigYAML,
	})

	gatewayCM := newTestConfigMap("gateway-config", "test-ns", map[string]any{
		"some-key": "some-value",
	})

	target := newTestTarget(t, orch, orchCM, gatewayCM)

	chk := guardrails.NewImpactedWorkloadsCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(3))

	// CR config passes
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(guardrails.ConditionTypeOrchestratorCRConfigured),
		"Status": Equal(metav1.ConditionTrue),
	}))

	// Orchestrator ConfigMap fails with specific field issues
	g.Expect(result.Status.Conditions[1].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(guardrails.ConditionTypeOrchestratorConfigMapValid),
		"Status": Equal(metav1.ConditionFalse),
		"Reason": Equal(check.ReasonConfigurationInvalid),
		"Message": And(
			ContainSubstring("orchestrator ConfigMap issues"),
			ContainSubstring("hostname"),
			ContainSubstring("port"),
			ContainSubstring("detectors"),
		),
	}))
	g.Expect(result.Status.Conditions[1].Impact).To(Equal(resultpkg.ImpactAdvisory))

	// Gateway ConfigMap passes
	g.Expect(result.Status.Conditions[2].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(guardrails.ConditionTypeGatewayConfigMapValid),
		"Status": Equal(metav1.ConditionTrue),
	}))

	g.Expect(result.ImpactedObjects).To(HaveLen(1))
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

	target := newTestTarget(t, orch, orchCM)

	chk := guardrails.NewImpactedWorkloadsCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(3))

	// CR config and orchestrator CM pass
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(guardrails.ConditionTypeOrchestratorCRConfigured),
		"Status": Equal(metav1.ConditionTrue),
	}))
	g.Expect(result.Status.Conditions[1].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(guardrails.ConditionTypeOrchestratorConfigMapValid),
		"Status": Equal(metav1.ConditionTrue),
	}))

	// Gateway ConfigMap fails with specific issue detail
	g.Expect(result.Status.Conditions[2].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(guardrails.ConditionTypeGatewayConfigMapValid),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonConfigurationInvalid),
		"Message": And(ContainSubstring("gateway ConfigMap issues"), ContainSubstring("missing-gateway")),
	}))
	g.Expect(result.Status.Conditions[2].Impact).To(Equal(resultpkg.ImpactAdvisory))

	g.Expect(result.ImpactedObjects).To(HaveLen(1))
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
		g.Expect(chk.CanApply(t.Context(), target)).To(BeFalse())
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
		g.Expect(chk.CanApply(t.Context(), target)).To(BeFalse())
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
		g.Expect(chk.CanApply(t.Context(), target)).To(BeTrue())
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
		g.Expect(chk.CanApply(t.Context(), target)).To(BeFalse())
	})
}

func TestImpactedWorkloadsCheck_AnnotationTargetVersion(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	target := newTestTarget(t)

	chk := guardrails.NewImpactedWorkloadsCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationCheckTargetVersion, "3.0.0"))
}
