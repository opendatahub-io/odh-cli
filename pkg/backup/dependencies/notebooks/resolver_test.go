package notebooks_test

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	"github.com/lburgazzoli/odh-cli/pkg/backup/dependencies/notebooks"
	"github.com/lburgazzoli/odh-cli/pkg/resources"
	"github.com/lburgazzoli/odh-cli/pkg/util/client"

	. "github.com/onsi/gomega"
)

//nolint:gochecknoglobals // Test fixture - shared across test functions
var listKinds = map[schema.GroupVersionResource]string{
	resources.ConfigMap.GVR(): "ConfigMapList",
	resources.Secret.GVR():    "SecretList",
}

func TestResolverWithSecrets(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	notebook := createNotebookWithSecret("test-notebook", "default", "test-secret")
	secret := createSecret("test-secret", "default")

	fakeClient := createFakeClient(t, notebook, secret)

	resolver := notebooks.NewResolver()

	deps, err := resolver.Resolve(ctx, fakeClient, notebook)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(deps).To(HaveLen(1))
	g.Expect(deps[0].GVR).To(Equal(resources.Secret.GVR()))
	g.Expect(deps[0].Resource.GetName()).To(Equal("test-secret"))
}

func TestResolverWithConfigMapAndSecret(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	notebook := createNotebookWithConfigMapAndSecret(
		"test-notebook",
		"default",
		"test-configmap",
		"test-secret",
	)
	configMap := createConfigMap("test-configmap", "default")
	secret := createSecret("test-secret", "default")

	fakeClient := createFakeClient(t, notebook, configMap, secret)

	resolver := notebooks.NewResolver()

	deps, err := resolver.Resolve(ctx, fakeClient, notebook)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(deps).To(HaveLen(2))

	configMapFound := false
	secretFound := false
	for _, dep := range deps {
		if dep.GVR == resources.ConfigMap.GVR() {
			configMapFound = true
			g.Expect(dep.Resource.GetName()).To(Equal("test-configmap"))
		}
		if dep.GVR == resources.Secret.GVR() {
			secretFound = true
			g.Expect(dep.Resource.GetName()).To(Equal("test-secret"))
		}
	}

	g.Expect(configMapFound).To(BeTrue())
	g.Expect(secretFound).To(BeTrue())
}

func createNotebookWithSecret(
	name string,
	namespace string,
	secretName string,
) *unstructured.Unstructured {
	notebook := &unstructured.Unstructured{}
	notebook.SetGroupVersionKind(resources.Notebook.GVK())
	notebook.SetName(name)
	notebook.SetNamespace(namespace)

	notebook.Object["spec"] = map[string]any{
		"template": map[string]any{
			"spec": map[string]any{
				"volumes": []any{
					map[string]any{
						"name": "secret-volume",
						"secret": map[string]any{
							"secretName": secretName,
						},
					},
				},
				"containers": []any{
					map[string]any{
						"name":  "notebook",
						"image": "test:latest",
					},
				},
			},
		},
	}

	return notebook
}

func createNotebookWithConfigMapAndSecret(
	name string,
	namespace string,
	configMapName string,
	secretName string,
) *unstructured.Unstructured {
	notebook := &unstructured.Unstructured{}
	notebook.SetGroupVersionKind(resources.Notebook.GVK())
	notebook.SetName(name)
	notebook.SetNamespace(namespace)

	notebook.Object["spec"] = map[string]any{
		"template": map[string]any{
			"spec": map[string]any{
				"volumes": []any{
					map[string]any{
						"name": "configmap-volume",
						"configMap": map[string]any{
							"name": configMapName,
						},
					},
					map[string]any{
						"name": "secret-volume",
						"secret": map[string]any{
							"secretName": secretName,
						},
					},
				},
				"containers": []any{
					map[string]any{
						"name":  "notebook",
						"image": "test:latest",
					},
				},
			},
		},
	}

	return notebook
}

func createSecret(
	name string,
	namespace string,
) *unstructured.Unstructured {
	secret := &unstructured.Unstructured{}
	secret.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "",
		Version: "v1",
		Kind:    "Secret",
	})
	secret.SetName(name)
	secret.SetNamespace(namespace)

	secret.Object["data"] = map[string]any{
		"key": "dmFsdWU=", // base64 encoded "value"
	}

	return secret
}

func createConfigMap(
	name string,
	namespace string,
) *unstructured.Unstructured {
	cm := &unstructured.Unstructured{}
	cm.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "",
		Version: "v1",
		Kind:    "ConfigMap",
	})
	cm.SetName(name)
	cm.SetNamespace(namespace)

	cm.Object["data"] = map[string]any{
		"key": "value",
	}

	return cm
}

func createFakeClient(
	t *testing.T,
	objs ...runtime.Object,
) *client.Client {
	t.Helper()

	scheme := runtime.NewScheme()
	err := corev1.AddToScheme(scheme)
	if err != nil {
		t.Fatalf("failed to add core v1 to scheme: %v", err)
	}

	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, listKinds, objs...)

	return &client.Client{
		Dynamic: dynamicClient,
	}
}
