package version_test

import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	"github.com/lburgazzoli/odh-cli/pkg/doctor/version"
	"github.com/lburgazzoli/odh-cli/pkg/resources"
	"github.com/lburgazzoli/odh-cli/pkg/util/client"

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

	c := &client.Client{
		Dynamic: dynamicClient,
	}

	clusterVersion, err := version.Detect(ctx, c)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(clusterVersion).To(HaveField("Version", "2.17.0"))
	g.Expect(clusterVersion).To(HaveField("Source", version.SourceDataScienceCluster))
	g.Expect(clusterVersion).To(HaveField("Confidence", version.ConfidenceHigh))
	g.Expect(clusterVersion).To(HaveField("Branch", "stable-2.17"))
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

	c := &client.Client{
		Dynamic: dynamicClient,
	}

	clusterVersion, err := version.Detect(ctx, c)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(clusterVersion).To(HaveField("Version", "2.16.0"))
	g.Expect(clusterVersion).To(HaveField("Source", version.SourceDSCInitialization))
	g.Expect(clusterVersion).To(HaveField("Confidence", version.ConfidenceHigh))
	g.Expect(clusterVersion).To(HaveField("Branch", "stable-2.16"))
}

func TestDetect_FromOLM(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	// Create fake ClusterServiceVersion with version (no DSC/DSCI)
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
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, listKinds, csv)

	c := &client.Client{
		Dynamic: dynamicClient,
	}

	clusterVersion, err := version.Detect(ctx, c)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(clusterVersion).To(HaveField("Version", "2.15.0"))
	g.Expect(clusterVersion).To(HaveField("Source", version.SourceOLM))
	g.Expect(clusterVersion).To(HaveField("Confidence", version.ConfidenceMedium))
	g.Expect(clusterVersion).To(HaveField("Branch", "stable-2.15"))
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

	c := &client.Client{
		Dynamic: dynamicClient,
	}

	clusterVersion, err := version.Detect(ctx, c)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(clusterVersion).To(HaveField("Version", "2.17.0"))
	g.Expect(clusterVersion).To(HaveField("Source", version.SourceDataScienceCluster))
}

func TestDetect_NoVersionFound(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	// Empty cluster - no version sources
	scheme := runtime.NewScheme()
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, listKinds)

	c := &client.Client{
		Dynamic: dynamicClient,
	}

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
