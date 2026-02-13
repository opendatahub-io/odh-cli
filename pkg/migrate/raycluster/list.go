package raycluster

import (
	"context"
	"encoding/json"
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/lburgazzoli/odh-cli/pkg/util/client"

	"github.com/lburgazzoli/odh-cli/pkg/util/iostreams"
	"github.com/lburgazzoli/odh-cli/pkg/util/jq"
	"sigs.k8s.io/yaml"
)

// ClusterInfo holds display info for one RayCluster.
type ClusterInfo struct {
	Name              string   `json:"name"`
	Namespace         string   `json:"namespace"`
	Status            string   `json:"status"`
	NumWorkers        int64    `json:"num_workers"`
	Migrated          bool     `json:"migrated"`
	MigrationStatus   string   `json:"migration_status"`
	TLSOAuthComponents []string `json:"tls_oauth_components,omitempty"`
}

// ListRayClusters lists RayClusters with migration status and writes to io.
// Progress messages (Fetching, Found, Analyzing, Analysis complete) are written to stderr
// to match the codeflare-sdk script; table/json/yaml output goes to stdout.
func ListRayClusters(
	ctx context.Context,
	c client.Client,
	namespace string,
	outputFormat string,
	io iostreams.Interface,
) ([]ClusterInfo, error) {
	scopeMsg := "all namespaces"
	if namespace != "" {
		scopeMsg = "namespace '" + namespace + "'"
	}
	io.Errorf("Fetching RayClusters (%s)...", scopeMsg)

	clusters, err := GetClusters(ctx, c, "", namespace)
	if err != nil {
		return nil, err
	}

	if len(clusters) == 0 {
		io.Errorf("No RayClusters found")
		return nil, nil
	}

	io.Errorf("Found %d RayCluster(s)", len(clusters))
	io.Errorf("Analyzing clusters...")

	total := len(clusters)
	var infos []ClusterInfo
	for idx, rc := range clusters {
		name := rc.GetName()
		ns := rc.GetNamespace()
		if ns == "" {
			ns = "default"
		}
		io.Errorf("  [%d/%d] Analyzing %s (ns: %s)...", idx+1, total, name, ns)

		info := clusterInfoFrom(rc)
		infos = append(infos, info)
	}

	io.Errorf("Analysis complete.")
	io.Errorf("")

	switch outputFormat {
	case "yaml":
		b, err := yaml.Marshal(infos)
		if err != nil {
			io.Errorf("failed to marshal output: %v", err)
			return nil, fmt.Errorf("marshal YAML: %w", err)
		}
		io.Fprintf("%s", string(b))
	case "json":
		b, err := json.MarshalIndent(infos, "", "  ")
		if err != nil {
			io.Errorf("failed to marshal output: %v", err)
			return nil, fmt.Errorf("marshal JSON: %w", err)
		}
		io.Fprintf("%s", string(b))
	default:
		printTable(infos, io)
	}

	return infos, nil
}

func clusterInfoFrom(rc *unstructured.Unstructured) ClusterInfo {
	name := rc.GetName()
	ns := rc.GetNamespace()
	if ns == "" {
		ns = "default"
	}
	status, _ := jq.Query[string](rc, ".status.state")
	if status == "" {
		status = "unknown"
	}
	migrated, migrationStatus := IsClusterMigrated(rc)
	_, components := HasTLSOAuthComponents(rc)

	numWorkers := int64(0)
	if workers, ok, _ := unstructured.NestedSlice(rc.Object, "spec", "workerGroupSpecs"); ok && len(workers) > 0 {
		if w, ok := workers[0].(map[string]any); ok {
			switch r := w["replicas"].(type) {
			case int64:
				numWorkers = r
			case int:
				numWorkers = int64(r)
			case int32:
				numWorkers = int64(r)
			case float64:
				numWorkers = int64(r)
			}
		}
	}

	return ClusterInfo{
		Name:                name,
		Namespace:           ns,
		Status:              status,
		NumWorkers:          numWorkers,
		Migrated:            migrated,
		MigrationStatus:     migrationStatus,
		TLSOAuthComponents:  components,
	}
}

func printTable(infos []ClusterInfo, io iostreams.Interface) {
	io.Fprintf("RayCluster Migration Status:")
	io.Fprintf("")
	io.Fprintf("%-25s %-18s %-12s %-8s %-30s", "Name", "Namespace", "Status", "Workers", "Migration Status")
	io.Fprintf("%s", "----------------------------------------------------------------------------------------------------")
	var migratedCount, needsCount int
	for _, c := range infos {
		indicator := "[NEEDS MIGRATION]"
		if c.Migrated {
			indicator = "[MIGRATED]"
			migratedCount++
		} else {
			needsCount++
		}
		io.Fprintf("%-25s %-18s %-12s %-8d %s", c.Name, c.Namespace, c.Status, c.NumWorkers, indicator)
	}
	io.Fprintf("")
	io.Fprintf("Migration Summary: %d migrated, %d need migration", migratedCount, needsCount)
}