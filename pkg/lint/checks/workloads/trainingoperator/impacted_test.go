package trainingoperator_test

import (
	"testing"

	"github.com/blang/semver/v4"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/lburgazzoli/odh-cli/pkg/lint/check"
	resultpkg "github.com/lburgazzoli/odh-cli/pkg/lint/check/result"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/shared/testutil"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/workloads/trainingoperator"
	"github.com/lburgazzoli/odh-cli/pkg/resources"

	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

//nolint:gochecknoglobals
var listKinds = map[schema.GroupVersionResource]string{
	resources.PyTorchJob.GVR(): resources.PyTorchJob.ListKind(),
}

func TestImpactedWorkloadsCheck_NoResources(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:     listKinds,
		TargetVersion: "3.3.0",
	})

	impactedCheck := &trainingoperator.ImpactedWorkloadsCheck{}
	result, err := impactedCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(trainingoperator.ConditionTypePyTorchJobsCompatible),
		"Status":  Equal(metav1.ConditionTrue),
		"Reason":  Equal(check.ReasonVersionCompatible),
		"Message": ContainSubstring("No PyTorchJob(s) found"),
	}))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "0"))
}

func TestImpactedWorkloadsCheck_ActiveJobs(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	activeJob := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.PyTorchJob.APIVersion(),
			"kind":       resources.PyTorchJob.Kind,
			"metadata": map[string]any{
				"name":      "active-pytorch-job",
				"namespace": "test-ns",
			},
			"status": map[string]any{
				"conditions": []any{
					map[string]any{
						"type":   "Running",
						"status": "True",
					},
				},
			},
		},
	}

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:     listKinds,
		Objects:       []*unstructured.Unstructured{activeJob},
		TargetVersion: "3.3.0",
	})

	impactedCheck := &trainingoperator.ImpactedWorkloadsCheck{}
	result, err := impactedCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(trainingoperator.ConditionTypePyTorchJobsCompatible),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonWorkloadsImpacted),
		"Message": And(ContainSubstring("Found 1 active PyTorchJob(s)"), ContainSubstring("deprecated TrainingOperator")),
	}))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactAdvisory))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "1"))
	g.Expect(result.ImpactedObjects).To(HaveLen(1))
}

func TestImpactedWorkloadsCheck_CompletedJobsSucceeded(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	completedJob := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.PyTorchJob.APIVersion(),
			"kind":       resources.PyTorchJob.Kind,
			"metadata": map[string]any{
				"name":      "completed-pytorch-job",
				"namespace": "test-ns",
			},
			"status": map[string]any{
				"conditions": []any{
					map[string]any{
						"type":   "Succeeded",
						"status": "True",
					},
				},
			},
		},
	}

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:     listKinds,
		Objects:       []*unstructured.Unstructured{completedJob},
		TargetVersion: "3.3.0",
	})

	impactedCheck := &trainingoperator.ImpactedWorkloadsCheck{}
	result, err := impactedCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(trainingoperator.ConditionTypePyTorchJobsCompatible),
		"Status":  Equal(metav1.ConditionTrue),
		"Reason":  Equal(check.ReasonVersionCompatible),
		"Message": ContainSubstring("Found 1 completed PyTorchJob(s)"),
	}))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "1"))
	g.Expect(result.ImpactedObjects).To(HaveLen(1))
}

func TestImpactedWorkloadsCheck_CompletedJobsFailed(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	failedJob := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.PyTorchJob.APIVersion(),
			"kind":       resources.PyTorchJob.Kind,
			"metadata": map[string]any{
				"name":      "failed-pytorch-job",
				"namespace": "test-ns",
			},
			"status": map[string]any{
				"conditions": []any{
					map[string]any{
						"type":   "Failed",
						"status": "True",
					},
				},
			},
		},
	}

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:     listKinds,
		Objects:       []*unstructured.Unstructured{failedJob},
		TargetVersion: "3.3.0",
	})

	impactedCheck := &trainingoperator.ImpactedWorkloadsCheck{}
	result, err := impactedCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(trainingoperator.ConditionTypePyTorchJobsCompatible),
		"Status":  Equal(metav1.ConditionTrue),
		"Reason":  Equal(check.ReasonVersionCompatible),
		"Message": ContainSubstring("Found 1 completed PyTorchJob(s)"),
	}))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "1"))
	g.Expect(result.ImpactedObjects).To(HaveLen(1))
}

