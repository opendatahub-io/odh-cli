package kserve_test

import (
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/testutil"
	"github.com/opendatahub-io/odh-cli/pkg/lint/checks/workloads/kserve"
	"github.com/opendatahub-io/odh-cli/pkg/resources"

	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

//nolint:gochecknoglobals // Test fixture - shared across test functions
var inferenceServiceConfigListKinds = map[schema.GroupVersionResource]string{
	resources.DataScienceCluster.GVR(): resources.DataScienceCluster.ListKind(),
	resources.DSCInitialization.GVR():  resources.DSCInitialization.ListKind(),
	resources.ConfigMap.GVR():          resources.ConfigMap.ListKind(),
}

func TestInferenceServiceConfigCheck_NoDSCI(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	// Create DSC with kserve managed but no DSCInitialization
	dsc := testutil.NewDSC(map[string]string{"kserve": "Managed"})
	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      inferenceServiceConfigListKinds,
		Objects:        []*unstructured.Unstructured{dsc},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

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
	ctx := t.Context()

	// Create DSC with kserve managed
	dsc := testutil.NewDSC(map[string]string{"kserve": "Managed"})
	// Create DSCInitialization without the ConfigMap
	dsci := testutil.NewDSCI("opendatahub")
	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      inferenceServiceConfigListKinds,
		Objects:        []*unstructured.Unstructured{dsc, dsci},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

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

func TestInferenceServiceConfigCheck_ConfigMapManagedFalseWithAnnotations(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	dsc := testutil.NewDSC(map[string]string{"kserve": "Managed"})
	dsci := testutil.NewDSCI("opendatahub")
	configMap := newInferenceServiceConfigMap("opendatahub", map[string]any{
		"opendatahub.io/managed": "false",
	}, inferenceServiceDataWithAnnotations(
		"opendatahub.io/hardware-profile-name",
		"opendatahub.io/hardware-profile-namespace",
	))
	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      inferenceServiceConfigListKinds,
		Objects:        []*unstructured.Unstructured{dsc, dsci, configMap},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	inferenceConfigCheck := kserve.NewInferenceServiceConfigCheck()
	checkResult, err := inferenceConfigCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(checkResult.Status.Conditions).To(HaveLen(1))
	g.Expect(checkResult.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(check.ConditionTypeCompatible),
		"Status": Equal(metav1.ConditionTrue),
		"Reason": Equal(check.ReasonVersionCompatible),
	}))
}

func TestInferenceServiceConfigCheck_ConfigMapManagedFalseMissingAnnotations(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	dsc := testutil.NewDSC(map[string]string{"kserve": "Managed"})
	dsci := testutil.NewDSCI("opendatahub")
	// managed=false but no hardware-profile annotations in disallowed list
	configMap := newInferenceServiceConfigMap("opendatahub", map[string]any{
		"opendatahub.io/managed": "false",
	}, inferenceServiceDataWithAnnotations())
	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      inferenceServiceConfigListKinds,
		Objects:        []*unstructured.Unstructured{dsc, dsci, configMap},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	inferenceConfigCheck := kserve.NewInferenceServiceConfigCheck()
	checkResult, err := inferenceConfigCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(checkResult.Status.Conditions).To(HaveLen(1))
	g.Expect(checkResult.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeConfigured),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonConfigurationInvalid),
		"Message": And(ContainSubstring("hardware-profile-name"), ContainSubstring("hardware-profile-namespace")),
	}))
	g.Expect(checkResult.Status.Conditions[0].Impact).To(Equal(result.ImpactAdvisory))
}

