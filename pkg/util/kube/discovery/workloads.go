package discovery

import (
	"context"
	"fmt"

	"github.com/opendatahub-io/odh-cli/pkg/util/client"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

// DiscoverWorkloads discovers user-created workload resources via CRD labels
// Workloads are CRDs labeled with "platform.opendatahub.io/part-of".
func DiscoverWorkloads(ctx context.Context, c client.Client) ([]schema.GroupVersionResource, error) {
	// Use client.DiscoverGVRs with the default label selector
	// This will find all CRDs labeled with "platform.opendatahub.io/part-of"
	gvrs, err := client.DiscoverGVRs(ctx, c)
	if err != nil {
		return nil, fmt.Errorf("discovering workload CRDs: %w", err)
	}

	return gvrs, nil
}

// DiscoverWorkloadsWithLabel discovers workload resources with a custom label selector.
func DiscoverWorkloadsWithLabel(ctx context.Context, c client.Client, labelSelector string) ([]schema.GroupVersionResource, error) {
	gvrs, err := client.DiscoverGVRs(ctx, c, client.WithCRDLabelSelector(labelSelector))
	if err != nil {
		return nil, fmt.Errorf("discovering workload CRDs with label %q: %w", labelSelector, err)
	}

	return gvrs, nil
}
