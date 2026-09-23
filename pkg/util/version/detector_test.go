package version_test

import (
	"context"
	"testing"

	crfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	discoveryfake "k8s.io/client-go/discovery/fake"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	coretesting "k8s.io/client-go/testing"

	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
	"github.com/opendatahub-io/odh-cli/pkg/util/version"

	. "github.com/onsi/gomega"
)

// newFakeDiscoveryWithResources creates a fake discovery client that reports the given group/versions.
func newFakeDiscoveryWithResources(groupVersions ...string) *discoveryfake.FakeDiscovery {
	fakeDiscovery := &discoveryfake.FakeDiscovery{Fake: &coretesting.Fake{}}

	apiResourceLists := make([]*metav1.APIResourceList, 0, len(groupVersions))
	for _, gv := range groupVersions {
		apiResourceLists = append(apiResourceLists, &metav1.APIResourceList{
			GroupVersion: gv,
			APIResources: []metav1.APIResource{
				{Name: "datascienceclusters", Kind: "DataScienceCluster"},
				{Name: "dscinitializations", Kind: "DSCInitialization"},
			},
		})
	}

	fakeDiscovery.Resources = apiResourceLists

	return fakeDiscovery
}

//nolint:gochecknoglobals // Test fixture - shared across test functions
var listKinds = map[schema.GroupVersionResource]string{
	resources.DataScienceCluster.GVR():    resources.DataScienceCluster.ListKind(),
	resources.DataScienceClusterV1.GVR():  resources.DataScienceClusterV1.ListKind(),
	resources.DSCInitialization.GVR():     resources.DSCInitialization.ListKind(),
	resources.DSCInitializationV1.GVR():   resources.DSCInitializationV1.ListKind(),
	resources.ClusterServiceVersion.GVR(): resources.ClusterServiceVersion.ListKind(),
}

const (
	testOLMV1ExtensionName = "rhoai-operator"
	testOLMV1PackageName   = "rhods-operator"
	testOLMV1Version       = "2.22.1"
)

func TestDetect_FromDataScienceCluster(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	// Create fake DataScienceCluster with version (using v1 API - fallback when v2 absent)
	dsc := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.DataScienceClusterV1.APIVersion(),
			"kind":       resources.DataScienceClusterV1.Kind,
			"metadata": map[string]any{
				"name": "default-dsc",
			},
			"status": map[string]any{
				"release": map[string]any{
					"version": "2.17.0",
				},
			},
		},
	}

	scheme := runtime.NewScheme()
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, listKinds, dsc)

	c := client.NewForTesting(client.TestClientConfig{
		Dynamic:   dynamicClient,
		Discovery: newFakeDiscoveryWithResources(resources.DataScienceCluster.Group + "/v1"),
	})

	clusterVersion, err := version.Detect(ctx, c)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(clusterVersion).ToNot(BeNil())
	g.Expect(clusterVersion.String()).To(Equal("2.17.0"))
	g.Expect(clusterVersion.Major).To(Equal(uint64(2)))
	g.Expect(clusterVersion.Minor).To(Equal(uint64(17)))
	g.Expect(clusterVersion.Patch).To(Equal(uint64(0)))
}

func TestDetect_FromDataScienceCluster_V2(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	// Create fake DataScienceCluster with version (using v2 API - when Discovery finds v2)
	dsc := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.DataScienceCluster.APIVersion(),
			"kind":       resources.DataScienceCluster.Kind,
			"metadata": map[string]any{
				"name": "default-dsc",
			},
			"status": map[string]any{
				"release": map[string]any{
					"version": "3.5.0",
				},
			},
		},
	}

	scheme := runtime.NewScheme()
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, listKinds, dsc)

	// Create a fake discovery client that returns v2 resources
	fakeDiscovery := newFakeDiscoveryWithResources(
		resources.DataScienceCluster.Group + "/v2",
	)

	c := client.NewForTesting(client.TestClientConfig{
		Dynamic:   dynamicClient,
		Discovery: fakeDiscovery,
	})

	clusterVersion, err := version.Detect(ctx, c)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(clusterVersion).ToNot(BeNil())
	g.Expect(clusterVersion.String()).To(Equal("3.5.0"))
	g.Expect(clusterVersion.Major).To(Equal(uint64(3)))
	g.Expect(clusterVersion.Minor).To(Equal(uint64(5)))
}

func TestDetect_MidUpgrade_V2CRDExistsButOnlyV1Instance(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	// Mid-upgrade scenario: v2 CRD is registered but only v1 instance exists
	// This tests the fallback from v2 NotFound to v1
	dsc := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.DataScienceClusterV1.APIVersion(),
			"kind":       resources.DataScienceClusterV1.Kind,
			"metadata": map[string]any{
				"name": "default-dsc",
			},
			"status": map[string]any{
				"release": map[string]any{
					"version": "2.35.0",
				},
			},
		},
	}

	scheme := runtime.NewScheme()
	// Only register the v1 instance - no v2 instance exists
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, listKinds, dsc)

	// Discovery reports v2 API exists (CRD installed during upgrade)
	fakeDiscovery := newFakeDiscoveryWithResources(
		resources.DataScienceCluster.Group + "/v2",
	)

	c := client.NewForTesting(client.TestClientConfig{
		Dynamic:   dynamicClient,
		Discovery: fakeDiscovery,
	})

	// Should fall back to v1 and find the instance
	clusterVersion, err := version.Detect(ctx, c)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(clusterVersion).ToNot(BeNil())
	g.Expect(clusterVersion.String()).To(Equal("2.35.0"))
	g.Expect(clusterVersion.Major).To(Equal(uint64(2)))
	g.Expect(clusterVersion.Minor).To(Equal(uint64(35)))
}

