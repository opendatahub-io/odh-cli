package trainingoperator_test

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	resultpkg "github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/testutil"
	"github.com/opendatahub-io/odh-cli/pkg/lint/checks/components/trainingoperator"
	"github.com/opendatahub-io/odh-cli/pkg/resources"

	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

//nolint:gochecknoglobals
var listKinds = map[schema.GroupVersionResource]string{
	resources.DataScienceCluster.GVR(): resources.DataScienceCluster.ListKind(),
}

func TestTrainingOperatorDeprecationCheck_CanApply_NoDSC(t *testing.T) {
	g := NewWithT(t)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:     listKinds,
		TargetVersion: "3.3.0",
	})

	chk := trainingoperator.NewDeprecationCheck()
	canApply, err := chk.CanApply(t.Context(), target)

	g.Expect(err).To(HaveOccurred())
	g.Expect(canApply).To(BeFalse())
}

func TestTrainingOperatorDeprecationCheck_CanApply_NotConfigured(t *testing.T) {
	g := NewWithT(t)

	// DSC without trainingoperator component — should not apply
	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:     listKinds,
		Objects:       []*unstructured.Unstructured{testutil.NewDSC(map[string]string{"dashboard": "Managed"})},
		TargetVersion: "3.3.0",
	})

	chk := trainingoperator.NewDeprecationCheck()
	canApply, err := chk.CanApply(t.Context(), target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeFalse())
}

func TestTrainingOperatorDeprecationCheck_ManagedDeprecated(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:     listKinds,
		Objects:       []*unstructured.Unstructured{testutil.NewDSC(map[string]string{"trainingoperator": "Managed"})},
		TargetVersion: "3.3.0",
	})

	trainingoperatorCheck := trainingoperator.NewDeprecationCheck()
	result, err := trainingoperatorCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeCompatible),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonDeprecated),
		"Message": And(ContainSubstring("enabled"), ContainSubstring("deprecated in RHOAI 3.3")),
	}))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactAdvisory))
	g.Expect(result.Annotations).To(And(
		HaveKeyWithValue("component.opendatahub.io/management-state", "Managed"),
		HaveKeyWithValue("check.opendatahub.io/target-version", "3.3.0"),
	))
}

func TestTrainingOperatorDeprecationCheck_CanApply_Unmanaged(t *testing.T) {
	g := NewWithT(t)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:     listKinds,
		Objects:       []*unstructured.Unstructured{testutil.NewDSC(map[string]string{"trainingoperator": "Unmanaged"})},
		TargetVersion: "3.4.0",
	})

	chk := trainingoperator.NewDeprecationCheck()
	canApply, err := chk.CanApply(t.Context(), target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeFalse())
}

func TestTrainingOperatorDeprecationCheck_CanApply_Removed(t *testing.T) {
	g := NewWithT(t)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:     listKinds,
		Objects:       []*unstructured.Unstructured{testutil.NewDSC(map[string]string{"trainingoperator": "Removed"})},
		TargetVersion: "3.3.0",
	})

	chk := trainingoperator.NewDeprecationCheck()
	canApply, err := chk.CanApply(t.Context(), target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeFalse())
}

func TestTrainingOperatorDeprecationCheck_CanApply_Managed(t *testing.T) {
	g := NewWithT(t)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:     listKinds,
		Objects:       []*unstructured.Unstructured{testutil.NewDSC(map[string]string{"trainingoperator": "Managed"})},
		TargetVersion: "3.3.0",
	})

	chk := trainingoperator.NewDeprecationCheck()
	canApply, err := chk.CanApply(t.Context(), target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeTrue())
}

func TestTrainingOperatorDeprecationCheck_CanApply_Version32(t *testing.T) {
	g := NewWithT(t)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:     listKinds,
		Objects:       []*unstructured.Unstructured{testutil.NewDSC(map[string]string{"trainingoperator": "Managed"})},
		TargetVersion: "3.2.0",
	})

	chk := trainingoperator.NewDeprecationCheck()
	canApply, err := chk.CanApply(t.Context(), target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeFalse())
}

func TestTrainingOperatorDeprecationCheck_CanApply_Version33(t *testing.T) {
	g := NewWithT(t)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:     listKinds,
		Objects:       []*unstructured.Unstructured{testutil.NewDSC(map[string]string{"trainingoperator": "Managed"})},
		TargetVersion: "3.3.0",
	})

	chk := trainingoperator.NewDeprecationCheck()
	canApply, err := chk.CanApply(t.Context(), target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeTrue())
}

func TestTrainingOperatorDeprecationCheck_CanApply_Version34(t *testing.T) {
	g := NewWithT(t)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:     listKinds,
		Objects:       []*unstructured.Unstructured{testutil.NewDSC(map[string]string{"trainingoperator": "Managed"})},
		TargetVersion: "3.4.0",
	})

	chk := trainingoperator.NewDeprecationCheck()
	canApply, err := chk.CanApply(t.Context(), target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeTrue())
}

func TestTrainingOperatorDeprecationCheck_Metadata(t *testing.T) {
	g := NewWithT(t)

	trainingoperatorCheck := trainingoperator.NewDeprecationCheck()

	g.Expect(trainingoperatorCheck.ID()).To(Equal("components.trainingoperator.deprecation"))
	g.Expect(trainingoperatorCheck.Name()).To(Equal("Components :: TrainingOperator :: Deprecation (3.3+)"))
	g.Expect(trainingoperatorCheck.Group()).To(Equal(check.GroupComponent))
	g.Expect(trainingoperatorCheck.Description()).ToNot(BeEmpty())
}
