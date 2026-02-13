package datasciencepipelines_test

import (
	"testing"

	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/testutil"
	"github.com/opendatahub-io/odh-cli/pkg/lint/checks/components/datasciencepipelines"
	"github.com/opendatahub-io/odh-cli/pkg/resources"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

//nolint:gochecknoglobals // Test fixture - shared across test functions
var listKinds = map[schema.GroupVersionResource]string{
	resources.DataScienceCluster.GVR(): resources.DataScienceCluster.ListKind(),
}

func TestRenamingCheck_CanApply_NoDSC(t *testing.T) {
	g := NewWithT(t)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := datasciencepipelines.NewRenamingCheck()
	canApply, err := chk.CanApply(t.Context(), target)

	g.Expect(err).To(HaveOccurred())
	g.Expect(canApply).To(BeFalse())
}

func TestRenamingCheck_ManagedRenamed(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{testutil.NewDSC(map[string]string{"datasciencepipelines": "Managed"})},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	renamingCheck := datasciencepipelines.NewRenamingCheck()
	dr, err := renamingCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(dr.Status.Conditions).To(HaveLen(1))
	g.Expect(dr.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeCompatible),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonComponentRenamed),
		"Message": And(ContainSubstring("renamed to AIPipelines"), ContainSubstring("Managed")),
	}))
	g.Expect(dr.Status.Conditions[0].Impact).To(Equal(result.ImpactAdvisory))
	g.Expect(dr.Annotations).To(And(
		HaveKeyWithValue("component.opendatahub.io/management-state", "Managed"),
		HaveKeyWithValue("check.opendatahub.io/target-version", "3.0.0"),
	))
}

func TestRenamingCheck_CanApply_Unmanaged(t *testing.T) {
	g := NewWithT(t)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{testutil.NewDSC(map[string]string{"datasciencepipelines": "Unmanaged"})},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := datasciencepipelines.NewRenamingCheck()
	canApply, err := chk.CanApply(t.Context(), target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeFalse())
}

func TestRenamingCheck_CanApply_Removed(t *testing.T) {
	g := NewWithT(t)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{testutil.NewDSC(map[string]string{"datasciencepipelines": "Removed"})},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := datasciencepipelines.NewRenamingCheck()
	canApply, err := chk.CanApply(t.Context(), target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeFalse())
}

func TestRenamingCheck_CanApply_Managed(t *testing.T) {
	g := NewWithT(t)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{testutil.NewDSC(map[string]string{"datasciencepipelines": "Managed"})},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := datasciencepipelines.NewRenamingCheck()
	canApply, err := chk.CanApply(t.Context(), target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeTrue())
}

func TestRenamingCheck_CanApply_VersionFiltering(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	chk := datasciencepipelines.NewRenamingCheck()
	dsc := testutil.NewDSC(map[string]string{"datasciencepipelines": "Managed"})

	// Should not apply in lint mode (same version)
	canApply, err := chk.CanApply(ctx, testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{dsc},
		CurrentVersion: "2.17.0",
		TargetVersion:  "2.17.0",
	}))
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeFalse())

	// Should apply for 2.x -> 3.x upgrade with Managed
	canApply, err = chk.CanApply(ctx, testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{dsc},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	}))
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeTrue())

	// Should not apply for 3.x -> 3.x upgrade
	canApply, err = chk.CanApply(ctx, testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{dsc},
		CurrentVersion: "3.0.0",
		TargetVersion:  "3.1.0",
	}))
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeFalse())

	// Should not apply with nil versions
	canApply, err = chk.CanApply(ctx, check.Target{})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeFalse())
}

func TestRenamingCheck_Metadata(t *testing.T) {
	g := NewWithT(t)

	renamingCheck := datasciencepipelines.NewRenamingCheck()

	g.Expect(renamingCheck.ID()).To(Equal("components.datasciencepipelines.renaming"))
	g.Expect(renamingCheck.Name()).To(Equal("Components :: DataSciencePipelines :: Component Renaming (3.x)"))
	g.Expect(renamingCheck.Group()).To(Equal(check.GroupComponent))
	g.Expect(renamingCheck.Description()).ToNot(BeEmpty())
}
