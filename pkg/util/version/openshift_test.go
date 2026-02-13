package version_test

import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
	"github.com/opendatahub-io/odh-cli/pkg/util/version"

	. "github.com/onsi/gomega"
)

func TestDetectOpenShiftVersion_FromDesiredVersion(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	cv := &unstructured.Unstructured{}
	cv.SetAPIVersion("config.openshift.io/v1")
	cv.SetKind("ClusterVersion")
	cv.SetName("version")
	_ = unstructured.SetNestedField(cv.Object, "4.19.1", "status", "desired", "version")

	scheme := runtime.NewScheme()
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		scheme,
		map[schema.GroupVersionResource]string{
			resources.ClusterVersion.GVR(): "ClusterVersionList",
		},
		cv,
	)

	c := client.NewForTesting(client.TestClientConfig{Dynamic: dynamicClient})

	ver, err := version.DetectOpenShiftVersion(ctx, c)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(ver).ToNot(BeNil())
	g.Expect(ver.String()).To(Equal("4.19.1"))
}

func TestDetectOpenShiftVersion_FromHistory(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	cv := &unstructured.Unstructured{}
	cv.SetAPIVersion("config.openshift.io/v1")
	cv.SetKind("ClusterVersion")
	cv.SetName("version")

	history := []any{
		map[string]any{
			"version": "4.20.0",
			"state":   "Completed",
		},
	}
	_ = unstructured.SetNestedSlice(cv.Object, history, "status", "history")

	scheme := runtime.NewScheme()
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		scheme,
		map[schema.GroupVersionResource]string{
			resources.ClusterVersion.GVR(): "ClusterVersionList",
		},
		cv,
	)

	c := client.NewForTesting(client.TestClientConfig{Dynamic: dynamicClient})

	ver, err := version.DetectOpenShiftVersion(ctx, c)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(ver).ToNot(BeNil())
	g.Expect(ver.String()).To(Equal("4.20.0"))
}

func TestDetectOpenShiftVersion_NotFound(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	scheme := runtime.NewScheme()
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, nil)

	c := client.NewForTesting(client.TestClientConfig{Dynamic: dynamicClient})

	ver, err := version.DetectOpenShiftVersion(ctx, c)

	g.Expect(err).To(HaveOccurred())
	g.Expect(ver).To(BeNil())
	g.Expect(err.Error()).To(ContainSubstring("failed to get ClusterVersion"))
}

func TestDetectOpenShiftVersion_NilClient(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	ver, err := version.DetectOpenShiftVersion(ctx, nil)

	g.Expect(err).To(HaveOccurred())
	g.Expect(ver).To(BeNil())
	g.Expect(err.Error()).To(ContainSubstring("kubernetes client not available"))
}

func TestDetectOpenShiftVersion_NoVersionData(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	cv := &unstructured.Unstructured{}
	cv.SetAPIVersion("config.openshift.io/v1")
	cv.SetKind("ClusterVersion")
	cv.SetName("version")

	scheme := runtime.NewScheme()
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		scheme,
		map[schema.GroupVersionResource]string{
			resources.ClusterVersion.GVR(): "ClusterVersionList",
		},
		cv,
	)

	c := client.NewForTesting(client.TestClientConfig{Dynamic: dynamicClient})

	ver, err := version.DetectOpenShiftVersion(ctx, c)

	g.Expect(err).To(HaveOccurred())
	g.Expect(ver).To(BeNil())
	g.Expect(err.Error()).To(ContainSubstring("unable to determine OpenShift version"))
}
