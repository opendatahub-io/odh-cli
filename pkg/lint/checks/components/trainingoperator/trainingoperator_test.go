package trainingoperator_test

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
	resultpkg "github.com/lburgazzoli/odh-cli/pkg/lint/check/result"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/components/trainingoperator"
	"github.com/lburgazzoli/odh-cli/pkg/resources"
	"github.com/lburgazzoli/odh-cli/pkg/util/client"

	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

//nolint:gochecknoglobals
var listKinds = map[schema.GroupVersionResource]string{
	resources.DataScienceCluster.GVR(): resources.DataScienceCluster.ListKind(),
}

func TestTrainingOperatorDeprecationCheck_NoDSC(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	scheme := runtime.NewScheme()
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, listKinds)

	c := client.NewForTesting(client.TestClientConfig{
		Dynamic: dynamicClient,
	})

	ver := semver.MustParse("3.3.0")
	target := check.Target{
		Client:        c,
		TargetVersion: &ver,
	}

	trainingoperatorCheck := trainingoperator.NewDeprecationCheck()
	result, err := trainingoperatorCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeAvailable),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonResourceNotFound),
		"Message": ContainSubstring("No DataScienceCluster"),
	}))
}

func TestTrainingOperatorDeprecationCheck_NotConfigured(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	// Create DataScienceCluster without trainingoperator component
	// "Not configured" is now treated as "Removed" - both mean component is not active
	dsc := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.DataScienceCluster.APIVersion(),
			"kind":       resources.DataScienceCluster.Kind,
			"metadata": map[string]any{
				"name": "default-dsc",
			},
			"spec": map[string]any{
				"components": map[string]any{
					"dashboard": map[string]any{
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

	ver := semver.MustParse("3.3.0")
	target := check.Target{
		Client:        c,
		TargetVersion: &ver,
	}

	trainingoperatorCheck := trainingoperator.NewDeprecationCheck()
	result, err := trainingoperatorCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	// When component is not configured, it's treated as Removed - check passes
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeCompatible),
		"Status":  Equal(metav1.ConditionTrue),
		"Reason":  Equal(check.ReasonVersionCompatible),
		"Message": ContainSubstring("state: Removed"),
	}))
	g.Expect(result.Annotations).To(HaveKeyWithValue("component.opendatahub.io/management-state", "Removed"))
}

func TestTrainingOperatorDeprecationCheck_ManagedDeprecated(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	dsc := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.DataScienceCluster.APIVersion(),
			"kind":       resources.DataScienceCluster.Kind,
			"metadata": map[string]any{
				"name": "default-dsc",
			},
			"spec": map[string]any{
				"components": map[string]any{
					"trainingoperator": map[string]any{
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

	ver := semver.MustParse("3.3.0")
	target := check.Target{
		Client:        c,
		TargetVersion: &ver,
	}

	trainingoperatorCheck := trainingoperator.NewDeprecationCheck()
	result, err := trainingoperatorCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeCompatible),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonDeprecated),
		"Message": And(ContainSubstring("enabled"), ContainSubstring("deprecated in RHOAI 3.3")),
	}))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactAdvisory))
	g.Expect(result.Annotations).To(And(
		HaveKeyWithValue("component.opendatahub.io/management-state", "Managed"),
		HaveKeyWithValue("check.opendatahub.io/target-version", "3.3.0"),
	))
}

func TestTrainingOperatorDeprecationCheck_UnmanagedDeprecated(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	dsc := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.DataScienceCluster.APIVersion(),
			"kind":       resources.DataScienceCluster.Kind,
			"metadata": map[string]any{
				"name": "default-dsc",
			},
			"spec": map[string]any{
				"components": map[string]any{
					"trainingoperator": map[string]any{
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

	ver := semver.MustParse("3.4.0")
	target := check.Target{
		Client:        c,
		TargetVersion: &ver,
	}

	trainingoperatorCheck := trainingoperator.NewDeprecationCheck()
	result, err := trainingoperatorCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeCompatible),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonDeprecated),
		"Message": ContainSubstring("state: Unmanaged"),
	}))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactAdvisory))
	g.Expect(result.Annotations).To(HaveKeyWithValue("component.opendatahub.io/management-state", "Unmanaged"))
}

func TestTrainingOperatorDeprecationCheck_RemovedReady(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	dsc := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.DataScienceCluster.APIVersion(),
			"kind":       resources.DataScienceCluster.Kind,
			"metadata": map[string]any{
				"name": "default-dsc",
			},
			"spec": map[string]any{
				"components": map[string]any{
					"trainingoperator": map[string]any{
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

	ver := semver.MustParse("3.3.0")
	target := check.Target{
		Client:        c,
		TargetVersion: &ver,
	}

	trainingoperatorCheck := trainingoperator.NewDeprecationCheck()
	result, err := trainingoperatorCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeCompatible),
		"Status":  Equal(metav1.ConditionTrue),
		"Reason":  Equal(check.ReasonVersionCompatible),
		"Message": And(ContainSubstring("disabled"), ContainSubstring("no deprecation warning needed")),
	}))
	g.Expect(result.Annotations).To(HaveKeyWithValue("component.opendatahub.io/management-state", "Removed"))
}

func TestTrainingOperatorDeprecationCheck_CanApply_Version32(t *testing.T) {
	g := NewWithT(t)

	ver := semver.MustParse("3.2.0")
	target := check.Target{
		TargetVersion: &ver,
	}

	trainingoperatorCheck := trainingoperator.NewDeprecationCheck()
	canApply := trainingoperatorCheck.CanApply(t.Context(), target)

	g.Expect(canApply).To(BeFalse())
}

func TestTrainingOperatorDeprecationCheck_CanApply_Version33(t *testing.T) {
	g := NewWithT(t)

	ver := semver.MustParse("3.3.0")
	target := check.Target{
		TargetVersion: &ver,
	}

	trainingoperatorCheck := trainingoperator.NewDeprecationCheck()
	canApply := trainingoperatorCheck.CanApply(t.Context(), target)

	g.Expect(canApply).To(BeTrue())
}

func TestTrainingOperatorDeprecationCheck_CanApply_Version34(t *testing.T) {
	g := NewWithT(t)

	ver := semver.MustParse("3.4.0")
	target := check.Target{
		TargetVersion: &ver,
	}

	trainingoperatorCheck := trainingoperator.NewDeprecationCheck()
	canApply := trainingoperatorCheck.CanApply(t.Context(), target)

	g.Expect(canApply).To(BeTrue())
}

func TestTrainingOperatorDeprecationCheck_Metadata(t *testing.T) {
	g := NewWithT(t)

	trainingoperatorCheck := trainingoperator.NewDeprecationCheck()

	g.Expect(trainingoperatorCheck.ID()).To(Equal("components.trainingoperator.deprecation"))
	g.Expect(trainingoperatorCheck.Name()).To(Equal("Components :: TrainingOperator :: Deprecation (3.3+)"))
	g.Expect(trainingoperatorCheck.Group()).To(Equal(check.GroupComponent))
	g.Expect(trainingoperatorCheck.Description()).ToNot(BeEmpty())
}
