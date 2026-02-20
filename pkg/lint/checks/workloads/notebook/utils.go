package notebook

import (
	"context"
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/opendatahub-io/odh-cli/pkg/constants"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
	"github.com/opendatahub-io/odh-cli/pkg/util/components"
	"github.com/opendatahub-io/odh-cli/pkg/util/jq"
)

// isWorkbenchesManaged returns true when the Workbenches component is set to Managed on the DSC.
// Used as the common precondition for all notebook checks.
func isWorkbenchesManaged(ctx context.Context, target check.Target) (bool, error) {
	dsc, err := client.GetDataScienceCluster(ctx, target.Client)
	if err != nil {
		return false, fmt.Errorf("getting DataScienceCluster: %w", err)
	}

	return components.HasManagementState(dsc, componentWorkbenches, constants.ManagementStateManaged), nil
}

// NotebookContainer holds the parsed name and image of a container from a notebook spec.
type NotebookContainer struct {
	Name  string
	Image string
}

// ExtractWorkloadContainers extracts non-infrastructure containers from a notebook's pod template spec.
// Infrastructure sidecars (e.g., oauth-proxy) are excluded from the result.
func ExtractWorkloadContainers(nb *unstructured.Unstructured) ([]NotebookContainer, error) {
	rawContainers, err := jq.Query[[]any](nb, ".spec.template.spec.containers")
	if err != nil {
		return nil, fmt.Errorf("querying containers: %w", err)
	}

	var result []NotebookContainer

	for _, raw := range rawContainers {
		containerMap, ok := raw.(map[string]any)
		if !ok {
			continue
		}

		name, _ := containerMap["name"].(string)
		image, _ := containerMap["image"].(string)

		// Skip known infrastructure/sidecar containers that are not notebook images.
		if IsInfrastructureContainer(name, image) {
			continue
		}

		result = append(result, NotebookContainer{
			Name:  name,
			Image: image,
		})
	}

	return result, nil
}

// IsInfrastructureContainer returns true if the container is a known infrastructure sidecar
// that should not be analyzed for notebook image compatibility.
// Both the container name AND image must match known patterns to be skipped.
// This prevents false positives where a user might name their container "oauth-proxy"
// but use a custom image that needs compatibility verification.
func IsInfrastructureContainer(containerName string, image string) bool {
	// Only skip oauth-proxy sidecars when BOTH conditions are met:
	// 1. Container name is "oauth-proxy"
	// 2. Image contains "ose-oauth-proxy-rhel9" (the official OpenShift oauth-proxy image)
	if containerName == "oauth-proxy" && strings.Contains(image, "ose-oauth-proxy-rhel9") {
		return true
	}

	return false
}
