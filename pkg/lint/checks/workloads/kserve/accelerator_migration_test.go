package kserve_test

import (
	"testing"

	"github.com/blang/semver/v4"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/lburgazzoli/odh-cli/pkg/lint/check"
	resultpkg "github.com/lburgazzoli/odh-cli/pkg/lint/check/result"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/shared/testutil"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/workloads/kserve"
	"github.com/lburgazzoli/odh-cli/pkg/resources"

	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

//nolint:gochecknoglobals // Test fixture - shared across test functions
var acceleratorListKinds = map[schema.GroupVersionResource]string{
	resources.InferenceService.GVR():   resources.InferenceService.ListKind(),
	resources.AcceleratorProfile.GVR(): resources.AcceleratorProfile.ListKind(),
	resources.DSCInitialization.GVR():  resources.DSCInitialization.ListKind(),
}

func TestAcceleratorMigrationCheck_NoInferenceServices(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      acceleratorListKinds,
		Objects:        []*unstructured.Unstructured{testutil.NewDSCI("redhat-ods-applications")},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	acceleratorCheck := kserve.NewAcceleratorMigrationCheck()
	result, err := acceleratorCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(kserve.ConditionTypeISVCAcceleratorProfileCompatible),
		"Status":  Equal(metav1.ConditionTrue),
		"Reason":  Equal(check.ReasonVersionCompatible),
		"Message": ContainSubstring("No InferenceServices found"),
	}))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "0"))
	g.Expect(result.ImpactedObjects).To(BeEmpty())
}

func TestAcceleratorMigrationCheck_ISVCWithoutAcceleratorProfile(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	// InferenceService without accelerator annotations
	isvc := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.InferenceService.APIVersion(),
			"kind":       resources.InferenceService.Kind,
			"metadata": map[string]any{
				"name":      "test-isvc",
				"namespace": "test-ns",
			},
			"spec": map[string]any{},
		},
	}

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      acceleratorListKinds,
		Objects:        []*unstructured.Unstructured{testutil.NewDSCI("redhat-ods-applications"), isvc},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	acceleratorCheck := kserve.NewAcceleratorMigrationCheck()
	result, err := acceleratorCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(kserve.ConditionTypeISVCAcceleratorProfileCompatible),
		"Status": Equal(metav1.ConditionTrue),
		"Reason": Equal(check.ReasonVersionCompatible),
	}))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "0"))
	g.Expect(result.ImpactedObjects).To(BeEmpty())
}

func TestAcceleratorMigrationCheck_ISVCWithExistingAcceleratorProfile(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	// AcceleratorProfile that exists
	profile := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.AcceleratorProfile.APIVersion(),
			"kind":       resources.AcceleratorProfile.Kind,
			"metadata": map[string]any{
				"name":      "nvidia-gpu",
				"namespace": "redhat-ods-applications",
			},
		},
	}

	// InferenceService referencing existing AcceleratorProfile
	isvc := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.InferenceService.APIVersion(),
			"kind":       resources.InferenceService.Kind,
			"metadata": map[string]any{
				"name":      "gpu-isvc",
				"namespace": "user-ns",
				"annotations": map[string]any{
					"opendatahub.io/accelerator-name":              "nvidia-gpu",
					"opendatahub.io/accelerator-profile-namespace": "redhat-ods-applications",
				},
			},
			"spec": map[string]any{},
		},
	}

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      acceleratorListKinds,
		Objects:        []*unstructured.Unstructured{testutil.NewDSCI("redhat-ods-applications"), isvc, profile},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	acceleratorCheck := kserve.NewAcceleratorMigrationCheck()
	result, err := acceleratorCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(kserve.ConditionTypeISVCAcceleratorProfileCompatible),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonConfigurationInvalid),
		"Message": And(ContainSubstring("Found 1 InferenceService(s)"), ContainSubstring("HardwareProfiles")),
	}))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactAdvisory))
	g.Expect(result.Status.Conditions[0].Remediation).To(ContainSubstring("HardwareProfiles"))
	g.Expect(result.GetRemediation()).To(ContainSubstring("HardwareProfiles"))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "1"))
	g.Expect(result.ImpactedObjects).To(HaveLen(1))
	g.Expect(result.ImpactedObjects[0].Name).To(Equal("gpu-isvc"))
	g.Expect(result.ImpactedObjects[0].Namespace).To(Equal("user-ns"))
}

