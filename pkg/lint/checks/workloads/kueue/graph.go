package kueue

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/opendatahub-io/odh-cli/pkg/util/client"
)

// graphNode represents a single resource in the ownership graph,
// holding only the metadata needed for label consistency checks.
type graphNode struct {
	UID       types.UID
	Name      string
	Namespace string
	Kind      string
	Labels    map[string]string
}

// ownershipGraph maps parent UIDs to their direct children.
// Built once per namespace and reused for all top-level CRs in that namespace.
type ownershipGraph struct {
	children map[types.UID][]graphNode
}

// buildGraph lists all intermediate resource types in the given namespace
// and builds a parent→children map keyed by ownerReference UID.
func buildGraph(
	ctx context.Context,
	r client.Reader,
	namespace string,
) (*ownershipGraph, error) {
	graph := &ownershipGraph{
		children: make(map[types.UID][]graphNode),
	}

	for _, rt := range intermediateTypes {
		items, err := r.ListMetadata(ctx, rt, client.WithNamespace(namespace))
		if err != nil {
			// A missing CRD means the resource type is not installed on this cluster,
			// so there are zero instances. Ideally ListMetadata would handle this
			// the same way it handles permission errors (return empty list).
			if client.IsResourceTypeNotFound(err) {
				continue
			}

			return nil, fmt.Errorf("listing %s in namespace %s: %w", rt.Kind, namespace, err)
		}

		// ListMetadata returns PartialObjectMetadata whose Kind is
		// "PartialObjectMetadata" rather than the real resource kind.
		// Pass the real kind from the resource type when building nodes
		// to avoid mutating pointers owned by the caller.
		for _, item := range items {
			node := newGraphNode(item, rt.Kind)

			for _, ref := range item.GetOwnerReferences() {
				graph.children[ref.UID] = append(graph.children[ref.UID], node)
			}
		}
	}

	return graph, nil
}

// walkSubtree collects all descendants of the given root UID recursively.
// Returns an empty slice if the root has no children (single-node tree).
// Uses a visited set to guard against owner-reference cycles that could
// arise from stale cached data or buggy controllers.
func (g *ownershipGraph) walkSubtree(rootUID types.UID) []graphNode {
	var result []graphNode

	visited := make(map[types.UID]struct{})
	queue := []types.UID{rootUID}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		if _, seen := visited[current]; seen {
			continue
		}

		visited[current] = struct{}{}

		for _, child := range g.children[current] {
			result = append(result, child)
			queue = append(queue, child.UID)
		}
	}

	return result
}

func newGraphNode(item *metav1.PartialObjectMetadata, kind string) graphNode {
	return graphNode{
		UID:       item.GetUID(),
		Name:      item.GetName(),
		Namespace: item.GetNamespace(),
		Kind:      kind,
		Labels:    item.GetLabels(),
	}
}
