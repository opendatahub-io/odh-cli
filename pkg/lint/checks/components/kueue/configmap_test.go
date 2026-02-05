package kueue_test

import (
	"context"
	"testing"

	"github.com/blang/semver/v4"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	"github.com/lburgazzoli/odh-cli/pkg/lint/check"
	"github.com/lburgazzoli/odh-cli/pkg/lint/check/result"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/components/kueue"
	"github.com/lburgazzoli/odh-cli/pkg/resources"
	"github.com/lburgazzoli/odh-cli/pkg/util/client"

	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

const testApplicationsNamespace = "redhat-ods-applications"

//nolint:gochecknoglobals // Test fixture - shared across test functions
var configMapListKinds = map[schema.GroupVersionResource]string{
	resources.DSCInitialization.GVR(): resources.DSCInitialization.ListKind(),
	resources.ConfigMap.GVR():         resources.ConfigMap.ListKind(),
}

func newDSCIWithNamespace(namespace string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.DSCInitialization.APIVersion(),
			"kind":       resources.DSCInitialization.Kind,
			"metadata": map[string]any{
				"name": "default-dsci",
			},
			"spec": map[string]any{
				"applicationsNamespace": namespace,
			},
		},
	}
}

func newConfigMap(namespace string, name string, annotations map[string]string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "ConfigMap",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   namespace,
			Annotations: annotations,
		},
	}
}

func newConfigMapUnstructured(namespace string, name string, annotations map[string]string) *unstructured.Unstructured {
	cm := newConfigMap(namespace, name, annotations)
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(resources.ConfigMap.GVK())
	obj.SetName(cm.Name)
	obj.SetNamespace(cm.Namespace)
	obj.SetAnnotations(cm.Annotations)

	return obj
}

func TestConfigMapManagedCheck_NoDSCI(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	// Create empty cluster (no DSCInitialization)
	scheme := runtime.NewScheme()
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, configMapListKinds)

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

	configMapCheck := kueue.NewConfigMapManagedCheck()
	dr, err := configMapCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(dr.Status.Conditions).To(HaveLen(1))
	g.Expect(dr.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeAvailable),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonResourceNotFound),
		"Message": ContainSubstring("No DSCInitialization"),
	}))
}

func TestConfigMapManagedCheck_DSCINoNamespace(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	// Create DSCI without applicationsNamespace - treated as NotFound since namespace is required
	dsci := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.DSCInitialization.APIVersion(),
			"kind":       resources.DSCInitialization.Kind,
			"metadata": map[string]any{
				"name": "default-dsci",
			},
			"spec": map[string]any{
				"serviceMesh": map[string]any{
					"managementState": "Managed",
				},
			},
		},
	}

	scheme := runtime.NewScheme()
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, configMapListKinds, dsci)

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

	configMapCheck := kueue.NewConfigMapManagedCheck()
	dr, err := configMapCheck.Validate(ctx, target)

	// When applicationsNamespace is not set, the helper returns NotFound,
	// which results in DSCInitializationNotFound being returned.
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(dr.Status.Conditions).To(HaveLen(1))
	g.Expect(dr.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeAvailable),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonResourceNotFound),
		"Message": ContainSubstring("No DSCInitialization"),
	}))
}

func TestConfigMapManagedCheck_ConfigMapNotFound(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	// Create DSCI with namespace but no ConfigMap
	dsci := newDSCIWithNamespace(testApplicationsNamespace)

	scheme := runtime.NewScheme()
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, configMapListKinds, dsci)

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

	configMapCheck := kueue.NewConfigMapManagedCheck()
	dr, err := configMapCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(dr.Status.Conditions).To(HaveLen(1))
	g.Expect(dr.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeCompatible),
		"Status":  Equal(metav1.ConditionTrue),
		"Reason":  Equal(check.ReasonVersionCompatible),
		"Message": And(ContainSubstring("not found"), ContainSubstring("no action required")),
	}))
}

func TestConfigMapManagedCheck_ConfigMapManaged(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	// Create DSCI and ConfigMap without managed=false annotation
	dsci := newDSCIWithNamespace(testApplicationsNamespace)
	configMap := newConfigMapUnstructured(testApplicationsNamespace, "kueue-manager-config", nil)

	scheme := runtime.NewScheme()
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		scheme, configMapListKinds, dsci, configMap,
	)

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

	configMapCheck := kueue.NewConfigMapManagedCheck()
	dr, err := configMapCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(dr.Status.Conditions).To(HaveLen(1))
	g.Expect(dr.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeCompatible),
		"Status":  Equal(metav1.ConditionTrue),
		"Reason":  Equal(check.ReasonVersionCompatible),
		"Message": ContainSubstring("managed by operator"),
	}))
	g.Expect(dr.Annotations).To(HaveKeyWithValue("check.opendatahub.io/target-version", "3.0.0"))
}

