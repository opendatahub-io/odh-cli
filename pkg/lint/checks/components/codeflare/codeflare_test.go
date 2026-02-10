package codeflare_test

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/lburgazzoli/odh-cli/pkg/lint/check"
	resultpkg "github.com/lburgazzoli/odh-cli/pkg/lint/check/result"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/components/codeflare"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/shared/testutil"
	"github.com/lburgazzoli/odh-cli/pkg/resources"

	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

//nolint:gochecknoglobals // Test fixture - shared across test functions
var listKinds = map[schema.GroupVersionResource]string{
	resources.DataScienceCluster.GVR(): resources.DataScienceCluster.ListKind(),
}

func TestCodeFlareRemovalCheck_CanApply_NoDSC(t *testing.T) {
	g := NewWithT(t)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := codeflare.NewRemovalCheck()
	canApply, err := chk.CanApply(t.Context(), target)

	g.Expect(err).To(HaveOccurred())
	g.Expect(canApply).To(BeFalse())
}

func TestCodeFlareRemovalCheck_CanApply_NotConfigured(t *testing.T) {
	g := NewWithT(t)

	// DSC without codeflare component — should not apply
	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{testutil.NewDSC(map[string]string{"dashboard": "Managed"})},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := codeflare.NewRemovalCheck()
	canApply, err := chk.CanApply(t.Context(), target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeFalse())
}

func TestCodeFlareRemovalCheck_ManagedBlocking(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	// Create DataScienceCluster with codeflare Managed (blocking upgrade)
	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{testutil.NewDSC(map[string]string{"codeflare": "Managed"})},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	codeflareCheck := codeflare.NewRemovalCheck()
	result, err := codeflareCheck.Validate(ctx, target)

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

func TestCodeFlareRemovalCheck_CanApply_ManagementState(t *testing.T) {
	g := NewWithT(t)

	chk := codeflare.NewRemovalCheck()

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
			dsc := testutil.NewDSC(map[string]string{"codeflare": tc.state})
			target := testutil.NewTarget(t, testutil.TargetConfig{
				ListKinds:      listKinds,
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

func TestCodeFlareRemovalCheck_Metadata(t *testing.T) {
	g := NewWithT(t)

	codeflareCheck := codeflare.NewRemovalCheck()

	g.Expect(codeflareCheck.ID()).To(Equal("components.codeflare.removal"))
	g.Expect(codeflareCheck.Name()).To(Equal("Components :: CodeFlare :: Removal (3.x)"))
	g.Expect(codeflareCheck.Group()).To(Equal(check.GroupComponent))
	g.Expect(codeflareCheck.Description()).ToNot(BeEmpty())
}
