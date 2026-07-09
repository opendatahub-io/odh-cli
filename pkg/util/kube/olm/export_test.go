package olm

import "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

// ExportResolveChannelFromManifest exposes resolveChannelFromManifest for external tests.
func ExportResolveChannelFromManifest(pm *unstructured.Unstructured) (string, error) {
	return resolveChannelFromManifest(pm)
}
