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

//nolint:gochecknoglobals // Test fixture - shared across test functions
var hardwareProfileListKinds = map[schema.GroupVersionResource]string{
	resources.Notebook.GVR():           resources.Notebook.ListKind(),
	resources.DataScienceCluster.GVR(): resources.DataScienceCluster.ListKind(),
}

func TestHardwareProfileMigration_NoNotebooks(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      hardwareProfileListKinds,
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := notebook.NewHardwareProfileMigrationCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(notebook.ConditionTypeHardwareProfileCompatible),
		"Status":  Equal(metav1.ConditionTrue),
		"Reason":  Equal(check.ReasonNoMigrationRequired),
		"Message": ContainSubstring("No Notebooks found"),
	}))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "0"))
	g.Expect(result.ImpactedObjects).To(BeEmpty())
}

func TestHardwareProfileMigration_NotebookWithoutAnnotation(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	nb := newNotebook("test-notebook", "test-ns", notebookOptions{})

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      hardwareProfileListKinds,
		Objects:        []*unstructured.Unstructured{nb},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := notebook.NewHardwareProfileMigrationCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(notebook.ConditionTypeHardwareProfileCompatible),
		"Status": Equal(metav1.ConditionTrue),
		"Reason": Equal(check.ReasonNoMigrationRequired),
	}))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "0"))
	g.Expect(result.ImpactedObjects).To(BeEmpty())
}

func TestHardwareProfileMigration_NotebookWithEmptyAnnotation(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	nb := newNotebook("test-notebook", "test-ns", notebookOptions{
		Annotations: map[string]any{
			"opendatahub.io/legacy-hardware-profile-name": "",
		},
	})

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      hardwareProfileListKinds,
		Objects:        []*unstructured.Unstructured{nb},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := notebook.NewHardwareProfileMigrationCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(notebook.ConditionTypeHardwareProfileCompatible),
		"Status": Equal(metav1.ConditionTrue),
		"Reason": Equal(check.ReasonNoMigrationRequired),
	}))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "0"))
	g.Expect(result.ImpactedObjects).To(BeEmpty())
}

func TestHardwareProfileMigration_NotebookWithAnnotation(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	nb := newNotebook("legacy-notebook", "user-ns", notebookOptions{
		Annotations: map[string]any{
			"opendatahub.io/legacy-hardware-profile-name": "old-profile",
		},
	})

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      hardwareProfileListKinds,
		Objects:        []*unstructured.Unstructured{nb},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := notebook.NewHardwareProfileMigrationCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(notebook.ConditionTypeHardwareProfileCompatible),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonMigrationPending),
		"Message": ContainSubstring("Found 1 Notebook(s)"),
	}))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactAdvisory))
	g.Expect(result.Status.Conditions[0].Remediation).To(ContainSubstring("HardwareProfiles"))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "1"))
	g.Expect(result.ImpactedObjects).To(HaveLen(1))
	g.Expect(result.ImpactedObjects[0].Name).To(Equal("legacy-notebook"))
	g.Expect(result.ImpactedObjects[0].Namespace).To(Equal("user-ns"))
}

func TestHardwareProfileMigration_MixedNotebooks(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	// Notebook without annotation
	nb1 := newNotebook("plain-notebook", "ns1", notebookOptions{})

	// Notebook with legacy annotation
	nb2 := newNotebook("legacy-notebook-1", "ns2", notebookOptions{
		Annotations: map[string]any{
			"opendatahub.io/legacy-hardware-profile-name": "old-profile-a",
		},
	})

	// Another notebook with legacy annotation
	nb3 := newNotebook("legacy-notebook-2", "ns3", notebookOptions{
		Annotations: map[string]any{
			"opendatahub.io/legacy-hardware-profile-name": "old-profile-b",
		},
	})

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      hardwareProfileListKinds,
		Objects:        []*unstructured.Unstructured{nb1, nb2, nb3},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := notebook.NewHardwareProfileMigrationCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(notebook.ConditionTypeHardwareProfileCompatible),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonMigrationPending),
		"Message": ContainSubstring("Found 2 Notebook(s)"),
	}))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactAdvisory))
	g.Expect(result.Status.Conditions[0].Remediation).To(ContainSubstring("HardwareProfiles"))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "2"))
	g.Expect(result.ImpactedObjects).To(HaveLen(2))
}

func TestHardwareProfileMigration_CanApply_Managed(t *testing.T) {
	g := NewWithT(t)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds: hardwareProfileListKinds,
		Objects:   []*unstructured.Unstructured{testutil.NewDSC(map[string]string{"workbenches": "Managed"})},
	})

	chk := notebook.NewHardwareProfileMigrationCheck()
	canApply, err := chk.CanApply(t.Context(), target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeTrue())
}

func TestHardwareProfileMigration_CanApply_Removed(t *testing.T) {
	g := NewWithT(t)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds: hardwareProfileListKinds,
		Objects:   []*unstructured.Unstructured{testutil.NewDSC(map[string]string{"workbenches": "Removed"})},
	})

	chk := notebook.NewHardwareProfileMigrationCheck()
	canApply, err := chk.CanApply(t.Context(), target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeFalse())
}

func TestHardwareProfileMigration_Metadata(t *testing.T) {
	g := NewWithT(t)

	chk := notebook.NewHardwareProfileMigrationCheck()

	g.Expect(chk.ID()).To(Equal("workloads.notebook.hardwareprofile-migration"))
	g.Expect(chk.Name()).To(Equal("Workloads :: Notebook :: Legacy HardwareProfile Migration"))
	g.Expect(chk.Group()).To(Equal(check.GroupWorkload))
	g.Expect(chk.Description()).ToNot(BeEmpty())
	g.Expect(chk.Remediation()).To(ContainSubstring("HardwareProfiles"))
}
