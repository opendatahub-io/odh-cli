package lint_test

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/opendatahub-io/odh-cli/pkg/lint"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/result"

	. "github.com/onsi/gomega"
)

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
	// Table borders should be present.
	g.Expect(output).To(ContainSubstring("┌─"))
	g.Expect(output).To(ContainSubstring("├─"))
	g.Expect(output).To(ContainSubstring("└─"))
	// Table header row should be present with borders.
	g.Expect(output).To(ContainSubstring("│"))
	g.Expect(output).To(ContainSubstring("STATUS"))
	g.Expect(output).To(ContainSubstring("KIND"))
	g.Expect(output).To(ContainSubstring("GROUP"))
	g.Expect(output).To(ContainSubstring("CHECK"))
	g.Expect(output).To(ContainSubstring("IMPACT"))
	// Data rows contain check metadata.
	g.Expect(output).To(ContainSubstring("kserve"))
	g.Expect(output).To(ContainSubstring("workloads"))
	g.Expect(output).To(ContainSubstring("accelerator-migration"))
	// Objects are grouped by namespace with Kind shown.
	g.Expect(output).To(ContainSubstring("ns1:"))
	g.Expect(output).To(ContainSubstring("- isvc-1 (InferenceService)"))
	g.Expect(output).To(ContainSubstring("ns2:"))
	g.Expect(output).To(ContainSubstring("- isvc-2 (InferenceService)"))
	g.Expect(output).To(ContainSubstring("notebook"))
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
	// Table header and data row should be present.
	g.Expect(output).To(ContainSubstring("STATUS"))
	g.Expect(output).To(ContainSubstring("notebook"))
	g.Expect(output).To(ContainSubstring("workloads"))
	g.Expect(output).To(ContainSubstring("impacted-workloads"))
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
	aIdx := strings.Index(output, "a-ns:")
	mIdx := strings.Index(output, "m-ns:")
	zIdx := strings.Index(output, "z-ns:")
	g.Expect(aIdx).To(BeNumerically(">=", 0), "a-ns: not found in output")
	g.Expect(mIdx).To(BeNumerically(">=", 0), "m-ns: not found in output")
	g.Expect(zIdx).To(BeNumerically(">=", 0), "z-ns: not found in output")
	g.Expect(aIdx).To(BeNumerically("<", mIdx))
	g.Expect(mIdx).To(BeNumerically("<", zIdx))
}

func TestOutputTable_SortsByGroupKindImpactCheck(t *testing.T) {
	g := NewWithT(t)

	mkCondition := func(impact result.Impact, msg string) result.Condition {
		status := metav1.ConditionTrue
		if impact == result.ImpactBlocking {
			status = metav1.ConditionFalse
		}

		return result.Condition{
			Condition: metav1.Condition{
				Type:    "Validated",
				Status:  status,
				Reason:  "TestReason",
				Message: msg,
			},
			Impact: impact,
		}
	}

	mkExec := func(group string, kind string, name string, conditions ...result.Condition) check.CheckExecution {
		return check.CheckExecution{
			Result: &result.DiagnosticResult{
				Group: group,
				Kind:  kind,
				Name:  name,
				Status: result.DiagnosticStatus{
					Conditions: conditions,
				},
			},
		}
	}

	results := []check.CheckExecution{
		// Deliberately unordered to exercise sorting.
		mkExec("component", "kserve", "config-check", mkCondition(result.ImpactNone, "kserve-comp-info")),
		mkExec("workload", "dashboard", "wl-check", mkCondition(result.ImpactAdvisory, "dashboard-wl-warn")),
		mkExec("dependency", "openshift-platform", "dep-check", mkCondition(result.ImpactBlocking, "ocp-dep-crit")),
		mkExec("dependency", "cert-manager", "installed", mkCondition(result.ImpactNone, "cert-dep-info")),
		mkExec("component", "dashboard", "removal", mkCondition(result.ImpactBlocking, "dashboard-comp-crit")),
		mkExec("service", "dashboard", "svc-check", mkCondition(result.ImpactAdvisory, "dashboard-svc-warn")),
		mkExec("component", "dashboard", "config-migration", mkCondition(result.ImpactAdvisory, "dashboard-comp-warn")),
		mkExec("service", "kserve", "svc-check", mkCondition(result.ImpactBlocking, "kserve-svc-crit")),
	}

	var buf bytes.Buffer
	err := lint.OutputTable(&buf, results, lint.TableOutputOptions{})
	g.Expect(err).ToNot(HaveOccurred())

	output := buf.String()

	// Expected order (Group canonical -> Kind alpha -> Impact severity -> Check alpha):
	//   dependency   cert-manager          info      installed
	//   dependency   openshift-platform    critical  dep-check
	//   service      dashboard             warning   svc-check
	//   service      kserve                critical  svc-check
	//   component    dashboard             critical  removal
	//   component    dashboard             warning   config-migration
	//   component    kserve                info      config-check
	//   workload     dashboard             warning   wl-check
	expectedOrder := []string{
		"cert-dep-info",
		"ocp-dep-crit",
		"dashboard-svc-warn",
		"kserve-svc-crit",
		"dashboard-comp-crit",
		"dashboard-comp-warn",
		"kserve-comp-info",
		"dashboard-wl-warn",
	}

	prevIdx := -1
	for _, msg := range expectedOrder {
		idx := strings.Index(output, msg)
		g.Expect(idx).To(BeNumerically(">", prevIdx),
			fmt.Sprintf("%q should appear after previous entry (prevIdx=%d, idx=%d)", msg, prevIdx, idx))

		prevIdx = idx
	}
}

