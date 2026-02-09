package ray_test

import (
	"testing"

	"github.com/blang/semver/v4"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/lburgazzoli/odh-cli/pkg/lint/check"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/shared/testutil"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/workloads/ray"
	"github.com/lburgazzoli/odh-cli/pkg/resources"

	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

const (
	finalizerCodeFlareOAuth = "ray.openshift.ai/oauth-finalizer"
)

//nolint:gochecknoglobals // Test fixture - shared across test functions
var listKinds = map[schema.GroupVersionResource]string{
	resources.RayCluster.GVR(): resources.RayCluster.ListKind(),
}

func TestImpactedWorkloadsCheck_NoResources(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:     listKinds,
		TargetVersion: "3.0.0",
	})

	impactedCheck := ray.NewImpactedWorkloadsCheck()
	result, err := impactedCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(ray.ConditionTypeCodeFlareRayClusterCompatible),
		"Status":  Equal(metav1.ConditionTrue),
		"Reason":  Equal(check.ReasonVersionCompatible),
		"Message": ContainSubstring("No CodeFlare-managed RayCluster(s) found"),
	}))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "0"))
}

func TestImpactedWorkloadsCheck_WithCodeFlareFinalizer(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	rayCluster := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.RayCluster.APIVersion(),
			"kind":       resources.RayCluster.Kind,
			"metadata": map[string]any{
				"name":      "my-ray-cluster",
				"namespace": "test-ns",
				"finalizers": []any{
					finalizerCodeFlareOAuth,
				},
			},
		},
	}

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:     listKinds,
		Objects:       []*unstructured.Unstructured{rayCluster},
		TargetVersion: "3.0.0",
	})

	impactedCheck := ray.NewImpactedWorkloadsCheck()
	result, err := impactedCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(ray.ConditionTypeCodeFlareRayClusterCompatible),
		"Status": Equal(metav1.ConditionFalse),
		"Reason": Equal(check.ReasonVersionIncompatible),
		"Message": And(
			ContainSubstring("Found 1 CodeFlare-managed RayCluster(s)"),
			ContainSubstring("will be impacted in RHOAI 3.x"),
		),
	}))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "1"))
}

func TestImpactedWorkloadsCheck_WithoutCodeFlareFinalizer(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	rayCluster := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.RayCluster.APIVersion(),
			"kind":       resources.RayCluster.Kind,
			"metadata": map[string]any{
				"name":      "standalone-cluster",
				"namespace": "test-ns",
				"finalizers": []any{
					"some-other-finalizer",
				},
			},
		},
	}

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:     listKinds,
		Objects:       []*unstructured.Unstructured{rayCluster},
		TargetVersion: "3.0.0",
	})

	impactedCheck := ray.NewImpactedWorkloadsCheck()
	result, err := impactedCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(ray.ConditionTypeCodeFlareRayClusterCompatible),
		"Status": Equal(metav1.ConditionTrue),
	}))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "0"))
}

func TestImpactedWorkloadsCheck_NoFinalizers(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	rayCluster := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.RayCluster.APIVersion(),
			"kind":       resources.RayCluster.Kind,
			"metadata": map[string]any{
				"name":      "standalone-cluster",
				"namespace": "test-ns",
			},
		},
	}

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:     listKinds,
		Objects:       []*unstructured.Unstructured{rayCluster},
		TargetVersion: "3.0.0",
	})

	impactedCheck := ray.NewImpactedWorkloadsCheck()
	result, err := impactedCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(ray.ConditionTypeCodeFlareRayClusterCompatible),
		"Status": Equal(metav1.ConditionTrue),
	}))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "0"))
}

func TestImpactedWorkloadsCheck_MultipleClusters(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	cluster1 := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.RayCluster.APIVersion(),
			"kind":       resources.RayCluster.Kind,
			"metadata": map[string]any{
				"name":      "codeflare-cluster-1",
				"namespace": "ns1",
				"finalizers": []any{
					finalizerCodeFlareOAuth,
				},
			},
		},
	}

	cluster2 := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.RayCluster.APIVersion(),
			"kind":       resources.RayCluster.Kind,
			"metadata": map[string]any{
				"name":      "codeflare-cluster-2",
				"namespace": "ns2",
				"finalizers": []any{
					finalizerCodeFlareOAuth,
					"other-finalizer",
				},
			},
		},
	}

	cluster3 := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.RayCluster.APIVersion(),
			"kind":       resources.RayCluster.Kind,
			"metadata": map[string]any{
				"name":      "standalone-cluster",
				"namespace": "ns3",
			},
		},
	}

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:     listKinds,
		Objects:       []*unstructured.Unstructured{cluster1, cluster2, cluster3},
		TargetVersion: "3.0.0",
	})

	impactedCheck := ray.NewImpactedWorkloadsCheck()
	result, err := impactedCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(ray.ConditionTypeCodeFlareRayClusterCompatible),
		"Status": Equal(metav1.ConditionFalse),
		"Reason": Equal(check.ReasonVersionIncompatible),
		"Message": And(
			ContainSubstring("Found 2 CodeFlare-managed RayCluster(s)"),
			ContainSubstring("will be impacted in RHOAI 3.x"),
		),
	}))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "2"))
}

func TestImpactedWorkloadsCheck_Metadata(t *testing.T) {
	g := NewWithT(t)

	impactedCheck := ray.NewImpactedWorkloadsCheck()

	g.Expect(impactedCheck.ID()).To(Equal("workloads.ray.impacted-workloads"))
	g.Expect(impactedCheck.Name()).To(Equal("Workloads :: Ray :: Impacted Workloads (3.x)"))
	g.Expect(impactedCheck.Group()).To(Equal(check.GroupWorkload))
	g.Expect(impactedCheck.Description()).ToNot(BeEmpty())
}

func TestImpactedWorkloadsCheck_CanApply(t *testing.T) {
	g := NewWithT(t)

	impactedCheck := ray.NewImpactedWorkloadsCheck()
	ctx := t.Context()

	// Should not apply when target is nil
	g.Expect(impactedCheck.CanApply(ctx, check.Target{})).To(BeFalse())

	// Should not apply for 2.x to 2.x
	v2_15 := semver.MustParse("2.15.0")
	v2_17 := semver.MustParse("2.17.0")
	target2x := check.Target{CurrentVersion: &v2_15, TargetVersion: &v2_17}
	g.Expect(impactedCheck.CanApply(ctx, target2x)).To(BeFalse())

	// Should apply for 2.x to 3.x
	v3_0 := semver.MustParse("3.0.0")
	target2xTo3x := check.Target{CurrentVersion: &v2_17, TargetVersion: &v3_0}
	g.Expect(impactedCheck.CanApply(ctx, target2xTo3x)).To(BeTrue())

	// Should not apply for 3.x to 3.x
	v3_1 := semver.MustParse("3.1.0")
	target3x := check.Target{CurrentVersion: &v3_0, TargetVersion: &v3_1}
	g.Expect(impactedCheck.CanApply(ctx, target3x)).To(BeFalse())
}
