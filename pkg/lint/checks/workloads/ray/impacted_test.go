package ray_test

import (
	"testing"

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
	resources.RayCluster.GVR():         resources.RayCluster.ListKind(),
	resources.DataScienceCluster.GVR(): resources.DataScienceCluster.ListKind(),
}

func TestImpactedWorkloadsCheck_NoResources(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{testutil.NewDSC(map[string]string{"ray": "Managed"})},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
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

	dsc := testutil.NewDSC(map[string]string{"ray": "Managed"})
	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{rayCluster, dsc},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
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

	dsc := testutil.NewDSC(map[string]string{"ray": "Managed"})
	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{rayCluster, dsc},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
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

	dsc := testutil.NewDSC(map[string]string{"ray": "Managed"})
	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{rayCluster, dsc},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
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

	dsc := testutil.NewDSC(map[string]string{"ray": "Managed"})
	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{cluster1, cluster2, cluster3, dsc},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
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

	// Should not apply when versions are nil
	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds: listKinds,
		Objects:   []*unstructured.Unstructured{testutil.NewDSC(map[string]string{"ray": "Managed"})},
	})
	canApply, err := impactedCheck.CanApply(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeFalse())

	// Should not apply for 2.x to 2.x
	target = testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{testutil.NewDSC(map[string]string{"ray": "Managed"})},
		CurrentVersion: "2.15.0",
		TargetVersion:  "2.17.0",
	})
	canApply, err = impactedCheck.CanApply(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeFalse())

	// Should apply for 2.x to 3.x with ray Managed
	target = testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{testutil.NewDSC(map[string]string{"ray": "Managed"})},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})
	canApply, err = impactedCheck.CanApply(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeTrue())

	// Should not apply for 2.x to 3.x with ray Removed
	target = testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{testutil.NewDSC(map[string]string{"ray": "Removed"})},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})
	canApply, err = impactedCheck.CanApply(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeFalse())

	// Should not apply for 3.x to 3.x
	target = testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{testutil.NewDSC(map[string]string{"ray": "Managed"})},
		CurrentVersion: "3.0.0",
		TargetVersion:  "3.1.0",
	})
	canApply, err = impactedCheck.CanApply(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeFalse())
}
