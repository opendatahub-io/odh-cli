package discovery

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
)

// GetGroupResources returns all API resources for a given group.
func GetGroupResources(
	discoveryClient discovery.DiscoveryInterface,
	groupName string,
) ([]metav1.APIResource, error) {
	_, apiResourceLists, err := discoveryClient.ServerGroupsAndResources()
	if err != nil {
		return nil, fmt.Errorf("failed to discover server resources: %w", err)
	}

	var resources []metav1.APIResource

	for _, apiResourceList := range apiResourceLists {
		for _, resource := range apiResourceList.APIResources {
			if resource.Group == groupName {
				resources = append(resources, resource)
			}
		}
	}

	// Empty list is valid - means no resources of this type exist
	return resources, nil
}

// GetGroupVersionResources returns all API resources for a specific group and version.
func GetGroupVersionResources(
	discoveryClient discovery.DiscoveryInterface,
	groupVersion schema.GroupVersion,
) ([]metav1.APIResource, error) {
	// Use ServerGroupsAndResources which is more robust for CRDs
	// Note: This can return partial errors but still provide results
	_, apiResourceLists, _ := discoveryClient.ServerGroupsAndResources()

	if apiResourceLists == nil {
		return []metav1.APIResource{}, nil
	}

	// Find the matching group/version
	for _, apiResourceList := range apiResourceLists {
		if apiResourceList.GroupVersion == groupVersion.String() {
			return apiResourceList.APIResources, nil
		}
	}

	// Empty list is valid - means no resources of this type exist
	return []metav1.APIResource{}, nil
}
