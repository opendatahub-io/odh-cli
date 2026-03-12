package kueue

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/opendatahub-io/odh-cli/pkg/constants"
)

// violation describes a single consistency failure for a top-level CR.
type violation struct {
	// Resource identifies the top-level CR that is in violation.
	Resource types.NamespacedName

	// Kind is the Kubernetes Kind of the top-level CR (e.g. "Notebook").
	Kind string

	// APIVersion is the API version of the top-level CR (e.g. "kubeflow.org/v1").
	APIVersion string

	// Message is a detailed, human-readable description of the violation.
	// Rendered as per-object context in verbose output via AnnotationObjectContext.
	Message string
}

// checkNamespaceToWorkload checks invariant 1: a CR in a kueue-managed namespace
// must have the kueue.x-k8s.io/queue-name label.
// Returns a violation if the CR is in a kueue-managed namespace but missing the label.
func checkNamespaceToWorkload(
	cr *metav1.PartialObjectMetadata,
	kueueNamespaces sets.Set[string],
) *violation {
	if !kueueNamespaces.Has(cr.GetNamespace()) {
		return nil
	}

	if _, ok := cr.GetLabels()[constants.LabelKueueQueueName]; ok {
		return nil
	}

	return &violation{
		Resource: types.NamespacedName{
			Namespace: cr.GetNamespace(),
			Name:      cr.GetName(),
		},
		Kind:       cr.Kind,
		APIVersion: cr.APIVersion,
		Message: fmt.Sprintf(
			msgInvariant1, cr.Kind, cr.GetNamespace(), cr.GetName(), cr.GetNamespace(),
		),
	}
}

// checkWorkloadToNamespace checks invariant 2: a CR with the kueue.x-k8s.io/queue-name
// label must reside in a kueue-managed namespace.
// Returns a violation if the CR has the label but its namespace is not kueue-managed.
func checkWorkloadToNamespace(
	cr *metav1.PartialObjectMetadata,
	kueueNamespaces sets.Set[string],
) *violation {
	queueName, ok := cr.GetLabels()[constants.LabelKueueQueueName]
	if !ok {
		return nil
	}

	if kueueNamespaces.Has(cr.GetNamespace()) {
		return nil
	}

	return &violation{
		Resource: types.NamespacedName{
			Namespace: cr.GetNamespace(),
			Name:      cr.GetName(),
		},
		Kind:       cr.Kind,
		APIVersion: cr.APIVersion,
		Message: fmt.Sprintf(
			msgInvariant2, cr.Kind, cr.GetNamespace(), cr.GetName(), queueName,
		),
	}
}

// checkOwnerTreeConsistency checks invariant 3: within the ownership tree of a
// top-level CR, all resources must agree on the kueue queue-name label.
// Either every resource has the label with the same value, or none have it.
// Returns a violation on the first disagreement found (first-violation-wins).
func checkOwnerTreeConsistency(
	cr *metav1.PartialObjectMetadata,
	graph *ownershipGraph,
) *violation {
	descendants := graph.walkSubtree(cr.GetUID())
	if len(descendants) == 0 {
		// Single-node tree is trivially consistent.
		return nil
	}

	// Collect the label state from the root CR.
	rootValue, rootHas := cr.GetLabels()[constants.LabelKueueQueueName]

	// Check each descendant for agreement with the root.
	for i := range descendants {
		childValue, childHas := descendants[i].Labels[constants.LabelKueueQueueName]

		switch {
		case rootHas && !childHas:
			return &violation{
				Resource: types.NamespacedName{
					Namespace: cr.GetNamespace(),
					Name:      cr.GetName(),
				},
				Kind:       cr.Kind,
				APIVersion: cr.APIVersion,
				Message: fmt.Sprintf(
					msgInvariant3Missing,
					cr.Kind, cr.GetNamespace(), cr.GetName(), rootValue,
					descendants[i].Kind, descendants[i].Namespace, descendants[i].Name,
				),
			}
		case !rootHas && childHas:
			return &violation{
				Resource: types.NamespacedName{
					Namespace: cr.GetNamespace(),
					Name:      cr.GetName(),
				},
				Kind:       cr.Kind,
				APIVersion: cr.APIVersion,
				Message: fmt.Sprintf(
					msgInvariant3Unexpected,
					descendants[i].Kind, descendants[i].Namespace, descendants[i].Name, childValue,
					cr.Kind, cr.GetNamespace(), cr.GetName(),
				),
			}
		case rootHas && childHas && rootValue != childValue:
			return &violation{
				Resource: types.NamespacedName{
					Namespace: cr.GetNamespace(),
					Name:      cr.GetName(),
				},
				Kind:       cr.Kind,
				APIVersion: cr.APIVersion,
				Message: fmt.Sprintf(
					msgInvariant3Mismatch,
					descendants[i].Kind, descendants[i].Namespace, descendants[i].Name, childValue,
					cr.Kind, cr.GetNamespace(), cr.GetName(), rootValue,
				),
			}
		}
	}

	return nil
}
