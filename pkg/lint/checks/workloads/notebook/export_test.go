package notebook

import "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

// NewTestNotebook creates a minimal unstructured Notebook for tests.
func NewTestNotebook(containers []any) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "kubeflow.org/v1",
			"kind":       "Notebook",
			"metadata": map[string]any{
				"name":      "test-nb",
				"namespace": "test-ns",
			},
			"spec": map[string]any{
				"template": map[string]any{
					"spec": map[string]any{
						"containers": containers,
					},
				},
			},
		},
	}
}
