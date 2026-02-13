package kserve_test

import (
	"testing"

	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	resultpkg "github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/testutil"
	"github.com/opendatahub-io/odh-cli/pkg/lint/checks/workloads/kserve"
	"github.com/opendatahub-io/odh-cli/pkg/resources"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

const (
	annotationDeploymentMode = "serving.kserve.io/deploymentMode"
)

//nolint:gochecknoglobals // Test fixture - shared across test functions
var listKinds = map[schema.GroupVersionResource]string{
	resources.InferenceService.GVR():   resources.InferenceService.ListKind(),
	resources.ServingRuntime.GVR():     resources.ServingRuntime.ListKind(),
	resources.DataScienceCluster.GVR(): resources.DataScienceCluster.ListKind(),
}

func TestImpactedWorkloadsCheck_NoResources(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	impactedCheck := &kserve.ImpactedWorkloadsCheck{}
	result, err := impactedCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(7))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal("ServerlessInferenceServicesCompatible"),
		"Status":  Equal(metav1.ConditionTrue),
		"Reason":  Equal(check.ReasonVersionCompatible),
		"Message": ContainSubstring("No Serverless InferenceService(s) found"),
	}))
	g.Expect(result.Status.Conditions[1].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal("ModelMeshInferenceServicesCompatible"),
		"Status":  Equal(metav1.ConditionTrue),
		"Reason":  Equal(check.ReasonVersionCompatible),
		"Message": ContainSubstring("No ModelMesh InferenceService(s) found"),
	}))
	g.Expect(result.Status.Conditions[2].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal("ModelMeshServingRuntimesCompatible"),
		"Status":  Equal(metav1.ConditionTrue),
		"Reason":  Equal(check.ReasonVersionCompatible),
		"Message": ContainSubstring("No ModelMesh ServingRuntime(s) found"),
	}))
	g.Expect(result.Status.Conditions[3].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal("RemovedServingRuntimesCompatible"),
		"Status":  Equal(metav1.ConditionTrue),
		"Reason":  Equal(check.ReasonVersionCompatible),
		"Message": ContainSubstring("No InferenceService(s) using removed ServingRuntime(s) found"),
	}))
	g.Expect(result.Status.Conditions[4].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(kserve.ConditionTypeAcceleratorOnlySRCompatible),
		"Status": Equal(metav1.ConditionTrue),
	}))
	g.Expect(result.Status.Conditions[5].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(kserve.ConditionTypeAcceleratorAndHWProfileSRCompat),
		"Status": Equal(metav1.ConditionTrue),
	}))
	g.Expect(result.Status.Conditions[6].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(kserve.ConditionTypeAcceleratorSRISVCCompatible),
		"Status": Equal(metav1.ConditionTrue),
	}))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "0"))
}

func TestImpactedWorkloadsCheck_ModelMeshInferenceService(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

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

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{isvc},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	impactedCheck := &kserve.ImpactedWorkloadsCheck{}
	result, err := impactedCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(7))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal("ServerlessInferenceServicesCompatible"),
		"Status":  Equal(metav1.ConditionTrue),
		"Reason":  Equal(check.ReasonVersionCompatible),
		"Message": ContainSubstring("No Serverless InferenceService(s) found"),
	}))
	g.Expect(result.Status.Conditions[1].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal("ModelMeshInferenceServicesCompatible"),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonVersionIncompatible),
		"Message": ContainSubstring("Found 1 ModelMesh InferenceService(s)"),
	}))
	g.Expect(result.Status.Conditions[1].Impact).To(Equal(resultpkg.ImpactBlocking))
	g.Expect(result.Status.Conditions[2].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal("ModelMeshServingRuntimesCompatible"),
		"Status":  Equal(metav1.ConditionTrue),
		"Reason":  Equal(check.ReasonVersionCompatible),
		"Message": ContainSubstring("No ModelMesh ServingRuntime(s) found"),
	}))
	g.Expect(result.Status.Conditions[3].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal("RemovedServingRuntimesCompatible"),
		"Status":  Equal(metav1.ConditionTrue),
		"Reason":  Equal(check.ReasonVersionCompatible),
		"Message": ContainSubstring("No InferenceService(s) using removed ServingRuntime(s) found"),
	}))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "1"))
}

