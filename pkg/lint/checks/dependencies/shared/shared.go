package shared

import (
	"context"
	"slices"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"

	"github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
)

// RHOAIManagedNamespaces returns the set of namespaces considered RHOAI-managed.
// It combines the provided well-known namespaces with the applications and monitoring
// namespaces read from DSCInitialization.
func RHOAIManagedNamespaces(ctx context.Context, r client.Reader, wellKnown []string) []string {
	namespaces := make([]string, 0, len(wellKnown)+2) //nolint:mnd // room for applications + monitoring
	namespaces = append(namespaces, wellKnown...)

	dsciNS, err := client.GetDSCINamespaces(ctx, r)
	if err == nil {
		if dsciNS.Applications != "" {
			namespaces = append(namespaces, dsciNS.Applications)
		}

		if dsciNS.Monitoring != "" {
			namespaces = append(namespaces, dsciNS.Monitoring)
		}
	}

	return namespaces
}

// CollectNamespaces returns a sorted, deduplicated list of namespaces from the given resource slices.
func CollectNamespaces(resourceSlices ...[]*unstructured.Unstructured) []string {
	seen := make(map[string]struct{})

	for _, items := range resourceSlices {
		for _, item := range items {
			seen[item.GetNamespace()] = struct{}{}
		}
	}

	namespaces := make([]string, 0, len(seen))
	for ns := range seen {
		namespaces = append(namespaces, ns)
	}

	slices.Sort(namespaces)

	return namespaces
}

// ToNamespacedNames converts unstructured resources to NamespacedName references.
func ToNamespacedNames(items []*unstructured.Unstructured) []types.NamespacedName {
	names := make([]types.NamespacedName, 0, len(items))
	for _, item := range items {
		names = append(names, types.NamespacedName{
			Namespace: item.GetNamespace(),
			Name:      item.GetName(),
		})
	}

	return names
}

// AddAllImpactedObjects appends impacted objects for each resource type that has results.
func AddAllImpactedObjects(dr *result.DiagnosticResult, entries ...ImpactedEntry) {
	for _, e := range entries {
		if len(e.Items) > 0 {
			dr.AddImpactedObjects(e.ResourceType, ToNamespacedNames(e.Items))
		}
	}
}

// ImpactedEntry pairs a resource type with its discovered items.
type ImpactedEntry struct {
	ResourceType resources.ResourceType
	Items        []*unstructured.Unstructured
}

// IsNonRHOAIFilter returns a filter function that keeps only resources outside the given namespaces.
func IsNonRHOAIFilter(managedNamespaces []string) func(*unstructured.Unstructured) (bool, error) {
	return func(item *unstructured.Unstructured) (bool, error) {
		return !slices.Contains(managedNamespaces, item.GetNamespace()), nil
	}
}
