//go:build integration

package storagemigration_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	rbacv1 "k8s.io/api/rbac/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/jq"

	. "github.com/onsi/gomega"
)

const (
	kindNodeImage = "kindest/node:v1.35.0@sha256:452d707d4862f52530247495d180205e029056831160e22870e37e3f6c1ac31f"

	// clusterEnv selects the cluster the tests run on.
	clusterEnv      = "STORAGE_MIGRATION_CLUSTER"
	clusterKind     = "kind"
	clusterExternal = "external"

	// artifactsEnv names the directory that receives the Kind logs of a failed run.
	artifactsEnv = "STORAGE_MIGRATION_ARTIFACTS"

	withMigrationAPI    = true
	withoutMigrationAPI = false

	clusterReadyTimeout = 3 * time.Minute
	apiTimeout          = 30 * time.Second
	servedTimeout       = time.Minute
	pollInterval        = time.Second

	// The one DataScienceCluster the tests create, and the values they expect
	// to find on it after every migration.
	objectName    = "default-dsc"
	marker        = "preserve-me"
	sourceRelease = "3.5.0"
	releasePatch  = `{"status":{"release":{"version":"3.5.0"}}}`

	restrictedUser = "storage-migration-reader"
)

// cluster is a Kubernetes cluster the tests can use, plus the clients for it.
type cluster struct {
	// kubeconfig is the file to connect with. It is empty for an external
	// cluster, which is reached through the ambient kubeconfig.
	kubeconfig string

	crds      *apiextensionsclient.Clientset
	dynamic   dynamic.Interface
	discovery *discovery.DiscoveryClient
	kube      *kubernetes.Clientset
}

// newCluster returns the cluster a test runs on: a new Kind cluster by
// default, or the one the ambient kubeconfig points at when clusterEnv is
// "external". The test is skipped if an external cluster does not match the
// requested migration API setting.
func newCluster(t *testing.T, migrationAPI bool) *cluster {
	t.Helper()

	kind := os.Getenv(clusterEnv)

	switch kind {
	case "", clusterKind:
		return newKindCluster(t, migrationAPI)
	case clusterExternal:
		return newExternalCluster(t, migrationAPI)
	default:
		t.Fatalf("Unknown %s %q; use %s or %s", clusterEnv, kind, clusterKind, clusterExternal)

		return nil
	}
}

func newClusterFromConfig(t *testing.T, config *rest.Config, kubeconfig string) *cluster {
	t.Helper()
	g := NewWithT(t)

	config.Timeout = apiTimeout

	crds, err := apiextensionsclient.NewForConfig(config)
	g.Expect(err).ToNot(HaveOccurred())

	dyn, err := dynamic.NewForConfig(config)
	g.Expect(err).ToNot(HaveOccurred())

	disco, err := discovery.NewDiscoveryClientForConfig(config)
	g.Expect(err).ToNot(HaveOccurred())

	kube, err := kubernetes.NewForConfig(config)
	g.Expect(err).ToNot(HaveOccurred())

	return &cluster{
		kubeconfig: kubeconfig,
		crds:       crds,
		dynamic:    dyn,
		discovery:  disco,
		kube:       kube,
	}
}

// migrationAPI returns the StorageVersionMigration API the cluster serves,
// trying the same candidates as the action, or nil if it serves none.
func (c *cluster) migrationAPI(t *testing.T) *resources.ResourceType {
	t.Helper()

	candidates := []resources.ResourceType{
		resources.StorageVersionMigration,
		resources.StorageVersionMigrationBeta,
		resources.StorageVersionMigrationAlpha,
		resources.OpenShiftStorageVersionMigration,
	}

	for _, candidate := range candidates {
		_, err := c.discovery.ServerResourcesForGroupVersion(candidate.APIVersion())

		switch {
		case apierrors.IsNotFound(err):
			continue
		default:
			NewWithT(t).Expect(err).ToNot(HaveOccurred())

			return &candidate
		}
	}

	return nil
}

func (c *cluster) servesMigrationAPI(t *testing.T) bool {
	t.Helper()

	return c.migrationAPI(t) != nil
}

// newCRD builds a minimal DataScienceCluster CRD serving the given versions.
// The last one is the storage version.
func newCRD(versions ...string) *apiextensionsv1.CustomResourceDefinition {
	preserveUnknownFields := true

	crd := &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name: resources.DataScienceCluster.CRDFQN(),
		},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group: resources.DataScienceCluster.Group,
			Scope: apiextensionsv1.ClusterScoped,
			Names: apiextensionsv1.CustomResourceDefinitionNames{
				Kind:     resources.DataScienceCluster.Kind,
				Plural:   resources.DataScienceCluster.Resource,
				Singular: "datasciencecluster",
			},
		},
	}

	for i, name := range versions {
		crd.Spec.Versions = append(crd.Spec.Versions, apiextensionsv1.CustomResourceDefinitionVersion{
			Name:    name,
			Served:  true,
			Storage: i == len(versions)-1,
			Subresources: &apiextensionsv1.CustomResourceSubresources{
				Status: &apiextensionsv1.CustomResourceSubresourceStatus{},
			},
			// Identical open schemas in every version, so no conversion webhook is needed.
			Schema: &apiextensionsv1.CustomResourceValidation{
				OpenAPIV3Schema: &apiextensionsv1.JSONSchemaProps{
					Type:                   "object",
					XPreserveUnknownFields: &preserveUnknownFields,
				},
			},
		})
	}

	return crd
}