func TestImpactedWorkloadsCheck_ServerlessInferenceService(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

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

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{isvc},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	impactedCheck := &kserve.ImpactedWorkloadsCheck{}
	result, err := impactedCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(7))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal("ServerlessInferenceServicesCompatible"),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonVersionIncompatible),
		"Message": ContainSubstring("Found 1 Serverless InferenceService(s)"),
	}))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactBlocking))
	g.Expect(result.Status.Conditions[1].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal("ModelMeshInferenceServicesCompatible"),
		"Status":  Equal(metav1.ConditionTrue),
		"Reason":  Equal(check.ReasonVersionCompatible),
		"Message": ContainSubstring("No ModelMesh InferenceService(s) found"),
	}))
	g.Expect(result.Status.Conditions[2].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal("ModelMeshServingRuntimesCompatible"),
		"Status":  Equal(metav1.ConditionTrue),
		"Reason":  Equal(check.ReasonVersionCompatible),
		"Message": ContainSubstring("No ModelMesh ServingRuntime(s) found"),
	}))
	g.Expect(result.Status.Conditions[3].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal("RemovedServingRuntimesCompatible"),
		"Status":  Equal(metav1.ConditionTrue),
		"Reason":  Equal(check.ReasonVersionCompatible),
		"Message": ContainSubstring("No InferenceService(s) using removed ServingRuntime(s) found"),
	}))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "1"))
}

func TestImpactedWorkloadsCheck_ModelMeshServingRuntime(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	sr := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.ServingRuntime.APIVersion(),
			"kind":       resources.ServingRuntime.Kind,
			"metadata": map[string]any{
				"name":      "my-runtime",
				"namespace": "test-ns",
			},
			"spec": map[string]any{
				"multiModel": true,
			},
		},
	}

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{sr},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	impactedCheck := &kserve.ImpactedWorkloadsCheck{}
	result, err := impactedCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(7))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal("ServerlessInferenceServicesCompatible"),
		"Status":  Equal(metav1.ConditionTrue),
		"Reason":  Equal(check.ReasonVersionCompatible),
		"Message": ContainSubstring("No Serverless InferenceService(s) found"),
	}))
	g.Expect(result.Status.Conditions[1].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal("ModelMeshInferenceServicesCompatible"),
		"Status":  Equal(metav1.ConditionTrue),
		"Reason":  Equal(check.ReasonVersionCompatible),
		"Message": ContainSubstring("No ModelMesh InferenceService(s) found"),
	}))
	g.Expect(result.Status.Conditions[2].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal("ModelMeshServingRuntimesCompatible"),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonVersionIncompatible),
		"Message": ContainSubstring("Found 1 ModelMesh ServingRuntime(s)"),
	}))
	g.Expect(result.Status.Conditions[2].Impact).To(Equal(resultpkg.ImpactBlocking))
	g.Expect(result.Status.Conditions[3].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal("RemovedServingRuntimesCompatible"),
		"Status":  Equal(metav1.ConditionTrue),
		"Reason":  Equal(check.ReasonVersionCompatible),
		"Message": ContainSubstring("No InferenceService(s) using removed ServingRuntime(s) found"),
	}))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "1"))
}

func TestImpactedWorkloadsCheck_ServerlessServingRuntime_NotFlagged(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	// ServingRuntime without multiModel should NOT be flagged
	sr := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.ServingRuntime.APIVersion(),
			"kind":       resources.ServingRuntime.Kind,
			"metadata": map[string]any{
				"name":      "serverless-runtime",
				"namespace": "test-ns",
			},
			"spec": map[string]any{
				"multiModel": false,
			},
		},
	}

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{sr},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	impactedCheck := &kserve.ImpactedWorkloadsCheck{}
	result, err := impactedCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(7))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal("ServerlessInferenceServicesCompatible"),
		"Status": Equal(metav1.ConditionTrue),
	}))
	g.Expect(result.Status.Conditions[1].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal("ModelMeshInferenceServicesCompatible"),
		"Status": Equal(metav1.ConditionTrue),
	}))
	g.Expect(result.Status.Conditions[2].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal("ModelMeshServingRuntimesCompatible"),
		"Status": Equal(metav1.ConditionTrue),
	}))
	g.Expect(result.Status.Conditions[3].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal("RemovedServingRuntimesCompatible"),
		"Status": Equal(metav1.ConditionTrue),
	}))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "0"))
}

