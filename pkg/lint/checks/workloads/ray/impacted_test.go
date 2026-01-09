package ray_test

import (
	"context"
	"testing"

	"github.com/blang/semver/v4"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	metadatafake "k8s.io/client-go/metadata/fake"

	"github.com/lburgazzoli/odh-cli/pkg/lint/check"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/workloads/ray"
	"github.com/lburgazzoli/odh-cli/pkg/resources"
	"github.com/lburgazzoli/odh-cli/pkg/util/client"
	"github.com/lburgazzoli/odh-cli/pkg/util/version"

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

func toPartialObjectMetadata(objs ...*unstructured.Unstructured) []runtime.Object {
	result := make([]runtime.Object, 0, len(objs))
	for _, obj := range objs {
		pom := &metav1.PartialObjectMetadata{
			TypeMeta: metav1.TypeMeta{
				APIVersion: obj.GetAPIVersion(),
				Kind:       obj.GetKind(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:        obj.GetName(),
				Namespace:   obj.GetNamespace(),
				Labels:      obj.GetLabels(),
				Annotations: obj.GetAnnotations(),
				Finalizers:  obj.GetFinalizers(),
			},
		}
		result = append(result, pom)
	}

	return result
}

func TestImpactedWorkloadsCheck_NoResources(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	scheme := runtime.NewScheme()
	_ = metav1.AddMetaToScheme(scheme)
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, listKinds)
	metadataClient := metadatafake.NewSimpleMetadataClient(scheme)

	c := &client.Client{
		Dynamic:  dynamicClient,
		Metadata: metadataClient,
	}

	target := &check.CheckTarget{
		Client: c,
		Version: &version.ClusterVersion{
			Version: "3.0.0",
		},
	}

	impactedCheck := &ray.ImpactedWorkloadsCheck{}
	result, err := impactedCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0]).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(ray.ConditionTypeCodeFlareRayClusterCompatible),
		"Status":  Equal(metav1.ConditionTrue),
		"Reason":  Equal(check.ReasonVersionCompatible),
		"Message": ContainSubstring("No CodeFlare-managed RayCluster(s) found"),
	}))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "0"))
}

func TestImpactedWorkloadsCheck_WithCodeFlareFinalizer(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

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

	scheme := runtime.NewScheme()
	_ = metav1.AddMetaToScheme(scheme)
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, listKinds, rayCluster)
	metadataClient := metadatafake.NewSimpleMetadataClient(scheme, toPartialObjectMetadata(rayCluster)...)

	c := &client.Client{
		Dynamic:  dynamicClient,
		Metadata: metadataClient,
	}

	target := &check.CheckTarget{
		Client: c,
		Version: &version.ClusterVersion{
			Version: "3.0.0",
		},
	}

	impactedCheck := &ray.ImpactedWorkloadsCheck{}
	result, err := impactedCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0]).To(MatchFields(IgnoreExtras, Fields{
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
	ctx := context.Background()

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

	scheme := runtime.NewScheme()
	_ = metav1.AddMetaToScheme(scheme)
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, listKinds, rayCluster)
	metadataClient := metadatafake.NewSimpleMetadataClient(scheme, toPartialObjectMetadata(rayCluster)...)

	c := &client.Client{
		Dynamic:  dynamicClient,
		Metadata: metadataClient,
	}

	target := &check.CheckTarget{
		Client: c,
		Version: &version.ClusterVersion{
			Version: "3.0.0",
		},
	}

	impactedCheck := &ray.ImpactedWorkloadsCheck{}
	result, err := impactedCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0]).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(ray.ConditionTypeCodeFlareRayClusterCompatible),
		"Status": Equal(metav1.ConditionTrue),
	}))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "0"))
}

func TestImpactedWorkloadsCheck_NoFinalizers(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

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

	scheme := runtime.NewScheme()
	_ = metav1.AddMetaToScheme(scheme)
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, listKinds, rayCluster)
	metadataClient := metadatafake.NewSimpleMetadataClient(scheme, toPartialObjectMetadata(rayCluster)...)

	c := &client.Client{
		Dynamic:  dynamicClient,
		Metadata: metadataClient,
	}

	target := &check.CheckTarget{
		Client: c,
		Version: &version.ClusterVersion{
			Version: "3.0.0",
		},
	}

	impactedCheck := &ray.ImpactedWorkloadsCheck{}
	result, err := impactedCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0]).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(ray.ConditionTypeCodeFlareRayClusterCompatible),
		"Status": Equal(metav1.ConditionTrue),
	}))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "0"))
}

func TestImpactedWorkloadsCheck_MultipleClusters(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

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

	scheme := runtime.NewScheme()
	_ = metav1.AddMetaToScheme(scheme)
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		scheme,
		listKinds,
		cluster1,
		cluster2,
		cluster3,
	)
	metadataClient := metadatafake.NewSimpleMetadataClient(scheme, toPartialObjectMetadata(cluster1, cluster2, cluster3)...)

	c := &client.Client{
		Dynamic:  dynamicClient,
		Metadata: metadataClient,
	}

	target := &check.CheckTarget{
		Client: c,
		Version: &version.ClusterVersion{
			Version: "3.0.0",
		},
	}

	impactedCheck := &ray.ImpactedWorkloadsCheck{}
	result, err := impactedCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0]).To(MatchFields(IgnoreExtras, Fields{
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

	// Should not apply when versions are nil
	g.Expect(impactedCheck.CanApply(nil, nil)).To(BeFalse())

	// Should not apply for 2.x to 2.x
	v2_15, _ := semver.Parse("2.15.0")
	v2_17, _ := semver.Parse("2.17.0")
	g.Expect(impactedCheck.CanApply(&v2_15, &v2_17)).To(BeFalse())

	// Should apply for 2.x to 3.x
	v3_0, _ := semver.Parse("3.0.0")
	g.Expect(impactedCheck.CanApply(&v2_17, &v3_0)).To(BeTrue())

	// Should not apply for 3.x to 3.x
	v3_1, _ := semver.Parse("3.1.0")
	g.Expect(impactedCheck.CanApply(&v3_0, &v3_1)).To(BeFalse())
}
