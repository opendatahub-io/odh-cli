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

	"github.com/lburgazzoli/odh-cli/pkg/lint/check"
	"github.com/lburgazzoli/odh-cli/pkg/lint/check/result"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/components/kserve"
	"github.com/lburgazzoli/odh-cli/pkg/resources"
	"github.com/lburgazzoli/odh-cli/pkg/util/client"

	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

//nolint:gochecknoglobals // Test fixture - shared across test functions
var inferenceServiceConfigListKinds = map[schema.GroupVersionResource]string{
	resources.DSCInitialization.GVR(): resources.DSCInitialization.ListKind(),
	resources.ConfigMap.GVR():         resources.ConfigMap.ListKind(),
}

func TestInferenceServiceConfigCheck_NoDSCI(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	// Create empty cluster (no DSCInitialization)
	scheme := runtime.NewScheme()
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, inferenceServiceConfigListKinds)

	c := &client.Client{
		Dynamic: dynamicClient,
	}

	currentVer := semver.MustParse("2.17.0")
	targetVer := semver.MustParse("3.0.0")
	target := check.Target{
		Client:         c,
		CurrentVersion: &currentVer,
		TargetVersion:  &targetVer,
	}

	inferenceConfigCheck := kserve.NewInferenceServiceConfigCheck()
	checkResult, err := inferenceConfigCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(checkResult.Status.Conditions).To(HaveLen(1))
	g.Expect(checkResult.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeAvailable),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonResourceNotFound),
		"Message": ContainSubstring("No DSCInitialization"),
	}))
}

func TestInferenceServiceConfigCheck_ConfigMapNotFound(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	// Create DSCInitialization without the ConfigMap
	dsci := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.DSCInitialization.APIVersion(),
			"kind":       resources.DSCInitialization.Kind,
			"metadata": map[string]any{
				"name": "default-dsci",
			},
			"spec": map[string]any{
				"applicationsNamespace": "opendatahub",
			},
		},
	}

	scheme := runtime.NewScheme()
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, inferenceServiceConfigListKinds, dsci)

	c := &client.Client{
		Dynamic: dynamicClient,
	}

	currentVer := semver.MustParse("2.17.0")
	targetVer := semver.MustParse("3.0.0")
	target := check.Target{
		Client:         c,
		CurrentVersion: &currentVer,
		TargetVersion:  &targetVer,
	}

	inferenceConfigCheck := kserve.NewInferenceServiceConfigCheck()
	checkResult, err := inferenceConfigCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(checkResult.Status.Conditions).To(HaveLen(1))
	g.Expect(checkResult.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeCompatible),
		"Status":  Equal(metav1.ConditionTrue),
		"Reason":  Equal(check.ReasonVersionCompatible),
		"Message": ContainSubstring("not found"),
	}))
}

func TestInferenceServiceConfigCheck_ConfigMapManagedFalse(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	// Create DSCInitialization and ConfigMap with managed=false annotation
	dsci := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.DSCInitialization.APIVersion(),
			"kind":       resources.DSCInitialization.Kind,
			"metadata": map[string]any{
				"name": "default-dsci",
			},
			"spec": map[string]any{
				"applicationsNamespace": "opendatahub",
			},
		},
	}

	configMap := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.ConfigMap.APIVersion(),
			"kind":       resources.ConfigMap.Kind,
			"metadata": map[string]any{
				"name":      "inferenceservice-config",
				"namespace": "opendatahub",
				"annotations": map[string]any{
					"opendatahub.io/managed": "false",
				},
			},
			"data": map[string]any{
				"key": "value",
			},
		},
	}

	scheme := runtime.NewScheme()
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, inferenceServiceConfigListKinds, dsci, configMap)

	c := &client.Client{
		Dynamic: dynamicClient,
	}

	currentVer := semver.MustParse("2.17.0")
	targetVer := semver.MustParse("3.0.0")
	target := check.Target{
		Client:         c,
		CurrentVersion: &currentVer,
		TargetVersion:  &targetVer,
	}

	inferenceConfigCheck := kserve.NewInferenceServiceConfigCheck()
	checkResult, err := inferenceConfigCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(checkResult.Status.Conditions).To(HaveLen(1))
	g.Expect(checkResult.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeConfigured),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonConfigurationUnmanaged),
		"Message": And(ContainSubstring("managed=false"), ContainSubstring("out of sync")),
	}))
	// Verify it's advisory (non-blocking)
	g.Expect(checkResult.Status.Conditions[0].Impact).To(Equal(result.ImpactAdvisory))
	g.Expect(checkResult.Annotations).To(HaveKeyWithValue(check.AnnotationInferenceServiceConfigManaged, "false"))
}