func TestImpactedWorkloadsCheck_RawDeploymentAnnotation(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

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

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{isvc},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	impactedCheck := &kserve.ImpactedWorkloadsCheck{}
	result, err := impactedCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(7))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal("ServerlessInferenceServicesCompatible"),
		"Status": Equal(metav1.ConditionTrue),
	}))
	g.Expect(result.Status.Conditions[1].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal("ModelMeshInferenceServicesCompatible"),
		"Status": Equal(metav1.ConditionTrue),
	}))
	g.Expect(result.Status.Conditions[2].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal("ModelMeshServingRuntimesCompatible"),
		"Status": Equal(metav1.ConditionTrue),
	}))
	g.Expect(result.Status.Conditions[3].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal("RemovedServingRuntimesCompatible"),
		"Status": Equal(metav1.ConditionTrue),
	}))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "0"))
}

func TestImpactedWorkloadsCheck_NoAnnotation(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

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

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{isvc},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	impactedCheck := &kserve.ImpactedWorkloadsCheck{}
	result, err := impactedCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(7))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal("ServerlessInferenceServicesCompatible"),
		"Status": Equal(metav1.ConditionTrue),
	}))
	g.Expect(result.Status.Conditions[1].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal("ModelMeshInferenceServicesCompatible"),
		"Status": Equal(metav1.ConditionTrue),
	}))
	g.Expect(result.Status.Conditions[2].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal("ModelMeshServingRuntimesCompatible"),
		"Status": Equal(metav1.ConditionTrue),
	}))
	g.Expect(result.Status.Conditions[3].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal("RemovedServingRuntimesCompatible"),
		"Status": Equal(metav1.ConditionTrue),
	}))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "0"))
}

func TestImpactedWorkloadsCheck_MixedWorkloads(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

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
			},
			"spec": map[string]any{
				"multiModel": true,
			},
		},
	}

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{isvc1, isvc2, isvc3, sr1},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	impactedCheck := &kserve.ImpactedWorkloadsCheck{}
	result, err := impactedCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(7))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal("ServerlessInferenceServicesCompatible"),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonVersionIncompatible),
		"Message": ContainSubstring("Found 1 Serverless InferenceService(s)"),
	}))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactBlocking))
	g.Expect(result.Status.Conditions[1].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal("ModelMeshInferenceServicesCompatible"),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonVersionIncompatible),
		"Message": ContainSubstring("Found 1 ModelMesh InferenceService(s)"),
	}))
	g.Expect(result.Status.Conditions[1].Impact).To(Equal(resultpkg.ImpactBlocking))
	g.Expect(result.Status.Conditions[2].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal("ModelMeshServingRuntimesCompatible"),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonVersionIncompatible),
		"Message": ContainSubstring("Found 1 ModelMesh ServingRuntime(s)"),
	}))
	g.Expect(result.Status.Conditions[2].Impact).To(Equal(resultpkg.ImpactBlocking))
	g.Expect(result.Status.Conditions[3].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal("RemovedServingRuntimesCompatible"),
		"Status":  Equal(metav1.ConditionTrue),
		"Reason":  Equal(check.ReasonVersionCompatible),
		"Message": ContainSubstring("No InferenceService(s) using removed ServingRuntime(s) found"),
	}))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "3"))
}

