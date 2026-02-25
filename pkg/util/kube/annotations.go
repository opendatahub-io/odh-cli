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

// GetAnnotation returns the value of an annotation on a Kubernetes object.
// Returns empty string if the annotation doesn't exist or annotations map is nil.
func GetAnnotation(obj client.Object, key string) string {
	annotations := obj.GetAnnotations()
	if annotations == nil {
		return ""
	}

	return annotations[key]
}

// HasAnnotation checks if a Kubernetes object has a specific annotation with the given key
// and that its value matches the expected value.
func HasAnnotation(obj client.Object, key string, value string) bool {
	annotations := obj.GetAnnotations()
	if annotations == nil {
		return false
	}

	v, ok := annotations[key]

	return ok && v == value
}

// ContainsAnnotation checks if a Kubernetes object has an annotation with the given key,
// regardless of its value (including empty string).
func ContainsAnnotation(obj client.Object, key string) bool {
	annotations := obj.GetAnnotations()
	if annotations == nil {
		return false
	}

	_, ok := annotations[key]

	return ok
}
