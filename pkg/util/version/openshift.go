package version

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/blang/semver/v4"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// DetectOpenShiftVersion queries the OpenShift ClusterVersion resource to determine the cluster version.
func DetectOpenShiftVersion(
	ctx context.Context,
	k8sClient client.Reader,
) (*semver.Version, error) {
	if k8sClient == nil {
		return nil, errors.New("kubernetes client not available")
	}

	cv, err := k8sClient.Get(ctx, resources.ClusterVersion.GVR(), "version")
	if err != nil {
		return nil, fmt.Errorf("failed to get ClusterVersion: %w", err)
	}

	// Try to get version from status.desired.version first (current desired version)
	desiredVersion, found, err := unstructured.NestedString(cv.Object, "status", "desired", "version")
	if err == nil && found && desiredVersion != "" {
		version, err := parseOpenShiftVersion(desiredVersion)
		if err == nil {
			return version, nil
		}
	}

	// Fallback to status.history[0].version (most recent update)
	history, found, err := unstructured.NestedSlice(cv.Object, "status", "history")
	if err == nil && found && len(history) > 0 {
		if historyItem, ok := history[0].(map[string]any); ok {
			if version, ok := historyItem["version"].(string); ok && version != "" {
				return parseOpenShiftVersion(version)
			}
		}
	}

	return nil, errors.New("unable to determine OpenShift version from ClusterVersion resource")
}

// parseOpenShiftVersion parses an OpenShift version string (e.g., "4.19.1") into semver.
func parseOpenShiftVersion(versionStr string) (*semver.Version, error) {
	versionStr = strings.TrimPrefix(versionStr, "v")

	ver, err := semver.Parse(versionStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse OpenShift version %q: %w", versionStr, err)
	}

	return &ver, nil
}
