//go:build integration

package storagemigration_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	kindconfig "sigs.k8s.io/kind/pkg/apis/config/v1alpha4"
	kindcluster "sigs.k8s.io/kind/pkg/cluster"

	"k8s.io/client-go/tools/clientcmd"

	"github.com/opendatahub-io/odh-cli/pkg/resources"

	. "github.com/onsi/gomega"
)

// newKindCluster starts a Kind cluster and deletes it when the test ends.
func newKindCluster(t *testing.T, migrationAPI bool) *cluster {
	t.Helper()
	g := NewWithT(t)

	provider := kindcluster.NewProvider()
	name := fmt.Sprintf("odh-storage-%d", time.Now().UnixNano())
	kubeconfig := filepath.Join(t.TempDir(), "kubeconfig")

	// Registered before creation so a partial startup is cleaned up too.
	t.Cleanup(func() {
		deleteKindCluster(t, provider, name, kubeconfig)
	})

	t.Logf("Creating Kind cluster %s (%s), storage migration API enabled=%t", name, kindNodeImage, migrationAPI)

	err := provider.Create(name,
		kindcluster.CreateWithV1Alpha4Config(kindConfig(migrationAPI)),
		kindcluster.CreateWithNodeImage(kindNodeImage),
		kindcluster.CreateWithKubeconfigPath(kubeconfig),
		kindcluster.CreateWithWaitForReady(clusterReadyTimeout),
	)
	g.Expect(err).ToNot(HaveOccurred())

	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	g.Expect(err).ToNot(HaveOccurred())

	return newClusterFromConfig(t, config, kubeconfig)
}

// kindConfig turns the upstream storage migration API and its controller on or off.
func kindConfig(migrationAPI bool) *kindconfig.Cluster {
	return &kindconfig.Cluster{
		Nodes: []kindconfig.Node{
			{
				Role: kindconfig.ControlPlaneRole,
			},
		},
		FeatureGates: map[string]bool{
			"StorageVersionMigrator": migrationAPI,
			// The migrator controller needs both of these.
			"InformerResourceVersion": true,
			"InOrderInformers":        true,
		},
		RuntimeConfig: map[string]string{
			resources.StorageVersionMigrationBeta.APIVersion(): strconv.FormatBool(migrationAPI),
		},
	}
}

// deleteKindCluster removes the cluster, saving its logs first if the test failed.
func deleteKindCluster(t *testing.T, provider *kindcluster.Provider, name string, kubeconfig string) {
	t.Helper()

	if t.Failed() {
		dir := os.Getenv(artifactsEnv)
		if dir == "" {
			dir = os.TempDir()
		}

		dir = filepath.Join(dir, name)

		err := provider.CollectLogs(name, dir)
		if err != nil {
			t.Logf("Collecting Kind logs: %v", err)
		}

		t.Logf("Kind logs: %s", dir)
	}

	err := provider.Delete(name, kubeconfig)
	if err != nil {
		t.Errorf("Deleting Kind cluster %s: %v", name, err)
	}
}
