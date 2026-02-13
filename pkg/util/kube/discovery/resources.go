package discovery

import (
	"context"
	"fmt"
	"strings"

	"github.com/opendatahub-io/odh-cli/pkg/util/client"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// ComponentAndService represents a discovered OpenShift AI component or service.
type ComponentAndService struct {
	// APIGroup is the Kubernetes API group (e.g., "dashboard.opendatahub.io")
	APIGroup string

	// Version is the API version (e.g., "v1")
	Version string

	// Resources are the resources available in this API group version
	Resources []metav1.APIResource
}

// DiscoverComponentsAndServices discovers OpenShift AI components and services by API groups
// Components and services are identified by API groups matching OpenShift AI patterns.
func DiscoverComponentsAndServices(_ context.Context, c client.Client) ([]ComponentAndService, error) {
	// Get all API groups
	apiGroupList, err := c.Discovery().ServerGroups()
	if err != nil {
		return nil, fmt.Errorf("listing API groups: %w", err)
	}

	discovered := make([]ComponentAndService, 0, len(apiGroupList.Groups))

	// Filter for OpenShift AI related groups
	for _, apiGroup := range apiGroupList.Groups {
		// Check if this is an OpenShift AI or related group
		if !isOpenShiftAIGroup(apiGroup.Name) {
			continue
		}

		// Get the preferred version or latest version
		var version string
		if apiGroup.PreferredVersion.Version != "" {
			version = apiGroup.PreferredVersion.Version
		} else if len(apiGroup.Versions) > 0 {
			version = apiGroup.Versions[0].Version
		} else {
			continue
		}

		// Get resources for this API group version
		resourceList, err := c.Discovery().ServerResourcesForGroupVersion(
			schema.GroupVersion{Group: apiGroup.Name, Version: version}.String(),
		)
		if err != nil {
			// Skip groups we can't access (may be forbidden or not fully installed)
			continue
		}

		discovered = append(discovered, ComponentAndService{
			APIGroup:  apiGroup.Name,
			Version:   version,
			Resources: resourceList.APIResources,
		})
	}

	return discovered, nil
}

// isOpenShiftAIGroup determines if an API group belongs to OpenShift AI.
func isOpenShiftAIGroup(group string) bool {
	// OpenShift AI groups
	odhPrefixes := []string{
		"opendatahub.io",
		"datasciencecluster.opendatahub.io",
		"dscinitialization.opendatahub.io",
		"dashboard.opendatahub.io",
		"notebook.opendatahub.io",
		"features.opendatahub.io",
	}

	// Red Hat OpenShift AI groups
	rhaiPrefixes := []string{
		"datasciencecluster.openshift.io",
		"dscinitialization.openshift.io",
	}

	// Kubeflow groups (when using ODH with Kubeflow components)
	kubeflowPrefixes := []string{
		"kubeflow.org",
		"serving.kserve.io",
		"serving.knative.dev",
	}

	allPrefixes := append(append(odhPrefixes, rhaiPrefixes...), kubeflowPrefixes...)

	for _, prefix := range allPrefixes {
		if strings.HasSuffix(group, prefix) || group == prefix {
			return true
		}
	}

	return false
}
