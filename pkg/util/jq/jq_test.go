package jq_test

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/opendatahub-io/odh-cli/pkg/util/jq"

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

// TestQuery_DirectTypeAssertion tests that Query uses direct type assertion
// when the result type matches the requested type (zero-cost path).
func TestQuery_DirectTypeAssertion(t *testing.T) {
	g := NewWithT(t)

	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"spec": map[string]any{
				"items": []any{"a", "b", "c"},
			},
		},
	}

	// Query for []any should use direct type assertion (no JSON conversion)
	items, err := jq.Query[[]any](obj, ".spec.items")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(items).To(Equal([]any{"a", "b", "c"}))
}

// TestQuery_JSONConversion tests that Query falls back to JSON conversion
// when direct type assertion fails (e.g., []any to []string).
func TestQuery_JSONConversion(t *testing.T) {
	g := NewWithT(t)

	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"spec": map[string]any{
				"tags": []any{"tag1", "tag2", "tag3"},
			},
		},
	}

	// Query for []string requires JSON conversion from []any
	tags, err := jq.Query[[]string](obj, ".spec.tags")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(tags).To(Equal([]string{"tag1", "tag2", "tag3"}))
}

// TestQuery_StructConversion tests converting map[string]any to a struct type.
func TestQuery_StructConversion(t *testing.T) {
	g := NewWithT(t)

	type Config struct {
		Name    string `json:"name"`
		Enabled bool   `json:"enabled"`
		Count   int    `json:"count"`
	}

	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"spec": map[string]any{
				"config": map[string]any{
					"name":    "test-config",
					"enabled": true,
					"count":   42,
				},
			},
		},
	}

	// Query should convert map[string]any to Config struct via JSON
	config, err := jq.Query[Config](obj, ".spec.config")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(config).To(Equal(Config{
		Name:    "test-config",
		Enabled: true,
		Count:   42,
	}))
}

// TestQuery_SliceOfStructs tests converting []any to []Struct.
func TestQuery_SliceOfStructs(t *testing.T) {
	g := NewWithT(t)

	type Volume struct {
		Name string `json:"name"`
		Type string `json:"type"`
	}

	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"spec": map[string]any{
				"volumes": []any{
					map[string]any{"name": "vol1", "type": "pvc"},
					map[string]any{"name": "vol2", "type": "configMap"},
				},
			},
		},
	}

	// Query should convert []any to []Volume via JSON
	volumes, err := jq.Query[[]Volume](obj, ".spec.volumes")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(volumes).To(Equal([]Volume{
		{Name: "vol1", Type: "pvc"},
		{Name: "vol2", Type: "configMap"},
	}))
}

// TestQuery_NumberConversion tests numeric type conversions.
func TestQuery_NumberConversion(t *testing.T) {
	g := NewWithT(t)

	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"spec": map[string]any{
				"port":     float64(8080), // JSON numbers are float64
				"replicas": float64(3),
			},
		},
	}

	// Query for int should convert from float64
	port, err := jq.Query[int](obj, ".spec.port")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(port).To(Equal(8080))

	replicas, err := jq.Query[int](obj, ".spec.replicas")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(replicas).To(Equal(3))
}

// TestQuery_MapConversion tests map type conversions.
func TestQuery_MapConversion(t *testing.T) {
	g := NewWithT(t)

	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"metadata": map[string]any{
				"labels": map[string]any{
					"app":  "myapp",
					"tier": "frontend",
				},
			},
		},
	}

	// Query for map[string]string should convert from map[string]any
	labels, err := jq.Query[map[string]string](obj, ".metadata.labels")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(labels).To(Equal(map[string]string{
		"app":  "myapp",
		"tier": "frontend",
	}))
}

// TestQuery_BooleanType tests boolean type handling.
func TestQuery_BooleanType(t *testing.T) {
	g := NewWithT(t)

	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"spec": map[string]any{
				"enabled": true,
			},
		},
	}

	enabled, err := jq.Query[bool](obj, ".spec.enabled")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(enabled).To(BeTrue())
}

