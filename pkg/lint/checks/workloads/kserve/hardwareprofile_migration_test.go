package kserve_test

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	resultpkg "github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/testutil"
	"github.com/opendatahub-io/odh-cli/pkg/lint/checks/workloads/kserve"
	"github.com/opendatahub-io/odh-cli/pkg/resources"

	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

//nolint:gochecknoglobals // Test fixture - shared across test functions
var hardwareProfileListKinds = map[schema.GroupVersionResource]string{
	resources.InferenceService.GVR():   resources.InferenceService.ListKind(),
	resources.DataScienceCluster.GVR(): resources.DataScienceCluster.ListKind(),
}

func TestHardwareProfileMigration_NoInferenceServices(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      hardwareProfileListKinds,
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := kserve.NewHardwareProfileMigrationCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(kserve.ConditionTypeISVCHardwareProfileCompatible),
		"Status":  Equal(metav1.ConditionTrue),
		"Reason":  Equal(check.ReasonNoMigrationRequired),
		"Message": ContainSubstring("No InferenceServices found"),
	}))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "0"))
	g.Expect(result.ImpactedObjects).To(BeEmpty())
}

func TestHardwareProfileMigration_ISVCWithoutAnnotation(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	isvc := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.InferenceService.APIVersion(),
			"kind":       resources.InferenceService.Kind,
			"metadata": map[string]any{
				"name":      "test-isvc",
				"namespace": "test-ns",
			},
		},
	}

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      hardwareProfileListKinds,
		Objects:        []*unstructured.Unstructured{isvc},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := kserve.NewHardwareProfileMigrationCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(kserve.ConditionTypeISVCHardwareProfileCompatible),
		"Status": Equal(metav1.ConditionTrue),
		"Reason": Equal(check.ReasonNoMigrationRequired),
	}))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "0"))
	g.Expect(result.ImpactedObjects).To(BeEmpty())
}

func TestHardwareProfileMigration_ISVCWithEmptyAnnotation(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	isvc := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.InferenceService.APIVersion(),
			"kind":       resources.InferenceService.Kind,
			"metadata": map[string]any{
				"name":      "test-isvc",
				"namespace": "test-ns",
				"annotations": map[string]any{
					"opendatahub.io/legacy-hardware-profile-name": "",
				},
			},
		},
	}

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      hardwareProfileListKinds,
		Objects:        []*unstructured.Unstructured{isvc},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := kserve.NewHardwareProfileMigrationCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(kserve.ConditionTypeISVCHardwareProfileCompatible),
		"Status": Equal(metav1.ConditionTrue),
		"Reason": Equal(check.ReasonNoMigrationRequired),
	}))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "0"))
	g.Expect(result.ImpactedObjects).To(BeEmpty())
}

func TestHardwareProfileMigration_ISVCWithAnnotation(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	isvc := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.InferenceService.APIVersion(),
			"kind":       resources.InferenceService.Kind,
			"metadata": map[string]any{
				"name":      "legacy-isvc",
				"namespace": "user-ns",
				"annotations": map[string]any{
					"opendatahub.io/legacy-hardware-profile-name": "old-profile",
				},
			},
		},
	}

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      hardwareProfileListKinds,
		Objects:        []*unstructured.Unstructured{isvc},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := kserve.NewHardwareProfileMigrationCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(kserve.ConditionTypeISVCHardwareProfileCompatible),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonMigrationPending),
		"Message": ContainSubstring("Found 1 InferenceService(s)"),
	}))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactAdvisory))
	g.Expect(result.Status.Conditions[0].Remediation).To(ContainSubstring("HardwareProfiles"))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "1"))
	g.Expect(result.ImpactedObjects).To(HaveLen(1))
	g.Expect(result.ImpactedObjects[0].Name).To(Equal("legacy-isvc"))
	g.Expect(result.ImpactedObjects[0].Namespace).To(Equal("user-ns"))
}

func TestHardwareProfileMigration_MixedInferenceServices(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	// InferenceService without annotation
	isvc1 := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.InferenceService.APIVersion(),
			"kind":       resources.InferenceService.Kind,
			"metadata": map[string]any{
				"name":      "plain-isvc",
				"namespace": "ns1",
			},
		},
	}

	// InferenceService with legacy annotation
	isvc2 := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.InferenceService.APIVersion(),
			"kind":       resources.InferenceService.Kind,
			"metadata": map[string]any{
				"name":      "legacy-isvc-1",
				"namespace": "ns2",
				"annotations": map[string]any{
					"opendatahub.io/legacy-hardware-profile-name": "old-profile-a",
				},
			},
		},
	}

	// Another InferenceService with legacy annotation
	isvc3 := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.InferenceService.APIVersion(),
			"kind":       resources.InferenceService.Kind,
			"metadata": map[string]any{
				"name":      "legacy-isvc-2",
				"namespace": "ns3",
				"annotations": map[string]any{
					"opendatahub.io/legacy-hardware-profile-name": "old-profile-b",
				},
			},
		},
	}

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      hardwareProfileListKinds,
		Objects:        []*unstructured.Unstructured{isvc1, isvc2, isvc3},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := kserve.NewHardwareProfileMigrationCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(kserve.ConditionTypeISVCHardwareProfileCompatible),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonMigrationPending),
		"Message": ContainSubstring("Found 2 InferenceService(s)"),
	}))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactAdvisory))
	g.Expect(result.Status.Conditions[0].Remediation).To(ContainSubstring("HardwareProfiles"))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "2"))
	g.Expect(result.ImpactedObjects).To(HaveLen(2))
}

func TestHardwareProfileMigration_CanApply_Managed(t *testing.T) {
	g := NewWithT(t)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      hardwareProfileListKinds,
		Objects:        []*unstructured.Unstructured{testutil.NewDSC(map[string]string{"kserve": "Managed"})},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := kserve.NewHardwareProfileMigrationCheck()
	canApply, err := chk.CanApply(t.Context(), target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeTrue())
}

func TestHardwareProfileMigration_CanApply_Removed(t *testing.T) {
	g := NewWithT(t)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      hardwareProfileListKinds,
		Objects:        []*unstructured.Unstructured{testutil.NewDSC(map[string]string{"kserve": "Removed"})},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := kserve.NewHardwareProfileMigrationCheck()
	canApply, err := chk.CanApply(t.Context(), target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeFalse())
}

func TestHardwareProfileMigration_Metadata(t *testing.T) {
	g := NewWithT(t)

	chk := kserve.NewHardwareProfileMigrationCheck()

	g.Expect(chk.ID()).To(Equal("workloads.kserve.hardwareprofile-migration"))
	g.Expect(chk.Name()).To(Equal("Workloads :: KServe :: Legacy HardwareProfile Migration"))
	g.Expect(chk.Group()).To(Equal(check.GroupWorkload))
	g.Expect(chk.Description()).ToNot(BeEmpty())
	g.Expect(chk.Remediation()).To(ContainSubstring("HardwareProfiles"))
}
