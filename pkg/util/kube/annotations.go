package kube

import "sigs.k8s.io/controller-runtime/pkg/client"

const (
	// AnnotationManaged is the annotation key indicating operator management status.
	AnnotationManaged = "opendatahub.io/managed"

	// managedFalseValue is the value that indicates a resource is not managed.
	managedFalseValue = "false"
)

// IsManaged checks if a Kubernetes object is managed by the operator.
// Returns false only if the opendatahub.io/managed annotation exists and equals "false".
// Returns true for all other cases (missing annotation, empty value, or any other value).
func IsManaged(obj client.Object) bool {
	annotations := obj.GetAnnotations()
	if annotations == nil {
		return true
	}

	return annotations[AnnotationManaged] != managedFalseValue
}
