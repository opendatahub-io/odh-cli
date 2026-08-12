package trainer_test

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	resultpkg "github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/testutil"
	"github.com/opendatahub-io/odh-cli/pkg/lint/checks/workloads/trainer"
	"github.com/opendatahub-io/odh-cli/pkg/resources"

	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

//nolint:gochecknoglobals
var listKinds = map[schema.GroupVersionResource]string{
	resources.TrainJob.GVR():           resources.TrainJob.ListKind(),
	resources.DataScienceCluster.GVR(): resources.DataScienceCluster.ListKind(),
}

func managedTrainerDSC() *unstructured.Unstructured {
	return testutil.NewDSC(map[string]string{"trainer": "Managed"})
}

func newTrainJob(name, namespace string, spec map[string]any) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.TrainJob.APIVersion(),
			"kind":       resources.TrainJob.Kind,
			"metadata": map[string]any{
				"name":      name,
				"namespace": namespace,
			},
			"spec": spec,
		},
	}
}

func TestPodTemplateOverridesCheck_Metadata(t *testing.T) {
	g := NewWithT(t)

	chk := trainer.NewPodTemplateOverridesCheck()

	g.Expect(chk.ID()).To(Equal("workloads.trainer.podtemplateoverrides"))
	g.Expect(chk.Name()).To(Equal("Workloads :: Trainer :: PodTemplateOverrides (3.6+)"))
	g.Expect(chk.Group()).To(Equal(check.GroupWorkload))
	g.Expect(chk.CheckKind()).To(Equal("trainer"))
	g.Expect(chk.Description()).ToNot(BeEmpty())
}

func TestPodTemplateOverridesCheck_CanApply_TargetBelow36(t *testing.T) {
	g := NewWithT(t)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{managedTrainerDSC()},
		CurrentVersion: "3.5.0",
		TargetVersion:  "3.5.0",
	})

	canApply, err := trainer.NewPodTemplateOverridesCheck().CanApply(t.Context(), target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeFalse())
}

func TestPodTemplateOverridesCheck_CanApply_CurrentAlready36(t *testing.T) {
	g := NewWithT(t)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{managedTrainerDSC()},
		CurrentVersion: "3.6.0",
		TargetVersion:  "3.6.0",
	})

	canApply, err := trainer.NewPodTemplateOverridesCheck().CanApply(t.Context(), target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeFalse())
}

func TestPodTemplateOverridesCheck_CanApply_TrainerRemoved(t *testing.T) {
	g := NewWithT(t)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds: listKinds,
		Objects: []*unstructured.Unstructured{
			testutil.NewDSC(map[string]string{"trainer": "Removed"}),
		},
		CurrentVersion: "3.5.0",
		TargetVersion:  "3.6.0",
	})

	canApply, err := trainer.NewPodTemplateOverridesCheck().CanApply(t.Context(), target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeFalse())
}

func TestPodTemplateOverridesCheck_CanApply_Upgrade35To36_Managed(t *testing.T) {
	g := NewWithT(t)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{managedTrainerDSC()},
		CurrentVersion: "3.5.0",
		TargetVersion:  "3.6.0",
	})

	canApply, err := trainer.NewPodTemplateOverridesCheck().CanApply(t.Context(), target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeTrue())
}

func TestPodTemplateOverridesCheck_NoTrainJobs(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{managedTrainerDSC()},
		CurrentVersion: "3.5.0",
		TargetVersion:  "3.6.0",
	})

	result, err := trainer.NewPodTemplateOverridesCheck().Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(trainer.ConditionTypePodTemplateOverridesCompatible),
		"Status":  Equal(metav1.ConditionTrue),
		"Reason":  Equal(check.ReasonVersionCompatible),
		"Message": ContainSubstring("No TrainJob(s) with podTemplateOverrides found"),
	}))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "0"))
	g.Expect(result.ImpactedObjects).To(BeEmpty())
}

func TestPodTemplateOverridesCheck_TrainJobWithoutOverrides(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	job := newTrainJob("plain-job", "test-ns", map[string]any{
		"runtimeRef": map[string]any{
			"name": "torch-distributed",
		},
	})

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{managedTrainerDSC(), job},
		CurrentVersion: "3.5.0",
		TargetVersion:  "3.6.0",
	})

	result, err := trainer.NewPodTemplateOverridesCheck().Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(trainer.ConditionTypePodTemplateOverridesCompatible),
		"Status": Equal(metav1.ConditionTrue),
		"Reason": Equal(check.ReasonVersionCompatible),
	}))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "0"))
	g.Expect(result.ImpactedObjects).To(BeEmpty())
}

func TestPodTemplateOverridesCheck_TrainJobWithEmptyOverrides(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	job := newTrainJob("empty-overrides", "test-ns", map[string]any{
		"podTemplateOverrides": []any{},
	})

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{managedTrainerDSC(), job},
		CurrentVersion: "3.5.0",
		TargetVersion:  "3.6.0",
	})

	result, err := trainer.NewPodTemplateOverridesCheck().Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(trainer.ConditionTypePodTemplateOverridesCompatible),
		"Status": Equal(metav1.ConditionTrue),
		"Reason": Equal(check.ReasonVersionCompatible),
	}))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "0"))
}

func TestPodTemplateOverridesCheck_TrainJobWithOverrides(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	job := newTrainJob("legacy-job", "test-ns", map[string]any{
		"podTemplateOverrides": []any{
			map[string]any{
				"targetJobs": []any{"node"},
				"spec": map[string]any{
					"serviceAccountName": "training-sa",
				},
			},
		},
	})

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{managedTrainerDSC(), job},
		CurrentVersion: "3.5.0",
		TargetVersion:  "3.6.0",
	})

	result, err := trainer.NewPodTemplateOverridesCheck().Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(trainer.ConditionTypePodTemplateOverridesCompatible),
		"Status": Equal(metav1.ConditionFalse),
		"Reason": Equal(check.ReasonWorkloadsImpacted),
		"Message": And(
			ContainSubstring("Found 1 TrainJob(s) that use podTemplateOverrides"),
			ContainSubstring("We recommend waiting for these jobs to complete before upgrading when possible"),
			ContainSubstring("recreate TrainJobs using runtimePatches"),
			ContainSubstring("https://access.redhat.com/articles/7146204"),
		),
	}))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactAdvisory))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "1"))
	g.Expect(result.ImpactedObjects).To(HaveLen(1))
	g.Expect(result.ImpactedObjects[0].Name).To(Equal("legacy-job"))
	g.Expect(result.ImpactedObjects[0].Namespace).To(Equal("test-ns"))
}

func TestPodTemplateOverridesCheck_MixedTrainJobs(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	legacyJob := newTrainJob("legacy-job", "ns1", map[string]any{
		"podTemplateOverrides": []any{
			map[string]any{
				"targetJobs": []any{"node"},
			},
		},
	})
	plainJob := newTrainJob("plain-job", "ns1", map[string]any{
		"runtimeRef": map[string]any{"name": "torch-distributed"},
	})

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{managedTrainerDSC(), legacyJob, plainJob},
		CurrentVersion: "3.5.0",
		TargetVersion:  "3.6.0",
	})

	result, err := trainer.NewPodTemplateOverridesCheck().Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(trainer.ConditionTypePodTemplateOverridesCompatible),
		"Status":  Equal(metav1.ConditionFalse),
		"Message": ContainSubstring("Found 1 TrainJob(s) that use podTemplateOverrides"),
	}))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "1"))
	g.Expect(result.ImpactedObjects).To(HaveLen(1))
	g.Expect(result.ImpactedObjects[0].Name).To(Equal("legacy-job"))
}
