package lint_test

import (
	"context"
	"testing"

	"github.com/blang/semver/v4"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	"github.com/lburgazzoli/odh-cli/pkg/lint/check"
	"github.com/lburgazzoli/odh-cli/pkg/lint/check/result"
	"github.com/lburgazzoli/odh-cli/pkg/lint/version"
	"github.com/lburgazzoli/odh-cli/pkg/util/client"

	. "github.com/onsi/gomega"
)

// T061: Integration test for end-to-end diagnostic CR execution.
// Tests the complete flow: check registration -> execution -> DiagnosticResult validation.
func TestDiagnosticCR_EndToEndExecution(t *testing.T) {
	g := NewWithT(t)

	// Setup fake Kubernetes cluster with DataScienceCluster
	scheme := runtime.NewScheme()
	dsc := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "datasciencecluster.opendatahub.io/v1",
			"kind":       "DataScienceCluster",
			"metadata": map[string]any{
				"name": "default-dsc",
			},
			"status": map[string]any{
				"release": map[string]any{
					"name": "stable-2.25",
				},
			},
		},
	}

	dynamicClient := dynamicfake.NewSimpleDynamicClient(scheme, dsc)
	k8sClient := &client.Client{
		Dynamic: dynamicClient,
	}

	// Create a test check that validates CR structure
	testCheck := &testDiagnosticCheck{}
	registry := check.NewRegistry()
	err := registry.Register(testCheck)
	g.Expect(err).ToNot(HaveOccurred())

	// Create executor and target
	executor := check.NewExecutor(registry)
	target := &check.CheckTarget{
		Client: k8sClient,
		Version: &version.ClusterVersion{
			Version:    "2.25.0",
			Source:     version.SourceDataScienceCluster,
			Confidence: version.ConfidenceHigh,
		},
		CurrentVersion: &version.ClusterVersion{
			Version:    "2.25.0",
			Source:     version.SourceDataScienceCluster,
			Confidence: version.ConfidenceHigh,
		},
	}

	// Execute check
	ctx := context.Background()
	results := executor.ExecuteAll(ctx, target)

	// Verify execution
	g.Expect(results).To(HaveLen(1))
	g.Expect(results[0].Error).ToNot(HaveOccurred())
	g.Expect(results[0].Result).ToNot(BeNil())

	// Validate DiagnosticResult CR structure
	dr := results[0].Result

	// Verify Metadata (CR-like structure)
	g.Expect(dr.Metadata.Group).To(Equal("test"))
	g.Expect(dr.Metadata.Kind).To(Equal("integration"))
	g.Expect(dr.Metadata.Name).To(Equal("e2e-test"))
	g.Expect(dr.Metadata.Annotations).ToNot(BeNil())
	g.Expect(dr.Metadata.Annotations).To(HaveKeyWithValue("test.opendatahub.io/purpose", "integration-testing"))

	// Verify Spec
	g.Expect(dr.Spec.Description).To(ContainSubstring("Integration test"))

	// Verify Status (multi-condition support)
	g.Expect(dr.Status.Conditions).To(HaveLen(2))

	// Verify first condition
	g.Expect(dr.Status.Conditions[0].Type).To(Equal(check.ConditionTypeAvailable))
	g.Expect(dr.Status.Conditions[0].Status).To(Equal(metav1.ConditionTrue))
	g.Expect(dr.Status.Conditions[0].Reason).To(Equal(check.ReasonResourceFound))
	g.Expect(dr.Status.Conditions[0].Message).To(ContainSubstring("First validation"))

	// Verify second condition
	g.Expect(dr.Status.Conditions[1].Type).To(Equal(check.ConditionTypeConfigured))
	g.Expect(dr.Status.Conditions[1].Status).To(Equal(metav1.ConditionTrue))
	g.Expect(dr.Status.Conditions[1].Reason).To(Equal(check.ReasonConfigurationValid))
	g.Expect(dr.Status.Conditions[1].Message).To(ContainSubstring("Second validation"))

	// Validate CR structure compliance
	err = dr.Validate()
	g.Expect(err).ToNot(HaveOccurred(), "DiagnosticResult should pass validation")
}

// T061: Test annotation key format validation in CR structure.
func TestDiagnosticCR_AnnotationValidation(t *testing.T) {
	g := NewWithT(t)

	// Create DiagnosticResult with valid annotations
	dr := result.New("component", "test", "annotation-test", "Test annotation validation")
	dr.Metadata.Annotations = map[string]string{
		"opendatahub.io/version":    "2.25.0",
		"test.example.com/check-id": "test-123",
	}
	dr.Status.Conditions = []metav1.Condition{
		check.NewCondition(
			check.ConditionTypeAvailable,
			metav1.ConditionTrue,
			check.ReasonResourceFound,
			"Test condition",
		),
	}

	// Should pass validation with valid annotation keys
	err := dr.Validate()
	g.Expect(err).ToNot(HaveOccurred())

	// Create DiagnosticResult with invalid annotation key
	drInvalid := result.New("component", "test", "invalid-annotation", "Test invalid annotation")
	drInvalid.Metadata.Annotations = map[string]string{
		"invalid-key": "value", // Missing domain/key format
	}

	// Should fail validation with invalid annotation key (no need for conditions)
	err = drInvalid.Metadata.Validate()
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("domain/key format"))
}

// testDiagnosticCheck is a test check implementation for integration testing.
type testDiagnosticCheck struct{}

func (c *testDiagnosticCheck) ID() string {
	return "test.integration.e2e-test"
}

func (c *testDiagnosticCheck) Name() string {
	return "Integration Test Check"
}

func (c *testDiagnosticCheck) Description() string {
	return "Integration test check for end-to-end diagnostic CR execution"
}

func (c *testDiagnosticCheck) Group() check.CheckGroup {
	return "test"
}

func (c *testDiagnosticCheck) CanApply(currentVersion *semver.Version, targetVersion *semver.Version) bool {
	return true // Always apply for testing
}

func (c *testDiagnosticCheck) Validate(ctx context.Context, target *check.CheckTarget) (*result.DiagnosticResult, error) {
	dr := result.New(
		"test",
		"integration",
		"e2e-test",
		"Integration test check for end-to-end diagnostic CR execution",
	)

	// Add test annotations
	dr.Metadata.Annotations["test.opendatahub.io/purpose"] = "integration-testing"

	// Add multiple conditions to test multi-condition support
	dr.Status.Conditions = []metav1.Condition{
		check.NewCondition(
			check.ConditionTypeAvailable,
			metav1.ConditionTrue,
			check.ReasonResourceFound,
			"First validation condition passed",
		),
		check.NewCondition(
			check.ConditionTypeConfigured,
			metav1.ConditionTrue,
			check.ReasonConfigurationValid,
			"Second validation condition passed",
		),
	}

	return dr, nil
}