func TestImpactedWorkloadsCheck_RemovedRuntime_OVMS(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	isvc := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.InferenceService.APIVersion(),
			"kind":       resources.InferenceService.Kind,
			"metadata": map[string]any{
				"name":      "ovms-model",
				"namespace": "test-ns",
			},
			"spec": map[string]any{
				"predictor": map[string]any{
					"model": map[string]any{
						"runtime": "ovms",
					},
				},
			},
		},
	}

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{isvc},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	impactedCheck := &kserve.ImpactedWorkloadsCheck{}
	result, err := impactedCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(7))
	g.Expect(result.Status.Conditions[3].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal("RemovedServingRuntimesCompatible"),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonVersionIncompatible),
		"Message": ContainSubstring("Found 1 InferenceService(s) using removed ServingRuntime(s)"),
	}))
	g.Expect(result.Status.Conditions[3].Impact).To(Equal(resultpkg.ImpactBlocking))
	g.Expect(result.ImpactedObjects).To(ContainElement(MatchFields(IgnoreExtras, Fields{
		"ObjectMeta": MatchFields(IgnoreExtras, Fields{
			"Name":      Equal("ovms-model"),
			"Namespace": Equal("test-ns"),
			"Annotations": And(
				HaveKeyWithValue("serving.kserve.io/runtime", "ovms"),
			),
		}),
	})))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "1"))
}

func TestImpactedWorkloadsCheck_RemovedRuntime_CaikitTGIS(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	isvc := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.InferenceService.APIVersion(),
			"kind":       resources.InferenceService.Kind,
			"metadata": map[string]any{
				"name":      "caikit-model",
				"namespace": "test-ns",
			},
			"spec": map[string]any{
				"predictor": map[string]any{
					"model": map[string]any{
						"runtime": "caikit-tgis-serving-template",
					},
				},
			},
		},
	}

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{isvc},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	impactedCheck := &kserve.ImpactedWorkloadsCheck{}
	result, err := impactedCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(7))
	g.Expect(result.Status.Conditions[3].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal("RemovedServingRuntimesCompatible"),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonVersionIncompatible),
		"Message": ContainSubstring("Found 1 InferenceService(s) using removed ServingRuntime(s)"),
	}))
	g.Expect(result.Status.Conditions[3].Impact).To(Equal(resultpkg.ImpactBlocking))
	g.Expect(result.ImpactedObjects).To(ContainElement(MatchFields(IgnoreExtras, Fields{
		"ObjectMeta": MatchFields(IgnoreExtras, Fields{
			"Name":      Equal("caikit-model"),
			"Namespace": Equal("test-ns"),
			"Annotations": And(
				HaveKeyWithValue("serving.kserve.io/runtime", "caikit-tgis-serving-template"),
			),
		}),
	})))
}

func TestImpactedWorkloadsCheck_NonRemovedRuntime(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	isvc := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.InferenceService.APIVersion(),
			"kind":       resources.InferenceService.Kind,
			"metadata": map[string]any{
				"name":      "custom-model",
				"namespace": "test-ns",
			},
			"spec": map[string]any{
				"predictor": map[string]any{
					"model": map[string]any{
						"runtime": "my-custom-runtime",
					},
				},
			},
		},
	}

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{isvc},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	impactedCheck := &kserve.ImpactedWorkloadsCheck{}
	result, err := impactedCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(7))
	g.Expect(result.Status.Conditions[3].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal("RemovedServingRuntimesCompatible"),
		"Status": Equal(metav1.ConditionTrue),
		"Reason": Equal(check.ReasonVersionCompatible),
	}))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "0"))
}

func TestImpactedWorkloadsCheck_NoRuntimeField(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	// ISVC without spec.predictor.model.runtime should not be flagged
	isvc := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.InferenceService.APIVersion(),
			"kind":       resources.InferenceService.Kind,
			"metadata": map[string]any{
				"name":      "no-runtime-model",
				"namespace": "test-ns",
			},
			"spec": map[string]any{
				"predictor": map[string]any{
					"model": map[string]any{
						"modelFormat": map[string]any{
							"name": "sklearn",
						},
					},
				},
			},
		},
	}

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{isvc},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	impactedCheck := &kserve.ImpactedWorkloadsCheck{}
	result, err := impactedCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(7))
	g.Expect(result.Status.Conditions[3].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal("RemovedServingRuntimesCompatible"),
		"Status": Equal(metav1.ConditionTrue),
		"Reason": Equal(check.ReasonVersionCompatible),
	}))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "0"))
}

