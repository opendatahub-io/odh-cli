package datasciencepipelines_test

import (
	"testing"

	"github.com/blang/semver/v4"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/lburgazzoli/odh-cli/pkg/lint/check"
	"github.com/lburgazzoli/odh-cli/pkg/lint/check/result"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/components/datasciencepipelines"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/shared/testutil"
	"github.com/lburgazzoli/odh-cli/pkg/resources"

	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

//nolint:gochecknoglobals // Test fixture - shared across test functions
var instructLabListKinds = map[schema.GroupVersionResource]string{
	resources.DataScienceCluster.GVR():                      resources.DataScienceCluster.ListKind(),
	resources.DataSciencePipelinesApplicationV1.GVR():       resources.DataSciencePipelinesApplicationV1.ListKind(),
	resources.DataSciencePipelinesApplicationV1Alpha1.GVR(): resources.DataSciencePipelinesApplicationV1Alpha1.ListKind(),
}

func newDSPAv1(name string, namespace string, withInstructLab bool) *unstructured.Unstructured {
	spec := map[string]any{}
	if withInstructLab {
		spec["apiServer"] = map[string]any{
			"managedPipelines": map[string]any{
				"instructLab": map[string]any{
					"enabled": true,
				},
			},
		}
	}

	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.DataSciencePipelinesApplicationV1.APIVersion(),
			"kind":       resources.DataSciencePipelinesApplicationV1.Kind,
			"metadata": map[string]any{
				"name":      name,
				"namespace": namespace,
			},
			"spec": spec,
		},
	}
}

func TestInstructLabRemovalCheck_NoDSC(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:     instructLabListKinds,
		Objects:       []*unstructured.Unstructured{},
		TargetVersion: "3.0.0",
	})

	ilCheck := datasciencepipelines.NewInstructLabRemovalCheck()
	dr, err := ilCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(dr.Status.Conditions).To(HaveLen(1))
	g.Expect(dr.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeAvailable),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonResourceNotFound),
		"Message": ContainSubstring("No DataScienceCluster"),
	}))
}

func TestInstructLabRemovalCheck_ComponentNotManaged(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	dsc := testutil.NewDSC(map[string]string{"datasciencepipelines": "Removed"})
	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:     instructLabListKinds,
		Objects:       []*unstructured.Unstructured{dsc},
		TargetVersion: "3.0.0",
	})

	ilCheck := datasciencepipelines.NewInstructLabRemovalCheck()
	dr, err := ilCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(dr.Status.Conditions).To(HaveLen(1))
	// Removed is not in InState(Managed), so the builder passes
	g.Expect(dr.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(check.ConditionTypeConfigured),
		"Status": Equal(metav1.ConditionTrue),
		"Reason": Equal(check.ReasonRequirementsMet),
	}))
}

func TestInstructLabRemovalCheck_NoDSPAs(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	dsc := testutil.NewDSC(map[string]string{"datasciencepipelines": "Managed"})
	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:     instructLabListKinds,
		Objects:       []*unstructured.Unstructured{dsc},
		TargetVersion: "3.0.0",
	})

	ilCheck := datasciencepipelines.NewInstructLabRemovalCheck()
	dr, err := ilCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(dr.Status.Conditions).To(HaveLen(1))
	g.Expect(dr.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeCompatible),
		"Status":  Equal(metav1.ConditionTrue),
		"Reason":  Equal(check.ReasonVersionCompatible),
		"Message": ContainSubstring("No DataSciencePipelinesApplications found"),
	}))
	g.Expect(dr.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "0"))
}

