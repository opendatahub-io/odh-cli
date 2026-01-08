package kserve_test

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
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/workloads/kserve"
	"github.com/lburgazzoli/odh-cli/pkg/resources"
	"github.com/lburgazzoli/odh-cli/pkg/util/client"
	"github.com/lburgazzoli/odh-cli/pkg/util/version"

	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

const (
	annotationDeploymentMode = "serving.kserve.io/deploymentMode"
)

//nolint:gochecknoglobals // Test fixture - shared across test functions
var listKinds = map[schema.GroupVersionResource]string{
	resources.InferenceService.GVR(): resources.InferenceService.ListKind(),
	resources.ServingRuntime.GVR():   resources.ServingRuntime.ListKind(),
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

	impactedCheck := &kserve.ImpactedWorkloadsCheck{}
	result, err := impactedCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0]).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeCompatible),
		"Status":  Equal(metav1.ConditionTrue),
		"Reason":  Equal(check.ReasonVersionCompatible),
		"Message": ContainSubstring("No InferenceServices or ServingRuntimes using deprecated deployment modes found"),
	}))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "0"))
}

func TestImpactedWorkloadsCheck_ModelMeshInferenceService(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	isvc := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.InferenceService.APIVersion(),
			"kind":       resources.InferenceService.Kind,
			"metadata": map[string]any{
				"name":      "my-model",
				"namespace": "test-ns",
				"annotations": map[string]any{
					annotationDeploymentMode: "ModelMesh",
				},
			},
		},
	}

	scheme := runtime.NewScheme()
	_ = metav1.AddMetaToScheme(scheme)
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, listKinds, isvc)
	metadataClient := metadatafake.NewSimpleMetadataClient(scheme, toPartialObjectMetadata(isvc)...)

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

	impactedCheck := &kserve.ImpactedWorkloadsCheck{}
	result, err := impactedCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0]).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeCompatible),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonVersionIncompatible),
		"Message": And(ContainSubstring("Found 1 deprecated KServe workload(s)"), ContainSubstring("[1 ModelMesh]")),
	}))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "1"))
}

func TestImpactedWorkloadsCheck_ServerlessInferenceService(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	isvc := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.InferenceService.APIVersion(),
			"kind":       resources.InferenceService.Kind,
			"metadata": map[string]any{
				"name":      "serverless-model",
				"namespace": "test-ns",
				"annotations": map[string]any{
					annotationDeploymentMode: "Serverless",
				},
			},
		},
	}

	scheme := runtime.NewScheme()
	_ = metav1.AddMetaToScheme(scheme)
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, listKinds, isvc)
	metadataClient := metadatafake.NewSimpleMetadataClient(scheme, toPartialObjectMetadata(isvc)...)

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

	impactedCheck := &kserve.ImpactedWorkloadsCheck{}
	result, err := impactedCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0]).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeCompatible),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonVersionIncompatible),
		"Message": And(ContainSubstring("Found 1 deprecated KServe workload(s)"), ContainSubstring("[1 Serverless]")),
	}))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "1"))
}

func TestImpactedWorkloadsCheck_ModelMeshServingRuntime(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	sr := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.ServingRuntime.APIVersion(),
			"kind":       resources.ServingRuntime.Kind,
			"metadata": map[string]any{
				"name":      "my-runtime",
				"namespace": "test-ns",
				"annotations": map[string]any{
					annotationDeploymentMode: "ModelMesh",
				},
			},
		},
	}

	scheme := runtime.NewScheme()
	_ = metav1.AddMetaToScheme(scheme)
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, listKinds, sr)
	metadataClient := metadatafake.NewSimpleMetadataClient(scheme, toPartialObjectMetadata(sr)...)

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

	impactedCheck := &kserve.ImpactedWorkloadsCheck{}
	result, err := impactedCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0]).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeCompatible),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonVersionIncompatible),
		"Message": And(ContainSubstring("Found 1 deprecated KServe workload(s)"), ContainSubstring("[1 ModelMesh]")),
	}))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "1"))
}

func TestImpactedWorkloadsCheck_ServerlessServingRuntime_NotFlagged(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	// ServingRuntime with Serverless should NOT be flagged
	sr := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.ServingRuntime.APIVersion(),
			"kind":       resources.ServingRuntime.Kind,
			"metadata": map[string]any{
				"name":      "serverless-runtime",
				"namespace": "test-ns",
				"annotations": map[string]any{
					annotationDeploymentMode: "Serverless",
				},
			},
		},
	}

	scheme := runtime.NewScheme()
	_ = metav1.AddMetaToScheme(scheme)
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, listKinds, sr)
	metadataClient := metadatafake.NewSimpleMetadataClient(scheme, toPartialObjectMetadata(sr)...)

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

	impactedCheck := &kserve.ImpactedWorkloadsCheck{}
	result, err := impactedCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0]).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(check.ConditionTypeCompatible),
		"Status": Equal(metav1.ConditionTrue),
	}))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "0"))
}

