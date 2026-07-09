package olm_test

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/kube/olm"

	. "github.com/onsi/gomega"
)

func newKueuePackageManifest(channels []string, defaultChannel string) *unstructured.Unstructured {
	channelObjs := make([]any, 0, len(channels))
	for _, ch := range channels {
		channelObjs = append(channelObjs, map[string]any{
			"name": ch,
			"entries": []any{
				map[string]any{"name": "kueue-operator.v1.0.0"},
			},
		})
	}

	status := map[string]any{
		"catalogSource": "redhat-operators",
		"channels":      channelObjs,
	}
	if defaultChannel != "" {
		status["defaultChannel"] = defaultChannel
	}

	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": resources.PackageManifest.APIVersion(),
		"kind":       resources.PackageManifest.Kind,
		"metadata": map[string]any{
			"name":      "kueue-operator",
			"namespace": "openshift-marketplace",
		},
		"status": status,
	}}
}

func TestResolveChannelFromManifest_HighestStableV(t *testing.T) {
	g := NewWithT(t)

	pm := newKueuePackageManifest([]string{"stable-v1.0", "stable-v1.2", "stable-v1.1", "candidate"}, "")

	channel, err := olm.ExportResolveChannelFromManifest(pm)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(channel).To(Equal("stable-v1.2"))
}

func TestResolveChannelFromManifest_FallbackDefaultChannel(t *testing.T) {
	g := NewWithT(t)

	pm := newKueuePackageManifest([]string{"candidate"}, "stable")

	channel, err := olm.ExportResolveChannelFromManifest(pm)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(channel).To(Equal("stable"))
}

func TestResolveChannelFromManifest_NoChannels(t *testing.T) {
	g := NewWithT(t)

	pm := newKueuePackageManifest(nil, "")

	_, err := olm.ExportResolveChannelFromManifest(pm)
	g.Expect(err).To(HaveOccurred())
}