// installCRD creates the CRD, or replaces its versions, and waits until every
// version is served. The last version listed is the storage version.
func (c *cluster) installCRD(t *testing.T, versions ...string) {
	t.Helper()
	g := NewWithT(t)
	ctx := t.Context()

	g.Expect(c.applyCRD(ctx, versions...)).To(Succeed())

	g.Eventually(func(g Gomega) {
		for _, version := range versions {
			_, err := c.dscs(version).List(ctx, metav1.ListOptions{})
			g.Expect(err).ToNot(HaveOccurred())
		}
	}).WithContext(ctx).WithTimeout(servedTimeout).WithPolling(pollInterval).Should(Succeed())
}

// expectCRDUpdateRejected checks the API server refuses to switch the CRD to
// the given versions, which it does while a dropped version is still recorded
// in storedVersions.
func (c *cluster) expectCRDUpdateRejected(t *testing.T, versions ...string) {
	t.Helper()

	err := c.applyCRD(t.Context(), versions...)

	NewWithT(t).Expect(apierrors.IsInvalid(err)).To(BeTrue(), "CRD update should be rejected, got: %v", err)
}

func (c *cluster) applyCRD(ctx context.Context, versions ...string) error {
	crd := newCRD(versions...)
	definitions := c.crds.ApiextensionsV1().CustomResourceDefinitions()

	existing, err := definitions.Get(ctx, crd.Name, metav1.GetOptions{})

	switch {
	case apierrors.IsNotFound(err):
		_, err = definitions.Create(ctx, crd, metav1.CreateOptions{})
	case err == nil:
		existing.Spec = crd.Spec
		_, err = definitions.Update(ctx, existing, metav1.UpdateOptions{})
	}

	if err != nil {
		return fmt.Errorf("applying DataScienceCluster CRD: %w", err)
	}

	return nil
}

func (c *cluster) expectStoredVersions(t *testing.T, versions ...string) {
	t.Helper()
	g := NewWithT(t)

	definitions := c.crds.ApiextensionsV1().CustomResourceDefinitions()

	crd, err := definitions.Get(t.Context(), resources.DataScienceCluster.CRDFQN(), metav1.GetOptions{})

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(crd.Status.StoredVersions).To(ConsistOf(versions))
}

// dscs returns a client for DataScienceClusters at the given API version.
func (c *cluster) dscs(version string) dynamic.ResourceInterface {
	rt := resources.DataScienceCluster
	rt.Version = version

	return c.dynamic.Resource(rt.GVR())
}

func (c *cluster) createDSC(t *testing.T, version string) {
	t.Helper()
	g := NewWithT(t)
	ctx := t.Context()

	rt := resources.DataScienceCluster
	rt.Version = version

	dsc := &unstructured.Unstructured{
		Object: map[string]any{
			"spec": map[string]any{
				"marker": marker,
			},
		},
	}

	dsc.SetGroupVersionKind(rt.GVK())
	dsc.SetName(objectName)

	_, err := c.dscs(version).Create(ctx, dsc, metav1.CreateOptions{})
	g.Expect(err).ToNot(HaveOccurred())

	// The CLI detects the installed release from the DSC status.
	_, err = c.dscs(version).Patch(ctx, objectName, types.MergePatchType, []byte(releasePatch), metav1.PatchOptions{}, "status")
	g.Expect(err).ToNot(HaveOccurred())
}

// resourceVersion changes on every write, so comparing two readings tells
// whether the object was rewritten in between.
func (c *cluster) resourceVersion(t *testing.T, version string) string {
	t.Helper()

	dsc, err := c.dscs(version).Get(t.Context(), objectName, metav1.GetOptions{})
	NewWithT(t).Expect(err).ToNot(HaveOccurred())

	return dsc.GetResourceVersion()
}

// expectDataPreserved checks the object still carries its original values when
// read through each of the given API versions.
func (c *cluster) expectDataPreserved(t *testing.T, versions ...string) {
	t.Helper()
	g := NewWithT(t)

	for _, version := range versions {
		dsc, err := c.dscs(version).Get(t.Context(), objectName, metav1.GetOptions{})
		g.Expect(err).ToNot(HaveOccurred())

		g.Expect(jq.Query[string](dsc, ".spec.marker")).To(Equal(marker))
		g.Expect(jq.Query[string](dsc, ".status.release.version")).To(Equal(sourceRelease))
	}
}

// createRestrictedUser grants restrictedUser read access to the CRD and the
// DataScienceClusters, and nothing else.
func (c *cluster) createRestrictedUser(t *testing.T) {
	t.Helper()
	g := NewWithT(t)
	ctx := t.Context()

	role := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: restrictedUser,
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{resources.DataScienceCluster.Group},
				Resources: []string{resources.DataScienceCluster.Resource},
				Verbs:     []string{"get", "list"},
			},
			{
				APIGroups: []string{resources.CustomResourceDefinition.Group},
				Resources: []string{resources.CustomResourceDefinition.Resource},
				Verbs:     []string{"get"},
			},
		},
	}

	binding := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: restrictedUser,
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:     rbacv1.UserKind,
				APIGroup: rbacv1.GroupName,
				Name:     restrictedUser,
			},
		},
		RoleRef: rbacv1.RoleRef{
			Kind:     "ClusterRole",
			APIGroup: rbacv1.GroupName,
			Name:     restrictedUser,
		},
	}

	_, err := c.kube.RbacV1().ClusterRoles().Create(ctx, role, metav1.CreateOptions{})
	g.Expect(err).ToNot(HaveOccurred())

	_, err = c.kube.RbacV1().ClusterRoleBindings().Create(ctx, binding, metav1.CreateOptions{})
	g.Expect(err).ToNot(HaveOccurred())
}
