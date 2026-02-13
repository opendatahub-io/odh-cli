package kube_test

import (
	"testing"

	corev1 "k8s.io/api/core/v1"

	"github.com/opendatahub-io/odh-cli/pkg/util/kube"

	. "github.com/onsi/gomega"
)

const (
	testConfigMapName = "test-config"
	testSecretName    = "test-secret"
)

func TestExtractConfigMapRefFromEnvFromSource(t *testing.T) {
	g := NewWithT(t)

	t.Run("returns ConfigMap name when ConfigMapRef is set", func(t *testing.T) {
		envFrom := corev1.EnvFromSource{
			ConfigMapRef: &corev1.ConfigMapEnvSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: testConfigMapName,
				},
			},
		}

		result := kube.ExtractConfigMapRefFromEnvFromSource(envFrom)
		g.Expect(result).To(Equal(testConfigMapName))
	})

	t.Run("returns empty string when ConfigMapRef is nil", func(t *testing.T) {
		envFrom := corev1.EnvFromSource{}

		result := kube.ExtractConfigMapRefFromEnvFromSource(envFrom)
		g.Expect(result).To(Equal(""))
	})

	t.Run("returns empty string when ConfigMapRef.Name is empty", func(t *testing.T) {
		envFrom := corev1.EnvFromSource{
			ConfigMapRef: &corev1.ConfigMapEnvSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: "",
				},
			},
		}

		result := kube.ExtractConfigMapRefFromEnvFromSource(envFrom)
		g.Expect(result).To(Equal(""))
	})

	t.Run("returns empty string when SecretRef is set instead", func(t *testing.T) {
		envFrom := corev1.EnvFromSource{
			SecretRef: &corev1.SecretEnvSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: testSecretName,
				},
			},
		}

		result := kube.ExtractConfigMapRefFromEnvFromSource(envFrom)
		g.Expect(result).To(Equal(""))
	})
}

func TestExtractSecretRefFromEnvFromSource(t *testing.T) {
	g := NewWithT(t)

	t.Run("returns Secret name when SecretRef is set", func(t *testing.T) {
		envFrom := corev1.EnvFromSource{
			SecretRef: &corev1.SecretEnvSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: testSecretName,
				},
			},
		}

		result := kube.ExtractSecretRefFromEnvFromSource(envFrom)
		g.Expect(result).To(Equal(testSecretName))
	})

	t.Run("returns empty string when SecretRef is nil", func(t *testing.T) {
		envFrom := corev1.EnvFromSource{}

		result := kube.ExtractSecretRefFromEnvFromSource(envFrom)
		g.Expect(result).To(Equal(""))
	})

	t.Run("returns empty string when SecretRef.Name is empty", func(t *testing.T) {
		envFrom := corev1.EnvFromSource{
			SecretRef: &corev1.SecretEnvSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: "",
				},
			},
		}

		result := kube.ExtractSecretRefFromEnvFromSource(envFrom)
		g.Expect(result).To(Equal(""))
	})

	t.Run("returns empty string when ConfigMapRef is set instead", func(t *testing.T) {
		envFrom := corev1.EnvFromSource{
			ConfigMapRef: &corev1.ConfigMapEnvSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: testConfigMapName,
				},
			},
		}

		result := kube.ExtractSecretRefFromEnvFromSource(envFrom)
		g.Expect(result).To(Equal(""))
	})
}

func TestExtractConfigMapRefFromEnvVar(t *testing.T) {
	g := NewWithT(t)

	t.Run("returns ConfigMap name when ConfigMapKeyRef is set", func(t *testing.T) {
		env := corev1.EnvVar{
			Name: "TEST_VAR",
			ValueFrom: &corev1.EnvVarSource{
				ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: testConfigMapName,
					},
					Key: "test-key",
				},
			},
		}

		result := kube.ExtractConfigMapRefFromEnvVar(env)
		g.Expect(result).To(Equal(testConfigMapName))
	})

	t.Run("returns empty string when ValueFrom is nil", func(t *testing.T) {
		env := corev1.EnvVar{
			Name:  "TEST_VAR",
			Value: "static-value",
		}

		result := kube.ExtractConfigMapRefFromEnvVar(env)
		g.Expect(result).To(Equal(""))
	})

	t.Run("returns empty string when ConfigMapKeyRef is nil", func(t *testing.T) {
		env := corev1.EnvVar{
			Name:      "TEST_VAR",
			ValueFrom: &corev1.EnvVarSource{},
		}

		result := kube.ExtractConfigMapRefFromEnvVar(env)
		g.Expect(result).To(Equal(""))
	})

	t.Run("returns empty string when ConfigMapKeyRef.Name is empty", func(t *testing.T) {
		env := corev1.EnvVar{
			Name: "TEST_VAR",
			ValueFrom: &corev1.EnvVarSource{
				ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: "",
					},
					Key: "test-key",
				},
			},
		}

		result := kube.ExtractConfigMapRefFromEnvVar(env)
		g.Expect(result).To(Equal(""))
	})

	t.Run("returns empty string when SecretKeyRef is set instead", func(t *testing.T) {
		env := corev1.EnvVar{
			Name: "TEST_VAR",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: testSecretName,
					},
					Key: "test-key",
				},
			},
		}

		result := kube.ExtractConfigMapRefFromEnvVar(env)
		g.Expect(result).To(Equal(""))
	})
}

