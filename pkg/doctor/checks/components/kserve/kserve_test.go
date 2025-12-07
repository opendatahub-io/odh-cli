package kserve_test

import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	"github.com/lburgazzoli/odh-cli/pkg/doctor/check"
	"github.com/lburgazzoli/odh-cli/pkg/doctor/checks/components/kserve"
	"github.com/lburgazzoli/odh-cli/pkg/doctor/version"
	"github.com/lburgazzoli/odh-cli/pkg/resources"
	"github.com/lburgazzoli/odh-cli/pkg/util/client"

	. "github.com/onsi/gomega"
)

//nolint:gochecknoglobals // Test fixture - shared across test functions
var listKinds = map[schema.GroupVersionResource]string{
	resources.DataScienceCluster.GVR(): resources.DataScienceCluster.ListKind(),
}

func TestKServeServerlessRemovalCheck_NoDSC(t *testing.T) {
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

	kserveCheck := &kserve.ServerlessRemovalCheck{}
	result, err := kserveCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).To(HaveField("Status", check.StatusPass))
	g.Expect(result.Severity).To(BeNil())
	g.Expect(result.Message).To(ContainSubstring("No DataScienceCluster"))
}

func TestKServeServerlessRemovalCheck_KServeNotConfigured(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	// Create DataScienceCluster without kserve component
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

	kserveCheck := &kserve.ServerlessRemovalCheck{}
	result, err := kserveCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).To(HaveField("Status", check.StatusPass))
	g.Expect(result.Severity).To(BeNil())
	g.Expect(result.Message).To(ContainSubstring("not configured"))
}

func TestKServeServerlessRemovalCheck_KServeNotManaged(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	// Create DataScienceCluster with kserve Removed (not managed)
	dsc := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.DataScienceCluster.APIVersion(),
			"kind":       resources.DataScienceCluster.Kind,
			"metadata": map[string]any{
				"name": "default-dsc",
			},
			"spec": map[string]any{
				"components": map[string]any{
					"kserve": map[string]any{
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

	kserveCheck := &kserve.ServerlessRemovalCheck{}
	result, err := kserveCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).To(HaveField("Status", check.StatusPass))
	g.Expect(result.Severity).To(BeNil())
	g.Expect(result.Message).To(ContainSubstring("not managed"))
	g.Expect(result.Details).To(HaveKeyWithValue("kserveManagementState", "Removed"))
}

func TestKServeServerlessRemovalCheck_ServerlessNotConfigured(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	// Create DataScienceCluster with kserve Managed but no serverless config
	dsc := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.DataScienceCluster.APIVersion(),
			"kind":       resources.DataScienceCluster.Kind,
			"metadata": map[string]any{
				"name": "default-dsc",
			},
			"spec": map[string]any{
				"components": map[string]any{
					"kserve": map[string]any{
						"managementState": "Managed",
						// No serving config
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

	kserveCheck := &kserve.ServerlessRemovalCheck{}
	result, err := kserveCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).To(HaveField("Status", check.StatusPass))
	g.Expect(result.Severity).To(BeNil())
	g.Expect(result.Message).To(ContainSubstring("serverless mode is not configured"))
	g.Expect(result.Details).To(HaveKeyWithValue("kserveManagementState", "Managed"))
}

func TestKServeServerlessRemovalCheck_ServerlessManagedBlocking(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	// Create DataScienceCluster with kserve serverless Managed (blocking upgrade)
	dsc := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.DataScienceCluster.APIVersion(),
			"kind":       resources.DataScienceCluster.Kind,
			"metadata": map[string]any{
				"name": "default-dsc",
			},
			"spec": map[string]any{
				"components": map[string]any{
					"kserve": map[string]any{
						"managementState": "Managed",
						"serving": map[string]any{
							"managementState": "Managed",
						},
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

	kserveCheck := &kserve.ServerlessRemovalCheck{}
	result, err := kserveCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).To(HaveField("Status", check.StatusFail))
	g.Expect(result.Severity).ToNot(BeNil())
	g.Expect(*result.Severity).To(Equal(check.SeverityCritical))
	g.Expect(result.Message).To(ContainSubstring("serverless mode is enabled"))
	g.Expect(result.Message).To(ContainSubstring("removed in RHOAI 3.x"))
	g.Expect(result.Details).To(HaveKeyWithValue("kserveManagementState", "Managed"))
	g.Expect(result.Details).To(HaveKeyWithValue("servingManagementState", "Managed"))
	g.Expect(result.Details).To(HaveKeyWithValue("component", "kserve"))
	g.Expect(result.Details).To(HaveKeyWithValue("targetVersion", "3.0.0"))
}

func TestKServeServerlessRemovalCheck_ServerlessUnmanagedBlocking(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	// Create DataScienceCluster with kserve serverless Unmanaged (also blocking)
	dsc := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.DataScienceCluster.APIVersion(),
			"kind":       resources.DataScienceCluster.Kind,
			"metadata": map[string]any{
				"name": "default-dsc",
			},
			"spec": map[string]any{
				"components": map[string]any{
					"kserve": map[string]any{
						"managementState": "Managed",
						"serving": map[string]any{
							"managementState": "Unmanaged",
						},
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

	kserveCheck := &kserve.ServerlessRemovalCheck{}
	result, err := kserveCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).To(HaveField("Status", check.StatusFail))
	g.Expect(result.Severity).ToNot(BeNil())
	g.Expect(*result.Severity).To(Equal(check.SeverityCritical))
	g.Expect(result.Message).To(ContainSubstring("state: Unmanaged"))
	g.Expect(result.Details).To(HaveKeyWithValue("servingManagementState", "Unmanaged"))
}

func TestKServeServerlessRemovalCheck_ServerlessRemovedReady(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	// Create DataScienceCluster with kserve serverless Removed (ready for upgrade)
	dsc := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.DataScienceCluster.APIVersion(),
			"kind":       resources.DataScienceCluster.Kind,
			"metadata": map[string]any{
				"name": "default-dsc",
			},
			"spec": map[string]any{
				"components": map[string]any{
					"kserve": map[string]any{
						"managementState": "Managed",
						"serving": map[string]any{
							"managementState": "Removed",
						},
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

	kserveCheck := &kserve.ServerlessRemovalCheck{}
	result, err := kserveCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).To(HaveField("Status", check.StatusPass))
	g.Expect(result.Severity).To(BeNil())
	g.Expect(result.Message).To(ContainSubstring("serverless mode is disabled"))
	g.Expect(result.Message).To(ContainSubstring("ready for RHOAI 3.x upgrade"))
	g.Expect(result.Details).To(HaveKeyWithValue("kserveManagementState", "Managed"))
	g.Expect(result.Details).To(HaveKeyWithValue("servingManagementState", "Removed"))
}

func TestKServeServerlessRemovalCheck_Metadata(t *testing.T) {
	g := NewWithT(t)

	kserveCheck := &kserve.ServerlessRemovalCheck{}

	g.Expect(kserveCheck.ID()).To(Equal("components.kserve.serverless-removal"))
	g.Expect(kserveCheck.Name()).To(Equal("Components :: KServe :: Serverless Removal (3.x)"))
	g.Expect(kserveCheck.Category()).To(Equal(check.CategoryComponent))
	g.Expect(kserveCheck.Description()).ToNot(BeEmpty())
}
