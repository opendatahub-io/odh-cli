package jq_test

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/lburgazzoli/odh-cli/pkg/util/jq"

	. "github.com/onsi/gomega"
)

func TestTransform_SetString(t *testing.T) {
	g := NewWithT(t)

	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"spec": map[string]any{
				"foo": "old",
			},
		},
	}

	err := jq.Transform(obj, `.spec.foo = "new"`)
	g.Expect(err).ToNot(HaveOccurred())

	value, err := jq.Query[string](obj, ".spec.foo")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(value).To(Equal("new"))
}

func TestTransform_SetNestedField(t *testing.T) {
	g := NewWithT(t)

	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"spec": map[string]any{
				"components": map[string]any{
					"kueue": map[string]any{
						"managementState": "Managed",
					},
				},
			},
		},
	}

	err := jq.Transform(obj, `.spec.components.kueue.managementState = "Unmanaged"`)
	g.Expect(err).ToNot(HaveOccurred())

	value, err := jq.Query[string](obj, ".spec.components.kueue.managementState")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(value).To(Equal("Unmanaged"))
}

func TestTransform_SetMap(t *testing.T) {
	g := NewWithT(t)

	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"metadata": map[string]any{},
		},
	}

	// Use JQ object construction syntax
	err := jq.Transform(obj, `.metadata.annotations = {"key": "value", "foo": "bar"}`)
	g.Expect(err).ToNot(HaveOccurred())

	annotations, err := jq.Query[map[string]any](obj, ".metadata.annotations")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(annotations).To(Equal(map[string]any{
		"key": "value",
		"foo": "bar",
	}))
}

func TestTransform_ChainedUpdates(t *testing.T) {
	g := NewWithT(t)

	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"spec": map[string]any{
				"foo": "old",
				"bar": "old",
			},
		},
	}

	err := jq.Transform(obj, `.spec.foo = "new" | .spec.bar = "updated"`)
	g.Expect(err).ToNot(HaveOccurred())

	foo, err := jq.Query[string](obj, ".spec.foo")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(foo).To(Equal("new"))

	bar, err := jq.Query[string](obj, ".spec.bar")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(bar).To(Equal("updated"))
}

func TestTransform_InvalidExpression(t *testing.T) {
	g := NewWithT(t)

	obj := &unstructured.Unstructured{
		Object: map[string]any{},
	}

	err := jq.Transform(obj, "invalid jq syntax {")
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("failed to parse jq expression"))
}

func TestQuery_String(t *testing.T) {
	g := NewWithT(t)

	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"spec": map[string]any{
				"foo": "bar",
			},
		},
	}

	value, err := jq.Query[string](obj, ".spec.foo")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(value).To(Equal("bar"))
}

func TestQuery_MissingField(t *testing.T) {
	g := NewWithT(t)

	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"spec": map[string]any{},
		},
	}

	value, err := jq.Query[string](obj, ".spec.nonexistent")
	g.Expect(err).To(MatchError(jq.ErrNotFound))
	g.Expect(value).To(Equal("")) // Zero value for string returned with error
}

func TestTransform_WithPrintfFormatting(t *testing.T) {
	g := NewWithT(t)

	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"spec": map[string]any{},
		},
	}

	err := jq.Transform(obj, ".spec.foo = %q", "bar")
	g.Expect(err).ToNot(HaveOccurred())

	value, err := jq.Query[string](obj, ".spec.foo")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(value).To(Equal("bar"))
}

func TestTransform_WithMultipleArgs(t *testing.T) {
	g := NewWithT(t)

	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"spec": map[string]any{},
		},
	}

	err := jq.Transform(obj, ".spec.foo = %q | .spec.bar = %q", "value1", "value2")
	g.Expect(err).ToNot(HaveOccurred())

	foo, _ := jq.Query[string](obj, ".spec.foo")
	g.Expect(foo).To(Equal("value1"))

	bar, _ := jq.Query[string](obj, ".spec.bar")
	g.Expect(bar).To(Equal("value2"))
}
