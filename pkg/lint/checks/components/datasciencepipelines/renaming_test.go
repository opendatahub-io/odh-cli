package datasciencepipelines_test

import (
	"testing"

	"github.com/blang/semver/v4"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	"github.com/lburgazzoli/odh-cli/pkg/lint/check"
	"github.com/lburgazzoli/odh-cli/pkg/lint/check/result"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/components/datasciencepipelines"
	"github.com/lburgazzoli/odh-cli/pkg/resources"
	"github.com/lburgazzoli/odh-cli/pkg/util/client"

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

	scheme := runtime.NewScheme()
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, listKinds)

	c := client.NewForTesting(client.TestClientConfig{
		Dynamic: dynamicClient,
	})

	ver := semver.MustParse("3.0.0")
	target := check.Target{
		Client:        c,
		TargetVersion: &ver,
	}

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

	dsc := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.DataScienceCluster.APIVersion(),
			"kind":       resources.DataScienceCluster.Kind,
			"metadata": map[string]any{
				"name": "default-dsc",
			},
			"spec": map[string]any{
				"components": map[string]any{
					"datasciencepipelines": map[string]any{
						"managementState": "Managed",
					},
				},
			},
		},
	}

	scheme := runtime.NewScheme()
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, listKinds, dsc)

	c := client.NewForTesting(client.TestClientConfig{
		Dynamic: dynamicClient,
	})

	ver := semver.MustParse("3.0.0")
	target := check.Target{
		Client:        c,
		TargetVersion: &ver,
	}

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

	dsc := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.DataScienceCluster.APIVersion(),
			"kind":       resources.DataScienceCluster.Kind,
			"metadata": map[string]any{
				"name": "default-dsc",
			},
			"spec": map[string]any{
				"components": map[string]any{
					"datasciencepipelines": map[string]any{
						"managementState": "Unmanaged",
					},
				},
			},
		},
	}

	scheme := runtime.NewScheme()
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, listKinds, dsc)

	c := client.NewForTesting(client.TestClientConfig{
		Dynamic: dynamicClient,
	})

	ver := semver.MustParse("3.0.0")
	target := check.Target{
		Client:        c,
		TargetVersion: &ver,
	}

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

	dsc := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.DataScienceCluster.APIVersion(),
			"kind":       resources.DataScienceCluster.Kind,
			"metadata": map[string]any{
				"name": "default-dsc",
			},
			"spec": map[string]any{
				"components": map[string]any{
					"datasciencepipelines": map[string]any{
						"managementState": "Removed",
					},
				},
			},
		},
	}

	scheme := runtime.NewScheme()
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, listKinds, dsc)

	c := client.NewForTesting(client.TestClientConfig{
		Dynamic: dynamicClient,
	})

	ver := semver.MustParse("3.0.0")
	target := check.Target{
		Client:        c,
		TargetVersion: &ver,
	}

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
	v217 := semver.MustParse("2.17.0")
	canApply, err := renamingCheck.CanApply(ctx, check.Target{CurrentVersion: &v217, TargetVersion: &v217})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeFalse())

	// Should apply for 2.x -> 3.x upgrade
	v300 := semver.MustParse("3.0.0")
	canApply, err = renamingCheck.CanApply(ctx, check.Target{CurrentVersion: &v217, TargetVersion: &v300})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeTrue())

	// Should not apply for 3.x -> 3.x upgrade
	v310 := semver.MustParse("3.1.0")
	canApply, err = renamingCheck.CanApply(ctx, check.Target{CurrentVersion: &v300, TargetVersion: &v310})
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