func TestInferenceServiceConfigCheck_ConfigMapManagedTrue(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	// Create DSCInitialization and ConfigMap with managed=true annotation
	dsci := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.DSCInitialization.APIVersion(),
			"kind":       resources.DSCInitialization.Kind,
			"metadata": map[string]any{
				"name": "default-dsci",
			},
			"spec": map[string]any{
				"applicationsNamespace": "opendatahub",
			},
		},
	}

	configMap := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.ConfigMap.APIVersion(),
			"kind":       resources.ConfigMap.Kind,
			"metadata": map[string]any{
				"name":      "inferenceservice-config",
				"namespace": "opendatahub",
				"annotations": map[string]any{
					"opendatahub.io/managed": "true",
				},
			},
			"data": map[string]any{
				"key": "value",
			},
		},
	}

	scheme := runtime.NewScheme()
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, inferenceServiceConfigListKinds, dsci, configMap)

	c := &client.Client{
		Dynamic: dynamicClient,
	}

	currentVer := semver.MustParse("2.17.0")
	targetVer := semver.MustParse("3.0.0")
	target := check.Target{
		Client:         c,
		CurrentVersion: &currentVer,
		TargetVersion:  &targetVer,
	}

	inferenceConfigCheck := kserve.NewInferenceServiceConfigCheck()
	checkResult, err := inferenceConfigCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(checkResult.Status.Conditions).To(HaveLen(1))
	g.Expect(checkResult.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeCompatible),
		"Status":  Equal(metav1.ConditionTrue),
		"Reason":  Equal(check.ReasonVersionCompatible),
		"Message": ContainSubstring("managed by operator"),
	}))
	g.Expect(checkResult.Annotations).To(HaveKeyWithValue(check.AnnotationInferenceServiceConfigManaged, "true"))
}

func TestInferenceServiceConfigCheck_ConfigMapNoAnnotation(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	// Create DSCInitialization and ConfigMap without the managed annotation
	dsci := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.DSCInitialization.APIVersion(),
			"kind":       resources.DSCInitialization.Kind,
			"metadata": map[string]any{
				"name": "default-dsci",
			},
			"spec": map[string]any{
				"applicationsNamespace": "opendatahub",
			},
		},
	}

	configMap := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.ConfigMap.APIVersion(),
			"kind":       resources.ConfigMap.Kind,
			"metadata": map[string]any{
				"name":      "inferenceservice-config",
				"namespace": "opendatahub",
			},
			"data": map[string]any{
				"key": "value",
			},
		},
	}

	scheme := runtime.NewScheme()
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, inferenceServiceConfigListKinds, dsci, configMap)

	c := &client.Client{
		Dynamic: dynamicClient,
	}

	currentVer := semver.MustParse("2.17.0")
	targetVer := semver.MustParse("3.0.0")
	target := check.Target{
		Client:         c,
		CurrentVersion: &currentVer,
		TargetVersion:  &targetVer,
	}

	inferenceConfigCheck := kserve.NewInferenceServiceConfigCheck()
	checkResult, err := inferenceConfigCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(checkResult.Status.Conditions).To(HaveLen(1))
	g.Expect(checkResult.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeCompatible),
		"Status":  Equal(metav1.ConditionTrue),
		"Reason":  Equal(check.ReasonVersionCompatible),
		"Message": ContainSubstring("managed by operator"),
	}))
	// Empty annotation value when not present
	g.Expect(checkResult.Annotations).To(HaveKeyWithValue(check.AnnotationInferenceServiceConfigManaged, ""))
}

