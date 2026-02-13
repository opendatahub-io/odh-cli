package kube_test

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/opendatahub-io/odh-cli/pkg/util/kube"

	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

func TestStripFields(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]any
		fields   []string
		expected map[string]any
	}{
		{
			name: "strip single top-level field",
			input: map[string]any{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata": map[string]any{
					"name": "test",
				},
				"data": map[string]any{
					"key": "value",
				},
			},
			fields: []string{".data"},
			expected: map[string]any{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata": map[string]any{
					"name": "test",
				},
			},
		},
		{
			name: "strip nested field",
			input: map[string]any{
				"apiVersion": "v1",
				"kind":       "Pod",
				"metadata": map[string]any{
					"name":            "test-pod",
					"resourceVersion": "12345",
					"uid":             "abc-123",
				},
				"spec": map[string]any{
					"containers": []any{
						map[string]any{"name": "nginx"},
					},
				},
			},
			fields: []string{".metadata.resourceVersion", ".metadata.uid"},
			expected: map[string]any{
				"apiVersion": "v1",
				"kind":       "Pod",
				"metadata": map[string]any{
					"name": "test-pod",
				},
				"spec": map[string]any{
					"containers": []any{
						map[string]any{"name": "nginx"},
					},
				},
			},
		},
		{
			name: "strip status field",
			input: map[string]any{
				"apiVersion": "v1",
				"kind":       "Service",
				"metadata": map[string]any{
					"name": "test-svc",
				},
				"spec": map[string]any{
					"ports": []any{
						map[string]any{"port": int64(80)},
					},
				},
				"status": map[string]any{
					"loadBalancer": map[string]any{
						"ingress": []any{},
					},
				},
			},
			fields: []string{".status"},
			expected: map[string]any{
				"apiVersion": "v1",
				"kind":       "Service",
				"metadata": map[string]any{
					"name": "test-svc",
				},
				"spec": map[string]any{
					"ports": []any{
						map[string]any{"port": int64(80)},
					},
				},
			},
		},
		{
			name: "strip multiple metadata fields",
			input: map[string]any{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata": map[string]any{
					"name":              "test",
					"namespace":         "default",
					"creationTimestamp": "2024-01-20T00:00:00Z",
					"resourceVersion":   "12345",
					"uid":               "abc-123",
					"managedFields":     []any{},
				},
			},
			fields: []string{
				".metadata.creationTimestamp",
				".metadata.resourceVersion",
				".metadata.uid",
				".metadata.managedFields",
			},
			expected: map[string]any{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata": map[string]any{
					"name":      "test",
					"namespace": "default",
				},
			},
		},
		{
			name: "empty fields list returns copy",
			input: map[string]any{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata": map[string]any{
					"name": "test",
				},
			},
			fields: []string{},
			expected: map[string]any{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata": map[string]any{
					"name": "test",
				},
			},
		},
		{
			name: "nil fields list returns copy",
			input: map[string]any{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata": map[string]any{
					"name": "test",
				},
			},
			fields: nil,
			expected: map[string]any{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata": map[string]any{
					"name": "test",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			input := &unstructured.Unstructured{Object: tt.input}
			result, err := kube.StripFields(input, tt.fields)

			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(result.Object).To(Equal(tt.expected))

			originalName := input.Object["metadata"].(map[string]any)["name"]
			result.Object["metadata"].(map[string]any)["name"] = "modified"
			g.Expect(input.Object["metadata"].(map[string]any)["name"]).To(Equal(originalName))
		})
	}
}

