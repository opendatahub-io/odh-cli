package olm_test

import (
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/kube/olm"

	. "github.com/onsi/gomega"
)

const (
	testCatalogChannels = `{"schema":"olm.package","name":"kueue-operator","defaultChannel":"stable-v1.1"}
{"schema":"olm.channel","package":"another-operator","name":"stable-v9.9"}
{"schema":"olm.channel","package":"kueue-operator","name":"stable-v1.1"}
{"schema":"olm.channel","package":"kueue-operator","name":"stable-v1.3"}
{"schema":"olm.channel","package":"kueue-operator","name":"candidate"}
`
	testCatalogDefault = `{"schema":"olm.package","name":"kueue-operator","defaultChannel":"stable"}
{"schema":"olm.channel","package":"kueue-operator","name":"stable"}
{"schema":"olm.channel","package":"kueue-operator","name":"candidate"}
`
	testCatalogUnavailableDefault = `{"schema":"olm.package","name":"kueue-operator","defaultChannel":"stable"}
{"schema":"olm.channel","package":"kueue-operator","name":"candidate"}
`
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

func TestResolveCatalogChannel(t *testing.T) {
	t.Run("highest stable channel", func(t *testing.T) {
		g := NewWithT(t)
		channel, err := olm.ResolveCatalogChannel(strings.NewReader(testCatalogChannels), "kueue-operator")
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(channel).To(Equal("stable-v1.3"))
	})

	t.Run("package default without versioned stable channel", func(t *testing.T) {
		g := NewWithT(t)
		channel, err := olm.ResolveCatalogChannel(strings.NewReader(testCatalogDefault), "kueue-operator")
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(channel).To(Equal("stable"))
	})

	t.Run("missing package fails", func(t *testing.T) {
		g := NewWithT(t)
		_, err := olm.ResolveCatalogChannel(strings.NewReader(testCatalogChannels), "missing-operator")
		g.Expect(err).To(HaveOccurred())
	})

	t.Run("unavailable default channel fails", func(t *testing.T) {
		g := NewWithT(t)
		_, err := olm.ResolveCatalogChannel(strings.NewReader(testCatalogUnavailableDefault), "kueue-operator")
		g.Expect(err).To(HaveOccurred())
	})
}