func TestImpactedWorkloadsCheck_MixedRemovedAndNonRemovedRuntimes(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	isvc1 := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.InferenceService.APIVersion(),
			"kind":       resources.InferenceService.Kind,
			"metadata": map[string]any{
				"name":      "ovms-model",
				"namespace": "ns1",
			},
			"spec": map[string]any{
				"predictor": map[string]any{
					"model": map[string]any{
						"runtime": "ovms",
					},
				},
			},
		},
	}

	isvc2 := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.InferenceService.APIVersion(),
			"kind":       resources.InferenceService.Kind,
			"metadata": map[string]any{
				"name":      "caikit-model",
				"namespace": "ns2",
			},
			"spec": map[string]any{
				"predictor": map[string]any{
					"model": map[string]any{
						"runtime": "caikit-standalone-serving-template",
					},
				},
			},
		},
	}

	isvc3 := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.InferenceService.APIVersion(),
			"kind":       resources.InferenceService.Kind,
			"metadata": map[string]any{
				"name":      "custom-model",
				"namespace": "ns3",
			},
			"spec": map[string]any{
				"predictor": map[string]any{
					"model": map[string]any{
						"runtime": "my-custom-runtime",
					},
				},
			},
		},
	}

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{isvc1, isvc2, isvc3},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	impactedCheck := &kserve.ImpactedWorkloadsCheck{}
	result, err := impactedCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(7))
	g.Expect(result.Status.Conditions[3].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal("RemovedServingRuntimesCompatible"),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonVersionIncompatible),
		"Message": ContainSubstring("Found 2 InferenceService(s) using removed ServingRuntime(s)"),
	}))
	g.Expect(result.Status.Conditions[3].Impact).To(Equal(resultpkg.ImpactBlocking))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "2"))
}

func TestImpactedWorkloadsCheck_Metadata(t *testing.T) {
	g := NewWithT(t)

	impactedCheck := kserve.NewImpactedWorkloadsCheck()

	g.Expect(impactedCheck.ID()).To(Equal("workloads.kserve.impacted-workloads"))
	g.Expect(impactedCheck.Name()).To(Equal("Workloads :: KServe :: Impacted Workloads (3.x)"))
	g.Expect(impactedCheck.Group()).To(Equal(check.GroupWorkload))
	g.Expect(impactedCheck.Description()).ToNot(BeEmpty())
}

func TestImpactedWorkloadsCheck_CanApply_NilVersions(t *testing.T) {
	g := NewWithT(t)

	chk := kserve.NewImpactedWorkloadsCheck()
	canApply, err := chk.CanApply(t.Context(), check.Target{})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeFalse())
}

func TestImpactedWorkloadsCheck_CanApply_2xTo2x(t *testing.T) {
	g := NewWithT(t)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{testutil.NewDSC(map[string]string{"kserve": "Managed"})},
		CurrentVersion: "2.15.0",
		TargetVersion:  "2.17.0",
	})

	chk := kserve.NewImpactedWorkloadsCheck()
	canApply, err := chk.CanApply(t.Context(), target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeFalse())
}

func TestImpactedWorkloadsCheck_CanApply_2xTo3x_KServeManaged(t *testing.T) {
	g := NewWithT(t)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{testutil.NewDSC(map[string]string{"kserve": "Managed"})},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := kserve.NewImpactedWorkloadsCheck()
	canApply, err := chk.CanApply(t.Context(), target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeTrue())
}

func TestImpactedWorkloadsCheck_CanApply_2xTo3x_ModelMeshManaged(t *testing.T) {
	g := NewWithT(t)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{testutil.NewDSC(map[string]string{"modelmeshserving": "Managed"})},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := kserve.NewImpactedWorkloadsCheck()
	canApply, err := chk.CanApply(t.Context(), target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeTrue())
}

