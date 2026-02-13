package raycluster

import (
	"context"
	"fmt"

	"github.com/lburgazzoli/odh-cli/pkg/resources"
	"github.com/lburgazzoli/odh-cli/pkg/util/client"
	"github.com/lburgazzoli/odh-cli/pkg/util/jq"
)

// GetClusterRoute returns the dashboard URL for a RayCluster (HTTPS), or empty string if not found.
func GetClusterRoute(ctx context.Context, c client.Client, clusterName, namespace string) string {
	labelSelector := fmt.Sprintf("ray.io/cluster-name=%s,ray.io/cluster-namespace=%s", clusterName, namespace)

	// Try cluster-wide HTTPRoute list first
	list, err := c.List(ctx, resources.HTTPRoute, client.WithLabelSelector(labelSelector))
	if err != nil || len(list) == 0 {
		// Fallback: search in common namespaces
		for _, ns := range []string{"redhat-ods-applications", "opendatahub", "default", "ray-system"} {
			list, err = c.List(ctx, resources.HTTPRoute, client.WithNamespace(ns), client.WithLabelSelector(labelSelector))
			if err != nil || len(list) == 0 {
				continue
			}
			break
		}
	}
	if len(list) == 0 {
		return ""
	}

	httpRoute := list[0]
	gatewayName, err := jq.Query[string](httpRoute, ".spec.parentRefs[0].name")
	if err != nil || gatewayName == "" {
		return ""
	}
	gatewayNS, err := jq.Query[string](httpRoute, ".spec.parentRefs[0].namespace")
	if err != nil || gatewayNS == "" {
		return ""
	}

	gateway, err := c.Get(ctx, resources.Gateway.GVR(), gatewayName, client.InNamespace(gatewayNS))
	if err != nil {
		return ""
	}

	hostname, _ := jq.Query[string](gateway, ".spec.listeners[0].hostname")
	if hostname == "" {
		route, err := c.Get(ctx, resources.Route.GVR(), gatewayName, client.InNamespace(gatewayNS))
		if err != nil {
			return ""
		}
		hostname, _ = jq.Query[string](route, ".spec.host")
	}
	if hostname == "" {
		return ""
	}

	return fmt.Sprintf("https://%s/ray/%s/%s", hostname, namespace, clusterName)
}