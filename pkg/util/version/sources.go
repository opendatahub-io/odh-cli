package version

import (
	"context"
	"errors"
	"fmt"
	"strings"

	platformcluster "github.com/opendatahub-io/odh-platform-utilities/pkg/cluster"
	platformolm "github.com/opendatahub-io/odh-platform-utilities/pkg/cluster/olm"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"

	"github.com/opendatahub-io/odh-cli/pkg/util/client"
	"github.com/opendatahub-io/odh-cli/pkg/util/jq"
)

const (
	rhoaiOperatorPackage = "rhods-operator"
	odhOperatorPackage   = "opendatahub-operator"
)

// DetectFromDataScienceCluster attempts to detect version from DataScienceCluster resource.
// Dynamically detects whether to use v1 or v2 API based on cluster capabilities.
// Returns version string and true if found, empty string and false otherwise.
func DetectFromDataScienceCluster(ctx context.Context, c client.Client) (string, bool, error) {
	dsc, err := client.GetDataScienceCluster(ctx, c)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return "", false, nil
		}

		return "", false, fmt.Errorf("getting DataScienceCluster: %w", err)
	}

	versionStr, err := jq.Query[string](dsc, ".status.release.version")
	if err != nil {
		return "", false, fmt.Errorf("querying .status.release.version: %w", err)
	}

	if versionStr == "" {
		return "", false, nil
	}

	return versionStr, true, nil
}

// DetectFromDSCInitialization attempts to detect version from DSCInitialization resource.
// Dynamically detects whether to use v1 or v2 API based on cluster capabilities.
// Returns version string and true if found, empty string and false otherwise.
func DetectFromDSCInitialization(ctx context.Context, c client.Client) (string, bool, error) {
	dsci, err := client.GetDSCInitialization(ctx, c)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return "", false, nil
		}

		return "", false, fmt.Errorf("getting DSCInitialization: %w", err)
	}

	versionStr, err := jq.Query[string](dsci, ".status.release.version")
	if err != nil {
		return "", false, fmt.Errorf("querying .status.release.version: %w", err)
	}

	if versionStr == "" {
		return "", false, nil
	}

	return versionStr, true, nil
}

// DetectFromOLM attempts to detect version from the installed ODH/RHOAI operator.
// Returns version string and true if found, empty string and false otherwise.
func DetectFromOLM(ctx context.Context, c client.Client) (string, bool, error) {
	reader := c.ControllerRuntime()
	if reader == nil {
		return "", false, nil
	}

	platform, err := platformcluster.DetectPlatform(ctx, reader, "", "")
	if err != nil {
		if apierrors.IsNotFound(err) || meta.IsNoMatchError(err) {
			return "", false, nil
		}

		return "", false, fmt.Errorf("detecting platform from OLM: %w", err)
	}

	packageName := odhOperatorPackage
	if platform == platformcluster.ManagedRhoai || platform == platformcluster.SelfManagedRhoai {
		packageName = rhoaiOperatorPackage
	}

	info, err := platformolm.OperatorExists(ctx, reader, packageName)
	if errors.Is(err, platformolm.ErrOperatorNotInstalled) || meta.IsNoMatchError(err) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("detecting %s version from OLM: %w", packageName, err)
	}
	if info == nil || info.Version == "" {
		return "", false, nil
	}

	return strings.TrimPrefix(info.Version, "v"), true, nil
}
