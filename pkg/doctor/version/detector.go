package version

import (
	"context"
	"errors"
	"fmt"

	"github.com/lburgazzoli/odh-cli/pkg/util/client"
)

// Detect performs priority-based version detection from multiple sources
// Priority order: DataScienceCluster > DSCInitialization > OLM
// Returns ClusterVersion or error if version cannot be determined from any source.
func Detect(ctx context.Context, c *client.Client) (*ClusterVersion, error) {
	// Priority 1: DataScienceCluster
	if version, found, err := DetectFromDataScienceCluster(ctx, c); err != nil {
		return nil, fmt.Errorf("detecting from DataScienceCluster: %w", err)
	} else if found {
		branch, err := VersionToBranch(version)
		if err != nil {
			return nil, fmt.Errorf("mapping version to branch: %w", err)
		}

		return &ClusterVersion{
			Version:    version,
			Source:     SourceDataScienceCluster,
			Confidence: ConfidenceHigh,
			Branch:     branch,
		}, nil
	}

	// Priority 2: DSCInitialization
	if version, found, err := DetectFromDSCInitialization(ctx, c); err != nil {
		return nil, fmt.Errorf("detecting from DSCInitialization: %w", err)
	} else if found {
		branch, err := VersionToBranch(version)
		if err != nil {
			return nil, fmt.Errorf("mapping version to branch: %w", err)
		}

		return &ClusterVersion{
			Version:    version,
			Source:     SourceDSCInitialization,
			Confidence: ConfidenceHigh,
			Branch:     branch,
		}, nil
	}

	// Priority 3: OLM
	if version, found, err := DetectFromOLM(ctx, c); err != nil {
		return nil, fmt.Errorf("detecting from OLM: %w", err)
	} else if found {
		branch, err := VersionToBranch(version)
		if err != nil {
			return nil, fmt.Errorf("mapping version to branch: %w", err)
		}

		return &ClusterVersion{
			Version:    version,
			Source:     SourceOLM,
			Confidence: ConfidenceMedium,
			Branch:     branch,
		}, nil
	}

	// No version found from any source
	return nil, errors.New("unable to detect cluster version: no DataScienceCluster, DSCInitialization, or OLM resources found with version information")
}