func TestConvertToTyped(t *testing.T) {
	tests := []struct {
		name        string
		input       any
		expectError bool
	}{
		{
			name: "convert to volume slice",
			input: []any{
				map[string]any{
					"name": "config-volume",
					"configMap": map[string]any{
						"name": "my-config",
					},
				},
				map[string]any{
					"name": "secret-volume",
					"secret": map[string]any{
						"secretName": "my-secret",
					},
				},
			},
			expectError: false,
		},
		{
			name: "convert to container slice",
			input: []any{
				map[string]any{
					"name":  "nginx",
					"image": "nginx:latest",
					"env": []any{
						map[string]any{
							"name":  "ENV_VAR",
							"value": "test",
						},
					},
				},
			},
			expectError: false,
		},
		{
			name: "convert to single container",
			input: map[string]any{
				"name":  "redis",
				"image": "redis:latest",
			},
			expectError: false,
		},
		{
			name:        "nil input returns zero value",
			input:       nil,
			expectError: false,
		},
		{
			name:        "empty slice",
			input:       []any{},
			expectError: false,
		},
		{
			name: "invalid structure for container",
			input: map[string]any{
				"name": 12345,
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			switch tt.name {
			case "convert to volume slice":
				result, err := kube.ConvertToTyped[[]corev1.Volume](tt.input, "volumes")
				if tt.expectError {
					g.Expect(err).To(HaveOccurred())
				} else {
					g.Expect(err).ToNot(HaveOccurred())
					g.Expect(result).To(HaveLen(2))
					g.Expect(result[0]).To(MatchFields(IgnoreExtras, Fields{
						"Name": Equal("config-volume"),
					}))
					g.Expect(result[0].ConfigMap).To(PointTo(MatchFields(IgnoreExtras, Fields{
						"LocalObjectReference": MatchFields(IgnoreExtras, Fields{
							"Name": Equal("my-config"),
						}),
					})))
					g.Expect(result[1]).To(MatchFields(IgnoreExtras, Fields{
						"Name": Equal("secret-volume"),
					}))
					g.Expect(result[1].Secret).To(PointTo(MatchFields(IgnoreExtras, Fields{
						"SecretName": Equal("my-secret"),
					})))
				}

			case "convert to container slice":
				result, err := kube.ConvertToTyped[[]corev1.Container](tt.input, "containers")
				if tt.expectError {
					g.Expect(err).To(HaveOccurred())
				} else {
					g.Expect(err).ToNot(HaveOccurred())
					g.Expect(result).To(HaveLen(1))
					g.Expect(result[0]).To(MatchFields(IgnoreExtras, Fields{
						"Name":  Equal("nginx"),
						"Image": Equal("nginx:latest"),
					}))
					g.Expect(result[0].Env).To(HaveLen(1))
					g.Expect(result[0].Env[0]).To(MatchFields(IgnoreExtras, Fields{
						"Name":  Equal("ENV_VAR"),
						"Value": Equal("test"),
					}))
				}

			case "convert to single container":
				result, err := kube.ConvertToTyped[corev1.Container](tt.input, "container")
				if tt.expectError {
					g.Expect(err).To(HaveOccurred())
				} else {
					g.Expect(err).ToNot(HaveOccurred())
					g.Expect(result).To(MatchFields(IgnoreExtras, Fields{
						"Name":  Equal("redis"),
						"Image": Equal("redis:latest"),
					}))
				}

			case "nil input returns zero value":
				result, err := kube.ConvertToTyped[[]corev1.Volume](tt.input, "volumes")
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(result).To(BeNil())

			case "empty slice":
				result, err := kube.ConvertToTyped[[]corev1.Container](tt.input, "containers")
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(result).To(BeEmpty())

			case "invalid structure for container":
				result, err := kube.ConvertToTyped[corev1.Container](tt.input, "container")
				g.Expect(err).To(HaveOccurred())
				g.Expect(result).To(Equal(corev1.Container{}))
			}
		})
	}
}

func TestToPartialObjectMetadata_SingleObject(t *testing.T) {
	g := NewWithT(t)

	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]any{
				"name":      "test-cm",
				"namespace": "default",
				"labels": map[string]any{
					"app": "test",
				},
				"annotations": map[string]any{
					"note": "value",
				},
				"finalizers": []any{
					"my-finalizer",
				},
			},
		},
	}

	result := kube.ToPartialObjectMetadata(obj)

	g.Expect(result).To(HaveLen(1))

	pom, ok := result[0].(*metav1.PartialObjectMetadata)
	g.Expect(ok).To(BeTrue())
	g.Expect(pom).To(PointTo(MatchFields(IgnoreExtras, Fields{
		"TypeMeta": MatchFields(IgnoreExtras, Fields{
			"APIVersion": Equal("v1"),
			"Kind":       Equal("ConfigMap"),
		}),
		"ObjectMeta": MatchFields(IgnoreExtras, Fields{
			"Name":        Equal("test-cm"),
			"Namespace":   Equal("default"),
			"Labels":      HaveKeyWithValue("app", "test"),
			"Annotations": HaveKeyWithValue("note", "value"),
			"Finalizers":  ConsistOf("my-finalizer"),
		}),
	})))
}

func TestToPartialObjectMetadata_MultipleObjects(t *testing.T) {
	g := NewWithT(t)

	obj1 := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata": map[string]any{
				"name":      "deploy-1",
				"namespace": "ns1",
			},
		},
	}

	obj2 := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "Service",
			"metadata": map[string]any{
				"name":      "svc-1",
				"namespace": "ns2",
			},
		},
	}

	result := kube.ToPartialObjectMetadata(obj1, obj2)

	g.Expect(result).To(HaveLen(2))

	pom0, ok := result[0].(*metav1.PartialObjectMetadata)
	g.Expect(ok).To(BeTrue())
	g.Expect(pom0.Name).To(Equal("deploy-1"))
	g.Expect(pom0.Kind).To(Equal("Deployment"))

	pom1, ok := result[1].(*metav1.PartialObjectMetadata)
	g.Expect(ok).To(BeTrue())
	g.Expect(pom1.Name).To(Equal("svc-1"))
	g.Expect(pom1.Kind).To(Equal("Service"))
}

func TestToPartialObjectMetadata_EmptyInput(t *testing.T) {
	g := NewWithT(t)

	result := kube.ToPartialObjectMetadata()

	g.Expect(result).To(BeEmpty())
}

func TestToPartialObjectMetadata_NilMetadataFields(t *testing.T) {
	g := NewWithT(t)

	// Object with no labels, annotations, or finalizers.
	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "Pod",
			"metadata": map[string]any{
				"name":      "bare-pod",
				"namespace": "default",
			},
		},
	}

	result := kube.ToPartialObjectMetadata(obj)

	g.Expect(result).To(HaveLen(1))

	pom, ok := result[0].(*metav1.PartialObjectMetadata)
	g.Expect(ok).To(BeTrue())
	g.Expect(pom.Name).To(Equal("bare-pod"))
	g.Expect(pom.Labels).To(BeNil())
	g.Expect(pom.Annotations).To(BeNil())
	g.Expect(pom.Finalizers).To(BeNil())
}
