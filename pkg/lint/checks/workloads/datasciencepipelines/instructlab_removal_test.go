package datasciencepipelines_test

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/lburgazzoli/odh-cli/pkg/lint/check"
	"github.com/lburgazzoli/odh-cli/pkg/lint/check/result"
	"github.com/lburgazzoli/odh-cli/pkg/lint/check/testutil"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/workloads/datasciencepipelines"
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

func TestInstructLabRemovalCheck_CanApply_NoDSC(t *testing.T) {
	g := NewWithT(t)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      instructLabListKinds,
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := datasciencepipelines.NewInstructLabRemovalCheck()
	canApply, err := chk.CanApply(t.Context(), target)

	g.Expect(err).To(HaveOccurred())
	g.Expect(canApply).To(BeFalse())
}

func TestInstructLabRemovalCheck_CanApply_ManagementState(t *testing.T) {
	g := NewWithT(t)

	chk := datasciencepipelines.NewInstructLabRemovalCheck()

	testCases := []struct {
		name     string
		state    string
		expected bool
	}{
		{name: "Managed", state: "Managed", expected: true},
		{name: "Unmanaged", state: "Unmanaged", expected: false},
		{name: "Removed", state: "Removed", expected: false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			dsc := testutil.NewDSC(map[string]string{"datasciencepipelines": tc.state})
			target := testutil.NewTarget(t, testutil.TargetConfig{
				ListKinds:      instructLabListKinds,
				Objects:        []*unstructured.Unstructured{dsc},
				CurrentVersion: "2.17.0",
				TargetVersion:  "3.0.0",
			})

			canApply, err := chk.CanApply(t.Context(), target)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(canApply).To(Equal(tc.expected))
		})
	}
}

func TestInstructLabRemovalCheck_NoDSPAs(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	dsc := testutil.NewDSC(map[string]string{"datasciencepipelines": "Managed"})
	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      instructLabListKinds,
		Objects:        []*unstructured.Unstructured{dsc},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
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
		ListKinds:      instructLabListKinds,
		Objects:        []*unstructured.Unstructured{dsc, dspa},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
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
		ListKinds:      instructLabListKinds,
		Objects:        []*unstructured.Unstructured{dsc, dspa},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
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
		ListKinds:      instructLabListKinds,
		Objects:        []*unstructured.Unstructured{dsc, dspa1, dspa2, dspa3},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
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

	chk := datasciencepipelines.NewInstructLabRemovalCheck()
	dsc := testutil.NewDSC(map[string]string{"datasciencepipelines": "Managed"})

	// Should not apply in lint mode (same version)
	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      instructLabListKinds,
		Objects:        []*unstructured.Unstructured{dsc},
		CurrentVersion: "2.17.0",
		TargetVersion:  "2.17.0",
	})
	canApply, err := chk.CanApply(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeFalse())

	// Should apply for 2.x -> 3.x upgrade with Managed
	target = testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      instructLabListKinds,
		Objects:        []*unstructured.Unstructured{dsc},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})
	canApply, err = chk.CanApply(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeTrue())

	// Should not apply for 3.x -> 3.x upgrade
	target = testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      instructLabListKinds,
		Objects:        []*unstructured.Unstructured{dsc},
		CurrentVersion: "3.0.0",
		TargetVersion:  "3.1.0",
	})
	canApply, err = chk.CanApply(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeFalse())

	// Should not apply with nil versions
	target = testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds: instructLabListKinds,
		Objects:   []*unstructured.Unstructured{dsc},
	})
	canApply, err = chk.CanApply(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeFalse())
}

func TestInstructLabRemovalCheck_Metadata(t *testing.T) {
	g := NewWithT(t)

	ilCheck := datasciencepipelines.NewInstructLabRemovalCheck()

	g.Expect(ilCheck.ID()).To(Equal("workloads.datasciencepipelines.instructlab-removal"))
	g.Expect(ilCheck.Name()).To(Equal("Workloads :: DataSciencePipelines :: InstructLab ManagedPipelines Removal (3.x)"))
	g.Expect(ilCheck.Group()).To(Equal(check.GroupWorkload))
	g.Expect(ilCheck.Description()).ToNot(BeEmpty())
}
