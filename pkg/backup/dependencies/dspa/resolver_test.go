package dspa_test

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	"github.com/opendatahub-io/odh-cli/pkg/backup/dependencies/dspa"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"

	. "github.com/onsi/gomega"
)

//nolint:gochecknoglobals // Test fixture - shared across test functions
var listKinds = map[schema.GroupVersionResource]string{
	resources.ConfigMap.GVR():             "ConfigMapList",
	resources.Secret.GVR():                "SecretList",
	resources.PersistentVolumeClaim.GVR(): "PersistentVolumeClaimList",
}

func TestResolverWithExternalStorageSecret(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	dspaObj := createDSPAWithExternalStorage("test-dspa", "default", "s3-credentials")
	secret := createSecret("s3-credentials", "default")

	fakeClient := createFakeClient(t, dspaObj, secret)

	resolver := dspa.NewResolver()

	deps, err := resolver.Resolve(ctx, fakeClient, dspaObj)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(deps).To(HaveLen(1))
	g.Expect(deps[0].GVR).To(Equal(resources.Secret.GVR()))
	g.Expect(deps[0].Resource.GetName()).To(Equal("s3-credentials"))
}

func TestResolverWithMinioDeployment(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	dspaObj := createDSPAWithMinio("test-dspa", "default", "minio-credentials")
	secret := createSecret("minio-credentials", "default")
	pvc := createPVC("minio-test-dspa", "default")

	fakeClient := createFakeClient(t, dspaObj, secret, pvc)

	resolver := dspa.NewResolver()

	deps, err := resolver.Resolve(ctx, fakeClient, dspaObj)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(deps).To(HaveLen(2))

	secretFound := false
	pvcFound := false
	for _, dep := range deps {
		if dep.GVR == resources.Secret.GVR() {
			secretFound = true
			g.Expect(dep.Resource.GetName()).To(Equal("minio-credentials"))
		}
		if dep.GVR == resources.PersistentVolumeClaim.GVR() {
			pvcFound = true
			g.Expect(dep.Resource.GetName()).To(Equal("minio-test-dspa"))
		}
	}

	g.Expect(secretFound).To(BeTrue())
	g.Expect(pvcFound).To(BeTrue())
}

func TestResolverWithMariaDBDeployment(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	dspaObj := createDSPAWithMariaDB("test-dspa", "default", "mariadb-password")
	secret := createSecret("mariadb-password", "default")
	pvc := createPVC("mariadb-test-dspa", "default")

	fakeClient := createFakeClient(t, dspaObj, secret, pvc)

	resolver := dspa.NewResolver()

	deps, err := resolver.Resolve(ctx, fakeClient, dspaObj)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(deps).To(HaveLen(2))

	secretFound := false
	pvcFound := false
	for _, dep := range deps {
		if dep.GVR == resources.Secret.GVR() {
			secretFound = true
			g.Expect(dep.Resource.GetName()).To(Equal("mariadb-password"))
		}
		if dep.GVR == resources.PersistentVolumeClaim.GVR() {
			pvcFound = true
			g.Expect(dep.Resource.GetName()).To(Equal("mariadb-test-dspa"))
		}
	}

	g.Expect(secretFound).To(BeTrue())
	g.Expect(pvcFound).To(BeTrue())
}

func TestResolverWithExternalDB(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	dspaObj := createDSPAWithExternalDB("test-dspa", "default", "db-password")
	secret := createSecret("db-password", "default")

	fakeClient := createFakeClient(t, dspaObj, secret)

	resolver := dspa.NewResolver()

	deps, err := resolver.Resolve(ctx, fakeClient, dspaObj)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(deps).To(HaveLen(1))
	g.Expect(deps[0].GVR).To(Equal(resources.Secret.GVR()))
	g.Expect(deps[0].Resource.GetName()).To(Equal("db-password"))
}

func TestResolverWithCABundleConfigMap(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	dspaObj := createDSPAWithCABundle("test-dspa", "default", "custom-ca-bundle")
	configMap := createConfigMap("custom-ca-bundle", "default")

	fakeClient := createFakeClient(t, dspaObj, configMap)

	resolver := dspa.NewResolver()

	deps, err := resolver.Resolve(ctx, fakeClient, dspaObj)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(deps).To(HaveLen(1))
	g.Expect(deps[0].GVR).To(Equal(resources.ConfigMap.GVR()))
	g.Expect(deps[0].Resource.GetName()).To(Equal("custom-ca-bundle"))
}

