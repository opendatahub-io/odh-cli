package olm

import (
	"errors"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/discovery"
)

const (
	olmV1GroupVersion = "olm.operatorframework.io/v1"
	olmV1Resource     = "clusterextensions"
	olmV0GroupVersion = "operators.coreos.com/v1alpha1"
	olmV0Resource     = "subscriptions"
)

// ErrUnavailable indicates that neither the OLM v1 nor OLM v0 installation API is available.
var ErrUnavailable = errors.New("OLM installation API is not available")

// Mode identifies the OLM API used for installation resources.
type Mode string

const (
	ModeV1 Mode = "v1"
	ModeV0 Mode = "v0"
)

// DetectMode prefers OLM v1 when ClusterExtension is served and falls back to OLM v0.
func DetectMode(discoveryClient discovery.DiscoveryInterface) (Mode, error) {
	if discoveryClient == nil {
		return "", ErrUnavailable
	}

	available, err := resourceAvailable(discoveryClient, olmV1GroupVersion, olmV1Resource)
	if err != nil {
		return "", fmt.Errorf("discovering OLM v1: %w", err)
	}

	if available {
		return ModeV1, nil
	}

	available, err = resourceAvailable(discoveryClient, olmV0GroupVersion, olmV0Resource)
	if err != nil {
		return "", fmt.Errorf("discovering OLM v0: %w", err)
	}

	if available {
		return ModeV0, nil
	}

	return "", ErrUnavailable
}

// ResolveInstallMode defaults to OLM v0 when both APIs are served.
// A non-empty requested mode must be available; it never silently falls back.
func ResolveInstallMode(discoveryClient discovery.DiscoveryInterface, requested Mode) (Mode, error) {
	if discoveryClient == nil {
		return "", ErrUnavailable
	}

	if requested == ModeV1 {
		available, err := resourceAvailable(discoveryClient, olmV1GroupVersion, olmV1Resource)
		if err != nil {
			return "", err
		}
		if !available {
			return "", fmt.Errorf("requested OLM %s API is not available: %w", requested, ErrUnavailable)
		}

		return ModeV1, nil
	}

	available, err := resourceAvailable(discoveryClient, olmV0GroupVersion, olmV0Resource)
	if err != nil {
		return "", err
	}
	if available {
		return ModeV0, nil
	}
	if requested == ModeV0 {
		return "", fmt.Errorf("requested OLM %s API is not available: %w", requested, ErrUnavailable)
	}

	available, err = resourceAvailable(discoveryClient, olmV1GroupVersion, olmV1Resource)
	if err != nil {
		return "", err
	}
	if available {
		return ModeV1, nil
	}

	return "", ErrUnavailable
}

func resourceAvailable(discoveryClient discovery.DiscoveryInterface, groupVersion string, resource string) (bool, error) {
	resources, err := discoveryClient.ServerResourcesForGroupVersion(groupVersion)
	if resources != nil {
		for _, apiResource := range resources.APIResources {
			if apiResource.Name == resource {
				return true, nil
			}
		}
	}

	if err == nil || apierrors.IsNotFound(err) || meta.IsNoMatchError(err) {
		return false, nil
	}

	return false, fmt.Errorf("discovering resource %s in %s: %w", resource, groupVersion, err)
}
