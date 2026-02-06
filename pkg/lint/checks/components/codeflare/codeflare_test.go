package codeflare_test

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
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/components/codeflare"
	"github.com/lburgazzoli/odh-cli/pkg/resources"
	"github.com/lburgazzoli/odh-cli/pkg/util/client"

	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

//nolint:gochecknoglobals // Test fixture - shared across test functions
var listKinds = map[schema.GroupVersionResource]string{
	resources.DataScienceCluster.GVR(): resources.DataScienceCluster.ListKind(),
}

func TestCodeFlareRemovalCheck_NoDSC(t *testing.T) {
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

	codeflareCheck := codeflare.NewRemovalCheck()
	result, err := codeflareCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeAvailable),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonResourceNotFound),
		"Message": ContainSubstring("No DataScienceCluster"),
	}))
}

func TestCodeFlareRemovalCheck_NotConfigured(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	// Create DataScienceCluster without codeflare component
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

	codeflareCheck := codeflare.NewRemovalCheck()
	result, err := codeflareCheck.Validate(ctx, target)

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

func TestCodeFlareRemovalCheck_ManagedBlocking(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	// Create DataScienceCluster with codeflare Managed (blocking upgrade)
	dsc := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.DataScienceCluster.APIVersion(),
			"kind":       resources.DataScienceCluster.Kind,
			"metadata": map[string]any{
				"name": "default-dsc",
			},
			"spec": map[string]any{
				"components": map[string]any{
					"codeflare": map[string]any{
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

	codeflareCheck := codeflare.NewRemovalCheck()
	result, err := codeflareCheck.Validate(ctx, target)

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

func TestCodeFlareRemovalCheck_UnmanagedReady(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	// Create DataScienceCluster with codeflare Unmanaged
	// CodeFlare only supports Managed state, so Unmanaged is treated as disabled (ready for upgrade)
	dsc := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.DataScienceCluster.APIVersion(),
			"kind":       resources.DataScienceCluster.Kind,
			"metadata": map[string]any{
				"name": "default-dsc",
			},
			"spec": map[string]any{
				"components": map[string]any{
					"codeflare": map[string]any{
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

	codeflareCheck := codeflare.NewRemovalCheck()
	result, err := codeflareCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeCompatible),
		"Status":  Equal(metav1.ConditionTrue),
		"Reason":  Equal(check.ReasonVersionCompatible),
		"Message": ContainSubstring("disabled"),
	}))
	g.Expect(result.Annotations).To(HaveKeyWithValue("component.opendatahub.io/management-state", "Unmanaged"))
}

func TestCodeFlareRemovalCheck_RemovedReady(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	// Create DataScienceCluster with codeflare Removed (ready for upgrade)
	dsc := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.DataScienceCluster.APIVersion(),
			"kind":       resources.DataScienceCluster.Kind,
			"metadata": map[string]any{
				"name": "default-dsc",
			},
			"spec": map[string]any{
				"components": map[string]any{
					"codeflare": map[string]any{
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

	codeflareCheck := codeflare.NewRemovalCheck()
	result, err := codeflareCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeCompatible),
		"Status":  Equal(metav1.ConditionTrue),
		"Reason":  Equal(check.ReasonVersionCompatible),
		"Message": And(ContainSubstring("disabled"), ContainSubstring("ready for RHOAI 3.x upgrade")),
	}))
	g.Expect(result.Annotations).To(HaveKeyWithValue("component.opendatahub.io/management-state", "Removed"))
}

func TestCodeFlareRemovalCheck_Metadata(t *testing.T) {
	g := NewWithT(t)

	codeflareCheck := codeflare.NewRemovalCheck()

	g.Expect(codeflareCheck.ID()).To(Equal("components.codeflare.removal"))
	g.Expect(codeflareCheck.Name()).To(Equal("Components :: CodeFlare :: Removal (3.x)"))
	g.Expect(codeflareCheck.Group()).To(Equal(check.GroupComponent))
	g.Expect(codeflareCheck.Description()).ToNot(BeEmpty())
}
