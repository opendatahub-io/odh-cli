package servicemesh_test

import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	"github.com/lburgazzoli/odh-cli/pkg/doctor/check"
	"github.com/lburgazzoli/odh-cli/pkg/doctor/checks/services/servicemesh"
	"github.com/lburgazzoli/odh-cli/pkg/doctor/version"
	"github.com/lburgazzoli/odh-cli/pkg/resources"
	"github.com/lburgazzoli/odh-cli/pkg/util/client"

	. "github.com/onsi/gomega"
)

//nolint:gochecknoglobals // Test fixture - shared across test functions
var listKinds = map[schema.GroupVersionResource]string{
	resources.DSCInitialization.GVR(): resources.DSCInitialization.ListKind(),
}

func TestServiceMeshRemovalCheck_NoDSCI(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	// Create empty cluster (no DSCInitialization)
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

	servicemeshCheck := &servicemesh.RemovalCheck{}
	result, err := servicemeshCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).To(HaveField("Status", check.StatusPass))
	g.Expect(result.Severity).To(BeNil())
	g.Expect(result.Message).To(ContainSubstring("No DSCInitialization"))
}

func TestServiceMeshRemovalCheck_NotConfigured(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	// Create DSCInitialization without serviceMesh
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
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, listKinds, dsci)

	c := &client.Client{
		Dynamic: dynamicClient,
	}

	target := &check.CheckTarget{
		Client: c,
		Version: &version.ClusterVersion{
			Version: "3.0.0",
		},
	}

	servicemeshCheck := &servicemesh.RemovalCheck{}
	result, err := servicemeshCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).To(HaveField("Status", check.StatusPass))
	g.Expect(result.Severity).To(BeNil())
	g.Expect(result.Message).To(ContainSubstring("not configured"))
}

func TestServiceMeshRemovalCheck_ManagedBlocking(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	// Create DSCInitialization with serviceMesh Managed (blocking upgrade)
	dsci := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.DSCInitialization.APIVersion(),
			"kind":       resources.DSCInitialization.Kind,
			"metadata": map[string]any{
				"name": "default-dsci",
			},
			"spec": map[string]any{
				"applicationsNamespace": "opendatahub",
				"serviceMesh": map[string]any{
					"managementState": "Managed",
				},
			},
		},
	}

	scheme := runtime.NewScheme()
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, listKinds, dsci)

	c := &client.Client{
		Dynamic: dynamicClient,
	}

	target := &check.CheckTarget{
		Client: c,
		Version: &version.ClusterVersion{
			Version: "3.0.0",
		},
	}

	servicemeshCheck := &servicemesh.RemovalCheck{}
	result, err := servicemeshCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).To(HaveField("Status", check.StatusFail))
	g.Expect(result.Severity).ToNot(BeNil())
	g.Expect(*result.Severity).To(Equal(check.SeverityCritical))
	g.Expect(result.Message).To(ContainSubstring("enabled"))
	g.Expect(result.Message).To(ContainSubstring("not supported in RHOAI 3.x"))
	g.Expect(result.Details).To(HaveKeyWithValue("managementState", "Managed"))
	g.Expect(result.Details).To(HaveKeyWithValue("service", "serviceMesh"))
	g.Expect(result.Details).To(HaveKeyWithValue("targetVersion", "3.0.0"))
}

func TestServiceMeshRemovalCheck_UnmanagedBlocking(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	// Create DSCInitialization with serviceMesh Unmanaged (also blocking)
	dsci := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.DSCInitialization.APIVersion(),
			"kind":       resources.DSCInitialization.Kind,
			"metadata": map[string]any{
				"name": "default-dsci",
			},
			"spec": map[string]any{
				"applicationsNamespace": "opendatahub",
				"serviceMesh": map[string]any{
					"managementState": "Unmanaged",
				},
			},
		},
	}

	scheme := runtime.NewScheme()
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, listKinds, dsci)

	c := &client.Client{
		Dynamic: dynamicClient,
	}

	target := &check.CheckTarget{
		Client: c,
		Version: &version.ClusterVersion{
			Version: "3.1.0",
		},
	}

	servicemeshCheck := &servicemesh.RemovalCheck{}
	result, err := servicemeshCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).To(HaveField("Status", check.StatusFail))
	g.Expect(result.Severity).ToNot(BeNil())
	g.Expect(*result.Severity).To(Equal(check.SeverityCritical))
	g.Expect(result.Message).To(ContainSubstring("state: Unmanaged"))
	g.Expect(result.Details).To(HaveKeyWithValue("managementState", "Unmanaged"))
}

func TestServiceMeshRemovalCheck_RemovedReady(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	// Create DSCInitialization with serviceMesh Removed (ready for upgrade)
	dsci := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.DSCInitialization.APIVersion(),
			"kind":       resources.DSCInitialization.Kind,
			"metadata": map[string]any{
				"name": "default-dsci",
			},
			"spec": map[string]any{
				"applicationsNamespace": "opendatahub",
				"serviceMesh": map[string]any{
					"managementState": "Removed",
				},
			},
		},
	}

	scheme := runtime.NewScheme()
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, listKinds, dsci)

	c := &client.Client{
		Dynamic: dynamicClient,
	}

	target := &check.CheckTarget{
		Client: c,
		Version: &version.ClusterVersion{
			Version: "3.0.0",
		},
	}

	servicemeshCheck := &servicemesh.RemovalCheck{}
	result, err := servicemeshCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).To(HaveField("Status", check.StatusPass))
	g.Expect(result.Severity).To(BeNil())
	g.Expect(result.Message).To(ContainSubstring("disabled"))
	g.Expect(result.Message).To(ContainSubstring("ready for RHOAI 3.x upgrade"))
	g.Expect(result.Details).To(HaveKeyWithValue("managementState", "Removed"))
}

func TestServiceMeshRemovalCheck_Metadata(t *testing.T) {
	g := NewWithT(t)

	servicemeshCheck := &servicemesh.RemovalCheck{}

	g.Expect(servicemeshCheck.ID()).To(Equal("services.servicemesh.removal"))
	g.Expect(servicemeshCheck.Name()).To(Equal("Services :: ServiceMesh :: Removal (3.x)"))
	g.Expect(servicemeshCheck.Category()).To(Equal(check.CategoryService))
	g.Expect(servicemeshCheck.Description()).ToNot(BeEmpty())
}
