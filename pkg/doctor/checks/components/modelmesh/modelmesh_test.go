package modelmesh_test

import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	"github.com/lburgazzoli/odh-cli/pkg/doctor/check"
	"github.com/lburgazzoli/odh-cli/pkg/doctor/checks/components/modelmesh"
	"github.com/lburgazzoli/odh-cli/pkg/doctor/version"
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

	c := &client.Client{
		Dynamic: dynamicClient,
	}

	target := &check.CheckTarget{
		Client: c,
		Version: &version.ClusterVersion{
			Version: "3.0.0", // Targeting 3.x upgrade
		},
	}

	modelmeshCheck := &modelmesh.RemovalCheck{}
	result, err := modelmeshCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(*result).To(MatchFields(IgnoreExtras, Fields{
		"Status":   Equal(check.StatusPass),
		"Severity": BeNil(),
		"Message":  ContainSubstring("No DataScienceCluster"),
	}))
}

func TestModelmeshRemovalCheck_NotConfigured(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	// Create DataScienceCluster without modelmeshserving component
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

	c := &client.Client{
		Dynamic: dynamicClient,
	}

	target := &check.CheckTarget{
		Client: c,
		Version: &version.ClusterVersion{
			Version: "3.0.0",
		},
	}

	modelmeshCheck := &modelmesh.RemovalCheck{}
	result, err := modelmeshCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(*result).To(MatchFields(IgnoreExtras, Fields{
		"Status":   Equal(check.StatusPass),
		"Severity": BeNil(),
		"Message":  ContainSubstring("not configured"),
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

	c := &client.Client{
		Dynamic: dynamicClient,
	}

	target := &check.CheckTarget{
		Client: c,
		Version: &version.ClusterVersion{
			Version: "3.0.0",
		},
	}

	modelmeshCheck := &modelmesh.RemovalCheck{}
	result, err := modelmeshCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(*result).To(MatchFields(IgnoreExtras, Fields{
		"Status":   Equal(check.StatusFail),
		"Severity": PointTo(Equal(check.SeverityCritical)),
		"Message":  And(ContainSubstring("still enabled"), ContainSubstring("removed in RHOAI 3.x")),
		"Details": And(
			HaveKeyWithValue("managementState", "Managed"),
			HaveKeyWithValue("component", "modelmeshserving"),
			HaveKeyWithValue("targetVersion", "3.0.0"),
		),
	}))
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

	c := &client.Client{
		Dynamic: dynamicClient,
	}

	target := &check.CheckTarget{
		Client: c,
		Version: &version.ClusterVersion{
			Version: "3.1.0",
		},
	}

	modelmeshCheck := &modelmesh.RemovalCheck{}
	result, err := modelmeshCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(*result).To(MatchFields(IgnoreExtras, Fields{
		"Status":   Equal(check.StatusFail),
		"Severity": PointTo(Equal(check.SeverityCritical)),
		"Message":  ContainSubstring("state: Unmanaged"),
		"Details":  HaveKeyWithValue("managementState", "Unmanaged"),
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

	c := &client.Client{
		Dynamic: dynamicClient,
	}

	target := &check.CheckTarget{
		Client: c,
		Version: &version.ClusterVersion{
			Version: "3.0.0",
		},
	}

	modelmeshCheck := &modelmesh.RemovalCheck{}
	result, err := modelmeshCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(*result).To(MatchFields(IgnoreExtras, Fields{
		"Status":   Equal(check.StatusPass),
		"Severity": BeNil(),
		"Message":  And(ContainSubstring("disabled"), ContainSubstring("ready for RHOAI 3.x upgrade")),
		"Details":  HaveKeyWithValue("managementState", "Removed"),
	}))
}

func TestModelmeshRemovalCheck_Metadata(t *testing.T) {
	g := NewWithT(t)

	modelmeshCheck := &modelmesh.RemovalCheck{}

	g.Expect(modelmeshCheck.ID()).To(Equal("components.modelmesh.removal"))
	g.Expect(modelmeshCheck.Name()).To(Equal("Components :: ModelMesh :: Removal (3.x)"))
	g.Expect(modelmeshCheck.Category()).To(Equal(check.CategoryComponent))
	g.Expect(modelmeshCheck.Description()).ToNot(BeEmpty())
}
