package version

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/opendatahub-io/odh-cli/pkg/util/client"
	"github.com/opendatahub-io/odh-cli/pkg/util/jq"
)

// DetectFromDataScienceCluster attempts to detect version from DataScienceCluster resource
// Returns version string and true if found, empty string and false otherwise.
func DetectFromDataScienceCluster(ctx context.Context, c client.Reader) (string, bool, error) {
	// Get the DataScienceCluster singleton
	dsc, err := client.GetDataScienceCluster(ctx, c)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return "", false, nil
		}

		return "", false, fmt.Errorf("getting DataScienceCluster: %w", err)
	}

	// Query .status.release.version using JQ
	versionStr, err := jq.Query[string](dsc, ".status.release.version")
	if err != nil {
		return "", false, fmt.Errorf("querying .status.release.version: %w", err)
	}

	if versionStr == "" {
		return "", false, nil
	}

	return versionStr, true, nil
}

// DetectFromDSCInitialization attempts to detect version from DSCInitialization resource
// Returns version string and true if found, empty string and false otherwise.
func DetectFromDSCInitialization(ctx context.Context, c client.Reader) (string, bool, error) {
	// Get the DSCInitialization singleton
	dsci, err := client.GetDSCInitialization(ctx, c)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return "", false, nil
		}

		return "", false, fmt.Errorf("getting DSCInitialization: %w", err)
	}

	// Query .status.release.version using JQ
	versionStr, err := jq.Query[string](dsci, ".status.release.version")
	if err != nil {
		return "", false, fmt.Errorf("querying .status.release.version: %w", err)
	}

	if versionStr == "" {
		return "", false, nil
	}

	return versionStr, true, nil
}

// DetectFromOLM attempts to detect version from OLM ClusterServiceVersion
// Returns version string and true if found, empty string and false otherwise.
func DetectFromOLM(ctx context.Context, c client.Reader) (string, bool, error) {
	// Check if OLM client is available
	if !c.OLM().Available() {
		return "", false, nil
	}

	// List ClusterServiceVersions with label selector for OpenShift AI operator
	csvList, err := c.OLM().ClusterServiceVersions("").List(ctx, metav1.ListOptions{
		LabelSelector: "operators.coreos.com/rhods-operator.redhat-ods-operator",
	})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return "", false, nil
		}

		return "", false, fmt.Errorf("listing ClusterServiceVersion: %w", err)
	}

	if len(csvList.Items) == 0 {
		return "", false, nil
	}

	// Use the first CSV found
	csv := &csvList.Items[0]

	// Access .spec.version directly
	versionStr := csv.Spec.Version.String()

	if versionStr == "" {
		return "", false, nil
	}

	return versionStr, true, nil
}
