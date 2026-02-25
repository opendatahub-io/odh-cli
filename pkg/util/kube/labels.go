package kube

import "sigs.k8s.io/controller-runtime/pkg/client"

// GetLabel returns the value of a label on a Kubernetes object.
// Returns empty string if the label doesn't exist or labels map is nil.
func GetLabel(obj client.Object, key string) string {
	labels := obj.GetLabels()
	if labels == nil {
		return ""
	}

	return labels[key]
}

// HasLabel checks if a Kubernetes object has a specific label with the given key
// and that its value matches the expected value.
func HasLabel(obj client.Object, key string, value string) bool {
	labels := obj.GetLabels()
	if labels == nil {
		return false
	}

	v, ok := labels[key]

	return ok && v == value
}

// ContainsLabel checks if a Kubernetes object has a label with the given key,
// regardless of its value (including empty string).
func ContainsLabel(obj client.Object, key string) bool {
	labels := obj.GetLabels()
	if labels == nil {
		return false
	}

	_, ok := labels[key]

	return ok
}