func TestInferenceServiceConfigCheck_ConfigMapManagedFalsePartialAnnotations(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	dsc := testutil.NewDSC(map[string]string{"kserve": "Managed"})
	dsci := testutil.NewDSCI("opendatahub")
	// managed=false but only one of two required hardware-profile annotations
	configMap := newInferenceServiceConfigMap("opendatahub", map[string]any{
		"opendatahub.io/managed": "false",
	}, inferenceServiceDataWithAnnotations("opendatahub.io/hardware-profile-name"))
	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      inferenceServiceConfigListKinds,
		Objects:        []*unstructured.Unstructured{dsc, dsci, configMap},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	inferenceConfigCheck := kserve.NewInferenceServiceConfigCheck()
	checkResult, err := inferenceConfigCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(checkResult.Status.Conditions).To(HaveLen(1))
	g.Expect(checkResult.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeConfigured),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonConfigurationInvalid),
		"Message": And(ContainSubstring("hardware-profile-namespace"), Not(ContainSubstring("hardware-profile-name,"))),
	}))
	g.Expect(checkResult.Status.Conditions[0].Impact).To(Equal(result.ImpactAdvisory))
}

func TestInferenceServiceConfigCheck_ConfigMapManagedFalseNoDataKey(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	dsc := testutil.NewDSC(map[string]string{"kserve": "Managed"})
	dsci := testutil.NewDSCI("opendatahub")
	// managed=false but no inferenceService data key at all
	configMap := newInferenceServiceConfigMap("opendatahub", map[string]any{
		"opendatahub.io/managed": "false",
	}, "")
	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      inferenceServiceConfigListKinds,
		Objects:        []*unstructured.Unstructured{dsc, dsci, configMap},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	inferenceConfigCheck := kserve.NewInferenceServiceConfigCheck()
	checkResult, err := inferenceConfigCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(checkResult.Status.Conditions).To(HaveLen(1))
	g.Expect(checkResult.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeConfigured),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonConfigurationInvalid),
		"Message": And(ContainSubstring("hardware-profile-name"), ContainSubstring("hardware-profile-namespace")),
	}))
	g.Expect(checkResult.Status.Conditions[0].Impact).To(Equal(result.ImpactAdvisory))
}

func TestInferenceServiceConfigCheck_ConfigMapManagedTrue(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	dsc := testutil.NewDSC(map[string]string{"kserve": "Managed"})
	dsci := testutil.NewDSCI("opendatahub")
	configMap := newInferenceServiceConfigMap("opendatahub", map[string]any{
		"opendatahub.io/managed": "true",
	}, inferenceServiceDataWithAnnotations())
	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      inferenceServiceConfigListKinds,
		Objects:        []*unstructured.Unstructured{dsc, dsci, configMap},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	inferenceConfigCheck := kserve.NewInferenceServiceConfigCheck()
	checkResult, err := inferenceConfigCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(checkResult.Status.Conditions).To(HaveLen(1))
	g.Expect(checkResult.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeConfigured),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonConfigurationUnmanaged),
		"Message": ContainSubstring("opendatahub.io/managed"),
	}))
	g.Expect(checkResult.Status.Conditions[0].Impact).To(Equal(result.ImpactAdvisory))
}

func TestInferenceServiceConfigCheck_ConfigMapNoAnnotation(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	dsc := testutil.NewDSC(map[string]string{"kserve": "Managed"})
	dsci := testutil.NewDSCI("opendatahub")
	configMap := newInferenceServiceConfigMap("opendatahub", nil, inferenceServiceDataWithAnnotations())
	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      inferenceServiceConfigListKinds,
		Objects:        []*unstructured.Unstructured{dsc, dsci, configMap},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	inferenceConfigCheck := kserve.NewInferenceServiceConfigCheck()
	checkResult, err := inferenceConfigCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(checkResult.Status.Conditions).To(HaveLen(1))
	g.Expect(checkResult.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeConfigured),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonConfigurationUnmanaged),
		"Message": ContainSubstring("opendatahub.io/managed"),
	}))
	g.Expect(checkResult.Status.Conditions[0].Impact).To(Equal(result.ImpactAdvisory))
}

