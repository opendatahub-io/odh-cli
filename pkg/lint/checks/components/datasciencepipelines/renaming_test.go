package datasciencepipelines_test

import (
	"testing"

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
var listKinds = map[schema.GroupVersionResource]string{
	resources.DataScienceCluster.GVR(): resources.DataScienceCluster.ListKind(),
}

func TestRenamingCheck_NoDSC(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:     listKinds,
		TargetVersion: "3.0.0",
	})

	renamingCheck := datasciencepipelines.NewRenamingCheck()
	dr, err := renamingCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(dr.Status.Conditions).To(HaveLen(1))
	g.Expect(dr.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeAvailable),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonResourceNotFound),
		"Message": ContainSubstring("No DataScienceCluster"),
	}))
}

func TestRenamingCheck_ManagedRenamed(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:     listKinds,
		Objects:       []*unstructured.Unstructured{testutil.NewDSC(map[string]string{"datasciencepipelines": "Managed"})},
		TargetVersion: "3.0.0",
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

func TestRenamingCheck_UnmanagedNotApplicable(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:     listKinds,
		Objects:       []*unstructured.Unstructured{testutil.NewDSC(map[string]string{"datasciencepipelines": "Unmanaged"})},
		TargetVersion: "3.0.0",
	})

	renamingCheck := datasciencepipelines.NewRenamingCheck()
	dr, err := renamingCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(dr.Status.Conditions).To(HaveLen(1))
	// Unmanaged is not in InState(Managed), so the builder passes (check doesn't apply)
	g.Expect(dr.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(check.ConditionTypeConfigured),
		"Status": Equal(metav1.ConditionTrue),
		"Reason": Equal(check.ReasonRequirementsMet),
	}))
}

func TestRenamingCheck_RemovedNotApplicable(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:     listKinds,
		Objects:       []*unstructured.Unstructured{testutil.NewDSC(map[string]string{"datasciencepipelines": "Removed"})},
		TargetVersion: "3.0.0",
	})

	renamingCheck := datasciencepipelines.NewRenamingCheck()
	dr, err := renamingCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(dr.Status.Conditions).To(HaveLen(1))
	g.Expect(dr.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(check.ConditionTypeConfigured),
		"Status": Equal(metav1.ConditionTrue),
		"Reason": Equal(check.ReasonRequirementsMet),
	}))
}

func TestRenamingCheck_CanApply(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	renamingCheck := datasciencepipelines.NewRenamingCheck()

	// Should not apply in lint mode (same version)
	canApply, err := renamingCheck.CanApply(ctx, testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		CurrentVersion: "2.17.0",
		TargetVersion:  "2.17.0",
	}))
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeFalse())

	// Should apply for 2.x -> 3.x upgrade
	canApply, err = renamingCheck.CanApply(ctx, testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	}))
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeTrue())

	// Should not apply for 3.x -> 3.x upgrade
	canApply, err = renamingCheck.CanApply(ctx, testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		CurrentVersion: "3.0.0",
		TargetVersion:  "3.1.0",
	}))
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeFalse())

	// Should not apply with nil versions
	canApply, err = renamingCheck.CanApply(ctx, check.Target{})
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
