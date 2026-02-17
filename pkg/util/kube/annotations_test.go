package kube_test

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/opendatahub-io/odh-cli/pkg/util/kube"

	. "github.com/onsi/gomega"
)

func TestIsManaged(t *testing.T) {
	tests := []struct {
		name        string
		annotations map[string]string
		expected    bool
	}{
		{
			name:        "nil annotations returns true (managed by default)",
			annotations: nil,
			expected:    true,
		},
		{
			name:        "empty annotations map returns true",
			annotations: map[string]string{},
			expected:    true,
		},
		{
			name: "annotation missing returns true",
			annotations: map[string]string{
				"other.annotation/key": "value",
			},
			expected: true,
		},
		{
			name: "annotation equals 'true' returns true",
			annotations: map[string]string{
				kube.AnnotationManaged: "true",
			},
			expected: true,
		},
		{
			name: "annotation is empty string returns true",
			annotations: map[string]string{
				kube.AnnotationManaged: "",
			},
			expected: true,
		},
		{
			name: "annotation equals 'false' returns false (unmanaged)",
			annotations: map[string]string{
				kube.AnnotationManaged: "false",
			},
			expected: false,
		},
		{
			name: "annotation equals 'False' returns true (case-sensitive)",
			annotations: map[string]string{
				kube.AnnotationManaged: "False",
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			configMap := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test-configmap",
					Namespace:   "test-namespace",
					Annotations: tt.annotations,
				},
			}

			result := kube.IsManaged(configMap)
			g.Expect(result).To(Equal(tt.expected))
		})
	}
}

func TestGetAnnotation(t *testing.T) {
	tests := []struct {
		name        string
		annotations map[string]string
		key         string
		expected    string
	}{
		{
			name:        "nil annotations returns empty string",
			annotations: nil,
			key:         "test.key",
			expected:    "",
		},
		{
			name:        "empty annotations map returns empty string",
			annotations: map[string]string{},
			key:         "test.key",
			expected:    "",
		},
		{
			name: "key not present returns empty string",
			annotations: map[string]string{
				"other.key": "value",
			},
			key:      "test.key",
			expected: "",
		},
		{
			name: "key present returns value",
			annotations: map[string]string{
				"test.key": "test-value",
			},
			key:      "test.key",
			expected: "test-value",
		},
		{
			name: "key with empty value returns empty string",
			annotations: map[string]string{
				"test.key": "",
			},
			key:      "test.key",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			configMap := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test-configmap",
					Namespace:   "test-namespace",
					Annotations: tt.annotations,
				},
			}

			result := kube.GetAnnotation(configMap, tt.key)
			g.Expect(result).To(Equal(tt.expected))
		})
	}
}

func TestHasAnnotation(t *testing.T) {
	tests := []struct {
		name        string
		annotations map[string]string
		key         string
		value       string
		expected    bool
	}{
		{
			name:        "nil annotations returns false",
			annotations: nil,
			key:         "test.key",
			value:       "test-value",
			expected:    false,
		},
		{
			name: "key not present returns false",
			annotations: map[string]string{
				"other.key": "value",
			},
			key:      "test.key",
			value:    "test-value",
			expected: false,
		},
		{
			name: "key present with matching value returns true",
			annotations: map[string]string{
				"test.key": "test-value",
			},
			key:      "test.key",
			value:    "test-value",
			expected: true,
		},
		{
			name: "key present with different value returns false",
			annotations: map[string]string{
				"test.key": "other-value",
			},
			key:      "test.key",
			value:    "test-value",
			expected: false,
		},
		{
			name: "key present with empty value matches empty",
			annotations: map[string]string{
				"test.key": "",
			},
			key:      "test.key",
			value:    "",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			configMap := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test-configmap",
					Namespace:   "test-namespace",
					Annotations: tt.annotations,
				},
			}

			result := kube.HasAnnotation(configMap, tt.key, tt.value)
			g.Expect(result).To(Equal(tt.expected))
		})
	}
}
