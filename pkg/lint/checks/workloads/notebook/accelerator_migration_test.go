package notebook_test

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	resultpkg "github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/testutil"
	"github.com/opendatahub-io/odh-cli/pkg/lint/checks/workloads/notebook"
	"github.com/opendatahub-io/odh-cli/pkg/resources"

	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

//nolint:gochecknoglobals
var acceleratorListKinds = map[schema.GroupVersionResource]string{
	resources.Notebook.GVR():           resources.Notebook.ListKind(),
	resources.AcceleratorProfile.GVR(): resources.AcceleratorProfile.ListKind(),
	resources.DSCInitialization.GVR():  resources.DSCInitialization.ListKind(),
	resources.DataScienceCluster.GVR(): resources.DataScienceCluster.ListKind(),
}

func TestAcceleratorMigrationCheck_NoNotebooks(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      acceleratorListKinds,
		Objects:        []*unstructured.Unstructured{testutil.NewDSCI("redhat-ods-applications")},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	acceleratorCheck := notebook.NewAcceleratorMigrationCheck()
	result, err := acceleratorCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(notebook.ConditionTypeAcceleratorProfileCompatible),
		"Status":  Equal(metav1.ConditionTrue),
		"Reason":  Equal(check.ReasonVersionCompatible),
		"Message": ContainSubstring("No Notebooks found"),
	}))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "0"))
	g.Expect(result.ImpactedObjects).To(BeEmpty())
}

func TestAcceleratorMigrationCheck_NotebookWithoutAcceleratorProfile(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	// Notebook without accelerator annotations
	nb := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.Notebook.APIVersion(),
			"kind":       resources.Notebook.Kind,
			"metadata": map[string]any{
				"name":      "test-notebook",
				"namespace": "test-ns",
			},
			"spec": map[string]any{},
		},
	}

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      acceleratorListKinds,
		Objects:        []*unstructured.Unstructured{testutil.NewDSCI("redhat-ods-applications"), nb},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	acceleratorCheck := notebook.NewAcceleratorMigrationCheck()
	result, err := acceleratorCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(notebook.ConditionTypeAcceleratorProfileCompatible),
		"Status": Equal(metav1.ConditionTrue),
		"Reason": Equal(check.ReasonVersionCompatible),
	}))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "0"))
	g.Expect(result.ImpactedObjects).To(BeEmpty())
}

func TestAcceleratorMigrationCheck_NotebookWithExistingAcceleratorProfile(t *testing.T) {
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

	// Notebook referencing existing AcceleratorProfile
	nb := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.Notebook.APIVersion(),
			"kind":       resources.Notebook.Kind,
			"metadata": map[string]any{
				"name":      "gpu-notebook",
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
		Objects:        []*unstructured.Unstructured{testutil.NewDSCI("redhat-ods-applications"), nb, profile},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	acceleratorCheck := notebook.NewAcceleratorMigrationCheck()
	result, err := acceleratorCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(notebook.ConditionTypeAcceleratorProfileCompatible),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonConfigurationInvalid),
		"Message": And(ContainSubstring("Found 1 Notebook(s)"), ContainSubstring("HardwareProfiles")),
	}))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactAdvisory))
	g.Expect(result.Status.Conditions[0].Remediation).To(ContainSubstring("HardwareProfiles"))
	g.Expect(result.GetRemediation()).To(ContainSubstring("HardwareProfiles"))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "1"))
	g.Expect(result.ImpactedObjects).To(HaveLen(1))
	g.Expect(result.ImpactedObjects[0].Name).To(Equal("gpu-notebook"))
	g.Expect(result.ImpactedObjects[0].Namespace).To(Equal("user-ns"))
}

func TestAcceleratorMigrationCheck_NotebookWithMissingAcceleratorProfile(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	// Notebook referencing non-existent AcceleratorProfile
	nb := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.Notebook.APIVersion(),
			"kind":       resources.Notebook.Kind,
			"metadata": map[string]any{
				"name":      "broken-notebook",
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
		Objects:        []*unstructured.Unstructured{testutil.NewDSCI("redhat-ods-applications"), nb},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	acceleratorCheck := notebook.NewAcceleratorMigrationCheck()
	result, err := acceleratorCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(notebook.ConditionTypeAcceleratorProfileCompatible),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonResourceNotFound),
		"Message": And(ContainSubstring("1 missing"), ContainSubstring("automatically migrated to HardwareProfiles")),
	}))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactAdvisory))
	g.Expect(result.Status.Conditions[0].Remediation).To(ContainSubstring("HardwareProfiles"))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "1"))
	g.Expect(result.ImpactedObjects).To(HaveLen(1))
}

