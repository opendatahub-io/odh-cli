package notebook_test

import (
	"fmt"
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
var hwpIntegrityListKinds = map[schema.GroupVersionResource]string{
	resources.Notebook.GVR():                      resources.Notebook.ListKind(),
	resources.InfrastructureHardwareProfile.GVR(): resources.InfrastructureHardwareProfile.ListKind(),
	resources.DataScienceCluster.GVR():            resources.DataScienceCluster.ListKind(),
	resources.CustomResourceDefinition.GVR():      resources.CustomResourceDefinition.ListKind(),
}

func newHardwareProfileCRD(storageVersion string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.CustomResourceDefinition.APIVersion(),
			"kind":       resources.CustomResourceDefinition.Kind,
			"metadata": map[string]any{
				"name": "hardwareprofiles.infrastructure.opendatahub.io",
			},
			"spec": map[string]any{
				"group": "infrastructure.opendatahub.io",
				"versions": []any{
					map[string]any{
						"name":    storageVersion,
						"served":  true,
						"storage": true,
					},
				},
			},
		},
	}
}

func TestHardwareProfileIntegrityCheck_Metadata(t *testing.T) {
	g := NewWithT(t)

	chk := notebook.NewHardwareProfileIntegrityCheck()

	g.Expect(chk.ID()).To(Equal("workloads.notebook.hardware-profile-integrity"))
	g.Expect(chk.Name()).To(Equal("Workloads :: Notebook :: HardwareProfile Integrity"))
	g.Expect(chk.Group()).To(Equal(check.GroupWorkload))
	g.Expect(chk.CheckKind()).To(Equal("notebook"))
	g.Expect(chk.CheckType()).To(Equal(string(check.CheckTypeDataIntegrity)))
	g.Expect(chk.Description()).ToNot(BeEmpty())
	g.Expect(chk.Remediation()).To(ContainSubstring("missing HardwareProfile"))
}

func TestHardwareProfileIntegrityCheck_CanApply_WorkbenchesManaged(t *testing.T) {
	g := NewWithT(t)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      hwpIntegrityListKinds,
		Objects:        []*unstructured.Unstructured{testutil.NewDSC(map[string]string{"workbenches": "Managed"})},
		CurrentVersion: "3.0.0",
		TargetVersion:  "3.0.0",
	})

	chk := notebook.NewHardwareProfileIntegrityCheck()
	canApply, err := chk.CanApply(t.Context(), target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeTrue())
}

func TestHardwareProfileIntegrityCheck_CanApply_WorkbenchesRemoved(t *testing.T) {
	g := NewWithT(t)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      hwpIntegrityListKinds,
		Objects:        []*unstructured.Unstructured{testutil.NewDSC(map[string]string{"workbenches": "Removed"})},
		CurrentVersion: "3.0.0",
		TargetVersion:  "3.0.0",
	})

	chk := notebook.NewHardwareProfileIntegrityCheck()
	canApply, err := chk.CanApply(t.Context(), target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeFalse())
}

func TestHardwareProfileIntegrityCheck_CanApply_UpgradeMode(t *testing.T) {
	g := NewWithT(t)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      hwpIntegrityListKinds,
		Objects:        []*unstructured.Unstructured{testutil.NewDSC(map[string]string{"workbenches": "Managed"})},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := notebook.NewHardwareProfileIntegrityCheck()
	canApply, err := chk.CanApply(t.Context(), target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeTrue())
}

func TestHardwareProfileIntegrityCheck_NoNotebooks(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      hwpIntegrityListKinds,
		CurrentVersion: "3.0.0",
		TargetVersion:  "3.0.0",
	})

	chk := notebook.NewHardwareProfileIntegrityCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(notebook.ConditionTypeHardwareProfileIntegrity),
		"Status":  Equal(metav1.ConditionTrue),
		"Reason":  Equal(check.ReasonRequirementsMet),
		"Message": Equal(notebook.MsgAllHardwareProfilesValid),
	}))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactNone))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "0"))
	g.Expect(result.ImpactedObjects).To(BeEmpty())
}