func TestInferenceServiceConfigCheck_ConfigMapEmptyAnnotations(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	// Create DSCInitialization and ConfigMap with empty annotations
	dsci := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.DSCInitialization.APIVersion(),
			"kind":       resources.DSCInitialization.Kind,
			"metadata": map[string]any{
				"name": "default-dsci",
			},
			"spec": map[string]any{
				"applicationsNamespace": "opendatahub",
			},
		},
	}

	configMap := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.ConfigMap.APIVersion(),
			"kind":       resources.ConfigMap.Kind,
			"metadata": map[string]any{
				"name":        "inferenceservice-config",
				"namespace":   "opendatahub",
				"annotations": map[string]any{},
			},
			"data": map[string]any{
				"key": "value",
			},
		},
	}

	scheme := runtime.NewScheme()
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, inferenceServiceConfigListKinds, dsci, configMap)

	c := &client.Client{
		Dynamic: dynamicClient,
	}

	currentVer := semver.MustParse("2.17.0")
	targetVer := semver.MustParse("3.0.0")
	target := check.Target{
		Client:         c,
		CurrentVersion: &currentVer,
		TargetVersion:  &targetVer,
	}

	inferenceConfigCheck := kserve.NewInferenceServiceConfigCheck()
	checkResult, err := inferenceConfigCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(checkResult.Status.Conditions).To(HaveLen(1))
	g.Expect(checkResult.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeCompatible),
		"Status":  Equal(metav1.ConditionTrue),
		"Reason":  Equal(check.ReasonVersionCompatible),
		"Message": ContainSubstring("managed by operator"),
	}))
}

func TestInferenceServiceConfigCheck_DSCINoNamespace(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	// Create DSCInitialization without applicationsNamespace - treated as NotFound since namespace is required
	dsci := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.DSCInitialization.APIVersion(),
			"kind":       resources.DSCInitialization.Kind,
			"metadata": map[string]any{
				"name": "default-dsci",
			},
			"spec": map[string]any{
				// No applicationsNamespace set
			},
		},
	}

	// ConfigMap exists but won't be found since namespace lookup fails
	configMap := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.ConfigMap.APIVersion(),
			"kind":       resources.ConfigMap.Kind,
			"metadata": map[string]any{
				"name":      "inferenceservice-config",
				"namespace": "opendatahub",
				"annotations": map[string]any{
					"opendatahub.io/managed": "true",
				},
			},
			"data": map[string]any{
				"key": "value",
			},
		},
	}

	scheme := runtime.NewScheme()
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, inferenceServiceConfigListKinds, dsci, configMap)

	c := &client.Client{
		Dynamic: dynamicClient,
	}

	currentVer := semver.MustParse("2.17.0")
	targetVer := semver.MustParse("3.0.0")
	target := check.Target{
		Client:         c,
		CurrentVersion: &currentVer,
		TargetVersion:  &targetVer,
	}

	inferenceConfigCheck := kserve.NewInferenceServiceConfigCheck()
	checkResult, err := inferenceConfigCheck.Validate(ctx, target)

	// When applicationsNamespace is not set, the helper returns NotFound,
	// which results in DSCInitializationNotFound being returned.
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(checkResult.Status.Conditions).To(HaveLen(1))
	g.Expect(checkResult.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeAvailable),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonResourceNotFound),
		"Message": ContainSubstring("No DSCInitialization"),
	}))
}

func TestInferenceServiceConfigCheck_Metadata(t *testing.T) {
	g := NewWithT(t)

	inferenceConfigCheck := kserve.NewInferenceServiceConfigCheck()

	g.Expect(inferenceConfigCheck.ID()).To(Equal("components.kserve.inferenceservice-config"))
	g.Expect(inferenceConfigCheck.Name()).To(Equal("Components :: KServe :: InferenceService Config Migration"))
	g.Expect(inferenceConfigCheck.Group()).To(Equal(check.GroupComponent))
	g.Expect(inferenceConfigCheck.Description()).ToNot(BeEmpty())
}

func TestInferenceServiceConfigCheck_CanApply(t *testing.T) {
	g := NewWithT(t)

	inferenceConfigCheck := kserve.NewInferenceServiceConfigCheck()

	// Test 2.x to 3.x upgrade - should apply
	currentVer2x := semver.MustParse("2.17.0")
	targetVer3x := semver.MustParse("3.0.0")
	target2xTo3x := check.Target{
		CurrentVersion: &currentVer2x,
		TargetVersion:  &targetVer3x,
	}
	g.Expect(inferenceConfigCheck.CanApply(target2xTo3x)).To(BeTrue())

	// Test 3.x to 3.x upgrade - should not apply
	currentVer3x := semver.MustParse("3.0.0")
	targetVer31 := semver.MustParse("3.1.0")
	target3xTo3x := check.Target{
		CurrentVersion: &currentVer3x,
		TargetVersion:  &targetVer31,
	}
	g.Expect(inferenceConfigCheck.CanApply(target3xTo3x)).To(BeFalse())

	// Test nil versions - should not apply
	targetNil := check.Target{
		CurrentVersion: nil,
		TargetVersion:  nil,
	}
	g.Expect(inferenceConfigCheck.CanApply(targetNil)).To(BeFalse())
}
