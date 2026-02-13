package kube_test

import (
	"testing"

	"github.com/opendatahub-io/odh-cli/pkg/util/kube"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

func TestToNamespacedNames(t *testing.T) {
	t.Run("should convert unstructured object pointers", func(t *testing.T) {
		g := NewWithT(t)

		items := []*unstructured.Unstructured{
			{Object: map[string]any{
				"metadata": map[string]any{
					"name":      "nb-1",
					"namespace": "ns-a",
				},
			}},
			{Object: map[string]any{
				"metadata": map[string]any{
					"name":      "nb-2",
					"namespace": "ns-b",
				},
			}},
		}

		result := kube.ToNamespacedNames(items)

		g.Expect(result).To(HaveLen(2))
		g.Expect(result[0]).To(MatchFields(IgnoreExtras, Fields{
			"Namespace": Equal("ns-a"),
			"Name":      Equal("nb-1"),
		}))
		g.Expect(result[1]).To(MatchFields(IgnoreExtras, Fields{
			"Namespace": Equal("ns-b"),
			"Name":      Equal("nb-2"),
		}))
	})

	t.Run("should convert PartialObjectMetadata pointers", func(t *testing.T) {
		g := NewWithT(t)

		items := []*metav1.PartialObjectMetadata{
			{ObjectMeta: metav1.ObjectMeta{Name: "profile-1", Namespace: "ns-x"}},
			{ObjectMeta: metav1.ObjectMeta{Name: "profile-2", Namespace: "ns-y"}},
		}

		result := kube.ToNamespacedNames(items)

		g.Expect(result).To(HaveLen(2))
		g.Expect(result[0]).To(MatchFields(IgnoreExtras, Fields{
			"Namespace": Equal("ns-x"),
			"Name":      Equal("profile-1"),
		}))
		g.Expect(result[1]).To(MatchFields(IgnoreExtras, Fields{
			"Namespace": Equal("ns-y"),
			"Name":      Equal("profile-2"),
		}))
	})

	t.Run("should return empty slice for empty input", func(t *testing.T) {
		g := NewWithT(t)

		result := kube.ToNamespacedNames([]*unstructured.Unstructured{})

		g.Expect(result).To(BeEmpty())
	})

	t.Run("should return empty slice for nil input", func(t *testing.T) {
		g := NewWithT(t)

		result := kube.ToNamespacedNames[*metav1.PartialObjectMetadata](nil)

		g.Expect(result).To(BeEmpty())
	})

	t.Run("should handle cluster-scoped resources without namespace", func(t *testing.T) {
		g := NewWithT(t)

		items := []*metav1.PartialObjectMetadata{
			{ObjectMeta: metav1.ObjectMeta{Name: "cluster-resource"}},
		}

		result := kube.ToNamespacedNames(items)

		g.Expect(result).To(HaveLen(1))
		g.Expect(result[0]).To(MatchFields(IgnoreExtras, Fields{
			"Namespace": Equal(""),
			"Name":      Equal("cluster-resource"),
		}))
	})
}
