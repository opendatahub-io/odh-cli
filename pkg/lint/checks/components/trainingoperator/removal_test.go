package trainingoperator_test

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	resultpkg "github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/testutil"
	"github.com/opendatahub-io/odh-cli/pkg/lint/checks/components/trainingoperator"

	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

func TestTrainingOperatorRemovalCheck_CanApply_NoDSC(t *testing.T) {
	g := NewWithT(t)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:     listKinds,
		TargetVersion: "3.6.0",
	})

	chk := trainingoperator.NewRemovalCheck()
	canApply, err := chk.CanApply(t.Context(), target)

	g.Expect(err).To(HaveOccurred())
	g.Expect(canApply).To(BeFalse())
}

func TestTrainingOperatorRemovalCheck_CanApply_NotConfigured(t *testing.T) {
	g := NewWithT(t)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:     listKinds,
		Objects:       []*unstructured.Unstructured{testutil.NewDSC(map[string]string{"dashboard": "Managed"})},
		TargetVersion: "3.6.0",
	})

	chk := trainingoperator.NewRemovalCheck()
	canApply, err := chk.CanApply(t.Context(), target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeFalse())
}

func TestTrainingOperatorRemovalCheck_ManagedBlocking(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:     listKinds,
		Objects:       []*unstructured.Unstructured{testutil.NewDSC(map[string]string{"trainingoperator": "Managed"})},
		TargetVersion: "3.6.0",
	})

	removalCheck := trainingoperator.NewRemovalCheck()
	result, err := removalCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeCompatible),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonVersionIncompatible),
		"Message": And(ContainSubstring("enabled"), ContainSubstring("removed in RHOAI 3.6")),
	}))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactBlocking))
	g.Expect(result.Annotations).To(And(
		HaveKeyWithValue("component.opendatahub.io/management-state", "Managed"),
		HaveKeyWithValue("check.opendatahub.io/target-version", "3.6.0"),
	))
}

func TestTrainingOperatorRemovalCheck_CanApply_Unmanaged(t *testing.T) {
	g := NewWithT(t)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:     listKinds,
		Objects:       []*unstructured.Unstructured{testutil.NewDSC(map[string]string{"trainingoperator": "Unmanaged"})},
		TargetVersion: "3.6.0",
	})

	chk := trainingoperator.NewRemovalCheck()
	canApply, err := chk.CanApply(t.Context(), target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeFalse())
}

func TestTrainingOperatorRemovalCheck_CanApply_Removed(t *testing.T) {
	g := NewWithT(t)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:     listKinds,
		Objects:       []*unstructured.Unstructured{testutil.NewDSC(map[string]string{"trainingoperator": "Removed"})},
		TargetVersion: "3.6.0",
	})

	chk := trainingoperator.NewRemovalCheck()
	canApply, err := chk.CanApply(t.Context(), target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeFalse())
}

func TestTrainingOperatorRemovalCheck_CanApply_Managed(t *testing.T) {
	g := NewWithT(t)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:     listKinds,
		Objects:       []*unstructured.Unstructured{testutil.NewDSC(map[string]string{"trainingoperator": "Managed"})},
		TargetVersion: "3.6.0",
	})

	chk := trainingoperator.NewRemovalCheck()
	canApply, err := chk.CanApply(t.Context(), target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeTrue())
}

func TestTrainingOperatorRemovalCheck_CanApply_Version35(t *testing.T) {
	g := NewWithT(t)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:     listKinds,
		Objects:       []*unstructured.Unstructured{testutil.NewDSC(map[string]string{"trainingoperator": "Managed"})},
		TargetVersion: "3.5.0",
	})

	chk := trainingoperator.NewRemovalCheck()
	canApply, err := chk.CanApply(t.Context(), target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeFalse())
}

func TestTrainingOperatorRemovalCheck_CanApply_Version37(t *testing.T) {
	g := NewWithT(t)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:     listKinds,
		Objects:       []*unstructured.Unstructured{testutil.NewDSC(map[string]string{"trainingoperator": "Managed"})},
		TargetVersion: "3.7.0",
	})

	chk := trainingoperator.NewRemovalCheck()
	canApply, err := chk.CanApply(t.Context(), target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeTrue())
}

func TestTrainingOperatorRemovalCheck_Metadata(t *testing.T) {
	g := NewWithT(t)

	removalCheck := trainingoperator.NewRemovalCheck()

	g.Expect(removalCheck.ID()).To(Equal("components.trainingoperator.removal"))
	g.Expect(removalCheck.Name()).To(Equal("Components :: TrainingOperator :: Removal (3.6)"))
	g.Expect(removalCheck.Group()).To(Equal(check.GroupComponent))
	g.Expect(removalCheck.Description()).ToNot(BeEmpty())
}
