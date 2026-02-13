package guardrails_test

import (
	"testing"

	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	resultpkg "github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/testutil"
	"github.com/opendatahub-io/odh-cli/pkg/lint/checks/workloads/guardrails"
	"github.com/opendatahub-io/odh-cli/pkg/resources"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

//nolint:gochecknoglobals
var listKinds = map[schema.GroupVersionResource]string{
	resources.GuardrailsOrchestrator.GVR(): resources.GuardrailsOrchestrator.ListKind(),
	resources.DataScienceCluster.GVR():     resources.DataScienceCluster.ListKind(),
}

func TestOtelMigrationCheck_NoOrchestrators(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        nil,
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	otelCheck := guardrails.NewOtelMigrationCheck()
	result, err := otelCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(guardrails.ConditionTypeOtelConfigCompatible),
		"Status":  Equal(metav1.ConditionTrue),
		"Reason":  Equal(check.ReasonVersionCompatible),
		"Message": ContainSubstring("No GuardrailsOrchestrators found"),
	}))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "0"))
	g.Expect(result.ImpactedObjects).To(BeEmpty())
}

func TestOtelMigrationCheck_OrchestratorWithOtelExporter(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	// Any orchestrator with .spec.otelExporter is impacted since the entire struct is changing
	orch := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.GuardrailsOrchestrator.APIVersion(),
			"kind":       resources.GuardrailsOrchestrator.Kind,
			"metadata": map[string]any{
				"name":      "test-orchestrator",
				"namespace": "test-ns",
			},
			"spec": map[string]any{
				"otelExporter": map[string]any{
					"otlpProtocol":        "grpc",
					"otlpTracesEndpoint":  "http://traces:4317",
					"otlpMetricsEndpoint": "http://metrics:4317",
					"enableMetrics":       true,
					"enableTracing":       true,
				},
			},
		},
	}

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{orch},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	otelCheck := guardrails.NewOtelMigrationCheck()
	result, err := otelCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(guardrails.ConditionTypeOtelConfigCompatible),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonConfigurationInvalid),
		"Message": And(ContainSubstring("Found 1 GuardrailsOrchestrator"), ContainSubstring("deprecated")),
	}))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactAdvisory))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "1"))
	g.Expect(result.ImpactedObjects).To(HaveLen(1))
	g.Expect(result.ImpactedObjects[0].Name).To(Equal("test-orchestrator"))
	g.Expect(result.ImpactedObjects[0].Namespace).To(Equal("test-ns"))
}

func TestOtelMigrationCheck_OrchestratorWithEmptyOtelExporter(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	// Empty otelExporter: {} should not be flagged as deprecated.
	orch := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.GuardrailsOrchestrator.APIVersion(),
			"kind":       resources.GuardrailsOrchestrator.Kind,
			"metadata": map[string]any{
				"name":      "empty-otel-orch",
				"namespace": "test-ns",
			},
			"spec": map[string]any{
				"otelExporter": map[string]any{},
				"replicas":     int64(1),
			},
		},
	}

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{orch},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	otelCheck := guardrails.NewOtelMigrationCheck()
	result, err := otelCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(guardrails.ConditionTypeOtelConfigCompatible),
		"Status": Equal(metav1.ConditionTrue),
		"Reason": Equal(check.ReasonVersionCompatible),
	}))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "0"))
	g.Expect(result.ImpactedObjects).To(BeEmpty())
}

func TestOtelMigrationCheck_OrchestratorWithoutOtelExporter(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	// Orchestrator without any otelExporter configuration
	orch := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.GuardrailsOrchestrator.APIVersion(),
			"kind":       resources.GuardrailsOrchestrator.Kind,
			"metadata": map[string]any{
				"name":      "no-otel-orch",
				"namespace": "test-ns",
			},
			"spec": map[string]any{
				"replicas": int64(1),
			},
		},
	}

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{orch},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	otelCheck := guardrails.NewOtelMigrationCheck()
	result, err := otelCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(guardrails.ConditionTypeOtelConfigCompatible),
		"Status": Equal(metav1.ConditionTrue),
		"Reason": Equal(check.ReasonVersionCompatible),
	}))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "0"))
	g.Expect(result.ImpactedObjects).To(BeEmpty())
}

func TestOtelMigrationCheck_OrchestratorWithDeprecatedFields(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	// Orchestrator using deprecated fields
	orch := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.GuardrailsOrchestrator.APIVersion(),
			"kind":       resources.GuardrailsOrchestrator.Kind,
			"metadata": map[string]any{
				"name":      "deprecated-orch",
				"namespace": "prod-ns",
			},
			"spec": map[string]any{
				"otelExporter": map[string]any{
					"protocol":     "grpc",
					"otlpEndpoint": "http://collector:4317",
					"otlpExport":   true,
				},
			},
		},
	}

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{orch},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	otelCheck := guardrails.NewOtelMigrationCheck()
	result, err := otelCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(guardrails.ConditionTypeOtelConfigCompatible),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonConfigurationInvalid),
		"Message": And(ContainSubstring("Found 1 GuardrailsOrchestrator"), ContainSubstring("deprecated")),
	}))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactAdvisory))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "1"))
	g.Expect(result.ImpactedObjects).To(HaveLen(1))
	g.Expect(result.ImpactedObjects[0].Name).To(Equal("deprecated-orch"))
	g.Expect(result.ImpactedObjects[0].Namespace).To(Equal("prod-ns"))
}

