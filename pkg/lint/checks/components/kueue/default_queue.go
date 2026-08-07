package kueue

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"

	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
)

// defaultLocalQueueName is the LocalQueue name Kueue implicitly assigns to workloads that live
// in a kueue-managed namespace but are not explicitly bound to a queue. When Kueue is Unmanaged,
// reusing this name for an unrelated queue can silently route workloads to the wrong LocalQueue.
const defaultLocalQueueName = "default"

// detectDefaultLocalQueue reports whether a potentially dangerous "default" LocalQueue
// configuration exists. It triggers when either:
//   - DSC spec.components.kueue.defaultLocalQueueName is set to "default", or
//   - a LocalQueue resource named "default" exists in any namespace.
//
// triggers holds human-readable descriptions of each condition that fired (for the warning
// message); a non-empty triggers slice means the hazard was detected. impacted lists the
// LocalQueue objects named "default" (for ImpactedObjects). A cluster without the LocalQueue CRD
// is treated as "no LocalQueues" rather than an error.
func detectDefaultLocalQueue(
	ctx context.Context,
	r client.Reader,
	dsc *unstructured.Unstructured,
) ([]string, []types.NamespacedName, error) {
	var (
		triggers []string
		impacted []types.NamespacedName
	)

	if name, found, nErr := unstructured.NestedString(
		dsc.Object, "spec", "components", "kueue", "defaultLocalQueueName",
	); nErr == nil && found && name == defaultLocalQueueName {
		triggers = append(triggers,
			fmt.Sprintf("DSC spec.components.kueue.defaultLocalQueueName is set to %q", defaultLocalQueueName))
	}

	items, err := r.ListMetadata(ctx, resources.LocalQueue)
	switch {
	case client.IsResourceTypeNotFound(err):
		// LocalQueue CRD is not installed — nothing to inspect.
	case err != nil:
		return nil, nil, fmt.Errorf("listing LocalQueues: %w", err)
	default:
		for _, item := range items {
			if item.GetName() == defaultLocalQueueName {
				impacted = append(impacted, types.NamespacedName{
					Namespace: item.GetNamespace(),
					Name:      item.GetName(),
				})
			}
		}
	}

	if len(impacted) > 0 {
		triggers = append(triggers,
			fmt.Sprintf("a LocalQueue named %q exists in the cluster", defaultLocalQueueName))
	}

	return triggers, impacted, nil
}
