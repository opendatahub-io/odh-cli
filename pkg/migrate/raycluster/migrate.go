package raycluster

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"

	"github.com/lburgazzoli/odh-cli/pkg/resources"
	"github.com/lburgazzoli/odh-cli/pkg/util/client"
	"github.com/lburgazzoli/odh-cli/pkg/util/confirmation"
	"github.com/lburgazzoli/odh-cli/pkg/util/iostreams"
	"sigs.k8s.io/yaml"
)

// PostUpgradeResult holds counts after post-upgrade.
type PostUpgradeResult struct {
	Migrated int
	Skipped  int
	Failed   int
}

// routeWaitMaxAttempts is the number of polling attempts when waiting for cluster route (same style as deletion wait).
const routeWaitMaxAttempts = 60

// routeWaitInterval is the delay between route polling attempts.
const routeWaitInterval = 2 * time.Second

// deletionWaitMaxAttempts and deletionWaitInterval match the deletion poll loop (same as original 60 * 2s).
const deletionWaitMaxAttempts = 60
const deletionWaitInterval = 2 * time.Second

// waitForDeletion polls until the cluster is not found or ctx is cancelled.
// It logs "Waiting for cluster deletion to complete..." and "Cluster deleted successfully" via io.
// Returns (true, nil) when deleted, (false, err) on timeout or context cancel.
func waitForDeletion(ctx context.Context, dyn dynamic.NamespaceableResourceInterface, ns, name string, io iostreams.Interface) (deleted bool, err error) {
	io.Errorf("  [%s] Waiting for cluster deletion to complete...", name)
	for i := 0; i < deletionWaitMaxAttempts; i++ {
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		default:
		}
		_, getErr := dyn.Namespace(ns).Get(ctx, name, metav1.GetOptions{})
		if apierrors.IsNotFound(getErr) {
			io.Errorf("  [%s] Cluster deleted successfully", name)
			return true, nil
		}
		if getErr != nil {
			return false, getErr
		}
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		case <-time.After(deletionWaitInterval):
		}
	}
	return false, fmt.Errorf("timeout waiting for cluster deletion")
}

// PostUpgrade runs post-upgrade migration: either live (in-place update) or from backup (delete + create).
func PostUpgrade(
	ctx context.Context,
	c client.Client,
	clusterName string,
	namespace string,
	dryRun bool,
	skipConfirm bool,
	fromBackup string,
	io iostreams.Interface,
) (PostUpgradeResult, error) {
	if fromBackup != "" {
		return postUpgradeFromBackup(ctx, c, fromBackup, clusterName, namespace, dryRun, skipConfirm, io)
	}

	return postUpgradeLive(ctx, c, clusterName, namespace, dryRun, skipConfirm, io)
}

// waitForClusterRoute polls GetClusterRoute until a non-empty URL is returned or the attempt limit is reached.
// It logs "Waiting for cluster to become ready..." once and returns the dashboard URL or "" on timeout.
func waitForClusterRoute(ctx context.Context, c client.Client, name, ns string, io iostreams.Interface) string {
	io.Errorf("  [%s] Waiting for cluster to become ready...", name)
	for i := 0; i < routeWaitMaxAttempts; i++ {
		url := GetClusterRoute(ctx, c, name, ns)
		if url != "" {
			return url
		}
		time.Sleep(routeWaitInterval)
	}
	return ""
}