func TestResolverFiltersTrustedCABundle(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	dspaObj := createDSPAWithCABundle("test-dspa", "default", "trusted-ca-bundle")

	fakeClient := createFakeClient(t, dspaObj)

	resolver := dspa.NewResolver()

	deps, err := resolver.Resolve(ctx, fakeClient, dspaObj)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(deps).To(BeEmpty())
}

func TestResolverWithCustomServerConfig(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	dspaObj := createDSPAWithCustomServerConfig("test-dspa", "default", "server-config")
	configMap := createConfigMap("server-config", "default")

	fakeClient := createFakeClient(t, dspaObj, configMap)

	resolver := dspa.NewResolver()

	deps, err := resolver.Resolve(ctx, fakeClient, dspaObj)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(deps).To(HaveLen(1))
	g.Expect(deps[0].GVR).To(Equal(resources.ConfigMap.GVR()))
	g.Expect(deps[0].Resource.GetName()).To(Equal("server-config"))
}

func TestResolverWithCustomKFPLauncher(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	dspaObj := createDSPAWithCustomKFPLauncher("test-dspa", "default", "kfp-launcher-config")
	configMap := createConfigMap("kfp-launcher-config", "default")

	fakeClient := createFakeClient(t, dspaObj, configMap)

	resolver := dspa.NewResolver()

	deps, err := resolver.Resolve(ctx, fakeClient, dspaObj)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(deps).To(HaveLen(1))
	g.Expect(deps[0].GVR).To(Equal(resources.ConfigMap.GVR()))
	g.Expect(deps[0].Resource.GetName()).To(Equal("kfp-launcher-config"))
}

func TestResolverWithAllDependencies(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	dspaObj := createDSPAWithAllDependencies("test-dspa", "default")

	s3Secret := createSecret("s3-credentials", "default")
	mariaDBSecret := createSecret("mariadb-password", "default")
	caBundle := createConfigMap("custom-ca-bundle", "default")
	serverConfig := createConfigMap("server-config", "default")
	kfpLauncher := createConfigMap("kfp-launcher-config", "default")
	mariaDBPVC := createPVC("mariadb-test-dspa", "default")

	fakeClient := createFakeClient(
		t,
		dspaObj,
		s3Secret,
		mariaDBSecret,
		caBundle,
		serverConfig,
		kfpLauncher,
		mariaDBPVC,
	)

	resolver := dspa.NewResolver()

	deps, err := resolver.Resolve(ctx, fakeClient, dspaObj)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(deps).To(HaveLen(6))

	secretCount := 0
	configMapCount := 0
	pvcCount := 0

	for _, dep := range deps {
		switch dep.GVR {
		case resources.Secret.GVR():
			secretCount++
		case resources.ConfigMap.GVR():
			configMapCount++
		case resources.PersistentVolumeClaim.GVR():
			pvcCount++
		}
	}

	g.Expect(secretCount).To(Equal(2))
	g.Expect(configMapCount).To(Equal(3))
	g.Expect(pvcCount).To(Equal(1))
}

func TestResolverWithMinimalConfiguration(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	dspaObj := createMinimalDSPA("test-dspa", "default")

	fakeClient := createFakeClient(t, dspaObj)

	resolver := dspa.NewResolver()

	deps, err := resolver.Resolve(ctx, fakeClient, dspaObj)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(deps).To(BeEmpty())
}

func TestResolverCanHandleDSPAv1(t *testing.T) {
	g := NewWithT(t)

	resolver := dspa.NewResolver()

	result := resolver.CanHandle(resources.DataSciencePipelinesApplicationV1.GVR())

	g.Expect(result).To(BeTrue())
}

func TestResolverCanHandleDSPAv1alpha1(t *testing.T) {
	g := NewWithT(t)

	resolver := dspa.NewResolver()

	result := resolver.CanHandle(resources.DataSciencePipelinesApplicationV1Alpha1.GVR())

	g.Expect(result).To(BeTrue())
}

func TestResolverCannotHandleOtherResources(t *testing.T) {
	g := NewWithT(t)

	resolver := dspa.NewResolver()

	result := resolver.CanHandle(resources.Notebook.GVR())

	g.Expect(result).To(BeFalse())
}