func TestOtelMigrationCheck_MultipleOrchestratorsWithDeprecatedFields(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	orch1 := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.GuardrailsOrchestrator.APIVersion(),
			"kind":       resources.GuardrailsOrchestrator.Kind,
			"metadata": map[string]any{
				"name":      "orch-1",
				"namespace": "ns1",
			},
			"spec": map[string]any{
				"otelExporter": map[string]any{
					"tracesProtocol":  "http",
					"tracesEndpoint":  "http://traces:4318",
					"metricsProtocol": "grpc",
					"metricsEndpoint": "http://metrics:4317",
				},
			},
		},
	}

	orch2 := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.GuardrailsOrchestrator.APIVersion(),
			"kind":       resources.GuardrailsOrchestrator.Kind,
			"metadata": map[string]any{
				"name":      "orch-2",
				"namespace": "ns2",
			},
			"spec": map[string]any{
				"otelExporter": map[string]any{
					"protocol":     "grpc",
					"otlpEndpoint": "http://otel:4317",
				},
			},
		},
	}

	// Orchestrator with new config only - also impacted since the entire otelExporter struct is changing
	orch3 := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.GuardrailsOrchestrator.APIVersion(),
			"kind":       resources.GuardrailsOrchestrator.Kind,
			"metadata": map[string]any{
				"name":      "orch-3",
				"namespace": "ns3",
			},
			"spec": map[string]any{
				"otelExporter": map[string]any{
					"otlpProtocol":        "grpc",
					"otlpTracesEndpoint":  "http://traces:4317",
					"otlpMetricsEndpoint": "http://metrics:4317",
				},
			},
		},
	}

	// Orchestrator without otelExporter - should not be impacted
	orch4 := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.GuardrailsOrchestrator.APIVersion(),
			"kind":       resources.GuardrailsOrchestrator.Kind,
			"metadata": map[string]any{
				"name":      "orch-4",
				"namespace": "ns4",
			},
			"spec": map[string]any{
				"replicas": int64(1),
			},
		},
	}

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{orch1, orch2, orch3, orch4},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	otelCheck := guardrails.NewOtelMigrationCheck()
	result, err := otelCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(guardrails.ConditionTypeOtelConfigCompatible),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonConfigurationInvalid),
		"Message": ContainSubstring("Found 3 GuardrailsOrchestrator"),
	}))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactAdvisory))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "3"))
	g.Expect(result.ImpactedObjects).To(HaveLen(3))
}

func TestOtelMigrationCheck_Metadata(t *testing.T) {
	g := NewWithT(t)

	otelCheck := guardrails.NewOtelMigrationCheck()

	g.Expect(otelCheck.ID()).To(Equal("workloads.guardrails.otel-config-migration"))
	g.Expect(otelCheck.Name()).To(Equal("Workloads :: Guardrails :: OTEL Config Migration (3.x)"))
	g.Expect(otelCheck.Group()).To(Equal(check.GroupWorkload))
	g.Expect(otelCheck.Description()).ToNot(BeEmpty())
}

func TestOtelMigrationCheck_CanApply_NilVersions(t *testing.T) {
	g := NewWithT(t)

	chk := guardrails.NewOtelMigrationCheck()
	canApply, err := chk.CanApply(t.Context(), check.Target{})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeFalse())
}

func TestOtelMigrationCheck_CanApply_LintMode(t *testing.T) {
	g := NewWithT(t)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{testutil.NewDSC(map[string]string{"trustyai": "Managed"})},
		CurrentVersion: "2.17.0",
		TargetVersion:  "2.17.0",
	})

	chk := guardrails.NewOtelMigrationCheck()
	canApply, err := chk.CanApply(t.Context(), target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeFalse())
}

func TestOtelMigrationCheck_CanApply_UpgradeTo3x_Managed(t *testing.T) {
	g := NewWithT(t)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{testutil.NewDSC(map[string]string{"trustyai": "Managed"})},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := guardrails.NewOtelMigrationCheck()
	canApply, err := chk.CanApply(t.Context(), target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeTrue())
}

func TestOtelMigrationCheck_CanApply_UpgradeTo3x_Removed(t *testing.T) {
	g := NewWithT(t)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{testutil.NewDSC(map[string]string{"trustyai": "Removed"})},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := guardrails.NewOtelMigrationCheck()
	canApply, err := chk.CanApply(t.Context(), target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeFalse())
}

func TestOtelMigrationCheck_CanApply_3xTo3x(t *testing.T) {
	g := NewWithT(t)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{testutil.NewDSC(map[string]string{"trustyai": "Managed"})},
		CurrentVersion: "3.0.0",
		TargetVersion:  "3.3.0",
	})

	chk := guardrails.NewOtelMigrationCheck()
	canApply, err := chk.CanApply(t.Context(), target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeFalse())
}

func TestOtelMigrationCheck_AnnotationTargetVersion(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        nil,
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	otelCheck := guardrails.NewOtelMigrationCheck()
	result, err := otelCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationCheckTargetVersion, "3.0.0"))
}