func postUpgradeLive(
	ctx context.Context,
	c client.Client,
	clusterName string,
	namespace string,
	dryRun bool,
	skipConfirm bool,
	io iostreams.Interface,
) (PostUpgradeResult, error) {
	if clusterName != "" && namespace == "" {
		return PostUpgradeResult{}, fmt.Errorf("namespace must be specified when migrating a specific cluster")
	}

	scopeMsg := "all clusters across all namespaces"
	if clusterName != "" && namespace != "" {
		scopeMsg = fmt.Sprintf("cluster '%s' in namespace '%s'", clusterName, namespace)
	} else if namespace != "" {
		scopeMsg = fmt.Sprintf("all clusters in namespace '%s'", namespace)
	}

	if dryRun {
		io.Errorf("=== DRY RUN MODE - No changes will be made ===")
		io.Errorf("")
	}

	io.Errorf("Fetching RayClusters (%s)...", scopeMsg)
	clusters, err := GetClusters(ctx, c, clusterName, namespace)
	if err != nil {
		return PostUpgradeResult{}, err
	}
	if len(clusters) == 0 {
		io.Errorf("No RayClusters found to migrate")
		return PostUpgradeResult{Migrated: 0, Skipped: 0, Failed: 0}, nil
	}

	io.Errorf("Found %d RayCluster(s)", len(clusters))
	io.Errorf("")
	io.Errorf("Analyzing clusters for migration status...")

	total := len(clusters)
	var toMigrate, alreadyMigrated []*unstructured.Unstructured
	for idx, rc := range clusters {
		name := rc.GetName()
		ns := rc.GetNamespace()
		if ns == "" {
			ns = "default"
		}
		isMigrated, _ := IsClusterMigrated(rc)
		if isMigrated {
			alreadyMigrated = append(alreadyMigrated, rc)
			io.Errorf("  [%d/%d] Checking %s (ns: %s)... already migrated", idx+1, total, name, ns)
		} else {
			toMigrate = append(toMigrate, rc)
			io.Errorf("  [%d/%d] Checking %s (ns: %s)... needs migration", idx+1, total, name, ns)
		}
	}

	io.Errorf("")
	io.Errorf("Summary: %d to migrate, %d already migrated", len(toMigrate), len(alreadyMigrated))

	if len(toMigrate) == 0 {
		io.Errorf("")
		io.Errorf("All clusters are already migrated. Nothing to do.")
		return PostUpgradeResult{Skipped: len(alreadyMigrated)}, nil
	}

	if !dryRun && !skipConfirm {
		if clusterName == "" {
			io.Errorf("")
			io.Errorf("============================================================")
			if namespace != "" {
				io.Errorf("WARNING: You are about to migrate ALL clusters in namespace '%s'", namespace)
			} else {
				io.Errorf("WARNING: You are about to migrate ALL clusters across ALL namespaces")
			}
			io.Errorf("============================================================")
		}
		io.Errorf("")
		io.Errorf("The following %d cluster(s) will be migrated:", len(toMigrate))
		for _, rc := range toMigrate {
			ns := rc.GetNamespace()
			if ns == "" {
				ns = "default"
			}
			io.Errorf("  - %s (namespace: %s)", rc.GetName(), ns)
		}
		io.Errorf("")
		io.Errorf("IMPORTANT: Migration will cause temporary downtime for each RayCluster.")
		io.Errorf("  - Pods will be restarted as the KubeRay operator recreates them with the new configuration.")
		io.Errorf("  - Existing job state and logs will be lost.")
		io.Errorf("  - Currently running workloads/jobs will be interrupted and progress lost.")
		io.Errorf("")
		if !confirmation.Prompt(io, "Proceed with migration?") {
			io.Errorf("Migration cancelled.")
			return PostUpgradeResult{Skipped: len(alreadyMigrated)}, nil
		}
		io.Errorf("")
	}

	gvr := resources.RayCluster.GVR()
	dyn := c.Dynamic().Resource(gvr)
	var migrated, failed int

	for _, rc := range toMigrate {
		name := rc.GetName()
		ns := rc.GetNamespace()
		if ns == "" {
			ns = "default"
		}

		if dryRun {
			io.Errorf("  [DRY RUN] Would migrate: %s (ns: %s)", name, ns)
			migrated++
			continue
		}

		io.Errorf("  [%s] Fetching current cluster state...", name)
		// Get latest
		latest, err := dyn.Namespace(ns).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			io.Errorf("  [FAIL] %s (ns: %s): %v", name, ns, err)
			failed++
			continue
		}

		cleaned := latest.DeepCopy()
		RemoveAutogeneratedFields(cleaned.Object)
		ProcessRayClusterYAML(cleaned)
		// Restore resourceVersion and uid for update
		if rv := latest.GetResourceVersion(); rv != "" {
			cleaned.SetResourceVersion(rv)
		}
		if uid := latest.GetUID(); uid != "" {
			cleaned.SetUID(uid)
		}

		// Clean old CodeFlare ServiceAccounts
		io.Errorf("  [%s] Cleaning up old CodeFlare ServiceAccounts...", name)
		cleanupOldServiceAccounts(ctx, c, ns, name, io)

		io.Errorf("  [%s] Applying migration changes...", name)
		_, err = dyn.Namespace(ns).Update(ctx, cleaned, metav1.UpdateOptions{})
		if err != nil {
			io.Errorf("  [FAIL] %s (ns: %s): %v", name, ns, err)
			failed++
			continue
		}

		url := waitForClusterRoute(ctx, c, name, ns, io)
		if url != "" {
			io.Errorf("  [OK] Migrated: %s (ns: %s)", name, ns)
			io.Errorf("       Dashboard: %s", url)
		} else {
			io.Errorf("  [OK] Migrated: %s (ns: %s) - Dashboard route not yet available (timeout)", name, ns)
		}
		migrated++
	}

	io.Errorf("")
	io.Errorf("============================================================")
	io.Errorf("Migration Summary:")
	io.Errorf("  Migrated: %d", migrated)
	io.Errorf("  Skipped (already migrated): %d", len(alreadyMigrated))
	io.Errorf("  Failed: %d", failed)

	return PostUpgradeResult{Migrated: migrated, Skipped: len(alreadyMigrated), Failed: failed}, nil
}

