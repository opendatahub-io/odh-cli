package raycluster

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/lburgazzoli/odh-cli/pkg/resources"
	"github.com/lburgazzoli/odh-cli/pkg/util/client"
)

// GetClusters returns RayClusters for the given scope. If clusterName is set, namespace must be set.
func GetClusters(
	ctx context.Context,
	c client.Client,
	clusterName string,
	namespace string,
) ([]*unstructured.Unstructured, error) {
	if clusterName != "" && namespace == "" {
		return nil, fmt.Errorf("namespace must be specified when targeting a specific cluster")
	}

	if clusterName != "" {
		rc, err := c.Get(ctx, resources.RayCluster.GVR(), clusterName, client.InNamespace(namespace))
		if err != nil {
			if apierrors.IsNotFound(err) {
				return nil, nil
			}

			return nil, fmt.Errorf("getting RayCluster %s in %s: %w", clusterName, namespace, err)
		}

		return []*unstructured.Unstructured{rc}, nil
	}

	var namespaces []string
	if namespace != "" {
		namespaces = []string{namespace}
	} else {
		nsList, err := c.List(ctx, resources.Namespace)
		if err != nil {
			return nil, fmt.Errorf("listing namespaces: %w", err)
		}
		for _, ns := range nsList {
			namespaces = append(namespaces, ns.GetName())
		}
	}

	var out []*unstructured.Unstructured
	for _, ns := range namespaces {
		list, err := c.List(ctx, resources.RayCluster, client.WithNamespace(ns))
		if err != nil {
			if namespace != "" {
				return nil, fmt.Errorf("listing RayClusters in namespace %s: %w", ns, err)
			}

			continue
		}
		out = append(out, list...)
	}

	return out, nil
}
