package kube

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
)

// ToNamespacedNames converts objects with metadata to a slice of NamespacedName.
func ToNamespacedNames[T NamespacedNamer](items []T) []types.NamespacedName {
	result := make([]types.NamespacedName, 0, len(items))

	for _, item := range items {
		result = append(result, types.NamespacedName{
			Namespace: item.GetNamespace(),
			Name:      item.GetName(),
		})
	}

	return result
}

// BuildResourceNameSet lists all instances of a resource type and returns their
// namespace/name pairs as a set. Returns an empty set (not an error) when the
// CRD is not registered in the cluster.
func BuildResourceNameSet(
	ctx context.Context,
	c client.Reader,
	resourceType resources.ResourceType,
) (sets.Set[types.NamespacedName], error) {
	items, err := c.ListMetadata(ctx, resourceType)
	if err != nil {
		if client.IsResourceTypeNotFound(err) {
			return sets.New[types.NamespacedName](), nil
		}

		return nil, fmt.Errorf("listing %s: %w", resourceType.Kind, err)
	}

	result := sets.New[types.NamespacedName]()

	for _, item := range items {
		result.Insert(types.NamespacedName{
			Namespace: item.GetNamespace(),
			Name:      item.GetName(),
		})
	}

	return result, nil
}
