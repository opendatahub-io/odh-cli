package kueue_test

import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	"github.com/lburgazzoli/odh-cli/pkg/doctor/check"
	"github.com/lburgazzoli/odh-cli/pkg/doctor/checks/components/kueue"
	"github.com/lburgazzoli/odh-cli/pkg/doctor/version"
	"github.com/lburgazzoli/odh-cli/pkg/resources"
	"github.com/lburgazzoli/odh-cli/pkg/util/client"

	. "github.com/onsi/gomega"
)

//nolint:gochecknoglobals // Test fixture - shared across test functions
var listKinds = map[schema.GroupVersionResource]string{
	resources.DataScienceCluster.GVR(): resources.DataScienceCluster.ListKind(),
}

func TestKueueManagedRemovalCheck_NoDSC(t *testing.T) {
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

	kueueCheck := &kueue.ManagedRemovalCheck{}
	result, err := kueueCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).To(HaveField("Status", check.StatusPass))
	g.Expect(result.Severity).To(BeNil())
	g.Expect(result.Message).To(ContainSubstring("No DataScienceCluster"))
}

func TestKueueManagedRemovalCheck_NotConfigured(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	// Create DataScienceCluster without kueue component
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

	kueueCheck := &kueue.ManagedRemovalCheck{}
	result, err := kueueCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).To(HaveField("Status", check.StatusPass))
	g.Expect(result.Severity).To(BeNil())
	g.Expect(result.Message).To(ContainSubstring("not configured"))
}

func TestKueueManagedRemovalCheck_ManagedBlocking(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	// Create DataScienceCluster with kueue Managed (blocking upgrade)
	dsc := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.DataScienceCluster.APIVersion(),
			"kind":       resources.DataScienceCluster.Kind,
			"metadata": map[string]any{
				"name": "default-dsc",
			},
			"spec": map[string]any{
				"components": map[string]any{
					"kueue": map[string]any{
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

	kueueCheck := &kueue.ManagedRemovalCheck{}
	result, err := kueueCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).To(HaveField("Status", check.StatusFail))
	g.Expect(result.Severity).ToNot(BeNil())
	g.Expect(*result.Severity).To(Equal(check.SeverityCritical))
	g.Expect(result.Message).To(ContainSubstring("managed option is enabled"))
	g.Expect(result.Message).To(ContainSubstring("removed in RHOAI 3.x"))
	g.Expect(result.Details).To(HaveKeyWithValue("managementState", "Managed"))
	g.Expect(result.Details).To(HaveKeyWithValue("component", "kueue"))
	g.Expect(result.Details).To(HaveKeyWithValue("targetVersion", "3.0.0"))
}

func TestKueueManagedRemovalCheck_UnmanagedAllowed(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	// Create DataScienceCluster with kueue Unmanaged (allowed in 3.x, check passes)
	dsc := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.DataScienceCluster.APIVersion(),
			"kind":       resources.DataScienceCluster.Kind,
			"metadata": map[string]any{
				"name": "default-dsc",
			},
			"spec": map[string]any{
				"components": map[string]any{
					"kueue": map[string]any{
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

	kueueCheck := &kueue.ManagedRemovalCheck{}
	result, err := kueueCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).To(HaveField("Status", check.StatusPass))
	g.Expect(result.Severity).To(BeNil())
	g.Expect(result.Message).To(ContainSubstring("not enabled"))
	g.Expect(result.Message).To(ContainSubstring("state: Unmanaged"))
	g.Expect(result.Details).To(HaveKeyWithValue("managementState", "Unmanaged"))
}

func TestKueueManagedRemovalCheck_RemovedAllowed(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	// Create DataScienceCluster with kueue Removed (allowed in 3.x, check passes)
	dsc := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.DataScienceCluster.APIVersion(),
			"kind":       resources.DataScienceCluster.Kind,
			"metadata": map[string]any{
				"name": "default-dsc",
			},
			"spec": map[string]any{
				"components": map[string]any{
					"kueue": map[string]any{
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

	kueueCheck := &kueue.ManagedRemovalCheck{}
	result, err := kueueCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).To(HaveField("Status", check.StatusPass))
	g.Expect(result.Severity).To(BeNil())
	g.Expect(result.Message).To(ContainSubstring("not enabled"))
	g.Expect(result.Message).To(ContainSubstring("ready for RHOAI 3.x upgrade"))
	g.Expect(result.Details).To(HaveKeyWithValue("managementState", "Removed"))
}

func TestKueueManagedRemovalCheck_Metadata(t *testing.T) {
	g := NewWithT(t)

	kueueCheck := &kueue.ManagedRemovalCheck{}

	g.Expect(kueueCheck.ID()).To(Equal("components.kueue.managed-removal"))
	g.Expect(kueueCheck.Name()).To(Equal("Components :: Kueue :: Managed Removal (3.x)"))
	g.Expect(kueueCheck.Category()).To(Equal(check.CategoryComponent))
	g.Expect(kueueCheck.Description()).ToNot(BeEmpty())
}
