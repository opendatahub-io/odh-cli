package notebook_test

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/opendatahub-io/odh-cli/pkg/resources"
)

// notebookOptions holds optional metadata and spec fields for test notebook fixtures.
type notebookOptions struct {
	Labels      map[string]any
	Annotations map[string]any
	Containers  []any
}

// newNotebook creates a minimal Notebook unstructured object for testing.
// Use notebookOptions{} when no labels, annotations, or containers are needed.
func newNotebook(name, namespace string, opts notebookOptions) *unstructured.Unstructured {
	metadata := map[string]any{
		"name":      name,
		"namespace": namespace,
	}

	if len(opts.Labels) > 0 {
		metadata["labels"] = opts.Labels
	}

	if len(opts.Annotations) > 0 {
		metadata["annotations"] = opts.Annotations
	}

	obj := map[string]any{
		"apiVersion": resources.Notebook.APIVersion(),
		"kind":       resources.Notebook.Kind,
		"metadata":   metadata,
	}

	if opts.Containers != nil {
		obj["spec"] = map[string]any{
			"template": map[string]any{
				"spec": map[string]any{
					"containers": opts.Containers,
				},
			},
		}
	}

	return &unstructured.Unstructured{Object: obj}
}
