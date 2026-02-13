package kube

import (
	"fmt"
	"strings"

	"github.com/opendatahub-io/odh-cli/pkg/util/jq"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

// ToUnstructured converts a typed Kubernetes object to *unstructured.Unstructured.
func ToUnstructured(obj runtime.Object) (*unstructured.Unstructured, error) {
	unstructuredObj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
	if err != nil {
		return nil, fmt.Errorf("failed to convert to unstructured: %w", err)
	}

	return &unstructured.Unstructured{Object: unstructuredObj}, nil
}

// ToPartialObjectMetadata converts unstructured objects to runtime.Object slice
// containing PartialObjectMetadata. This is useful for populating metadata fake
// clients in tests.
func ToPartialObjectMetadata(objs ...*unstructured.Unstructured) []runtime.Object {
	result := make([]runtime.Object, 0, len(objs))
	for _, obj := range objs {
		pom := &metav1.PartialObjectMetadata{
			TypeMeta: metav1.TypeMeta{
				APIVersion: obj.GetAPIVersion(),
				Kind:       obj.GetKind(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:        obj.GetName(),
				Namespace:   obj.GetNamespace(),
				Labels:      obj.GetLabels(),
				Annotations: obj.GetAnnotations(),
				Finalizers:  obj.GetFinalizers(),
			},
		}
		result = append(result, pom)
	}

	return result
}

// StripFields removes specified fields from a resource using JQ.
func StripFields(obj *unstructured.Unstructured, fields []string) (*unstructured.Unstructured, error) {
	if len(fields) == 0 {
		return obj.DeepCopy(), nil
	}

	stripped := obj.DeepCopy()

	jqExpr := "del(" + strings.Join(fields, ", ") + ")"

	if err := jq.Transform(stripped, "%s", jqExpr); err != nil {
		return nil, fmt.Errorf("applying JQ transform: %w", err)
	}

	return stripped, nil
}
