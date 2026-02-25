package kube_test

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/opendatahub-io/odh-cli/pkg/util/kube"

	. "github.com/onsi/gomega"
)

func TestHasLabel(t *testing.T) {
	g := NewWithT(t)

	t.Run("label present with matching value", func(t *testing.T) {
		obj := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{"app": "test"},
			},
		}

		g.Expect(kube.HasLabel(obj, "app", "test")).To(BeTrue())
	})

	t.Run("label present with non-matching value", func(t *testing.T) {
		obj := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{"app": "other"},
			},
		}

		g.Expect(kube.HasLabel(obj, "app", "test")).To(BeFalse())
	})

	t.Run("label present with empty value checking for empty", func(t *testing.T) {
		obj := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{"app": ""},
			},
		}

		g.Expect(kube.HasLabel(obj, "app", "")).To(BeTrue())
	})

	t.Run("label absent checking for empty value", func(t *testing.T) {
		obj := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{"other": "value"},
			},
		}

		g.Expect(kube.HasLabel(obj, "app", "")).To(BeFalse())
	})

	t.Run("nil labels map", func(t *testing.T) {
		obj := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{},
		}

		g.Expect(kube.HasLabel(obj, "app", "")).To(BeFalse())
	})
}
