package modelmesh_test

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
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/components/modelmesh"
	"github.com/lburgazzoli/odh-cli/pkg/resources"
	"github.com/lburgazzoli/odh-cli/pkg/util/client"

	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

//nolint:gochecknoglobals // Test fixture - shared across test functions
var listKinds = map[schema.GroupVersionResource]string{
	resources.DataScienceCluster.GVR(): resources.DataScienceCluster.ListKind(),
}

func TestModelmeshRemovalCheck_NoDSC(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	// Create empty cluster (no DataScienceCluster)
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

	modelmeshCheck := modelmesh.NewRemovalCheck()
	result, err := modelmeshCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeAvailable),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonResourceNotFound),
		"Message": ContainSubstring("No DataScienceCluster"),
	}))
}

func TestModelmeshRemovalCheck_NotConfigured(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	// Create DataScienceCluster without modelmesh component
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

	ver := semver.MustParse("3.0.0")
	target := check.Target{
		Client:        c,
		TargetVersion: &ver,
	}

	modelmeshCheck := modelmesh.NewRemovalCheck()
	result, err := modelmeshCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	// When component is not configured, InState(Managed) filter passes (check doesn't apply)
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(check.ConditionTypeConfigured),
		"Status": Equal(metav1.ConditionTrue),
		"Reason": Equal(check.ReasonRequirementsMet),
	}))
}

func TestModelmeshRemovalCheck_ManagedBlocking(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	// Create DataScienceCluster with modelmeshserving Managed (blocking upgrade)
	dsc := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.DataScienceCluster.APIVersion(),
			"kind":       resources.DataScienceCluster.Kind,
			"metadata": map[string]any{
				"name": "default-dsc",
			},
			"spec": map[string]any{
				"components": map[string]any{
					"modelmeshserving": map[string]any{
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

	modelmeshCheck := modelmesh.NewRemovalCheck()
	result, err := modelmeshCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeCompatible),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonVersionIncompatible),
		"Message": And(ContainSubstring("enabled"), ContainSubstring("removed in RHOAI 3.x")),
	}))
	g.Expect(result.Annotations).To(And(
		HaveKeyWithValue("component.opendatahub.io/management-state", "Managed"),
		HaveKeyWithValue("check.opendatahub.io/target-version", "3.0.0"),
	))
}

func TestModelmeshRemovalCheck_UnmanagedBlocking(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	// Create DataScienceCluster with modelmeshserving Unmanaged (also blocking)
	dsc := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.DataScienceCluster.APIVersion(),
			"kind":       resources.DataScienceCluster.Kind,
			"metadata": map[string]any{
				"name": "default-dsc",
			},
			"spec": map[string]any{
				"components": map[string]any{
					"modelmeshserving": map[string]any{
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

	ver := semver.MustParse("3.1.0")
	target := check.Target{
		Client:        c,
		TargetVersion: &ver,
	}

	modelmeshCheck := modelmesh.NewRemovalCheck()
	result, err := modelmeshCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	// Unmanaged is not in InState(Managed), so the builder passes (check doesn't apply)
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(check.ConditionTypeConfigured),
		"Status": Equal(metav1.ConditionTrue),
		"Reason": Equal(check.ReasonRequirementsMet),
	}))
}

func TestModelmeshRemovalCheck_RemovedReady(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	// Create DataScienceCluster with modelmeshserving Removed (ready for upgrade)
	dsc := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.DataScienceCluster.APIVersion(),
			"kind":       resources.DataScienceCluster.Kind,
			"metadata": map[string]any{
				"name": "default-dsc",
			},
			"spec": map[string]any{
				"components": map[string]any{
					"modelmeshserving": map[string]any{
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

	modelmeshCheck := modelmesh.NewRemovalCheck()
	result, err := modelmeshCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	// Removed is not in InState(Managed), so the builder passes (check doesn't apply)
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(check.ConditionTypeConfigured),
		"Status": Equal(metav1.ConditionTrue),
		"Reason": Equal(check.ReasonRequirementsMet),
	}))
}

func TestModelmeshRemovalCheck_Metadata(t *testing.T) {
	g := NewWithT(t)

	modelmeshCheck := modelmesh.NewRemovalCheck()

	g.Expect(modelmeshCheck.ID()).To(Equal("components.modelmesh.removal"))
	g.Expect(modelmeshCheck.Name()).To(Equal("Components :: ModelMesh :: Removal (3.x)"))
	g.Expect(modelmeshCheck.Group()).To(Equal(check.GroupComponent))
	g.Expect(modelmeshCheck.Description()).ToNot(BeEmpty())
}
