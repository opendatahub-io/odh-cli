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
	// Header includes checkType for specificity
	g.Expect(output).To(ContainSubstring("workloads / kserve / accelerator-migration:"))
	// Objects are grouped by namespace with Kind shown
	g.Expect(output).To(ContainSubstring("ns1:"))
	g.Expect(output).To(ContainSubstring("- isvc-1 (InferenceService)"))
	g.Expect(output).To(ContainSubstring("ns2:"))
	g.Expect(output).To(ContainSubstring("- isvc-2 (InferenceService)"))
	g.Expect(output).To(ContainSubstring("workloads / notebook / accelerator-migration:"))
	g.Expect(output).To(ContainSubstring("- notebook-1 (Notebook)"))
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

func TestOutputTable_VerboseShowsAllObjects(t *testing.T) {
	g := NewWithT(t)

	// Build 60 impacted objects to verify no truncation.
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
	// All objects should be shown (no truncation).
	g.Expect(output).To(ContainSubstring("- isvc-0"))
	g.Expect(output).To(ContainSubstring("- isvc-49"))
	g.Expect(output).To(ContainSubstring("- isvc-59"))
	// No truncation message.
	g.Expect(output).ToNot(ContainSubstring("... and"))
	g.Expect(output).ToNot(ContainSubstring("--output json"))
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
	// Cluster-scoped objects listed directly without namespace header, with Kind shown.
	g.Expect(output).To(ContainSubstring("- my-cluster-resource (ClusterResource)"))
	g.Expect(output).ToNot(ContainSubstring("/my-cluster-resource"))
}

func TestOutputTable_VerboseNamespaceRequester(t *testing.T) {
	g := NewWithT(t)

	results := []check.CheckExecution{
		{
			Result: &result.DiagnosticResult{
				Group: "workloads",
				Kind:  "notebook",
				Name:  "impacted-workloads",
				Status: result.DiagnosticStatus{
					Conditions: []result.Condition{passCondition()},
				},
				ImpactedObjects: []metav1.PartialObjectMetadata{
					{
						TypeMeta:   metav1.TypeMeta{Kind: "Notebook", APIVersion: "kubeflow.org/v1"},
						ObjectMeta: metav1.ObjectMeta{Namespace: "project-a", Name: "nb-1"},
					},
					{
						TypeMeta:   metav1.TypeMeta{Kind: "Notebook", APIVersion: "kubeflow.org/v1"},
						ObjectMeta: metav1.ObjectMeta{Namespace: "project-b", Name: "nb-2"},
					},
					{
						TypeMeta:   metav1.TypeMeta{Kind: "Notebook", APIVersion: "kubeflow.org/v1"},
						ObjectMeta: metav1.ObjectMeta{Namespace: "project-a", Name: "nb-3"},
					},
				},
			},
		},
	}

	var buf bytes.Buffer
	opts := lint.TableOutputOptions{
		ShowImpactedObjects: true,
		NamespaceRequesters: map[string]string{
			"project-a": "alice@example.com",
			"project-b": "bob@example.com",
		},
	}

	err := lint.OutputTable(&buf, results, opts)
	g.Expect(err).ToNot(HaveOccurred())

	output := buf.String()
	g.Expect(output).To(ContainSubstring("Impacted Objects:"))
	g.Expect(output).To(ContainSubstring("workloads / notebook / impacted-workloads:"))
	// Namespace headers should include requester annotation.
	g.Expect(output).To(ContainSubstring("project-a (requester: alice@example.com):"))
	g.Expect(output).To(ContainSubstring("project-b (requester: bob@example.com):"))
	// Objects listed with Kind within namespace groups.
	g.Expect(output).To(ContainSubstring("- nb-1 (Notebook)"))
	g.Expect(output).To(ContainSubstring("- nb-2 (Notebook)"))
	g.Expect(output).To(ContainSubstring("- nb-3 (Notebook)"))
}

func TestOutputTable_VerboseNamespaceGroupingSorted(t *testing.T) {
	g := NewWithT(t)

	results := []check.CheckExecution{
		{
			Result: &result.DiagnosticResult{
				Group: "workloads",
				Kind:  "notebook",
				Name:  "check",
				Status: result.DiagnosticStatus{
					Conditions: []result.Condition{passCondition()},
				},
				ImpactedObjects: []metav1.PartialObjectMetadata{
					{
						TypeMeta:   metav1.TypeMeta{Kind: "Notebook"},
						ObjectMeta: metav1.ObjectMeta{Namespace: "z-ns", Name: "nb-z"},
					},
					{
						TypeMeta:   metav1.TypeMeta{Kind: "Notebook"},
						ObjectMeta: metav1.ObjectMeta{Namespace: "a-ns", Name: "nb-a"},
					},
					{
						TypeMeta:   metav1.TypeMeta{Kind: "Notebook"},
						ObjectMeta: metav1.ObjectMeta{Namespace: "m-ns", Name: "nb-m"},
					},
				},
			},
		},
	}

	var buf bytes.Buffer
	err := lint.OutputTable(&buf, results, lint.TableOutputOptions{ShowImpactedObjects: true})
	g.Expect(err).ToNot(HaveOccurred())

	output := buf.String()
	// Namespaces should be sorted alphabetically.
	aIdx := len(output) - len(output[indexOf(output, "a-ns:"):])
	mIdx := len(output) - len(output[indexOf(output, "m-ns:"):])
	zIdx := len(output) - len(output[indexOf(output, "z-ns:"):])
	g.Expect(aIdx).To(BeNumerically("<", mIdx))
	g.Expect(mIdx).To(BeNumerically("<", zIdx))
}

// indexOf returns the index of the first occurrence of substr in s, or -1.
func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}

	return -1
}