func TestImpactedWorkloadsCheck_CanApply_2xTo3x_BothRemoved(t *testing.T) {
	g := NewWithT(t)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{testutil.NewDSC(map[string]string{"kserve": "Removed", "modelmeshserving": "Removed"})},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := kserve.NewImpactedWorkloadsCheck()
	canApply, err := chk.CanApply(t.Context(), target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeFalse())
}

func TestImpactedWorkloadsCheck_CanApply_3xTo3x(t *testing.T) {
	g := NewWithT(t)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{testutil.NewDSC(map[string]string{"kserve": "Managed"})},
		CurrentVersion: "3.0.0",
		TargetVersion:  "3.1.0",
	})

	chk := kserve.NewImpactedWorkloadsCheck()
	canApply, err := chk.CanApply(t.Context(), target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeFalse())
}

func TestImpactedWorkloadsCheck_AcceleratorOnlySR(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	// ServingRuntime with only the accelerator annotation
	sr := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.ServingRuntime.APIVersion(),
			"kind":       resources.ServingRuntime.Kind,
			"metadata": map[string]any{
				"name":      "gpu-runtime",
				"namespace": "test-ns",
				"annotations": map[string]any{
					"opendatahub.io/accelerator-name": "nvidia-gpu",
				},
			},
			"spec": map[string]any{},
		},
	}

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{sr},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	impactedCheck := &kserve.ImpactedWorkloadsCheck{}
	result, err := impactedCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(7))
	g.Expect(result.Status.Conditions[4].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(kserve.ConditionTypeAcceleratorOnlySRCompatible),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonConfigurationInvalid),
		"Message": ContainSubstring("Found 1 ServingRuntime(s) with AcceleratorProfile annotation only"),
	}))
	g.Expect(result.Status.Conditions[4].Impact).To(Equal(resultpkg.ImpactAdvisory))
	g.Expect(result.Status.Conditions[5].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(kserve.ConditionTypeAcceleratorAndHWProfileSRCompat),
		"Status": Equal(metav1.ConditionTrue),
	}))
	g.Expect(result.Status.Conditions[6].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(kserve.ConditionTypeAcceleratorSRISVCCompatible),
		"Status": Equal(metav1.ConditionTrue),
	}))
	g.Expect(result.ImpactedObjects).To(ContainElement(MatchFields(IgnoreExtras, Fields{
		"ObjectMeta": MatchFields(IgnoreExtras, Fields{
			"Name":      Equal("gpu-runtime"),
			"Namespace": Equal("test-ns"),
			"Annotations": And(
				HaveKeyWithValue("opendatahub.io/accelerator-name", "nvidia-gpu"),
			),
		}),
	})))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "1"))
}

func TestImpactedWorkloadsCheck_AcceleratorAndHWProfileSR(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	// ServingRuntime with both accelerator and hardware profile annotations
	sr := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.ServingRuntime.APIVersion(),
			"kind":       resources.ServingRuntime.Kind,
			"metadata": map[string]any{
				"name":      "dual-runtime",
				"namespace": "test-ns",
				"annotations": map[string]any{
					"opendatahub.io/accelerator-name":      "nvidia-gpu",
					"opendatahub.io/hardware-profile-name": "gpu-large",
				},
			},
			"spec": map[string]any{},
		},
	}

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{sr},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	impactedCheck := &kserve.ImpactedWorkloadsCheck{}
	result, err := impactedCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(7))
	g.Expect(result.Status.Conditions[4].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(kserve.ConditionTypeAcceleratorOnlySRCompatible),
		"Status": Equal(metav1.ConditionTrue),
	}))
	g.Expect(result.Status.Conditions[5].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(kserve.ConditionTypeAcceleratorAndHWProfileSRCompat),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonConfigurationInvalid),
		"Message": ContainSubstring("Found 1 ServingRuntime(s) with both AcceleratorProfile and HardwareProfile annotations"),
	}))
	g.Expect(result.Status.Conditions[5].Impact).To(Equal(resultpkg.ImpactAdvisory))
	g.Expect(result.ImpactedObjects).To(ContainElement(MatchFields(IgnoreExtras, Fields{
		"ObjectMeta": MatchFields(IgnoreExtras, Fields{
			"Name":      Equal("dual-runtime"),
			"Namespace": Equal("test-ns"),
			"Annotations": And(
				HaveKeyWithValue("opendatahub.io/accelerator-name", "nvidia-gpu"),
				HaveKeyWithValue("opendatahub.io/hardware-profile-name", "gpu-large"),
			),
		}),
	})))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "1"))
}

