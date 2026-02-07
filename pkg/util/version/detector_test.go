package version_test

import (
	"context"
	"testing"

	"github.com/blang/semver/v4"
	operatorsv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	operatorfake "github.com/operator-framework/operator-lifecycle-manager/pkg/api/client/clientset/versioned/fake"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	"github.com/lburgazzoli/odh-cli/pkg/resources"
	"github.com/lburgazzoli/odh-cli/pkg/util/client"
	"github.com/lburgazzoli/odh-cli/pkg/util/version"

	. "github.com/onsi/gomega"
)

//nolint:gochecknoglobals // Test fixture - shared across test functions
var listKinds = map[schema.GroupVersionResource]string{
	resources.DataScienceCluster.GVR():    resources.DataScienceCluster.ListKind(),
	resources.DSCInitialization.GVR():     resources.DSCInitialization.ListKind(),
	resources.ClusterServiceVersion.GVR(): resources.ClusterServiceVersion.ListKind(),
}

func TestDetect_FromDataScienceCluster(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	// Create fake DataScienceCluster with version
	dsc := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.DataScienceCluster.APIVersion(),
			"kind":       resources.DataScienceCluster.Kind,
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
		Dynamic: dynamicClient,
	})

	clusterVersion, err := version.Detect(ctx, c)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(clusterVersion).ToNot(BeNil())
	g.Expect(clusterVersion.String()).To(Equal("2.17.0"))
	g.Expect(clusterVersion.Major).To(Equal(uint64(2)))
	g.Expect(clusterVersion.Minor).To(Equal(uint64(17)))
	g.Expect(clusterVersion.Patch).To(Equal(uint64(0)))
}

func TestDetect_FromDSCInitialization(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	// Create fake DSCInitialization with version (no DataScienceCluster)
	dsci := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.DSCInitialization.APIVersion(),
			"kind":       resources.DSCInitialization.Kind,
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
		Dynamic: dynamicClient,
	})

	clusterVersion, err := version.Detect(ctx, c)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(clusterVersion).ToNot(BeNil())
	g.Expect(clusterVersion.String()).To(Equal("2.16.0"))
	g.Expect(clusterVersion.Major).To(Equal(uint64(2)))
	g.Expect(clusterVersion.Minor).To(Equal(uint64(16)))
	g.Expect(clusterVersion.Patch).To(Equal(uint64(0)))
}

func TestDetect_FromOLM(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	// Create fake ClusterServiceVersion with version (no DSC/DSCI)
	v := semver.MustParse("2.15.0")
	csv := &operatorsv1alpha1.ClusterServiceVersion{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "rhods-operator.v2.15.0",
			Namespace: "redhat-ods-operator",
			Labels: map[string]string{
				"operators.coreos.com/rhods-operator.redhat-ods-operator": "",
			},
		},
	}
	// Manually set the version field using reflection-free approach
	csv.Spec.Version.Version = v

	scheme := runtime.NewScheme()
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, listKinds)
	olmClient := operatorfake.NewSimpleClientset(csv) //nolint:staticcheck // NewClientset requires generated apply configs not available in OLM

	c := client.NewForTesting(client.TestClientConfig{
		Dynamic: dynamicClient,
		OLM:     olmClient,
	})

	clusterVersion, err := version.Detect(ctx, c)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(clusterVersion).ToNot(BeNil())
	g.Expect(clusterVersion.String()).To(Equal("2.15.0"))
	g.Expect(clusterVersion.Major).To(Equal(uint64(2)))
	g.Expect(clusterVersion.Minor).To(Equal(uint64(15)))
	g.Expect(clusterVersion.Patch).To(Equal(uint64(0)))
}

func TestDetect_PriorityOrder(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	// Create all three sources - should prefer DataScienceCluster
	dsc := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.DataScienceCluster.APIVersion(),
			"kind":       resources.DataScienceCluster.Kind,
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
			"apiVersion": resources.DSCInitialization.APIVersion(),
			"kind":       resources.DSCInitialization.Kind,
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
		Dynamic: dynamicClient,
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