func TestDetect_FromDSCInitialization(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	// Create fake DSCInitialization with version (using v1 API - fallback when v2 absent)
	dsci := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.DSCInitializationV1.APIVersion(),
			"kind":       resources.DSCInitializationV1.Kind,
			"metadata": map[string]any{
				"name": "default-dsci",
			},
			"status": map[string]any{
				"release": map[string]any{
					"version": "2.16.0",
				},
			},
		},
	}

	scheme := runtime.NewScheme()
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, listKinds, dsci)

	c := client.NewForTesting(client.TestClientConfig{
		Dynamic:   dynamicClient,
		Discovery: newFakeDiscoveryWithResources(resources.DSCInitialization.Group + "/v1"),
	})

	clusterVersion, err := version.Detect(ctx, c)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(clusterVersion).ToNot(BeNil())
	g.Expect(clusterVersion.String()).To(Equal("2.16.0"))
	g.Expect(clusterVersion.Major).To(Equal(uint64(2)))
	g.Expect(clusterVersion.Minor).To(Equal(uint64(16)))
	g.Expect(clusterVersion.Patch).To(Equal(uint64(0)))
}

func TestDetect_FromDSCInitialization_V2(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	// Create fake DSCInitialization with version (using v2 API - when Discovery finds v2)
	dsci := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.DSCInitialization.APIVersion(),
			"kind":       resources.DSCInitialization.Kind,
			"metadata": map[string]any{
				"name": "default-dsci",
			},
			"status": map[string]any{
				"release": map[string]any{
					"version": "3.6.0",
				},
			},
		},
	}

	scheme := runtime.NewScheme()
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, listKinds, dsci)

	// Create a fake discovery client that returns v2 resources
	fakeDiscovery := newFakeDiscoveryWithResources(
		resources.DSCInitialization.Group + "/v2",
	)

	c := client.NewForTesting(client.TestClientConfig{
		Dynamic:   dynamicClient,
		Discovery: fakeDiscovery,
	})

	clusterVersion, err := version.Detect(ctx, c)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(clusterVersion).ToNot(BeNil())
	g.Expect(clusterVersion.String()).To(Equal("3.6.0"))
	g.Expect(clusterVersion.Major).To(Equal(uint64(3)))
	g.Expect(clusterVersion.Minor).To(Equal(uint64(6)))
}

func TestDetect_FromOLMV0(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	operatorConditionGVK := schema.GroupVersionKind{
		Group: "operators.coreos.com", Version: "v2", Kind: "OperatorCondition",
	}
	operatorCondition := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": operatorConditionGVK.GroupVersion().String(),
		"kind":       operatorConditionGVK.Kind,
		"metadata": map[string]any{
			"name": "rhods-operator.2.15.0",
		},
	}}

	scheme := runtime.NewScheme()
	for _, gvk := range []schema.GroupVersionKind{
		operatorConditionGVK,
		{Group: "operators.coreos.com", Version: "v1alpha1", Kind: "CatalogSource"},
		{Group: "olm.operatorframework.io", Version: "v1", Kind: "ClusterCatalog"},
		{Group: "olm.operatorframework.io", Version: "v1", Kind: "ClusterExtension"},
	} {
		scheme.AddKnownTypeWithName(gvk, &unstructured.Unstructured{})
		scheme.AddKnownTypeWithName(
			gvk.GroupVersion().WithKind(gvk.Kind+"List"),
			&unstructured.UnstructuredList{},
		)
	}

	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, listKinds)
	controllerRuntimeClient := crfake.NewClientBuilder().
		WithScheme(scheme).
		WithRuntimeObjects(operatorCondition).
		Build()

	c := client.NewForTesting(client.TestClientConfig{
		Dynamic:           dynamicClient,
		ControllerRuntime: controllerRuntimeClient,
	})

	clusterVersion, err := version.Detect(ctx, c)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(clusterVersion).ToNot(BeNil())
	g.Expect(clusterVersion.String()).To(Equal("2.15.0"))
	g.Expect(clusterVersion.Major).To(Equal(uint64(2)))
	g.Expect(clusterVersion.Minor).To(Equal(uint64(15)))
	g.Expect(clusterVersion.Patch).To(Equal(uint64(0)))
}

