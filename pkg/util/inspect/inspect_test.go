package inspect_test

import (
	"testing"

	"github.com/opendatahub-io/odh-cli/pkg/util/inspect"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	. "github.com/onsi/gomega"
)

func TestHasFields_AllFound(t *testing.T) {
	g := NewWithT(t)

	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"spec": map[string]any{
				"foo": "bar",
				"baz": float64(42),
			},
		},
	}

	found, err := inspect.HasFields(obj, ".spec.foo", ".spec.baz")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(found).To(HaveLen(2))
	g.Expect(found[0]).To(Equal("bar"))
	g.Expect(found[1]).To(Equal(float64(42)))
}

func TestHasFields_SomeFound(t *testing.T) {
	g := NewWithT(t)

	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"spec": map[string]any{
				"foo": "bar",
			},
		},
	}

	found, err := inspect.HasFields(obj, ".spec.foo", ".spec.missing")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(found).To(HaveLen(1))
	g.Expect(found[0]).To(Equal("bar"))
}

func TestHasFields_NoneFound(t *testing.T) {
	g := NewWithT(t)

	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"spec": map[string]any{},
		},
	}

	found, err := inspect.HasFields(obj, ".spec.missing", ".spec.also_missing")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(found).To(BeEmpty())
}

func TestHasFields_SingleExpression_Found(t *testing.T) {
	g := NewWithT(t)

	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"spec": map[string]any{
				"config": map[string]any{
					"enabled": true,
				},
			},
		},
	}

	found, err := inspect.HasFields(obj, ".spec.config.enabled")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(found).To(HaveLen(1))
	g.Expect(found[0]).To(BeTrue())
}

func TestHasFields_SingleExpression_NotFound(t *testing.T) {
	g := NewWithT(t)

	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"spec": map[string]any{},
		},
	}

	found, err := inspect.HasFields(obj, ".spec.config.enabled")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(found).To(BeEmpty())
}

func TestHasFields_InvalidExpression(t *testing.T) {
	g := NewWithT(t)

	obj := &unstructured.Unstructured{
		Object: map[string]any{},
	}

	_, err := inspect.HasFields(obj, "invalid jq syntax {")
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("querying"))
}

func TestHasFields_MapInput(t *testing.T) {
	g := NewWithT(t)

	// Verify it works with raw map[string]any (as used in otel_migration.go)
	obj := map[string]any{
		"spec": map[string]any{
			"otelExporter": map[string]any{
				"protocol":     "grpc",
				"otlpEndpoint": "http://collector:4317",
			},
		},
	}

	found, err := inspect.HasFields(obj,
		".spec.otelExporter.protocol",
		".spec.otelExporter.otlpEndpoint",
		".spec.otelExporter.tracesProtocol",
	)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(found).To(HaveLen(2))
	g.Expect(found[0]).To(Equal("grpc"))
	g.Expect(found[1]).To(Equal("http://collector:4317"))
}

func TestHasFields_NoExpressions(t *testing.T) {
	g := NewWithT(t)

	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"spec": map[string]any{
				"foo": "bar",
			},
		},
	}

	found, err := inspect.HasFields(obj)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(found).To(BeEmpty())
}