func TestImpactedWorkloadsCheck_AcceleratorSR_WithISVC(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	// ServingRuntime with accelerator annotation
	sr := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.ServingRuntime.APIVersion(),
			"kind":       resources.ServingRuntime.Kind,
			"metadata": map[string]any{
				"name":      "gpu-runtime",
				"namespace": "test-ns",
				"annotations": map[string]any{
					"opendatahub.io/accelerator-name": "nvidia-gpu",
				},
			},
			"spec": map[string]any{},
		},
	}

	// InferenceService referencing the accelerator-linked runtime (same namespace)
	isvc := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.InferenceService.APIVersion(),
			"kind":       resources.InferenceService.Kind,
			"metadata": map[string]any{
				"name":      "gpu-model",
				"namespace": "test-ns",
			},
			"spec": map[string]any{
				"predictor": map[string]any{
					"model": map[string]any{
						"runtime": "gpu-runtime",
					},
				},
			},
		},
	}

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{sr, isvc},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	impactedCheck := &kserve.ImpactedWorkloadsCheck{}
	result, err := impactedCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(7))

	// Accelerator-only SR condition
	g.Expect(result.Status.Conditions[4].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(kserve.ConditionTypeAcceleratorOnlySRCompatible),
		"Status":  Equal(metav1.ConditionFalse),
		"Message": ContainSubstring("Found 1"),
	}))
	g.Expect(result.Status.Conditions[4].Impact).To(Equal(resultpkg.ImpactAdvisory))

	// ISVC referencing accelerator SR condition
	g.Expect(result.Status.Conditions[6].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(kserve.ConditionTypeAcceleratorSRISVCCompatible),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonConfigurationInvalid),
		"Message": ContainSubstring("Found 1 InferenceService(s) referencing AcceleratorProfile-linked ServingRuntime(s)"),
	}))
	g.Expect(result.Status.Conditions[6].Impact).To(Equal(resultpkg.ImpactAdvisory))

	g.Expect(result.ImpactedObjects).To(ContainElement(MatchFields(IgnoreExtras, Fields{
		"ObjectMeta": MatchFields(IgnoreExtras, Fields{
			"Name":      Equal("gpu-model"),
			"Namespace": Equal("test-ns"),
			"Annotations": And(
				HaveKeyWithValue("serving.kserve.io/runtime", "gpu-runtime"),
			),
		}),
	})))

	// SR (1) + ISVC (1) = 2 impacted objects
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "2"))
}

func TestImpactedWorkloadsCheck_AcceleratorSR_ISVCDifferentNamespace(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	// ServingRuntime with accelerator annotation in ns1
	sr := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.ServingRuntime.APIVersion(),
			"kind":       resources.ServingRuntime.Kind,
			"metadata": map[string]any{
				"name":      "gpu-runtime",
				"namespace": "ns1",
				"annotations": map[string]any{
					"opendatahub.io/accelerator-name": "nvidia-gpu",
				},
			},
			"spec": map[string]any{},
		},
	}

	// InferenceService in ns2 referencing same runtime name (should NOT match due to different namespace)
	isvc := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.InferenceService.APIVersion(),
			"kind":       resources.InferenceService.Kind,
			"metadata": map[string]any{
				"name":      "gpu-model",
				"namespace": "ns2",
			},
			"spec": map[string]any{
				"predictor": map[string]any{
					"model": map[string]any{
						"runtime": "gpu-runtime",
					},
				},
			},
		},
	}

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{sr, isvc},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	impactedCheck := &kserve.ImpactedWorkloadsCheck{}
	result, err := impactedCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())

	// SR is flagged as accelerator-only
	g.Expect(result.Status.Conditions[4].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(kserve.ConditionTypeAcceleratorOnlySRCompatible),
		"Status": Equal(metav1.ConditionFalse),
	}))

	// ISVC in different namespace should NOT be flagged
	g.Expect(result.Status.Conditions[6].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(kserve.ConditionTypeAcceleratorSRISVCCompatible),
		"Status": Equal(metav1.ConditionTrue),
	}))

	// Only the SR is impacted, not the ISVC
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "1"))
}

