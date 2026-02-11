package lint_test

import (
	"bytes"
	"fmt"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/lburgazzoli/odh-cli/pkg/lint"
	"github.com/lburgazzoli/odh-cli/pkg/lint/check"
	"github.com/lburgazzoli/odh-cli/pkg/lint/check/result"

	. "github.com/onsi/gomega"
)

func TestValidateCheckSelectors(t *testing.T) {
	g := NewWithT(t)

	tests := []struct {
		name      string
		selectors []string
		wantErr   bool
	}{
		{
			name:      "single wildcard valid",
			selectors: []string{"*"},
			wantErr:   false,
		},
		{
			name:      "multiple patterns valid",
			selectors: []string{"components.*", "services.*"},
			wantErr:   false,
		},
		{
			name:      "mixed patterns valid",
			selectors: []string{"components", "*dashboard*", "services.oauth"},
			wantErr:   false,
		},
		{
			name:      "empty slice invalid",
			selectors: []string{},
			wantErr:   true,
		},
		{
			name:      "nil slice invalid",
			selectors: nil,
			wantErr:   true,
		},
		{
			name:      "one invalid pattern fails all",
			selectors: []string{"components.*", "["},
			wantErr:   true,
		},
		{
			name:      "empty string in slice invalid",
			selectors: []string{"components.*", ""},
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := lint.ValidateCheckSelectors(tt.selectors)

			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).ToNot(HaveOccurred())
			}
		})
	}
}

func TestValidateCheckSelector(t *testing.T) {
	g := NewWithT(t)

	tests := []struct {
		name     string
		selector string
		wantErr  bool
	}{
		{
			name:     "wildcard valid",
			selector: "*",
			wantErr:  false,
		},
		{
			name:     "category components valid",
			selector: "components",
			wantErr:  false,
		},
		{
			name:     "category services valid",
			selector: "services",
			wantErr:  false,
		},
		{
			name:     "category workloads valid",
			selector: "workloads",
			wantErr:  false,
		},
		{
			name:     "category dependencies valid",
			selector: "dependencies",
			wantErr:  false,
		},
		{
			name:     "glob pattern components.* valid",
			selector: "components.*",
			wantErr:  false,
		},
		{
			name:     "glob pattern *dashboard* valid",
			selector: "*dashboard*",
			wantErr:  false,
		},
		{
			name:     "glob pattern *.dashboard valid",
			selector: "*.dashboard",
			wantErr:  false,
		},
		{
			name:     "exact ID valid",
			selector: "components.dashboard",
			wantErr:  false,
		},
		{
			name:     "complex glob valid",
			selector: "components.dash*",
			wantErr:  false,
		},
		{
			name:     "empty invalid",
			selector: "",
			wantErr:  true,
		},
		{
			name:     "invalid glob pattern [",
			selector: "[",
			wantErr:  true,
		},
		{
			name:     "invalid glob pattern \\",
			selector: "\\",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := lint.ValidateCheckSelector(tt.selector)

			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).ToNot(HaveOccurred())
			}
		})
	}
}

// passCondition creates a simple passing condition for test results.
func passCondition() result.Condition {
	return result.Condition{
		Condition: metav1.Condition{
			Type:    "Available",
			Status:  metav1.ConditionTrue,
			Reason:  "Ready",
			Message: "check passed",
		},
		Impact: result.ImpactNone,
	}
}

func TestOutputTable_VerboseImpactedObjects(t *testing.T) {
	g := NewWithT(t)

	results := []check.CheckExecution{
		{
			Result: &result.DiagnosticResult{
				Group: "workloads",
				Kind:  "kserve",
				Name:  "accelerator-migration",
				Status: result.DiagnosticStatus{
					Conditions: []result.Condition{passCondition()},
				},
				ImpactedObjects: []metav1.PartialObjectMetadata{
					{
						TypeMeta:   metav1.TypeMeta{Kind: "InferenceService", APIVersion: "serving.kserve.io/v1beta1"},
						ObjectMeta: metav1.ObjectMeta{Namespace: "ns1", Name: "isvc-1"},
					},
					{
						TypeMeta:   metav1.TypeMeta{Kind: "InferenceService", APIVersion: "serving.kserve.io/v1beta1"},
						ObjectMeta: metav1.ObjectMeta{Namespace: "ns2", Name: "isvc-2"},
					},
				},
			},
		},
		{
			Result: &result.DiagnosticResult{
				Group: "workloads",
				Kind:  "notebook",
				Name:  "accelerator-migration",
				Status: result.DiagnosticStatus{
					Conditions: []result.Condition{passCondition()},
				},
				ImpactedObjects: []metav1.PartialObjectMetadata{
					{
						TypeMeta:   metav1.TypeMeta{Kind: "Notebook", APIVersion: "kubeflow.org/v1"},
						ObjectMeta: metav1.ObjectMeta{Namespace: "ns1", Name: "notebook-1"},
					},
				},
			},
		},
	}

	var buf bytes.Buffer
	err := lint.OutputTable(&buf, results, lint.TableOutputOptions{ShowImpactedObjects: true})
	g.Expect(err).ToNot(HaveOccurred())

	output := buf.String()
	g.Expect(output).To(ContainSubstring("Impacted Objects:"))
	g.Expect(output).To(ContainSubstring("workloads / kserve:"))
	g.Expect(output).To(ContainSubstring("ns1/isvc-1 (InferenceService)"))
	g.Expect(output).To(ContainSubstring("ns2/isvc-2 (InferenceService)"))
	g.Expect(output).To(ContainSubstring("workloads / notebook:"))
	g.Expect(output).To(ContainSubstring("ns1/notebook-1 (Notebook)"))
}

