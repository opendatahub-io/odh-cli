package olm_test

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	discoveryfake "k8s.io/client-go/discovery/fake"
	coretesting "k8s.io/client-go/testing"

	"github.com/opendatahub-io/odh-cli/pkg/util/kube/olm"

	. "github.com/onsi/gomega"
)

func TestDetectMode(t *testing.T) {
	tests := []struct {
		name      string
		resources []*metav1.APIResourceList
		want      olm.Mode
		wantErr   error
	}{
		{
			name: "prefers OLM v1 when both APIs are served",
			resources: []*metav1.APIResourceList{
				olmV0Resources(),
				olmV1Resources(),
			},
			want: olm.ModeV1,
		},
		{
			name:      "falls back to OLM v0",
			resources: []*metav1.APIResourceList{olmV0Resources()},
			want:      olm.ModeV0,
		},
		{
			name:    "reports unavailable when neither API is served",
			wantErr: olm.ErrUnavailable,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			fakeDiscovery := &discoveryfake.FakeDiscovery{Fake: &coretesting.Fake{}}
			fakeDiscovery.Resources = tt.resources

			mode, err := olm.DetectMode(fakeDiscovery)
			if tt.wantErr != nil {
				g.Expect(err).To(MatchError(tt.wantErr))

				return
			}

			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(mode).To(Equal(tt.want))
		})
	}
}

func TestResolveInstallMode(t *testing.T) {
	tests := []struct {
		name      string
		resources []*metav1.APIResourceList
		requested olm.Mode
		want      olm.Mode
		wantErr   error
	}{
		{name: "mixed cluster defaults to OLM v0", resources: []*metav1.APIResourceList{olmV0Resources(), olmV1Resources()}, want: olm.ModeV0},
		{name: "mixed cluster selects v1 explicitly", resources: []*metav1.APIResourceList{olmV0Resources(), olmV1Resources()}, requested: olm.ModeV1, want: olm.ModeV1},
		{name: "v1-only cluster uses v1", resources: []*metav1.APIResourceList{olmV1Resources()}, want: olm.ModeV1},
		{name: "explicit v1 cannot fall back", resources: []*metav1.APIResourceList{olmV0Resources()}, requested: olm.ModeV1, wantErr: olm.ErrUnavailable},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			fakeDiscovery := &discoveryfake.FakeDiscovery{Fake: &coretesting.Fake{}}
			fakeDiscovery.Resources = tt.resources

			mode, err := olm.ResolveInstallMode(fakeDiscovery, tt.requested)
			if tt.wantErr != nil {
				g.Expect(err).To(HaveOccurred())
				g.Expect(err).To(MatchError(ContainSubstring(tt.wantErr.Error())))

				return
			}

			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(mode).To(Equal(tt.want))
		})
	}
}

func olmV0Resources() *metav1.APIResourceList {
	return &metav1.APIResourceList{
		GroupVersion: "operators.coreos.com/v1alpha1",
		APIResources: []metav1.APIResource{{Name: "subscriptions", Kind: "Subscription"}},
	}
}

func olmV1Resources() *metav1.APIResourceList {
	return &metav1.APIResourceList{
		GroupVersion: "olm.operatorframework.io/v1",
		APIResources: []metav1.APIResource{{Name: "clusterextensions", Kind: "ClusterExtension"}},
	}
}