func TestAcceleratorMigrationCheck_ISVCWithMissingAcceleratorProfile(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	// InferenceService referencing non-existent AcceleratorProfile
	isvc := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.InferenceService.APIVersion(),
			"kind":       resources.InferenceService.Kind,
			"metadata": map[string]any{
				"name":      "broken-isvc",
				"namespace": "user-ns",
				"annotations": map[string]any{
					"opendatahub.io/accelerator-name":              "missing-profile",
					"opendatahub.io/accelerator-profile-namespace": "some-ns",
				},
			},
			"spec": map[string]any{},
		},
	}

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      acceleratorListKinds,
		Objects:        []*unstructured.Unstructured{testutil.NewDSCI("redhat-ods-applications"), isvc},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	acceleratorCheck := kserve.NewAcceleratorMigrationCheck()
	result, err := acceleratorCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(kserve.ConditionTypeISVCAcceleratorProfileCompatible),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonResourceNotFound),
		"Message": And(ContainSubstring("1 missing"), ContainSubstring("ensure AcceleratorProfiles exist")),
	}))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactAdvisory))
	g.Expect(result.Status.Conditions[0].Remediation).To(ContainSubstring("HardwareProfiles"))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "1"))
	g.Expect(result.ImpactedObjects).To(HaveLen(1))
}

func TestAcceleratorMigrationCheck_MixedInferenceServices(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	// Existing AcceleratorProfile
	profile := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.AcceleratorProfile.APIVersion(),
			"kind":       resources.AcceleratorProfile.Kind,
			"metadata": map[string]any{
				"name":      "nvidia-gpu",
				"namespace": "redhat-ods-applications",
			},
		},
	}

	// InferenceService without accelerator
	isvc1 := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.InferenceService.APIVersion(),
			"kind":       resources.InferenceService.Kind,
			"metadata": map[string]any{
				"name":      "plain-isvc",
				"namespace": "ns1",
			},
			"spec": map[string]any{},
		},
	}

	// InferenceService with existing accelerator
	isvc2 := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.InferenceService.APIVersion(),
			"kind":       resources.InferenceService.Kind,
			"metadata": map[string]any{
				"name":      "gpu-isvc",
				"namespace": "ns2",
				"annotations": map[string]any{
					"opendatahub.io/accelerator-name":              "nvidia-gpu",
					"opendatahub.io/accelerator-profile-namespace": "redhat-ods-applications",
				},
			},
			"spec": map[string]any{},
		},
	}

	// InferenceService with missing accelerator
	isvc3 := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.InferenceService.APIVersion(),
			"kind":       resources.InferenceService.Kind,
			"metadata": map[string]any{
				"name":      "broken-isvc",
				"namespace": "ns3",
				"annotations": map[string]any{
					"opendatahub.io/accelerator-name":              "missing-profile",
					"opendatahub.io/accelerator-profile-namespace": "some-ns",
				},
			},
			"spec": map[string]any{},
		},
	}

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      acceleratorListKinds,
		Objects:        []*unstructured.Unstructured{testutil.NewDSCI("redhat-ods-applications"), isvc1, isvc2, isvc3, profile},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	acceleratorCheck := kserve.NewAcceleratorMigrationCheck()
	result, err := acceleratorCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(kserve.ConditionTypeISVCAcceleratorProfileCompatible),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonResourceNotFound),
		"Message": And(ContainSubstring("2 InferenceService(s)"), ContainSubstring("1 missing")),
	}))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactAdvisory))
	g.Expect(result.Status.Conditions[0].Remediation).To(ContainSubstring("HardwareProfiles"))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "2"))
	g.Expect(result.ImpactedObjects).To(HaveLen(2))
}

func TestAcceleratorMigrationCheck_Metadata(t *testing.T) {
	g := NewWithT(t)

	acceleratorCheck := kserve.NewAcceleratorMigrationCheck()

	g.Expect(acceleratorCheck.ID()).To(Equal("workloads.kserve.accelerator-migration"))
	g.Expect(acceleratorCheck.Name()).To(Equal("Workloads :: KServe :: AcceleratorProfile Migration (3.x)"))
	g.Expect(acceleratorCheck.Group()).To(Equal(check.GroupWorkload))
	g.Expect(acceleratorCheck.Description()).ToNot(BeEmpty())
	g.Expect(acceleratorCheck.Remediation()).To(ContainSubstring("HardwareProfiles"))
}

