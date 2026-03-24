package discovery

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/opendatahub-io/odh-cli/pkg/constants"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
)

// MonitoredWorkloadTypes lists the top-level CR types monitored for kueue label consistency.
// These are the only resource types that appear in ImpactedObjects.
//
//nolint:gochecknoglobals // Static configuration for monitored workload types.
var MonitoredWorkloadTypes = []resources.ResourceType{
	resources.Notebook,
	resources.InferenceService,
	resources.LLMInferenceService,
	resources.RayCluster,
	resources.RayJob,
	resources.PyTorchJob,
}

// KueueEnabledNamespaces returns the set of namespaces that have a kueue-managed label.
// Uses two ListMetadata calls with label selectors for server-side filtering,
// giving a fixed cost regardless of how many namespaces exist.
func KueueEnabledNamespaces(
	ctx context.Context,
	r client.Reader,
) (sets.Set[string], error) {
	enabled := sets.New[string]()

	for _, selector := range []string{
		constants.LabelKueueManaged + "=true",
		constants.LabelKueueOpenshiftManaged + "=true",
	} {
		items, err := r.ListMetadata(ctx, resources.Namespace,
			client.WithLabelSelector(selector))
		if err != nil {
			return nil, fmt.Errorf("listing kueue-enabled namespaces: %w", err)
		}

		for _, ns := range items {
			enabled.Insert(ns.GetName())
		}
	}

	return enabled, nil
}

// WorkloadLabeledNamespaces returns the set of namespaces that contain at least one
// monitored workload with the kueue.x-k8s.io/queue-name label.
func WorkloadLabeledNamespaces(
	ctx context.Context,
	r client.Reader,
) (sets.Set[string], error) {
	namespaces := sets.New[string]()
	selector := constants.LabelKueueQueueName

	for _, rt := range MonitoredWorkloadTypes {
		items, err := r.ListMetadata(ctx, rt, client.WithLabelSelector(selector))
		if err != nil {
			if client.IsResourceTypeNotFound(err) {
				continue
			}

			return nil, fmt.Errorf("listing %s with kueue label: %w", rt.Kind, err)
		}

		for _, item := range items {
			namespaces.Insert(item.GetNamespace())
		}
	}

	return namespaces, nil
}