func TestImpactedWorkloadsCheck_RawDeploymentAnnotation(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	isvc := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.InferenceService.APIVersion(),
			"kind":       resources.InferenceService.Kind,
			"metadata": map[string]any{
				"name":      "my-model",
				"namespace": "test-ns",
				"annotations": map[string]any{
					annotationDeploymentMode: "RawDeployment",
				},
			},
		},
	}

	scheme := runtime.NewScheme()
	_ = metav1.AddMetaToScheme(scheme)
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, listKinds, isvc)
	metadataClient := metadatafake.NewSimpleMetadataClient(scheme, toPartialObjectMetadata(isvc)...)

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

	impactedCheck := &kserve.ImpactedWorkloadsCheck{}
	result, err := impactedCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0]).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(check.ConditionTypeCompatible),
		"Status": Equal(metav1.ConditionTrue),
	}))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "0"))
}

func TestImpactedWorkloadsCheck_NoAnnotation(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	isvc := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.InferenceService.APIVersion(),
			"kind":       resources.InferenceService.Kind,
			"metadata": map[string]any{
				"name":      "my-model",
				"namespace": "test-ns",
			},
			"spec": map[string]any{
				"predictor": map[string]any{},
			},
		},
	}

	scheme := runtime.NewScheme()
	_ = metav1.AddMetaToScheme(scheme)
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, listKinds, isvc)
	metadataClient := metadatafake.NewSimpleMetadataClient(scheme, toPartialObjectMetadata(isvc)...)

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

	impactedCheck := &kserve.ImpactedWorkloadsCheck{}
	result, err := impactedCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0]).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(check.ConditionTypeCompatible),
		"Status": Equal(metav1.ConditionTrue),
	}))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "0"))
}

func TestImpactedWorkloadsCheck_MixedWorkloads(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	isvc1 := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.InferenceService.APIVersion(),
			"kind":       resources.InferenceService.Kind,
			"metadata": map[string]any{
				"name":      "modelmesh-model",
				"namespace": "ns1",
				"annotations": map[string]any{
					annotationDeploymentMode: "ModelMesh",
				},
			},
		},
	}

	isvc2 := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.InferenceService.APIVersion(),
			"kind":       resources.InferenceService.Kind,
			"metadata": map[string]any{
				"name":      "serverless-model",
				"namespace": "ns2",
				"annotations": map[string]any{
					annotationDeploymentMode: "Serverless",
				},
			},
		},
	}

	isvc3 := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.InferenceService.APIVersion(),
			"kind":       resources.InferenceService.Kind,
			"metadata": map[string]any{
				"name":      "raw-model",
				"namespace": "ns3",
				"annotations": map[string]any{
					annotationDeploymentMode: "RawDeployment",
				},
			},
		},
	}

	sr1 := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.ServingRuntime.APIVersion(),
			"kind":       resources.ServingRuntime.Kind,
			"metadata": map[string]any{
				"name":      "modelmesh-runtime",
				"namespace": "ns4",
				"annotations": map[string]any{
					annotationDeploymentMode: "ModelMesh",
				},
			},
		},
	}

	scheme := runtime.NewScheme()
	_ = metav1.AddMetaToScheme(scheme)
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, listKinds, isvc1, isvc2, isvc3, sr1)
	metadataClient := metadatafake.NewSimpleMetadataClient(scheme, toPartialObjectMetadata(isvc1, isvc2, isvc3, sr1)...)

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

	impactedCheck := &kserve.ImpactedWorkloadsCheck{}
	result, err := impactedCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0]).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(check.ConditionTypeCompatible),
		"Status": Equal(metav1.ConditionFalse),
		"Reason": Equal(check.ReasonVersionIncompatible),
		"Message": And(
			ContainSubstring("Found 3 deprecated KServe workload(s)"),
			ContainSubstring("(2 InferenceService(s), 1 ServingRuntime(s))"),
			ContainSubstring("[1 Serverless, 2 ModelMesh]"),
		),
	}))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "3"))
}

func TestImpactedWorkloadsCheck_Metadata(t *testing.T) {
	g := NewWithT(t)

	impactedCheck := kserve.NewImpactedWorkloadsCheck()

	g.Expect(impactedCheck.ID()).To(Equal("workloads.kserve.impacted-workloads"))
	g.Expect(impactedCheck.Name()).To(Equal("Workloads :: KServe :: Impacted Workloads (3.x)"))
	g.Expect(impactedCheck.Group()).To(Equal(check.GroupWorkload))
	g.Expect(impactedCheck.Description()).ToNot(BeEmpty())
}

func TestImpactedWorkloadsCheck_CanApply(t *testing.T) {
	g := NewWithT(t)

	impactedCheck := kserve.NewImpactedWorkloadsCheck()

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