func TestInstructLabRemovalCheck_DSPAWithInstructLab(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	dsc := testutil.NewDSC(map[string]string{"datasciencepipelines": "Managed"})
	dspa := newDSPAv1("my-dspa", "test-ns", true)
	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:     instructLabListKinds,
		Objects:       []*unstructured.Unstructured{dsc, dspa},
		TargetVersion: "3.0.0",
	})

	ilCheck := datasciencepipelines.NewInstructLabRemovalCheck()
	dr, err := ilCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(dr.Status.Conditions).To(HaveLen(1))
	g.Expect(dr.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeCompatible),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonFeatureRemoved),
		"Message": And(ContainSubstring("Found 1"), ContainSubstring("instructLab")),
	}))
	g.Expect(dr.Status.Conditions[0].Impact).To(Equal(result.ImpactAdvisory))
	g.Expect(dr.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "1"))
	g.Expect(dr.ImpactedObjects).To(HaveLen(1))
	g.Expect(dr.ImpactedObjects[0].Name).To(Equal("my-dspa"))
	g.Expect(dr.ImpactedObjects[0].Namespace).To(Equal("test-ns"))
}

func TestInstructLabRemovalCheck_DSPAWithoutInstructLab(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	dsc := testutil.NewDSC(map[string]string{"datasciencepipelines": "Managed"})
	dspa := newDSPAv1("clean-dspa", "test-ns", false)
	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:     instructLabListKinds,
		Objects:       []*unstructured.Unstructured{dsc, dspa},
		TargetVersion: "3.0.0",
	})

	ilCheck := datasciencepipelines.NewInstructLabRemovalCheck()
	dr, err := ilCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(dr.Status.Conditions).To(HaveLen(1))
	g.Expect(dr.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeCompatible),
		"Status":  Equal(metav1.ConditionTrue),
		"Reason":  Equal(check.ReasonVersionCompatible),
		"Message": ContainSubstring("No DataSciencePipelinesApplications found"),
	}))
	g.Expect(dr.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "0"))
}

func TestInstructLabRemovalCheck_MultipleDSPAsMixed(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	dsc := testutil.NewDSC(map[string]string{"datasciencepipelines": "Managed"})
	dspa1 := newDSPAv1("dspa-with-il", "ns1", true)
	dspa2 := newDSPAv1("dspa-clean", "ns2", false)
	dspa3 := newDSPAv1("dspa-with-il-2", "ns3", true)
	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:     instructLabListKinds,
		Objects:       []*unstructured.Unstructured{dsc, dspa1, dspa2, dspa3},
		TargetVersion: "3.0.0",
	})

	ilCheck := datasciencepipelines.NewInstructLabRemovalCheck()
	dr, err := ilCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(dr.Status.Conditions).To(HaveLen(1))
	g.Expect(dr.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeCompatible),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonFeatureRemoved),
		"Message": ContainSubstring("Found 2"),
	}))
	g.Expect(dr.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "2"))
	g.Expect(dr.ImpactedObjects).To(HaveLen(2))
}

func TestInstructLabRemovalCheck_CanApply(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	ilCheck := datasciencepipelines.NewInstructLabRemovalCheck()

	// Should not apply in lint mode (same version)
	v217 := semver.MustParse("2.17.0")
	canApply, err := ilCheck.CanApply(ctx, check.Target{CurrentVersion: &v217, TargetVersion: &v217})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeFalse())

	// Should apply for 2.x -> 3.x upgrade
	v300 := semver.MustParse("3.0.0")
	canApply, err = ilCheck.CanApply(ctx, check.Target{CurrentVersion: &v217, TargetVersion: &v300})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeTrue())

	// Should not apply for 3.x -> 3.x upgrade
	v310 := semver.MustParse("3.1.0")
	canApply, err = ilCheck.CanApply(ctx, check.Target{CurrentVersion: &v300, TargetVersion: &v310})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeFalse())

	// Should not apply with nil versions
	canApply, err = ilCheck.CanApply(ctx, check.Target{})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeFalse())
}

func TestInstructLabRemovalCheck_Metadata(t *testing.T) {
	g := NewWithT(t)

	ilCheck := datasciencepipelines.NewInstructLabRemovalCheck()

	g.Expect(ilCheck.ID()).To(Equal("components.datasciencepipelines.instructlab-removal"))
	g.Expect(ilCheck.Name()).To(Equal("Components :: DataSciencePipelines :: InstructLab ManagedPipelines Removal (3.x)"))
	g.Expect(ilCheck.Group()).To(Equal(check.GroupComponent))
	g.Expect(ilCheck.Description()).ToNot(BeEmpty())
}