func TestConfigMapManagedCheck_ConfigMapManagedTrue(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	// Create DSCI and ConfigMap with managed=true annotation (should pass)
	dsci := newDSCIWithNamespace(testApplicationsNamespace)
	configMap := newConfigMapUnstructured(testApplicationsNamespace, "kueue-manager-config", map[string]string{
		"opendatahub.io/managed": "true",
	})

	scheme := runtime.NewScheme()
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		scheme, configMapListKinds, dsci, configMap,
	)

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

	configMapCheck := kueue.NewConfigMapManagedCheck()
	dr, err := configMapCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(dr.Status.Conditions).To(HaveLen(1))
	g.Expect(dr.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeCompatible),
		"Status":  Equal(metav1.ConditionTrue),
		"Reason":  Equal(check.ReasonVersionCompatible),
		"Message": ContainSubstring("managed by operator"),
	}))
}

func TestConfigMapManagedCheck_ConfigMapManagedFalse(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	// Create DSCI and ConfigMap with managed=false annotation (advisory warning)
	dsci := newDSCIWithNamespace(testApplicationsNamespace)
	configMap := newConfigMapUnstructured(testApplicationsNamespace, "kueue-manager-config", map[string]string{
		"opendatahub.io/managed": "false",
	})

	scheme := runtime.NewScheme()
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		scheme, configMapListKinds, dsci, configMap,
	)

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

	configMapCheck := kueue.NewConfigMapManagedCheck()
	dr, err := configMapCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(dr.Status.Conditions).To(HaveLen(1))
	g.Expect(dr.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(check.ConditionTypeConfigured),
		"Status": Equal(metav1.ConditionFalse),
		"Reason": Equal(check.ReasonConfigurationInvalid),
		"Message": And(
			ContainSubstring("opendatahub.io/managed=false"),
			ContainSubstring("migration will not update"),
			ContainSubstring("may become out of sync"),
		),
	}))
	// Verify it's an advisory warning, not blocking
	g.Expect(dr.Status.Conditions[0].Impact).To(Equal(result.ImpactAdvisory))
	g.Expect(dr.Annotations).To(HaveKeyWithValue("check.opendatahub.io/target-version", "3.0.0"))
}

func TestConfigMapManagedCheck_CanApply(t *testing.T) {
	g := NewWithT(t)

	configMapCheck := kueue.NewConfigMapManagedCheck()

	// Test cases for CanApply
	testCases := []struct {
		name           string
		currentVersion *semver.Version
		targetVersion  *semver.Version
		expected       bool
	}{
		{
			name:           "nil versions",
			currentVersion: nil,
			targetVersion:  nil,
			expected:       false,
		},
		{
			name:           "2.x to 3.x upgrade",
			currentVersion: semverPtr("2.17.0"),
			targetVersion:  semverPtr("3.0.0"),
			expected:       true,
		},
		{
			name:           "2.x to 3.1 upgrade",
			currentVersion: semverPtr("2.17.0"),
			targetVersion:  semverPtr("3.1.0"),
			expected:       true,
		},
		{
			name:           "2.x to 2.x upgrade (same major)",
			currentVersion: semverPtr("2.16.0"),
			targetVersion:  semverPtr("2.17.0"),
			expected:       false,
		},
		{
			name:           "3.x to 3.x upgrade (same major)",
			currentVersion: semverPtr("3.0.0"),
			targetVersion:  semverPtr("3.1.0"),
			expected:       false,
		},
		{
			name:           "lint mode (same version)",
			currentVersion: semverPtr("2.17.0"),
			targetVersion:  semverPtr("2.17.0"),
			expected:       false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			target := check.Target{
				CurrentVersion: tc.currentVersion,
				TargetVersion:  tc.targetVersion,
			}
			g.Expect(configMapCheck.CanApply(target)).To(Equal(tc.expected))
		})
	}
}

func TestConfigMapManagedCheck_Metadata(t *testing.T) {
	g := NewWithT(t)

	configMapCheck := kueue.NewConfigMapManagedCheck()

	g.Expect(configMapCheck.ID()).To(Equal("components.kueue.configmap-managed"))
	g.Expect(configMapCheck.Name()).To(Equal("Components :: Kueue :: ConfigMap Managed Check (3.x)"))
	g.Expect(configMapCheck.Group()).To(Equal(check.GroupComponent))
	g.Expect(configMapCheck.Description()).To(ContainSubstring("kueue-manager-config"))
	g.Expect(configMapCheck.Description()).To(ContainSubstring("2.x to 3.x"))
}

func semverPtr(s string) *semver.Version {
	v := semver.MustParse(s)

	return &v
}