func TestHardwareProfileIntegrityCheck_NotebookWithoutHWPAnnotation(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	nb := newNotebook("plain-notebook", "user-ns", notebookOptions{})

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      hwpIntegrityListKinds,
		Objects:        []*unstructured.Unstructured{nb},
		CurrentVersion: "3.0.0",
		TargetVersion:  "3.0.0",
	})

	chk := notebook.NewHardwareProfileIntegrityCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Status).To(Equal(metav1.ConditionTrue))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "0"))
	g.Expect(result.ImpactedObjects).To(BeEmpty())
}

func TestHardwareProfileIntegrityCheck_ProfileExists(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	crd := newHardwareProfileCRD("v1")

	profile := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.InfrastructureHardwareProfile.APIVersion(),
			"kind":       resources.InfrastructureHardwareProfile.Kind,
			"metadata": map[string]any{
				"name":      "gpu-large",
				"namespace": "opendatahub",
			},
		},
	}

	nb := newNotebook("gpu-notebook", "user-ns", notebookOptions{
		Annotations: map[string]any{
			notebook.AnnotationHardwareProfileName:      "gpu-large",
			notebook.AnnotationHardwareProfileNamespace: "opendatahub",
		},
	})

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      hwpIntegrityListKinds,
		Objects:        []*unstructured.Unstructured{crd, nb, profile},
		CurrentVersion: "3.0.0",
		TargetVersion:  "3.0.0",
	})

	chk := notebook.NewHardwareProfileIntegrityCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Status).To(Equal(metav1.ConditionTrue))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "0"))
	g.Expect(result.ImpactedObjects).To(BeEmpty())
}

func TestHardwareProfileIntegrityCheck_ProfileMissing(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	crd := newHardwareProfileCRD("v1")

	nb := newNotebook("broken-notebook", "user-ns", notebookOptions{
		Annotations: map[string]any{
			notebook.AnnotationHardwareProfileName:      "missing-profile",
			notebook.AnnotationHardwareProfileNamespace: "opendatahub",
		},
	})

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      hwpIntegrityListKinds,
		Objects:        []*unstructured.Unstructured{crd, nb},
		CurrentVersion: "3.0.0",
		TargetVersion:  "3.0.0",
	})

	chk := notebook.NewHardwareProfileIntegrityCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(notebook.ConditionTypeHardwareProfileIntegrity),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonResourceNotFound),
		"Message": Equal(fmt.Sprintf(notebook.MsgHardwareProfilesMissing, 1)),
	}))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactBlocking))
	g.Expect(result.Status.Conditions[0].Remediation).To(ContainSubstring("missing HardwareProfile"))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "1"))
	g.Expect(result.ImpactedObjects).To(HaveLen(1))
	g.Expect(result.ImpactedObjects[0].Name).To(Equal("broken-notebook"))
	g.Expect(result.ImpactedObjects[0].Namespace).To(Equal("user-ns"))
}

func TestHardwareProfileIntegrityCheck_MixedExistingAndMissing(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	crd := newHardwareProfileCRD("v1")

	profile := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.InfrastructureHardwareProfile.APIVersion(),
			"kind":       resources.InfrastructureHardwareProfile.Kind,
			"metadata": map[string]any{
				"name":      "gpu-large",
				"namespace": "opendatahub",
			},
		},
	}

	// Notebook with existing profile
	nbGood := newNotebook("good-notebook", "ns1", notebookOptions{
		Annotations: map[string]any{
			notebook.AnnotationHardwareProfileName:      "gpu-large",
			notebook.AnnotationHardwareProfileNamespace: "opendatahub",
		},
	})

	// Notebook without HWP annotation (should be skipped)
	nbPlain := newNotebook("plain-notebook", "ns2", notebookOptions{})

	// Notebook with missing profile
	nbBroken := newNotebook("broken-notebook", "ns3", notebookOptions{
		Annotations: map[string]any{
			notebook.AnnotationHardwareProfileName:      "does-not-exist",
			notebook.AnnotationHardwareProfileNamespace: "opendatahub",
		},
	})

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      hwpIntegrityListKinds,
		Objects:        []*unstructured.Unstructured{crd, profile, nbGood, nbPlain, nbBroken},
		CurrentVersion: "3.0.0",
		TargetVersion:  "3.0.0",
	})

	chk := notebook.NewHardwareProfileIntegrityCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(notebook.ConditionTypeHardwareProfileIntegrity),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonResourceNotFound),
		"Message": Equal(fmt.Sprintf(notebook.MsgHardwareProfilesMissing, 1)),
	}))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactBlocking))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "1"))
	g.Expect(result.ImpactedObjects).To(HaveLen(1))
	g.Expect(result.ImpactedObjects[0].Name).To(Equal("broken-notebook"))
}