func TestResolverWithMissingSecret(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	dspaObj := createDSPAWithExternalStorage("test-dspa", "default", "missing-secret")

	fakeClient := createFakeClient(t, dspaObj)

	resolver := dspa.NewResolver()

	deps, err := resolver.Resolve(ctx, fakeClient, dspaObj)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(deps).To(HaveLen(1))
	g.Expect(deps[0].GVR).To(Equal(resources.Secret.GVR()))
	g.Expect(deps[0].Name).To(Equal("missing-secret"))
	g.Expect(deps[0].Resource).To(BeNil())
	g.Expect(deps[0].Error).To(HaveOccurred())
}

func TestResolverWithMinioNotDeployed(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	dspaObj := createDSPAWithMinioNotDeployed("test-dspa", "default", "s3-credentials")
	secret := createSecret("s3-credentials", "default")

	fakeClient := createFakeClient(t, dspaObj, secret)

	resolver := dspa.NewResolver()

	deps, err := resolver.Resolve(ctx, fakeClient, dspaObj)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(deps).To(HaveLen(1))
	g.Expect(deps[0].GVR).To(Equal(resources.Secret.GVR()))
}

func TestResolverWithMariaDBNotDeployed(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	dspaObj := createDSPAWithMariaDBNotDeployed("test-dspa", "default", "db-password")
	secret := createSecret("db-password", "default")

	fakeClient := createFakeClient(t, dspaObj, secret)

	resolver := dspa.NewResolver()

	deps, err := resolver.Resolve(ctx, fakeClient, dspaObj)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(deps).To(HaveLen(1))
	g.Expect(deps[0].GVR).To(Equal(resources.Secret.GVR()))
}

func createDSPAWithExternalStorage(
	name string,
	namespace string,
	secretName string,
) *unstructured.Unstructured {
	dspaObj := createBaseDSPA(name, namespace)

	dspaObj.Object["spec"] = map[string]any{
		"objectStorage": map[string]any{
			"externalStorage": map[string]any{
				"s3CredentialsSecret": map[string]any{
					"secretName": secretName,
				},
			},
		},
	}

	return dspaObj
}

func createDSPAWithMinio(
	name string,
	namespace string,
	secretName string,
) *unstructured.Unstructured {
	dspaObj := createBaseDSPA(name, namespace)

	dspaObj.Object["spec"] = map[string]any{
		"objectStorage": map[string]any{
			"minio": map[string]any{
				"deploy": true,
				"s3CredentialsSecret": map[string]any{
					"secretName": secretName,
				},
			},
		},
	}

	return dspaObj
}

func createDSPAWithMinioNotDeployed(
	name string,
	namespace string,
	secretName string,
) *unstructured.Unstructured {
	dspaObj := createBaseDSPA(name, namespace)

	dspaObj.Object["spec"] = map[string]any{
		"objectStorage": map[string]any{
			"minio": map[string]any{
				"deploy": false,
				"s3CredentialsSecret": map[string]any{
					"secretName": secretName,
				},
			},
		},
	}

	return dspaObj
}

func createDSPAWithMariaDB(
	name string,
	namespace string,
	secretName string,
) *unstructured.Unstructured {
	dspaObj := createBaseDSPA(name, namespace)

	dspaObj.Object["spec"] = map[string]any{
		"database": map[string]any{
			"mariaDB": map[string]any{
				"deploy": true,
				"passwordSecret": map[string]any{
					"name": secretName,
				},
			},
		},
	}

	return dspaObj
}

func createDSPAWithMariaDBNotDeployed(
	name string,
	namespace string,
	secretName string,
) *unstructured.Unstructured {
	dspaObj := createBaseDSPA(name, namespace)

	dspaObj.Object["spec"] = map[string]any{
		"database": map[string]any{
			"mariaDB": map[string]any{
				"deploy": false,
				"passwordSecret": map[string]any{
					"name": secretName,
				},
			},
		},
	}

	return dspaObj
}

func createDSPAWithExternalDB(
	name string,
	namespace string,
	secretName string,
) *unstructured.Unstructured {
	dspaObj := createBaseDSPA(name, namespace)

	dspaObj.Object["spec"] = map[string]any{
		"database": map[string]any{
			"externalDB": map[string]any{
				"passwordSecret": map[string]any{
					"name": secretName,
				},
			},
		},
	}

	return dspaObj
}