func TestInferenceServiceConfigCheck_ConfigMapEmptyAnnotations(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	dsc := testutil.NewDSC(map[string]string{"kserve": "Managed"})
	dsci := testutil.NewDSCI("opendatahub")
	configMap := newInferenceServiceConfigMap("opendatahub", map[string]any{}, inferenceServiceDataWithAnnotations())
	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      inferenceServiceConfigListKinds,
		Objects:        []*unstructured.Unstructured{dsc, dsci, configMap},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	inferenceConfigCheck := kserve.NewInferenceServiceConfigCheck()
	checkResult, err := inferenceConfigCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(checkResult.Status.Conditions).To(HaveLen(1))
	g.Expect(checkResult.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeConfigured),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonConfigurationUnmanaged),
		"Message": ContainSubstring("opendatahub.io/managed"),
	}))
	g.Expect(checkResult.Status.Conditions[0].Impact).To(Equal(result.ImpactAdvisory))
}

func TestInferenceServiceConfigCheck_DSCINoNamespace(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	// Create DSC with kserve managed
	dsc := testutil.NewDSC(map[string]string{"kserve": "Managed"})
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
	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      inferenceServiceConfigListKinds,
		Objects:        []*unstructured.Unstructured{dsc, dsci, configMap},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

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

	g.Expect(inferenceConfigCheck.ID()).To(Equal("workloads.kserve.inferenceservice-config"))
	g.Expect(inferenceConfigCheck.Name()).To(Equal("Workloads :: KServe :: InferenceService Config Migration"))
	g.Expect(inferenceConfigCheck.Group()).To(Equal(check.GroupWorkload))
	g.Expect(inferenceConfigCheck.Description()).ToNot(BeEmpty())
}

func TestInferenceServiceConfigCheck_CanApply(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	chk := kserve.NewInferenceServiceConfigCheck()
	dsc := testutil.NewDSC(map[string]string{"kserve": "Managed"})

	// Should apply for 2.x to 3.x with Managed
	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      inferenceServiceConfigListKinds,
		Objects:        []*unstructured.Unstructured{dsc},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})
	canApply, err := chk.CanApply(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeTrue())

	// Should not apply for 3.x to 3.x
	target = testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      inferenceServiceConfigListKinds,
		Objects:        []*unstructured.Unstructured{dsc},
		CurrentVersion: "3.0.0",
		TargetVersion:  "3.1.0",
	})
	canApply, err = chk.CanApply(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeFalse())

	// Should not apply with nil versions
	target = testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds: inferenceServiceConfigListKinds,
		Objects:   []*unstructured.Unstructured{dsc},
	})
	canApply, err = chk.CanApply(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeFalse())
}

func TestInferenceServiceConfigCheck_CanApply_ManagementState(t *testing.T) {
	g := NewWithT(t)

	chk := kserve.NewInferenceServiceConfigCheck()

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
			dsc := testutil.NewDSC(map[string]string{"kserve": tc.state})
			target := testutil.NewTarget(t, testutil.TargetConfig{
				ListKinds:      inferenceServiceConfigListKinds,
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

// newInferenceServiceConfigMap creates a test ConfigMap fixture.
// If annotations is nil, no annotations are set.
// If inferenceServiceData is empty, no inferenceService data key is set.
func newInferenceServiceConfigMap(
	namespace string,
	annotations map[string]any,
	inferenceServiceData string,
) *unstructured.Unstructured {
	metadata := map[string]any{
		"name":      "inferenceservice-config",
		"namespace": namespace,
	}

	if annotations != nil {
		metadata["annotations"] = annotations
	}

	data := map[string]any{}
	if inferenceServiceData != "" {
		data["inferenceService"] = inferenceServiceData
	}

	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.ConfigMap.APIVersion(),
			"kind":       resources.ConfigMap.Kind,
			"metadata":   metadata,
			"data":       data,
		},
	}
}

// inferenceServiceDataWithAnnotations builds a JSON string for the inferenceService
// data key with the given annotations in serviceAnnotationDisallowedList.
func inferenceServiceDataWithAnnotations(annotations ...string) string {
	if len(annotations) == 0 {
		return `{"serviceAnnotationDisallowedList": []}`
	}

	quoted := make([]string, 0, len(annotations))
	for _, a := range annotations {
		quoted = append(quoted, `"`+a+`"`)
	}

	return `{"serviceAnnotationDisallowedList": [` + strings.Join(quoted, ", ") + `]}`
}