func cleanupOldServiceAccounts(ctx context.Context, c client.Client, namespace, clusterName string, io iostreams.Interface) {
	sas, err := c.List(ctx, resources.ServiceAccount, client.WithNamespace(namespace))
	if err != nil {
		return
	}
	prefix := clusterName + "-oauth-proxy-"
	kuberaySA := clusterName + "-oauth-proxy-sa"
	for _, sa := range sas {
		name := sa.GetName()
		if len(name) >= len(prefix) && name[:len(prefix)] == prefix && name != kuberaySA {
			_ = c.Dynamic().Resource(resources.ServiceAccount.GVR()).Namespace(namespace).Delete(ctx, name, metav1.DeleteOptions{})
			io.Errorf("  [%s] Deleting old ServiceAccount: %s", clusterName, name)
		}
	}
}

func postUpgradeFromBackup(
	ctx context.Context,
	c client.Client,
	backupPath string,
	clusterName string,
	namespace string,
	dryRun bool,
	skipConfirm bool,
	io iostreams.Interface,
) (PostUpgradeResult, error) {
	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		return PostUpgradeResult{}, fmt.Errorf("backup path does not exist: %s", backupPath)
	}

	var yamlFiles []string
	if info, _ := os.Stat(backupPath); info != nil && info.Mode().IsRegular() {
		if filepath.Ext(backupPath) != ".yaml" && filepath.Ext(backupPath) != ".yml" {
			return PostUpgradeResult{}, fmt.Errorf("backup file must be YAML: %s", backupPath)
		}
		yamlFiles = []string{backupPath}
	} else {
		entries, _ := os.ReadDir(backupPath)
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			name := e.Name()
			if filepath.Ext(name) == ".yaml" || filepath.Ext(name) == ".yml" {
				yamlFiles = append(yamlFiles, filepath.Join(backupPath, name))
			}
		}
		if len(yamlFiles) == 0 {
			rhoai3 := filepath.Join(backupPath, BackupSubdirRHOAI3x)
			if st, _ := os.Stat(rhoai3); st != nil && st.IsDir() {
				io.Errorf("No YAML files in '%s', using '%s' subdirectory...", backupPath, rhoai3)
				entries, _ := os.ReadDir(rhoai3)
				for _, e := range entries {
					name := e.Name()
					if filepath.Ext(name) == ".yaml" || filepath.Ext(name) == ".yml" {
						yamlFiles = append(yamlFiles, filepath.Join(rhoai3, name))
					}
				}
			}
		}
	}

	if len(yamlFiles) == 0 {
		io.Errorf("No YAML files found in: %s", backupPath)
		if info, _ := os.Stat(backupPath); info != nil && info.IsDir() {
			entries, _ := os.ReadDir(backupPath)
			var subdirs []string
			for _, e := range entries {
				if e.IsDir() {
					subdirs = append(subdirs, e.Name())
				}
			}
			if len(subdirs) > 0 {
				io.Errorf("Found subdirectories: %s", strings.Join(subdirs, ", "))
				io.Errorf("Hint: Use --from-backup %s/rhoai-3.x for RHOAI 3.x migration", backupPath)
				io.Errorf("      or --from-backup %s/rhoai-2.x for RHOAI 2.x rollback", backupPath)
			}
		}
		return PostUpgradeResult{}, nil
	}

	type backupItem struct {
		u    *unstructured.Unstructured
		file string
	}
	var toApply []backupItem
	for _, f := range yamlFiles {
		data, err := os.ReadFile(f)
		if err != nil {
			io.Errorf("failed to read file %s: %v", f, err)
			continue
		}
		var obj map[string]any
		if err := yaml.Unmarshal(data, &obj); err != nil {
			io.Errorf("failed to parse YAML %s: %v", f, err)
			continue
		}
		kind, _ := obj["kind"].(string)
		if kind != "RayCluster" {
			continue
		}
		meta, _ := obj["metadata"].(map[string]any)
		name, _ := meta["name"].(string)
		ns, _ := meta["namespace"].(string)
		if ns == "" {
			ns = "default"
		}
		if clusterName != "" && name != clusterName {
			continue
		}
		if namespace != "" && ns != namespace {
			continue
		}
		u := &unstructured.Unstructured{Object: obj}
		toApply = append(toApply, backupItem{u: u, file: f})
	}

	if len(toApply) == 0 {
		io.Errorf("No matching RayClusters found in backup")
		return PostUpgradeResult{}, nil
	}

	scopeMsg := "all namespaces"
	if clusterName != "" && namespace != "" {
		scopeMsg = fmt.Sprintf("cluster '%s' in namespace '%s'", clusterName, namespace)
	} else if namespace != "" {
		scopeMsg = fmt.Sprintf("namespace '%s'", namespace)
	}

	if dryRun {
		io.Errorf("=== DRY RUN MODE - No changes will be made ===")
		io.Errorf("")
	}

	io.Errorf("Found %d RayCluster(s) in backup to migrate (%s):", len(toApply), scopeMsg)
	io.Errorf("")
	for _, item := range toApply {
		name := item.u.GetName()
		ns := item.u.GetNamespace()
		if ns == "" {
			ns = "default"
		}
		io.Errorf("  - %s (ns: %s) from %s", name, ns, filepath.Base(item.file))
	}

	if !dryRun && !skipConfirm {
		io.Errorf("")
		io.Errorf("WARNING: Restore from backup will DELETE and RECREATE each RayCluster.")
		io.Errorf("  - If a cluster currently exists, it will be deleted first.")
		io.Errorf("  - All running pods, jobs, and workloads will be terminated.")
		io.Errorf("  - Existing job state and logs will be lost.")
		io.Errorf("  - The cluster will be recreated from the backup configuration.")
		io.Errorf("")
		if !confirmation.Prompt(io, "Proceed with restore from backup?") {
			io.Errorf("Restore cancelled.")
			return PostUpgradeResult{}, nil
		}
		io.Errorf("")
	}

	gvr := resources.RayCluster.GVR()
	dyn := c.Dynamic().Resource(gvr)
	var migrated, failed int

	for _, item := range toApply {
		u := item.u
		name := u.GetName()
		ns := u.GetNamespace()
		if ns == "" {
			ns = "default"
		}

		if dryRun {
			io.Errorf("  [DRY RUN] Would restore from backup: %s (ns: %s)", name, ns)
			io.Errorf("            (will delete existing cluster if present, then create from backup)")
			migrated++
			continue
		}

		_, err := dyn.Namespace(ns).Get(ctx, name, metav1.GetOptions{})
		if err == nil {
			io.Errorf("  [%s] Deleting existing cluster...", name)
			if err := dyn.Namespace(ns).Delete(ctx, name, metav1.DeleteOptions{}); err != nil {
				io.Errorf("  [FAIL] %s (ns: %s): delete failed: %v", name, ns, err)
				failed++
				continue
			}
			deleted, err := waitForDeletion(ctx, dyn, ns, name, io)
			if !deleted {
				io.Errorf("  [FAIL] %s (ns: %s): %v", name, ns, err)
				failed++
				continue
			}
		}

		io.Errorf("  [%s] Creating cluster from backup...", name)
		RemoveKueueWorkloadAnnotations(u)
		u.SetResourceVersion("")
		u.SetUID("")
		_, err = dyn.Namespace(ns).Create(ctx, u, metav1.CreateOptions{})
		if err != nil {
			io.Errorf("  [FAIL] %s (ns: %s): %v", name, ns, err)
			failed++
			continue
		}

		url := waitForClusterRoute(ctx, c, name, ns, io)
		if url != "" {
			io.Errorf("  [OK] Restored from backup: %s (ns: %s)", name, ns)
			io.Errorf("       Dashboard: %s", url)
		} else {
			io.Errorf("  [OK] Restored from backup: %s (ns: %s)", name, ns)
			io.Errorf("       Dashboard: Route not yet available (timeout)")
		}
		migrated++
	}

	io.Errorf("")
	io.Errorf("============================================================")
	if dryRun {
		io.Errorf("DRY RUN Summary:")
		io.Errorf("  Would restore: %d", migrated)
	} else {
		io.Errorf("Restore from Backup Summary:")
		io.Errorf("  Restored: %d", migrated)
	}
	io.Errorf("  Failed: %d", failed)

	return PostUpgradeResult{Migrated: migrated, Failed: failed}, nil
}