func TestAcceleratorMigrationCheck_MixedNotebooks(t *testing.T) {
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

	// Notebook without accelerator
	nb1 := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.Notebook.APIVersion(),
			"kind":       resources.Notebook.Kind,
			"metadata": map[string]any{
				"name":      "plain-notebook",
				"namespace": "ns1",
			},
			"spec": map[string]any{},
		},
	}

	// Notebook with existing accelerator
	nb2 := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.Notebook.APIVersion(),
			"kind":       resources.Notebook.Kind,
			"metadata": map[string]any{
				"name":      "gpu-notebook",
				"namespace": "ns2",
				"annotations": map[string]any{
					"opendatahub.io/accelerator-name":              "nvidia-gpu",
					"opendatahub.io/accelerator-profile-namespace": "redhat-ods-applications",
				},
			},
			"spec": map[string]any{},
		},
	}

	// Notebook with missing accelerator
	nb3 := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.Notebook.APIVersion(),
			"kind":       resources.Notebook.Kind,
			"metadata": map[string]any{
				"name":      "broken-notebook",
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
		Objects:        []*unstructured.Unstructured{testutil.NewDSCI("redhat-ods-applications"), nb1, nb2, nb3, profile},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	acceleratorCheck := notebook.NewAcceleratorMigrationCheck()
	result, err := acceleratorCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(notebook.ConditionTypeAcceleratorProfileCompatible),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonResourceNotFound),
		"Message": And(ContainSubstring("2 Notebook(s)"), ContainSubstring("1 missing")),
	}))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactAdvisory))
	g.Expect(result.Status.Conditions[0].Remediation).To(ContainSubstring("HardwareProfiles"))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "2"))
	g.Expect(result.ImpactedObjects).To(HaveLen(2))
}

func TestAcceleratorMigrationCheck_Metadata(t *testing.T) {
	g := NewWithT(t)

	acceleratorCheck := notebook.NewAcceleratorMigrationCheck()

	g.Expect(acceleratorCheck.ID()).To(Equal("workloads.notebook.accelerator-migration"))
	g.Expect(acceleratorCheck.Name()).To(Equal("Workloads :: Notebook :: AcceleratorProfile Migration (3.x)"))
	g.Expect(acceleratorCheck.Group()).To(Equal(check.GroupWorkload))
	g.Expect(acceleratorCheck.Description()).ToNot(BeEmpty())
	g.Expect(acceleratorCheck.Remediation()).To(ContainSubstring("HardwareProfiles"))
}

func TestAcceleratorMigrationCheck_CanApply_NilVersions(t *testing.T) {
	g := NewWithT(t)

	chk := notebook.NewAcceleratorMigrationCheck()
	canApply, err := chk.CanApply(t.Context(), check.Target{})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeFalse())
}

func TestAcceleratorMigrationCheck_CanApply_LintMode2x(t *testing.T) {
	g := NewWithT(t)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      acceleratorListKinds,
		Objects:        []*unstructured.Unstructured{testutil.NewDSC(map[string]string{"workbenches": "Managed"})},
		CurrentVersion: "2.17.0",
		TargetVersion:  "2.17.0",
	})

	chk := notebook.NewAcceleratorMigrationCheck()
	canApply, err := chk.CanApply(t.Context(), target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeFalse())
}

func TestAcceleratorMigrationCheck_CanApply_UpgradeTo3x_Managed(t *testing.T) {
	g := NewWithT(t)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      acceleratorListKinds,
		Objects:        []*unstructured.Unstructured{testutil.NewDSC(map[string]string{"workbenches": "Managed"})},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := notebook.NewAcceleratorMigrationCheck()
	canApply, err := chk.CanApply(t.Context(), target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeTrue())
}

func TestAcceleratorMigrationCheck_CanApply_UpgradeTo3x_Removed(t *testing.T) {
	g := NewWithT(t)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      acceleratorListKinds,
		Objects:        []*unstructured.Unstructured{testutil.NewDSC(map[string]string{"workbenches": "Removed"})},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := notebook.NewAcceleratorMigrationCheck()
	canApply, err := chk.CanApply(t.Context(), target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeFalse())
}

func TestAcceleratorMigrationCheck_CanApply_LintMode3x(t *testing.T) {
	g := NewWithT(t)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      acceleratorListKinds,
		Objects:        []*unstructured.Unstructured{testutil.NewDSC(map[string]string{"workbenches": "Managed"})},
		CurrentVersion: "3.0.0",
		TargetVersion:  "3.0.0",
	})

	chk := notebook.NewAcceleratorMigrationCheck()
	canApply, err := chk.CanApply(t.Context(), target)
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

	acceleratorCheck := notebook.NewAcceleratorMigrationCheck()
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

	// Notebook with accelerator name but no namespace annotation (should default to applications namespace)
	nb := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.Notebook.APIVersion(),
			"kind":       resources.Notebook.Kind,
			"metadata": map[string]any{
				"name":      "notebook-default-ns",
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
		Objects:        []*unstructured.Unstructured{testutil.NewDSCI("redhat-ods-applications"), nb, profile},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	acceleratorCheck := notebook.NewAcceleratorMigrationCheck()
	result, err := acceleratorCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	// Profile exists in applications namespace (resolved via DSCI), so should be advisory (not missing)
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(notebook.ConditionTypeAcceleratorProfileCompatible),
		"Status": Equal(metav1.ConditionFalse),
		"Reason": Equal(check.ReasonConfigurationInvalid),
	}))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactAdvisory))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "1"))
}