func TestExtractSecretRefFromEnvVar(t *testing.T) {
	g := NewWithT(t)

	t.Run("returns Secret name when SecretKeyRef is set", func(t *testing.T) {
		env := corev1.EnvVar{
			Name: "TEST_VAR",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: testSecretName,
					},
					Key: "test-key",
				},
			},
		}

		result := kube.ExtractSecretRefFromEnvVar(env)
		g.Expect(result).To(Equal(testSecretName))
	})

	t.Run("returns empty string when ValueFrom is nil", func(t *testing.T) {
		env := corev1.EnvVar{
			Name:  "TEST_VAR",
			Value: "static-value",
		}

		result := kube.ExtractSecretRefFromEnvVar(env)
		g.Expect(result).To(Equal(""))
	})

	t.Run("returns empty string when SecretKeyRef is nil", func(t *testing.T) {
		env := corev1.EnvVar{
			Name:      "TEST_VAR",
			ValueFrom: &corev1.EnvVarSource{},
		}

		result := kube.ExtractSecretRefFromEnvVar(env)
		g.Expect(result).To(Equal(""))
	})

	t.Run("returns empty string when SecretKeyRef.Name is empty", func(t *testing.T) {
		env := corev1.EnvVar{
			Name: "TEST_VAR",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: "",
					},
					Key: "test-key",
				},
			},
		}

		result := kube.ExtractSecretRefFromEnvVar(env)
		g.Expect(result).To(Equal(""))
	})

	t.Run("returns empty string when ConfigMapKeyRef is set instead", func(t *testing.T) {
		env := corev1.EnvVar{
			Name: "TEST_VAR",
			ValueFrom: &corev1.EnvVarSource{
				ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: testConfigMapName,
					},
					Key: "test-key",
				},
			},
		}

		result := kube.ExtractSecretRefFromEnvVar(env)
		g.Expect(result).To(Equal(""))
	})
}

func TestExtractConfigMapRefs(t *testing.T) {
	g := NewWithT(t)

	t.Run("extracts ConfigMap from envFrom", func(t *testing.T) {
		container := corev1.Container{
			Name: "test-container",
			EnvFrom: []corev1.EnvFromSource{
				{
					ConfigMapRef: &corev1.ConfigMapEnvSource{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: testConfigMapName,
						},
					},
				},
			},
		}

		result := kube.ExtractConfigMapRefs(container)
		g.Expect(result).To(ConsistOf(testConfigMapName))
	})

	t.Run("extracts ConfigMap from env", func(t *testing.T) {
		container := corev1.Container{
			Name: "test-container",
			Env: []corev1.EnvVar{
				{
					Name: "TEST_VAR",
					ValueFrom: &corev1.EnvVarSource{
						ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: testConfigMapName,
							},
							Key: "test-key",
						},
					},
				},
			},
		}

		result := kube.ExtractConfigMapRefs(container)
		g.Expect(result).To(ConsistOf(testConfigMapName))
	})

	t.Run("extracts ConfigMaps from both envFrom and env", func(t *testing.T) {
		container := corev1.Container{
			Name: "test-container",
			EnvFrom: []corev1.EnvFromSource{
				{
					ConfigMapRef: &corev1.ConfigMapEnvSource{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: "config1",
						},
					},
				},
			},
			Env: []corev1.EnvVar{
				{
					Name: "TEST_VAR",
					ValueFrom: &corev1.EnvVarSource{
						ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: "config2",
							},
							Key: "test-key",
						},
					},
				},
			},
		}

		result := kube.ExtractConfigMapRefs(container)
		g.Expect(result).To(ConsistOf("config1", "config2"))
	})

	t.Run("deduplicates ConfigMap names", func(t *testing.T) {
		container := corev1.Container{
			Name: "test-container",
			EnvFrom: []corev1.EnvFromSource{
				{
					ConfigMapRef: &corev1.ConfigMapEnvSource{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: testConfigMapName,
						},
					},
				},
			},
			Env: []corev1.EnvVar{
				{
					Name: "TEST_VAR1",
					ValueFrom: &corev1.EnvVarSource{
						ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: testConfigMapName,
							},
							Key: "key1",
						},
					},
				},
				{
					Name: "TEST_VAR2",
					ValueFrom: &corev1.EnvVarSource{
						ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: testConfigMapName,
							},
							Key: "key2",
						},
					},
				},
			},
		}

		result := kube.ExtractConfigMapRefs(container)
		g.Expect(result).To(ConsistOf(testConfigMapName))
	})

	t.Run("returns empty slice when no ConfigMaps referenced", func(t *testing.T) {
		container := corev1.Container{
			Name: "test-container",
			Env: []corev1.EnvVar{
				{
					Name:  "TEST_VAR",
					Value: "static-value",
				},
			},
		}

		result := kube.ExtractConfigMapRefs(container)
		g.Expect(result).To(BeEmpty())
	})

	t.Run("ignores Secret references", func(t *testing.T) {
		container := corev1.Container{
			Name: "test-container",
			EnvFrom: []corev1.EnvFromSource{
				{
					SecretRef: &corev1.SecretEnvSource{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: testSecretName,
						},
					},
				},
			},
			Env: []corev1.EnvVar{
				{
					Name: "TEST_VAR",
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: testSecretName,
							},
							Key: "test-key",
						},
					},
				},
			},
		}

		result := kube.ExtractConfigMapRefs(container)
		g.Expect(result).To(BeEmpty())
	})
}