func TestOutputTable_VersionInfoLintMode(t *testing.T) {
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
	opts := lint.TableOutputOptions{
		VersionInfo: &lint.VersionInfo{
			RHOAICurrentVersion: "2.17.0",
			OpenShiftVersion:    "4.19.1",
		},
	}

	err := lint.OutputTable(&buf, results, opts)
	g.Expect(err).ToNot(HaveOccurred())

	output := buf.String()
	g.Expect(output).To(ContainSubstring("Environment:"))
	g.Expect(output).To(ContainSubstring("OpenShift AI version: 2.17.0"))
	g.Expect(output).To(ContainSubstring("OpenShift version:    4.19.1"))
	g.Expect(output).ToNot(ContainSubstring("->"))
}

func TestOutputTable_VersionInfoUpgradeMode(t *testing.T) {
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
	opts := lint.TableOutputOptions{
		VersionInfo: &lint.VersionInfo{
			RHOAICurrentVersion: "2.17.0",
			RHOAITargetVersion:  "3.0.0",
			OpenShiftVersion:    "4.19.1",
		},
	}

	err := lint.OutputTable(&buf, results, opts)
	g.Expect(err).ToNot(HaveOccurred())

	output := buf.String()
	g.Expect(output).To(ContainSubstring("Environment:"))
	g.Expect(output).To(ContainSubstring("OpenShift AI version: 2.17.0 -> 3.0.0"))
	g.Expect(output).To(ContainSubstring("OpenShift version:    4.19.1"))
}

func TestOutputTable_VersionInfoWithoutOpenShift(t *testing.T) {
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
	opts := lint.TableOutputOptions{
		VersionInfo: &lint.VersionInfo{
			RHOAICurrentVersion: "2.17.0",
		},
	}

	err := lint.OutputTable(&buf, results, opts)
	g.Expect(err).ToNot(HaveOccurred())

	output := buf.String()
	g.Expect(output).To(ContainSubstring("Environment:"))
	g.Expect(output).To(ContainSubstring("OpenShift AI version: 2.17.0"))
	g.Expect(output).ToNot(ContainSubstring("OpenShift version:"))
}

func TestOutputTable_NoVersionInfo(t *testing.T) {
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
	err := lint.OutputTable(&buf, results, lint.TableOutputOptions{})
	g.Expect(err).ToNot(HaveOccurred())

	output := buf.String()
	g.Expect(output).ToNot(ContainSubstring("Environment:"))
}

func TestOutputTable_VersionInfoAppearsBetweenTableAndSummary(t *testing.T) {
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
	opts := lint.TableOutputOptions{
		VersionInfo: &lint.VersionInfo{
			RHOAICurrentVersion: "2.17.0",
			OpenShiftVersion:    "4.19.1",
		},
	}

	err := lint.OutputTable(&buf, results, opts)
	g.Expect(err).ToNot(HaveOccurred())

	output := buf.String()
	tableIdx := strings.Index(output, "version-check")
	envIdx := strings.Index(output, "Environment:")
	summaryIdx := strings.Index(output, "Summary:")
	g.Expect(tableIdx).To(BeNumerically(">=", 0))
	g.Expect(envIdx).To(BeNumerically(">=", 0))
	g.Expect(summaryIdx).To(BeNumerically(">=", 0))
	g.Expect(tableIdx).To(BeNumerically("<", envIdx))
	g.Expect(envIdx).To(BeNumerically("<", summaryIdx))
}