func TestDetect_FromOLMV1(t *testing.T) {
	g := NewWithT(t)

	clusterExtension := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "olm.operatorframework.io/v1",
		"kind":       "ClusterExtension",
		"metadata": map[string]any{
			"name": testOLMV1ExtensionName,
		},
		"spec": map[string]any{
			"source": map[string]any{
				"sourceType": "Catalog",
				"catalog": map[string]any{
					"packageName": testOLMV1PackageName,
				},
			},
		},
		"status": map[string]any{
			"conditions": []any{
				map[string]any{
					"type":   "Installed",
					"status": "True",
					"reason": "Succeeded",
				},
			},
			"install": map[string]any{
				"bundle": map[string]any{
					"version": testOLMV1Version,
				},
			},
		},
	}}

	scheme := runtime.NewScheme()
	for _, gvk := range []schema.GroupVersionKind{
		{Group: "operators.coreos.com", Version: "v2", Kind: "OperatorCondition"},
		{Group: "operators.coreos.com", Version: "v1alpha1", Kind: "CatalogSource"},
		{Group: "olm.operatorframework.io", Version: "v1", Kind: "ClusterCatalog"},
		{Group: "olm.operatorframework.io", Version: "v1", Kind: "ClusterExtension"},
	} {
		scheme.AddKnownTypeWithName(gvk, &unstructured.Unstructured{})
		scheme.AddKnownTypeWithName(
			gvk.GroupVersion().WithKind(gvk.Kind+"List"),
			&unstructured.UnstructuredList{},
		)
	}

	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, listKinds)
	controllerRuntimeClient := crfake.NewClientBuilder().
		WithScheme(scheme).
		WithRuntimeObjects(clusterExtension).
		Build()
	testClient := client.NewForTesting(client.TestClientConfig{
		Dynamic:           dynamicClient,
		ControllerRuntime: controllerRuntimeClient,
	})

	clusterVersion, err := version.Detect(t.Context(), testClient)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(clusterVersion).ToNot(BeNil())
	g.Expect(clusterVersion.String()).To(Equal(testOLMV1Version))
}

func TestDetect_PriorityOrder(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	// Create all three sources - should prefer DataScienceCluster (using v1 API)
	dsc := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.DataScienceClusterV1.APIVersion(),
			"kind":       resources.DataScienceClusterV1.Kind,
			"metadata": map[string]any{
				"name": "default-dsc",
			},
			"status": map[string]any{
				"release": map[string]any{
					"version": "2.17.0",
				},
			},
		},
	}

	dsci := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.DSCInitializationV1.APIVersion(),
			"kind":       resources.DSCInitializationV1.Kind,
			"metadata": map[string]any{
				"name": "default-dsci",
			},
			"status": map[string]any{
				"release": map[string]any{
					"version": "2.16.0",
				},
			},
		},
	}

	csv := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.ClusterServiceVersion.APIVersion(),
			"kind":       resources.ClusterServiceVersion.Kind,
			"metadata": map[string]any{
				"name":      "rhods-operator.v2.15.0",
				"namespace": "redhat-ods-operator",
				"labels": map[string]any{
					"operators.coreos.com/rhods-operator.redhat-ods-operator": "",
				},
			},
			"spec": map[string]any{
				"version": "2.15.0",
			},
		},
	}

	scheme := runtime.NewScheme()
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, listKinds, dsc, dsci, csv)

	c := client.NewForTesting(client.TestClientConfig{
		Dynamic:   dynamicClient,
		Discovery: newFakeDiscoveryWithResources(resources.DataScienceCluster.Group + "/v1"),
	})

	clusterVersion, err := version.Detect(ctx, c)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(clusterVersion).ToNot(BeNil())
	g.Expect(clusterVersion.String()).To(Equal("2.17.0"))
	g.Expect(clusterVersion.Major).To(Equal(uint64(2)))
	g.Expect(clusterVersion.Minor).To(Equal(uint64(17)))
}

func TestDetect_NoVersionFound(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	// Empty cluster - no version sources
	scheme := runtime.NewScheme()
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, listKinds)

	c := client.NewForTesting(client.TestClientConfig{
		Dynamic: dynamicClient,
	})

	clusterVersion, err := version.Detect(ctx, c)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("unable to detect cluster version"))
	g.Expect(clusterVersion).To(BeNil())
}

func TestVersionToBranch_Version2(t *testing.T) {
	g := NewWithT(t)

	branch, err := version.VersionToBranch("2.17.0")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(branch).To(Equal("stable-2.17"))
}

func TestVersionToBranch_Version3(t *testing.T) {
	g := NewWithT(t)

	branch, err := version.VersionToBranch("3.0.0")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(branch).To(Equal("main"))
}

func TestVersionToBranch_InvalidFormat(t *testing.T) {
	g := NewWithT(t)

	branch, err := version.VersionToBranch("invalid")
	g.Expect(err).To(HaveOccurred())
	g.Expect(branch).To(Equal(""))
}

func TestVersionToBranch_UnsupportedVersion(t *testing.T) {
	g := NewWithT(t)

	branch, err := version.VersionToBranch("1.0.0")
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("unsupported version"))
	g.Expect(branch).To(Equal(""))
}
