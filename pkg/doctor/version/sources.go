package version

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/lburgazzoli/odh-cli/pkg/resources"
	"github.com/lburgazzoli/odh-cli/pkg/util/client"
	"github.com/lburgazzoli/odh-cli/pkg/util/jq"
)

// DetectFromDataScienceCluster attempts to detect version from DataScienceCluster resource
// Returns version string and true if found, empty string and false otherwise.
func DetectFromDataScienceCluster(ctx context.Context, c *client.Client) (string, bool, error) {
	// Get the DataScienceCluster singleton
	dsc, err := c.GetDataScienceCluster(ctx)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return "", false, nil
		}

		return "", false, fmt.Errorf("getting DataScienceCluster: %w", err)
	}

	// Query .status.release.version using JQ
	version, err := jq.Query(dsc, ".status.release.version")
	if err != nil {
		return "", false, fmt.Errorf("querying .status.release.version: %w", err)
	}

	versionStr, ok := version.(string)
	if !ok || versionStr == "" {
		return "", false, nil
	}

	return versionStr, true, nil
}

// DetectFromDSCInitialization attempts to detect version from DSCInitialization resource
// Returns version string and true if found, empty string and false otherwise.
func DetectFromDSCInitialization(ctx context.Context, c *client.Client) (string, bool, error) {
	// Get the DSCInitialization singleton
	dsci, err := c.GetDSCInitialization(ctx)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return "", false, nil
		}

		return "", false, fmt.Errorf("getting DSCInitialization: %w", err)
	}

	// Query .status.release.version using JQ
	version, err := jq.Query(dsci, ".status.release.version")
	if err != nil {
		return "", false, fmt.Errorf("querying .status.release.version: %w", err)
	}

	versionStr, ok := version.(string)
	if !ok || versionStr == "" {
		return "", false, nil
	}

	return versionStr, true, nil
}

// DetectFromOLM attempts to detect version from OLM ClusterServiceVersion
// Returns version string and true if found, empty string and false otherwise.
func DetectFromOLM(ctx context.Context, c *client.Client) (string, bool, error) {
	// List ClusterServiceVersions with label selector for OpenShift AI operator
	csvList, err := c.List(ctx, resources.ClusterServiceVersion,
		client.WithLabelSelector("operators.coreos.com/rhods-operator.redhat-ods-operator"))
	if err != nil {
		if apierrors.IsNotFound(err) {
			return "", false, nil
		}

		return "", false, fmt.Errorf("listing ClusterServiceVersion: %w", err)
	}

	if len(csvList) == 0 {
		return "", false, nil
	}

	// Use the first CSV found
	csv := &csvList[0]

	// Query .spec.version using JQ
	version, err := jq.Query(csv, ".spec.version")
	if err != nil {
		return "", false, fmt.Errorf("querying .spec.version: %w", err)
	}

	versionStr, ok := version.(string)
	if !ok || versionStr == "" {
		return "", false, nil
	}

	return versionStr, true, nil
}