// TestQuery_WithJQFilter tests type conversion with JQ filters.
func TestQuery_WithJQFilter(t *testing.T) {
	g := NewWithT(t)

	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"spec": map[string]any{
				"items": []any{
					map[string]any{"name": "item1", "active": true},
					map[string]any{"name": "item2", "active": false},
					map[string]any{"name": "item3", "active": true},
				},
			},
		},
	}

	type Item struct {
		Name   string `json:"name"`
		Active bool   `json:"active"`
	}

	// Use JQ filter to select only active items, then convert to []Item
	activeItems, err := jq.Query[[]Item](obj, "[.spec.items[] | select(.active == true)]")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(activeItems).To(HaveLen(2))
	g.Expect(activeItems[0].Name).To(Equal("item1"))
	g.Expect(activeItems[1].Name).To(Equal("item3"))
}

// TestQuery_EmptySliceWithDefault tests that empty slices are handled correctly.
func TestQuery_EmptySliceWithDefault(t *testing.T) {
	g := NewWithT(t)

	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"spec": map[string]any{},
		},
	}

	// Use JQ's // operator to default to empty array
	items, err := jq.Query[[]string](obj, ".spec.items // []")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(items).To(BeEmpty())
}

// TestQuery_IncompatibleType tests error handling for incompatible types.
func TestQuery_IncompatibleType(t *testing.T) {
	g := NewWithT(t)

	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"spec": map[string]any{
				"value": "not-a-number",
			},
		},
	}

	// Trying to convert string to int should fail
	_, err := jq.Query[int](obj, ".spec.value")
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("unmarshaling to type int"))
}

func TestPredicate(t *testing.T) {
	g := NewWithT(t)

	t.Run("should return true when expression evaluates to true", func(t *testing.T) {
		obj := &unstructured.Unstructured{
			Object: map[string]any{
				"spec": map[string]any{
					"multiModel": true,
				},
			},
		}

		match, err := jq.Predicate(".spec.multiModel")(obj)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(match).To(BeTrue())
	})

	t.Run("should return false when expression evaluates to false", func(t *testing.T) {
		obj := &unstructured.Unstructured{
			Object: map[string]any{
				"spec": map[string]any{
					"multiModel": false,
				},
			},
		}

		match, err := jq.Predicate(".spec.multiModel")(obj)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(match).To(BeFalse())
	})

	t.Run("should return false when field does not exist", func(t *testing.T) {
		obj := &unstructured.Unstructured{
			Object: map[string]any{
				"spec": map[string]any{},
			},
		}

		match, err := jq.Predicate(".spec.multiModel")(obj)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(match).To(BeFalse())
	})

	t.Run("should return false when field is null", func(t *testing.T) {
		obj := &unstructured.Unstructured{
			Object: map[string]any{
				"spec": map[string]any{
					"multiModel": nil,
				},
			},
		}

		match, err := jq.Predicate(".spec.multiModel")(obj)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(match).To(BeFalse())
	})
}

// TestQuery_NestedStructConversion tests deep nested structure conversion.
func TestQuery_NestedStructConversion(t *testing.T) {
	g := NewWithT(t)

	type Container struct {
		Name  string `json:"name"`
		Image string `json:"image"`
	}

	type PodSpec struct {
		Containers []Container `json:"containers"`
		NodeName   string      `json:"nodeName"`
	}

	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"spec": map[string]any{
				"containers": []any{
					map[string]any{"name": "app", "image": "app:v1"},
					map[string]any{"name": "sidecar", "image": "sidecar:v2"},
				},
				"nodeName": "node-1",
			},
		},
	}

	// Convert entire spec to PodSpec struct
	spec, err := jq.Query[PodSpec](obj, ".spec")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(spec.NodeName).To(Equal("node-1"))
	g.Expect(spec.Containers).To(HaveLen(2))
	g.Expect(spec.Containers[0].Name).To(Equal("app"))
	g.Expect(spec.Containers[1].Name).To(Equal("sidecar"))
}