func createDSPAWithCABundle(
	name string,
	namespace string,
	configMapName string,
) *unstructured.Unstructured {
	dspaObj := createBaseDSPA(name, namespace)

	dspaObj.Object["spec"] = map[string]any{
		"apiServer": map[string]any{
			"cABundle": map[string]any{
				"configMapName": configMapName,
			},
		},
	}

	return dspaObj
}

func createDSPAWithCustomServerConfig(
	name string,
	namespace string,
	configMapName string,
) *unstructured.Unstructured {
	dspaObj := createBaseDSPA(name, namespace)

	dspaObj.Object["spec"] = map[string]any{
		"apiServer": map[string]any{
			"customServerConfigMap": map[string]any{
				"name": configMapName,
			},
		},
	}

	return dspaObj
}

func createDSPAWithCustomKFPLauncher(
	name string,
	namespace string,
	configMapName string,
) *unstructured.Unstructured {
	dspaObj := createBaseDSPA(name, namespace)

	dspaObj.Object["spec"] = map[string]any{
		"apiServer": map[string]any{
			"customKfpLauncherConfigMap": configMapName,
		},
	}

	return dspaObj
}

func createDSPAWithAllDependencies(
	name string,
	namespace string,
) *unstructured.Unstructured {
	dspaObj := createBaseDSPA(name, namespace)

	dspaObj.Object["spec"] = map[string]any{
		"objectStorage": map[string]any{
			"externalStorage": map[string]any{
				"s3CredentialsSecret": map[string]any{
					"secretName": "s3-credentials",
				},
			},
		},
		"database": map[string]any{
			"mariaDB": map[string]any{
				"deploy": true,
				"passwordSecret": map[string]any{
					"name": "mariadb-password",
				},
			},
		},
		"apiServer": map[string]any{
			"cABundle": map[string]any{
				"configMapName": "custom-ca-bundle",
			},
			"customServerConfigMap": map[string]any{
				"name": "server-config",
			},
			"customKfpLauncherConfigMap": "kfp-launcher-config",
		},
	}

	return dspaObj
}

func createMinimalDSPA(
	name string,
	namespace string,
) *unstructured.Unstructured {
	dspaObj := createBaseDSPA(name, namespace)

	dspaObj.Object["spec"] = map[string]any{
		"objectStorage": map[string]any{
			"externalStorage": map[string]any{
				"host": "s3.amazonaws.com",
			},
		},
	}

	return dspaObj
}

func createBaseDSPA(
	name string,
	namespace string,
) *unstructured.Unstructured {
	obj := resources.DataSciencePipelinesApplicationV1.Unstructured()
	obj.SetName(name)
	obj.SetNamespace(namespace)

	return &obj
}

func createSecret(
	name string,
	namespace string,
) *unstructured.Unstructured {
	obj := resources.Secret.Unstructured()
	obj.SetName(name)
	obj.SetNamespace(namespace)

	obj.Object["data"] = map[string]any{
		"key": "dmFsdWU=",
	}

	return &obj
}

func createConfigMap(
	name string,
	namespace string,
) *unstructured.Unstructured {
	obj := resources.ConfigMap.Unstructured()
	obj.SetName(name)
	obj.SetNamespace(namespace)

	obj.Object["data"] = map[string]any{
		"key": "value",
	}

	return &obj
}

func createPVC(
	name string,
	namespace string,
) *unstructured.Unstructured {
	obj := resources.PersistentVolumeClaim.Unstructured()
	obj.SetName(name)
	obj.SetNamespace(namespace)

	obj.Object["spec"] = map[string]any{
		"accessModes": []any{"ReadWriteOnce"},
		"resources": map[string]any{
			"requests": map[string]any{
				"storage": "10Gi",
			},
		},
	}

	return &obj
}

func createFakeClient(
	t *testing.T,
	objs ...runtime.Object,
) client.Client {
	t.Helper()

	scheme := runtime.NewScheme()
	err := corev1.AddToScheme(scheme)
	if err != nil {
		t.Fatalf("failed to add core v1 to scheme: %v", err)
	}

	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, listKinds, objs...)

	return client.NewForTesting(client.TestClientConfig{
		Dynamic: dynamicClient,
	})
}
