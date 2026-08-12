package aigateway_test

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	resultpkg "github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/testutil"
	"github.com/opendatahub-io/odh-cli/pkg/lint/checks/components/aigateway"
	"github.com/opendatahub-io/odh-cli/pkg/resources"

	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

//nolint:gochecknoglobals // Test fixture - shared across test functions
var listKinds = map[schema.GroupVersionResource]string{
	resources.DataScienceCluster.GVR(): resources.DataScienceCluster.ListKind(),
}

// newDSCWithKserveMaaS creates a DSC with the deprecated kserve.modelsAsService field set.
func newDSCWithKserveMaaS(state string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.DataScienceCluster.APIVersion(),
			"kind":       resources.DataScienceCluster.Kind,
			"metadata": map[string]any{
				"name": "default-dsc",
			},
			"spec": map[string]any{
				"components": map[string]any{
					"kserve": map[string]any{
						"managementState": "Managed",
						"modelsAsService": map[string]any{
							"managementState": state,
						},
					},
				},
			},
		},
	}
}

func TestMaaSFieldMigrationCheck_CanApply_NoDSC(t *testing.T) {
	g := NewWithT(t)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		CurrentVersion: "3.4.0",
		TargetVersion:  "3.5.0",
	})

	chk := aigateway.NewMaaSFieldMigrationCheck()
	canApply, err := chk.CanApply(t.Context(), target)

	g.Expect(err).To(HaveOccurred())
	g.Expect(canApply).To(BeFalse())
}

func TestMaaSFieldMigrationCheck_CanApply_OldFieldNotSet(t *testing.T) {
	g := NewWithT(t)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{testutil.NewDSC(map[string]string{"kserve": "Managed"})},
		CurrentVersion: "3.4.0",
		TargetVersion:  "3.5.0",
	})

	chk := aigateway.NewMaaSFieldMigrationCheck()
	canApply, err := chk.CanApply(t.Context(), target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeFalse())
}

func TestMaaSFieldMigrationCheck_CanApply_OldFieldManaged(t *testing.T) {
	g := NewWithT(t)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{newDSCWithKserveMaaS("Managed")},
		CurrentVersion: "3.4.0",
		TargetVersion:  "3.5.0",
	})

	chk := aigateway.NewMaaSFieldMigrationCheck()
	canApply, err := chk.CanApply(t.Context(), target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeTrue())
}

func TestMaaSFieldMigrationCheck_CanApply_OldFieldRemoved(t *testing.T) {
	g := NewWithT(t)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{newDSCWithKserveMaaS("Removed")},
		CurrentVersion: "3.4.0",
		TargetVersion:  "3.5.0",
	})

	chk := aigateway.NewMaaSFieldMigrationCheck()
	canApply, err := chk.CanApply(t.Context(), target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeFalse())
}

func TestMaaSFieldMigrationCheck_CanApply_NotUpgrade34To35(t *testing.T) {
	tests := []struct {
		name    string
		current string
		target  string
	}{
		{"no current version", "", "3.5.0"},
		{"same version 3.5", "3.5.0", "3.5.0"},
		{"3.5 to 3.6", "3.5.0", "3.6.0"},
		{"2.x to 3.5", "2.17.0", "3.5.0"},
		{"3.3 to 3.5", "3.3.0", "3.5.0"},
		{"3.4 to 3.6", "3.4.0", "3.6.0"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			cfg := testutil.TargetConfig{
				ListKinds: listKinds,
				Objects:   []*unstructured.Unstructured{newDSCWithKserveMaaS("Managed")},
			}
			if tt.current != "" {
				cfg.CurrentVersion = tt.current
			}
			if tt.target != "" {
				cfg.TargetVersion = tt.target
			}

			target := testutil.NewTarget(t, cfg)
			chk := aigateway.NewMaaSFieldMigrationCheck()
			canApply, err := chk.CanApply(t.Context(), target)

			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(canApply).To(BeFalse())
		})
	}
}

func TestMaaSFieldMigrationCheck_Validate_OldFieldManaged_Advisory(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{newDSCWithKserveMaaS("Managed")},
		CurrentVersion: "3.4.0",
		TargetVersion:  "3.5.0",
	})

	chk := aigateway.NewMaaSFieldMigrationCheck()
	res, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(res.Status.Conditions).To(HaveLen(1))
	g.Expect(res.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(check.ConditionTypeCompatible),
		"Status": Equal(metav1.ConditionFalse),
		"Reason": Equal(check.ReasonVersionIncompatible),
		"Message": And(
			ContainSubstring("kserve.modelsAsService"),
			ContainSubstring("aigateway.modelsAsAService"),
			ContainSubstring("3.5"),
		),
	}))
	g.Expect(res.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactAdvisory))
}

func TestMaaSFieldMigrationCheck_Validate_OldFieldNotSet(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{testutil.NewDSC(map[string]string{"kserve": "Managed"})},
		CurrentVersion: "3.4.0",
		TargetVersion:  "3.5.0",
	})

	chk := aigateway.NewMaaSFieldMigrationCheck()
	res, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(res.Status.Conditions).To(HaveLen(1))
	g.Expect(res.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(check.ConditionTypeConfigured),
		"Status": Equal(metav1.ConditionTrue),
		"Reason": Equal(check.ReasonRequirementsMet),
	}))
}

func TestMaaSFieldMigrationCheck_Metadata(t *testing.T) {
	g := NewWithT(t)

	chk := aigateway.NewMaaSFieldMigrationCheck()

	g.Expect(chk.ID()).To(Equal("components.aigateway.maas-field-migration"))
	g.Expect(chk.Name()).To(Equal("Components :: AIGateway :: MaaS Field Migration (3.4 to 3.5)"))
	g.Expect(chk.Group()).To(Equal(check.GroupComponent))
	g.Expect(chk.Description()).ToNot(BeEmpty())
	g.Expect(chk.Remediation()).ToNot(BeEmpty())
}