func TestImpactedWorkloadsCheck_AcceleratorSR_MixedAnnotations(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	// Accelerator-only SR
	sr1 := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.ServingRuntime.APIVersion(),
			"kind":       resources.ServingRuntime.Kind,
			"metadata": map[string]any{
				"name":      "accel-only-runtime",
				"namespace": "test-ns",
				"annotations": map[string]any{
					"opendatahub.io/accelerator-name": "nvidia-gpu",
				},
			},
			"spec": map[string]any{},
		},
	}

	// Both-annotations SR
	sr2 := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.ServingRuntime.APIVersion(),
			"kind":       resources.ServingRuntime.Kind,
			"metadata": map[string]any{
				"name":      "dual-runtime",
				"namespace": "test-ns",
				"annotations": map[string]any{
					"opendatahub.io/accelerator-name":      "amd-gpu",
					"opendatahub.io/hardware-profile-name": "gpu-medium",
				},
			},
			"spec": map[string]any{},
		},
	}

	// Plain SR without accelerator (should not be flagged)
	sr3 := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.ServingRuntime.APIVersion(),
			"kind":       resources.ServingRuntime.Kind,
			"metadata": map[string]any{
				"name":      "plain-runtime",
				"namespace": "test-ns",
			},
			"spec": map[string]any{},
		},
	}

	// ISVC referencing the accelerator-only SR
	isvc := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.InferenceService.APIVersion(),
			"kind":       resources.InferenceService.Kind,
			"metadata": map[string]any{
				"name":      "model-on-accel",
				"namespace": "test-ns",
			},
			"spec": map[string]any{
				"predictor": map[string]any{
					"model": map[string]any{
						"runtime": "accel-only-runtime",
					},
				},
			},
		},
	}

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{sr1, sr2, sr3, isvc},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	impactedCheck := &kserve.ImpactedWorkloadsCheck{}
	result, err := impactedCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(7))

	// 1 accelerator-only SR
	g.Expect(result.Status.Conditions[4].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(kserve.ConditionTypeAcceleratorOnlySRCompatible),
		"Status":  Equal(metav1.ConditionFalse),
		"Message": ContainSubstring("Found 1"),
	}))
	g.Expect(result.Status.Conditions[4].Impact).To(Equal(resultpkg.ImpactAdvisory))

	// 1 both-annotations SR
	g.Expect(result.Status.Conditions[5].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(kserve.ConditionTypeAcceleratorAndHWProfileSRCompat),
		"Status":  Equal(metav1.ConditionFalse),
		"Message": ContainSubstring("Found 1"),
	}))
	g.Expect(result.Status.Conditions[5].Impact).To(Equal(resultpkg.ImpactAdvisory))

	// 1 ISVC referencing an accelerator SR
	g.Expect(result.Status.Conditions[6].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(kserve.ConditionTypeAcceleratorSRISVCCompatible),
		"Status":  Equal(metav1.ConditionFalse),
		"Message": ContainSubstring("Found 1"),
	}))
	g.Expect(result.Status.Conditions[6].Impact).To(Equal(resultpkg.ImpactAdvisory))

	// 2 SRs + 1 ISVC = 3 impacted objects
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "3"))
}
