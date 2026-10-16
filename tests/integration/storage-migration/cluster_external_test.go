//go:build integration

package storagemigration_test

import (
	"context"
	"testing"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/opendatahub-io/odh-cli/pkg/resources"

	. "github.com/onsi/gomega"
)

const resetTimeout = 2 * time.Minute

// newExternalCluster connects to the cluster the ambient kubeconfig points at.
//
// The cluster is shared and outlives the test, so it is wiped before the test
// starts and again when it ends. Wiping deletes the DataScienceCluster CRD
// and every DataScienceCluster with it: do not point this at a cluster whose
// ODH or RHOAI installation matters.
func newExternalCluster(t *testing.T, migrationAPI bool) *cluster {
	t.Helper()
	g := NewWithT(t)

	// The same lookup the CLI does: KUBECONFIG, then ~/.kube/config.
	loader := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		clientcmd.NewDefaultClientConfigLoadingRules(),
		&clientcmd.ConfigOverrides{},
	)

	config, err := loader.ClientConfig()
	g.Expect(err).ToNot(HaveOccurred())

	t.Logf("Using external cluster %s", config.Host)

	c := newClusterFromConfig(t, config, "")

	// An external cluster cannot be reconfigured, so run only the tests that
	// match what it serves.
	if c.servesMigrationAPI(t) != migrationAPI {
		t.Skipf("Test needs storage migration API enabled=%t, which this cluster does not match", migrationAPI)
	}

	c.reset(t)

	t.Cleanup(func() {
		c.reset(t)
	})

	return c
}

// reset removes everything the tests create, whether left by this run or an
// earlier one, and waits until each resource is gone.
//
// Every delete is repeated until it reports NotFound: a delete keeps
// succeeding while the resource is terminating, so NotFound is the only proof
// that it is gone.
func (c *cluster) reset(t *testing.T) {
	t.Helper()

	g := NewWithT(t)
	g.SetDefaultEventuallyTimeout(resetTimeout)
	g.SetDefaultEventuallyPollingInterval(pollInterval)

	// Not t.Context(): that is already cancelled when cleanup functions run.
	ctx, cancel := context.WithTimeout(context.Background(), resetTimeout)
	defer cancel()

	api := c.migrationAPI(t)
	if api != nil {
		requests := c.dynamic.Resource(api.GVR())

		g.Eventually(func() error {
			return requests.Delete(ctx, migrationRequest, metav1.DeleteOptions{})
		}).Should(
			Satisfy(apierrors.IsNotFound),
		)
	}

	bindings := c.kube.RbacV1().ClusterRoleBindings()

	g.Eventually(func() error {
		return bindings.Delete(ctx, restrictedUser, metav1.DeleteOptions{})
	}).Should(
		Satisfy(apierrors.IsNotFound),
	)

	roles := c.kube.RbacV1().ClusterRoles()

	g.Eventually(func() error {
		return roles.Delete(ctx, restrictedUser, metav1.DeleteOptions{})
	}).Should(
		Satisfy(apierrors.IsNotFound),
	)

	definitions := c.crds.ApiextensionsV1().CustomResourceDefinitions()

	// Deleting the CRD also deletes every DataScienceCluster.
	g.Eventually(func() error {
		return definitions.Delete(ctx, resources.DataScienceCluster.CRDFQN(), metav1.DeleteOptions{})
	}).Should(
		Satisfy(apierrors.IsNotFound),
	)
}
