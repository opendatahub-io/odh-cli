package modelmesh_test

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/lburgazzoli/odh-cli/pkg/lint/check"
	resultpkg "github.com/lburgazzoli/odh-cli/pkg/lint/check/result"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/components/modelmesh"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/shared/testutil"
	"github.com/lburgazzoli/odh-cli/pkg/resources"

	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

//nolint:gochecknoglobals // Test fixture - shared across test functions
var listKinds = map[schema.GroupVersionResource]string{
	resources.DataScienceCluster.GVR(): resources.DataScienceCluster.ListKind(),
}

func TestModelmeshRemovalCheck_CanApply_NoDSC(t *testing.T) {
	g := NewWithT(t)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := modelmesh.NewRemovalCheck()
	canApply, err := chk.CanApply(t.Context(), target)

	g.Expect(err).To(HaveOccurred())
	g.Expect(canApply).To(BeFalse())
}

func TestModelmeshRemovalCheck_CanApply_NotConfigured(t *testing.T) {
	g := NewWithT(t)

	// DSC without modelmesh component — should not apply
	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{testutil.NewDSC(map[string]string{"dashboard": "Managed"})},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := modelmesh.NewRemovalCheck()
	canApply, err := chk.CanApply(t.Context(), target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeFalse())
}

func TestModelmeshRemovalCheck_ManagedBlocking(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	// Create DataScienceCluster with modelmeshserving Managed (blocking upgrade)
	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{testutil.NewDSC(map[string]string{"modelmeshserving": "Managed"})},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	modelmeshCheck := modelmesh.NewRemovalCheck()
	result, err := modelmeshCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeCompatible),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonVersionIncompatible),
		"Message": And(ContainSubstring("enabled"), ContainSubstring("removed in RHOAI 3.x")),
	}))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactBlocking))
	g.Expect(result.Annotations).To(And(
		HaveKeyWithValue("component.opendatahub.io/management-state", "Managed"),
		HaveKeyWithValue("check.opendatahub.io/target-version", "3.0.0"),
	))
}

func TestModelmeshRemovalCheck_CanApply_Unmanaged(t *testing.T) {
	g := NewWithT(t)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{testutil.NewDSC(map[string]string{"modelmeshserving": "Unmanaged"})},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.1.0",
	})

	chk := modelmesh.NewRemovalCheck()
	canApply, err := chk.CanApply(t.Context(), target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeFalse())
}

func TestModelmeshRemovalCheck_CanApply_Removed(t *testing.T) {
	g := NewWithT(t)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{testutil.NewDSC(map[string]string{"modelmeshserving": "Removed"})},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := modelmesh.NewRemovalCheck()
	canApply, err := chk.CanApply(t.Context(), target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeFalse())
}

func TestModelmeshRemovalCheck_CanApply_Managed(t *testing.T) {
	g := NewWithT(t)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{testutil.NewDSC(map[string]string{"modelmeshserving": "Managed"})},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := modelmesh.NewRemovalCheck()
	canApply, err := chk.CanApply(t.Context(), target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeTrue())
}

func TestModelmeshRemovalCheck_CanApply_NotUpgrade(t *testing.T) {
	g := NewWithT(t)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:     listKinds,
		Objects:       []*unstructured.Unstructured{testutil.NewDSC(map[string]string{"modelmeshserving": "Managed"})},
		TargetVersion: "3.0.0",
	})

	chk := modelmesh.NewRemovalCheck()
	canApply, err := chk.CanApply(t.Context(), target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeFalse())
}

func TestModelmeshRemovalCheck_Metadata(t *testing.T) {
	g := NewWithT(t)

	modelmeshCheck := modelmesh.NewRemovalCheck()

	g.Expect(modelmeshCheck.ID()).To(Equal("components.modelmesh.removal"))
	g.Expect(modelmeshCheck.Name()).To(Equal("Components :: ModelMesh :: Removal (3.x)"))
	g.Expect(modelmeshCheck.Group()).To(Equal(check.GroupComponent))
	g.Expect(modelmeshCheck.Description()).ToNot(BeEmpty())
}