func TestImpactedWorkloadsCheck_MixedActiveAndCompleted(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	activeJob1 := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.PyTorchJob.APIVersion(),
			"kind":       resources.PyTorchJob.Kind,
			"metadata": map[string]any{
				"name":      "active-job-1",
				"namespace": "ns1",
			},
			"status": map[string]any{
				"conditions": []any{
					map[string]any{
						"type":   "Running",
						"status": "True",
					},
				},
			},
		},
	}

	activeJob2 := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.PyTorchJob.APIVersion(),
			"kind":       resources.PyTorchJob.Kind,
			"metadata": map[string]any{
				"name":      "active-job-2",
				"namespace": "ns2",
			},
			"status": map[string]any{
				"conditions": []any{
					map[string]any{
						"type":   "Created",
						"status": "True",
					},
				},
			},
		},
	}

	completedJob := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.PyTorchJob.APIVersion(),
			"kind":       resources.PyTorchJob.Kind,
			"metadata": map[string]any{
				"name":      "completed-job",
				"namespace": "ns1",
			},
			"status": map[string]any{
				"conditions": []any{
					map[string]any{
						"type":   "Succeeded",
						"status": "True",
					},
				},
			},
		},
	}

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:     listKinds,
		Objects:       []*unstructured.Unstructured{activeJob1, activeJob2, completedJob},
		TargetVersion: "3.3.0",
	})

	impactedCheck := &trainingoperator.ImpactedWorkloadsCheck{}
	result, err := impactedCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(trainingoperator.ConditionTypePyTorchJobsCompatible),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonWorkloadsImpacted),
		"Message": And(ContainSubstring("Found 3 PyTorchJob(s)"), ContainSubstring("2 active"), ContainSubstring("1 completed")),
	}))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactAdvisory))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "3"))
	g.Expect(result.ImpactedObjects).To(HaveLen(3))
}

func TestImpactedWorkloadsCheck_JobWithoutStatus(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	jobWithoutStatus := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.PyTorchJob.APIVersion(),
			"kind":       resources.PyTorchJob.Kind,
			"metadata": map[string]any{
				"name":      "job-without-status",
				"namespace": "test-ns",
			},
		},
	}

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:     listKinds,
		Objects:       []*unstructured.Unstructured{jobWithoutStatus},
		TargetVersion: "3.3.0",
	})

	impactedCheck := &trainingoperator.ImpactedWorkloadsCheck{}
	result, err := impactedCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(trainingoperator.ConditionTypePyTorchJobsCompatible),
		"Status": Equal(metav1.ConditionFalse),
	}))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactAdvisory))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "1"))
	g.Expect(result.ImpactedObjects).To(HaveLen(1))
}

func TestImpactedWorkloadsCheck_Metadata(t *testing.T) {
	g := NewWithT(t)

	impactedCheck := trainingoperator.NewImpactedWorkloadsCheck()

	g.Expect(impactedCheck.ID()).To(Equal("workloads.trainingoperator.impacted-workloads"))
	g.Expect(impactedCheck.Name()).To(Equal("Workloads :: TrainingOperator :: Impacted Workloads (3.3+)"))
	g.Expect(impactedCheck.Group()).To(Equal(check.GroupWorkload))
	g.Expect(impactedCheck.Description()).ToNot(BeEmpty())
}

func TestImpactedWorkloadsCheck_CanApply_Version32(t *testing.T) {
	g := NewWithT(t)

	ver := semver.MustParse("3.2.0")
	target := check.Target{
		TargetVersion: &ver,
	}

	impactedCheck := trainingoperator.NewImpactedWorkloadsCheck()
	canApply := impactedCheck.CanApply(t.Context(), target)

	g.Expect(canApply).To(BeFalse())
}

func TestImpactedWorkloadsCheck_CanApply_Version33(t *testing.T) {
	g := NewWithT(t)

	ver := semver.MustParse("3.3.0")
	target := check.Target{
		TargetVersion: &ver,
	}

	impactedCheck := trainingoperator.NewImpactedWorkloadsCheck()
	canApply := impactedCheck.CanApply(t.Context(), target)

	g.Expect(canApply).To(BeTrue())
}

func TestImpactedWorkloadsCheck_CanApply_Version34(t *testing.T) {
	g := NewWithT(t)

	ver := semver.MustParse("3.4.0")
	target := check.Target{
		TargetVersion: &ver,
	}

	impactedCheck := trainingoperator.NewImpactedWorkloadsCheck()
	canApply := impactedCheck.CanApply(t.Context(), target)

	g.Expect(canApply).To(BeTrue())
}