func TestOutputTable_ProhibitedBannerAppearsBeforeTable(t *testing.T) {
	g := NewWithT(t)

	results := []check.CheckExecution{
		{
			Result: &result.DiagnosticResult{
				Group: "workload",
				Kind:  "kueue",
				Name:  "data-integrity",
				Status: result.DiagnosticStatus{
					Conditions: []result.Condition{
						{
							Condition: metav1.Condition{
								Type:    "KueueConsistency",
								Status:  metav1.ConditionFalse,
								Reason:  "ConfigurationInvalid",
								Message: "Found 3 kueue consistency violations",
							},
							Impact: result.ImpactProhibited,
						},
					},
				},
			},
		},
		{
			Result: &result.DiagnosticResult{
				Group: "component",
				Kind:  "dashboard",
				Name:  "version-check",
				Status: result.DiagnosticStatus{
					Conditions: []result.Condition{passCondition()},
				},
			},
		},
	}

	var buf bytes.Buffer
	err := lint.OutputTable(&buf, results, lint.TableOutputOptions{})
	g.Expect(err).ToNot(HaveOccurred())

	output := buf.String()

	// Banner should be present and appear before the table data.
	g.Expect(output).To(ContainSubstring("Prohibited Violations Detected"))
	g.Expect(output).To(ContainSubstring("Found 3 kueue consistency violations"))

	bannerIdx := strings.Index(output, "Prohibited Violations Detected")
	tableIdx := strings.Index(output, "STATUS")
	g.Expect(bannerIdx).To(BeNumerically("<", tableIdx))

	// Summary should include prohibited count.
	g.Expect(output).To(ContainSubstring("Prohibited: 1"))
}

func TestOutputTable_ProhibitedBannerShowsMultipleProhibitedFindings(t *testing.T) {
	g := NewWithT(t)

	results := []check.CheckExecution{
		{
			Result: &result.DiagnosticResult{
				Group: "workload",
				Kind:  "kueue",
				Name:  "data-integrity",
				Status: result.DiagnosticStatus{
					Conditions: []result.Condition{
						{
							Condition: metav1.Condition{
								Type:    "KueueConsistency",
								Status:  metav1.ConditionFalse,
								Reason:  "ConfigurationInvalid",
								Message: "kueue label inconsistencies found",
							},
							Impact: result.ImpactProhibited,
						},
					},
				},
			},
		},
		{
			Result: &result.DiagnosticResult{
				Group: "workload",
				Kind:  "other",
				Name:  "other-integrity",
				Status: result.DiagnosticStatus{
					Conditions: []result.Condition{
						{
							Condition: metav1.Condition{
								Type:    "DataConsistency",
								Status:  metav1.ConditionFalse,
								Reason:  "Inconsistent",
								Message: "second prohibited violation detected",
							},
							Impact: result.ImpactProhibited,
						},
					},
				},
			},
		},
	}

	var buf bytes.Buffer
	err := lint.OutputTable(&buf, results, lint.TableOutputOptions{})
	g.Expect(err).ToNot(HaveOccurred())

	output := buf.String()

	// Both prohibited findings should appear in the banner.
	g.Expect(output).To(ContainSubstring("kueue label inconsistencies found"))
	g.Expect(output).To(ContainSubstring("second prohibited violation detected"))
	g.Expect(output).To(ContainSubstring("Prohibited: 2"))
}

func TestOutputTable_NoBannerWhenNoProhibitedFindings(t *testing.T) {
	g := NewWithT(t)

	results := []check.CheckExecution{
		{
			Result: &result.DiagnosticResult{
				Group: "component",
				Kind:  "dashboard",
				Name:  "version-check",
				Status: result.DiagnosticStatus{
					Conditions: []result.Condition{
						{
							Condition: metav1.Condition{
								Type:    "Compatible",
								Status:  metav1.ConditionFalse,
								Reason:  "Incompatible",
								Message: "blocking but not prohibited",
							},
							Impact: result.ImpactBlocking,
						},
					},
				},
			},
		},
	}

	var buf bytes.Buffer
	err := lint.OutputTable(&buf, results, lint.TableOutputOptions{})
	g.Expect(err).ToNot(HaveOccurred())

	g.Expect(buf.String()).ToNot(ContainSubstring("Prohibited Violations Detected"))
}