func TestHardwareProfileIntegrityCheck_WrongNamespace(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	crd := newHardwareProfileCRD("v1")

	// Profile exists but in a different namespace
	profile := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.InfrastructureHardwareProfile.APIVersion(),
			"kind":       resources.InfrastructureHardwareProfile.Kind,
			"metadata": map[string]any{
				"name":      "gpu-large",
				"namespace": "other-ns",
			},
		},
	}

	nb := newNotebook("notebook-wrong-ns", "user-ns", notebookOptions{
		Annotations: map[string]any{
			notebook.AnnotationHardwareProfileName:      "gpu-large",
			notebook.AnnotationHardwareProfileNamespace: "opendatahub",
		},
	})

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      hwpIntegrityListKinds,
		Objects:        []*unstructured.Unstructured{crd, profile, nb},
		CurrentVersion: "3.0.0",
		TargetVersion:  "3.0.0",
	})

	chk := notebook.NewHardwareProfileIntegrityCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions[0].Status).To(Equal(metav1.ConditionFalse))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "1"))
	g.Expect(result.ImpactedObjects).To(HaveLen(1))
}

func TestHardwareProfileIntegrityCheck_ProfileExistsV1Alpha1(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	// CRD reports v1alpha1 as the storage version (RHOAI 2.25.x clusters).
	crd := newHardwareProfileCRD("v1alpha1")

	hwpV1Alpha1 := resources.ResourceType{
		Group:    "infrastructure.opendatahub.io",
		Version:  "v1alpha1",
		Kind:     "HardwareProfile",
		Resource: "hardwareprofiles",
	}

	profile := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": hwpV1Alpha1.APIVersion(),
			"kind":       hwpV1Alpha1.Kind,
			"metadata": map[string]any{
				"name":      "gpu-large",
				"namespace": "opendatahub",
			},
		},
	}

	nb := newNotebook("gpu-notebook", "user-ns", notebookOptions{
		Annotations: map[string]any{
			notebook.AnnotationHardwareProfileName:      "gpu-large",
			notebook.AnnotationHardwareProfileNamespace: "opendatahub",
		},
	})

	listKinds := map[schema.GroupVersionResource]string{
		resources.Notebook.GVR():                 resources.Notebook.ListKind(),
		hwpV1Alpha1.GVR():                        hwpV1Alpha1.ListKind(),
		resources.DataScienceCluster.GVR():       resources.DataScienceCluster.ListKind(),
		resources.CustomResourceDefinition.GVR(): resources.CustomResourceDefinition.ListKind(),
	}

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{crd, nb, profile},
		CurrentVersion: "3.0.0",
		TargetVersion:  "3.0.0",
	})

	chk := notebook.NewHardwareProfileIntegrityCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Status).To(Equal(metav1.ConditionTrue))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "0"))
	g.Expect(result.ImpactedObjects).To(BeEmpty())
}

func TestHardwareProfileIntegrityCheck_AnnotationTargetVersion(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      hwpIntegrityListKinds,
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := notebook.NewHardwareProfileIntegrityCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationCheckTargetVersion, "3.0.0"))
}
