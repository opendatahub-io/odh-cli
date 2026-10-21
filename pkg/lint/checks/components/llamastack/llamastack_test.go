package llamastack_test

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	resultpkg "github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/testutil"
	"github.com/opendatahub-io/odh-cli/pkg/lint/checks/components/llamastack"
	"github.com/opendatahub-io/odh-cli/pkg/resources"

	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

//nolint:gochecknoglobals // Test fixture - shared across test functions
var listKinds = map[schema.GroupVersionResource]string{
	resources.DataScienceCluster.GVR(): resources.DataScienceCluster.ListKind(),
}

func TestLlamaStackRemovalCheck_CanApply_NoDSC(t *testing.T) {
	g := NewWithT(t)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		CurrentVersion: "3.4.0",
		TargetVersion:  "3.5.0",
	})

	chk := llamastack.NewRemovalCheck()
	canApply, err := chk.CanApply(t.Context(), target)

	g.Expect(err).To(HaveOccurred())
	g.Expect(canApply).To(BeFalse())
}

func TestLlamaStackRemovalCheck_CanApply_NotConfigured(t *testing.T) {
	g := NewWithT(t)

	// DSC without llamastackoperator component — should not apply
	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{testutil.NewDSC(map[string]string{"dashboard": "Managed"})},
		CurrentVersion: "3.4.0",
		TargetVersion:  "3.5.0",
	})

	chk := llamastack.NewRemovalCheck()
	canApply, err := chk.CanApply(t.Context(), target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeFalse())
}

func TestLlamaStackRemovalCheck_ManagedBlocking(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	// Create DataScienceCluster with llamastackoperator Managed (blocking upgrade)
	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{testutil.NewDSC(map[string]string{"llamastackoperator": "Managed"})},
		CurrentVersion: "3.4.0",
		TargetVersion:  "3.5.0",
	})

	llamastackCheck := llamastack.NewRemovalCheck()
	result, err := llamastackCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeCompatible),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonVersionIncompatible),
		"Message": And(ContainSubstring("enabled"), ContainSubstring("replaced by ogx in RHOAI 3.5")),
	}))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactBlocking))
	g.Expect(result.Annotations).To(And(
		HaveKeyWithValue("component.opendatahub.io/management-state", "Managed"),
		HaveKeyWithValue("check.opendatahub.io/target-version", "3.5.0"),
	))
}

func TestLlamaStackRemovalCheck_CanApply_Unmanaged(t *testing.T) {
	g := NewWithT(t)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{testutil.NewDSC(map[string]string{"llamastackoperator": "Unmanaged"})},
		CurrentVersion: "3.4.0",
		TargetVersion:  "3.5.0",
	})

	chk := llamastack.NewRemovalCheck()
	canApply, err := chk.CanApply(t.Context(), target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeFalse())
}

func TestLlamaStackRemovalCheck_CanApply_Removed(t *testing.T) {
	g := NewWithT(t)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{testutil.NewDSC(map[string]string{"llamastackoperator": "Removed"})},
		CurrentVersion: "3.4.0",
		TargetVersion:  "3.5.0",
	})

	chk := llamastack.NewRemovalCheck()
	canApply, err := chk.CanApply(t.Context(), target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeFalse())
}

func TestLlamaStackRemovalCheck_CanApply_Managed(t *testing.T) {
	g := NewWithT(t)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{testutil.NewDSC(map[string]string{"llamastackoperator": "Managed"})},
		CurrentVersion: "3.4.0",
		TargetVersion:  "3.5.0",
	})

	chk := llamastack.NewRemovalCheck()
	canApply, err := chk.CanApply(t.Context(), target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeTrue())
}

func TestLlamaStackRemovalCheck_CanApply_VersionMatrix(t *testing.T) {
	tests := []struct {
		name    string
		current string
		target  string
		want    bool
	}{
		{name: "3.4 to 3.5 applies", current: "3.4.0", target: "3.5.0", want: true},
		{name: "3.5 to 3.6 applies", current: "3.5.0", target: "3.6.0", want: true},
		{name: "3.4 to 3.6 applies", current: "3.4.0", target: "3.6.0", want: true},
		{name: "3.3 to 3.5 applies", current: "3.3.0", target: "3.5.0", want: true},
		{name: "2.x to 3.5 applies", current: "2.17.0", target: "3.5.0", want: true},
		{name: "2.x to 3.6 applies", current: "2.17.0", target: "3.6.0", want: true},
		{name: "3.6 to 3.7 applies", current: "3.6.0", target: "3.7.0", want: true},
		{name: "no current version", current: "", target: "3.5.0", want: false},
		{name: "no target version", current: "3.4.0", target: "", want: false},
		{name: "same version does not apply", current: "3.5.0", target: "3.5.0", want: false},
		{name: "same major/minor patch change does not apply", current: "3.5.1", target: "3.5.2", want: false},
		{name: "downgrade does not apply", current: "3.6.0", target: "3.5.0", want: false},
		{name: "target 3.4 does not apply", current: "3.3.0", target: "3.4.0", want: false},
		{name: "target 2.x does not apply", current: "2.17.0", target: "2.20.0", want: false},
		{name: "target 4.x does not apply", current: "3.5.0", target: "4.0.0", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			cfg := testutil.TargetConfig{
				ListKinds: listKinds,
				Objects:   []*unstructured.Unstructured{testutil.NewDSC(map[string]string{"llamastackoperator": "Managed"})},
			}
			if tt.current != "" {
				cfg.CurrentVersion = tt.current
			}
			if tt.target != "" {
				cfg.TargetVersion = tt.target
			}

			target := testutil.NewTarget(t, cfg)
			chk := llamastack.NewRemovalCheck()
			canApply, err := chk.CanApply(t.Context(), target)

			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(canApply).To(Equal(tt.want))
		})
	}
}

func TestLlamaStackRemovalCheck_ManagedBlocking_36Target(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{testutil.NewDSC(map[string]string{"llamastackoperator": "Managed"})},
		CurrentVersion: "3.5.0",
		TargetVersion:  "3.6.0",
	})

	llamastackCheck := llamastack.NewRemovalCheck()
	result, err := llamastackCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeCompatible),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonVersionIncompatible),
		"Message": And(ContainSubstring("enabled"), ContainSubstring("replaced by ogx in RHOAI 3.6")),
	}))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactBlocking))
}

func TestLlamaStackRemovalCheck_Metadata(t *testing.T) {
	g := NewWithT(t)

	llamastackCheck := llamastack.NewRemovalCheck()

	g.Expect(llamastackCheck.ID()).To(Equal("components.llamastackoperator.removal"))
	g.Expect(llamastackCheck.Name()).To(Equal("Components :: LlamaStack Operator :: Removal (3.5+)"))
	g.Expect(llamastackCheck.Group()).To(Equal(check.GroupComponent))
	g.Expect(llamastackCheck.Description()).ToNot(BeEmpty())
}