func TestExtractSecretRefs(t *testing.T) {
	g := NewWithT(t)

	t.Run("extracts Secret from envFrom", func(t *testing.T) {
		container := corev1.Container{
			Name: "test-container",
			EnvFrom: []corev1.EnvFromSource{
				{
					SecretRef: &corev1.SecretEnvSource{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: testSecretName,
						},
					},
				},
			},
		}

		result := kube.ExtractSecretRefs(container)
		g.Expect(result).To(ConsistOf(testSecretName))
	})

	t.Run("extracts Secret from env", func(t *testing.T) {
		container := corev1.Container{
			Name: "test-container",
			Env: []corev1.EnvVar{
				{
					Name: "TEST_VAR",
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: testSecretName,
							},
							Key: "test-key",
						},
					},
				},
			},
		}

		result := kube.ExtractSecretRefs(container)
		g.Expect(result).To(ConsistOf(testSecretName))
	})

	t.Run("extracts Secrets from both envFrom and env", func(t *testing.T) {
		container := corev1.Container{
			Name: "test-container",
			EnvFrom: []corev1.EnvFromSource{
				{
					SecretRef: &corev1.SecretEnvSource{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: "secret1",
						},
					},
				},
			},
			Env: []corev1.EnvVar{
				{
					Name: "TEST_VAR",
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: "secret2",
							},
							Key: "test-key",
						},
					},
				},
			},
		}

		result := kube.ExtractSecretRefs(container)
		g.Expect(result).To(ConsistOf("secret1", "secret2"))
	})

	t.Run("deduplicates Secret names", func(t *testing.T) {
		container := corev1.Container{
			Name: "test-container",
			EnvFrom: []corev1.EnvFromSource{
				{
					SecretRef: &corev1.SecretEnvSource{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: testSecretName,
						},
					},
				},
			},
			Env: []corev1.EnvVar{
				{
					Name: "TEST_VAR1",
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: testSecretName,
							},
							Key: "key1",
						},
					},
				},
				{
					Name: "TEST_VAR2",
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: testSecretName,
							},
							Key: "key2",
						},
					},
				},
			},
		}

		result := kube.ExtractSecretRefs(container)
		g.Expect(result).To(ConsistOf(testSecretName))
	})

	t.Run("returns empty slice when no Secrets referenced", func(t *testing.T) {
		container := corev1.Container{
			Name: "test-container",
			Env: []corev1.EnvVar{
				{
					Name:  "TEST_VAR",
					Value: "static-value",
				},
			},
		}

		result := kube.ExtractSecretRefs(container)
		g.Expect(result).To(BeEmpty())
	})

	t.Run("ignores ConfigMap references", func(t *testing.T) {
		container := corev1.Container{
			Name: "test-container",
			EnvFrom: []corev1.EnvFromSource{
				{
					ConfigMapRef: &corev1.ConfigMapEnvSource{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: testConfigMapName,
						},
					},
				},
			},
			Env: []corev1.EnvVar{
				{
					Name: "TEST_VAR",
					ValueFrom: &corev1.EnvVarSource{
						ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: testConfigMapName,
							},
							Key: "test-key",
						},
					},
				},
			},
		}

		result := kube.ExtractSecretRefs(container)
		g.Expect(result).To(BeEmpty())
	})
}