func TestOutputTable_VerboseNoImpactedObjects(t *testing.T) {
	g := NewWithT(t)

	results := []check.CheckExecution{
		{
			Result: &result.DiagnosticResult{
				Group: "components",
				Kind:  "dashboard",
				Name:  "version-check",
				Status: result.DiagnosticStatus{
					Conditions: []result.Condition{passCondition()},
				},
			},
		},
	}

	var buf bytes.Buffer
	err := lint.OutputTable(&buf, results, lint.TableOutputOptions{ShowImpactedObjects: true})
	g.Expect(err).ToNot(HaveOccurred())

	output := buf.String()
	g.Expect(output).To(ContainSubstring("Summary:"))
	g.Expect(output).ToNot(ContainSubstring("Impacted Objects:"))
}

func TestOutputTable_NonVerboseHidesImpactedObjects(t *testing.T) {
	g := NewWithT(t)

	results := []check.CheckExecution{
		{
			Result: &result.DiagnosticResult{
				Group: "workloads",
				Kind:  "kserve",
				Name:  "accelerator-migration",
				Status: result.DiagnosticStatus{
					Conditions: []result.Condition{passCondition()},
				},
				ImpactedObjects: []metav1.PartialObjectMetadata{
					{
						TypeMeta:   metav1.TypeMeta{Kind: "InferenceService", APIVersion: "serving.kserve.io/v1beta1"},
						ObjectMeta: metav1.ObjectMeta{Namespace: "ns1", Name: "isvc-1"},
					},
				},
			},
		},
	}

	var buf bytes.Buffer
	err := lint.OutputTable(&buf, results, lint.TableOutputOptions{})
	g.Expect(err).ToNot(HaveOccurred())

	output := buf.String()
	g.Expect(output).To(ContainSubstring("Summary:"))
	g.Expect(output).ToNot(ContainSubstring("Impacted Objects:"))
}

func TestOutputTable_VerboseTruncatesAt50(t *testing.T) {
	g := NewWithT(t)

	// Build 60 impacted objects
	objects := make([]metav1.PartialObjectMetadata, 60)
	for i := range objects {
		objects[i] = metav1.PartialObjectMetadata{
			TypeMeta:   metav1.TypeMeta{Kind: "InferenceService", APIVersion: "serving.kserve.io/v1beta1"},
			ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: fmt.Sprintf("isvc-%d", i)},
		}
	}

	results := []check.CheckExecution{
		{
			Result: &result.DiagnosticResult{
				Group: "workloads",
				Kind:  "kserve",
				Name:  "impacted-support",
				Status: result.DiagnosticStatus{
					Conditions: []result.Condition{passCondition()},
				},
				ImpactedObjects: objects,
			},
		},
	}

	var buf bytes.Buffer
	err := lint.OutputTable(&buf, results, lint.TableOutputOptions{ShowImpactedObjects: true})
	g.Expect(err).ToNot(HaveOccurred())

	output := buf.String()
	g.Expect(output).To(ContainSubstring("Impacted Objects:"))
	// Should show the 50th object (index 49)
	g.Expect(output).To(ContainSubstring("ns/isvc-49 (InferenceService)"))
	// Should NOT show the 51st object (index 50)
	g.Expect(output).ToNot(ContainSubstring("ns/isvc-50"))
	// Should show truncation message with remaining count
	g.Expect(output).To(ContainSubstring("... and 10 more"))
	g.Expect(output).To(ContainSubstring("--output json"))
}

func TestOutputTable_VerboseClusterScopedObject(t *testing.T) {
	g := NewWithT(t)

	results := []check.CheckExecution{
		{
			Result: &result.DiagnosticResult{
				Group: "components",
				Kind:  "kserve",
				Name:  "config-check",
				Status: result.DiagnosticStatus{
					Conditions: []result.Condition{passCondition()},
				},
				ImpactedObjects: []metav1.PartialObjectMetadata{
					{
						TypeMeta:   metav1.TypeMeta{Kind: "ClusterResource", APIVersion: "v1"},
						ObjectMeta: metav1.ObjectMeta{Name: "my-cluster-resource"},
					},
				},
			},
		},
	}

	var buf bytes.Buffer
	err := lint.OutputTable(&buf, results, lint.TableOutputOptions{ShowImpactedObjects: true})
	g.Expect(err).ToNot(HaveOccurred())

	output := buf.String()
	// Cluster-scoped objects have no namespace prefix
	g.Expect(output).To(ContainSubstring("my-cluster-resource (ClusterResource)"))
	g.Expect(output).ToNot(ContainSubstring("/my-cluster-resource"))
}