func TestAcceleratorMigrationCheck_CanApply_LintMode2x(t *testing.T) {
	g := NewWithT(t)

	currentVer := semver.MustParse("2.17.0")
	target := check.Target{
		CurrentVersion: &currentVer,
		TargetVersion:  &currentVer,
	}

	acceleratorCheck := kserve.NewAcceleratorMigrationCheck()
	canApply, err := acceleratorCheck.CanApply(t.Context(), target)
	g.Expect(err).ToNot(HaveOccurred())
	// Lint mode at 2.x should not apply
	g.Expect(canApply).To(BeFalse())
}

func TestAcceleratorMigrationCheck_CanApply_LintMode3x(t *testing.T) {
	g := NewWithT(t)

	currentVer := semver.MustParse("3.0.0")
	target := check.Target{
		CurrentVersion: &currentVer,
		TargetVersion:  &currentVer,
	}

	acceleratorCheck := kserve.NewAcceleratorMigrationCheck()
	canApply, err := acceleratorCheck.CanApply(t.Context(), target)
	g.Expect(err).ToNot(HaveOccurred())
	// Lint mode at 3.x should not apply — this check is only for 2.x→3.x upgrades.
	g.Expect(canApply).To(BeFalse())
}

func TestAcceleratorMigrationCheck_CanApply_UpgradeTo3x(t *testing.T) {
	g := NewWithT(t)

	currentVer := semver.MustParse("2.17.0")
	targetVer := semver.MustParse("3.0.0")
	target := check.Target{
		CurrentVersion: &currentVer,
		TargetVersion:  &targetVer,
	}

	acceleratorCheck := kserve.NewAcceleratorMigrationCheck()
	canApply, err := acceleratorCheck.CanApply(t.Context(), target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeTrue())
}

func TestAcceleratorMigrationCheck_CanApply_NilVersions(t *testing.T) {
	g := NewWithT(t)

	target := check.Target{
		CurrentVersion: nil,
		TargetVersion:  nil,
	}

	acceleratorCheck := kserve.NewAcceleratorMigrationCheck()
	canApply, err := acceleratorCheck.CanApply(t.Context(), target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeFalse())
}

func TestAcceleratorMigrationCheck_AnnotationTargetVersion(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      acceleratorListKinds,
		Objects:        []*unstructured.Unstructured{testutil.NewDSCI("redhat-ods-applications")},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	acceleratorCheck := kserve.NewAcceleratorMigrationCheck()
	result, err := acceleratorCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationCheckTargetVersion, "3.0.0"))
}

func TestAcceleratorMigrationCheck_DefaultNamespace(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	// AcceleratorProfile in the applications namespace
	profile := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.AcceleratorProfile.APIVersion(),
			"kind":       resources.AcceleratorProfile.Kind,
			"metadata": map[string]any{
				"name":      "my-gpu",
				"namespace": "redhat-ods-applications",
			},
		},
	}

	// InferenceService with accelerator name but no namespace annotation (should default to applications namespace)
	isvc := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.InferenceService.APIVersion(),
			"kind":       resources.InferenceService.Kind,
			"metadata": map[string]any{
				"name":      "isvc-default-ns",
				"namespace": "user-ns",
				"annotations": map[string]any{
					"opendatahub.io/accelerator-name": "my-gpu",
					// No namespace annotation - should default to applications namespace
				},
			},
			"spec": map[string]any{},
		},
	}

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      acceleratorListKinds,
		Objects:        []*unstructured.Unstructured{testutil.NewDSCI("redhat-ods-applications"), isvc, profile},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	acceleratorCheck := kserve.NewAcceleratorMigrationCheck()
	result, err := acceleratorCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	// Profile exists in applications namespace (resolved via DSCI), so should be advisory (not missing)
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(kserve.ConditionTypeISVCAcceleratorProfileCompatible),
		"Status": Equal(metav1.ConditionFalse),
		"Reason": Equal(check.ReasonConfigurationInvalid),
	}))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactAdvisory))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "1"))
}
