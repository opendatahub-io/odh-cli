package version

import (
	"context"
	"errors"
	"fmt"

	"github.com/blang/semver/v4"

	"github.com/opendatahub-io/odh-cli/pkg/util/client"
)

// Detect performs priority-based version detection from multiple sources
// Priority order: DataScienceCluster > DSCInitialization > OLM
// Returns parsed semver.TargetVersion or error if version cannot be determined from any source.
func Detect(ctx context.Context, c client.Client) (*semver.Version, error) {
	// Priority 1: DataScienceCluster
	if versionStr, found, err := DetectFromDataScienceCluster(ctx, c); err != nil {
		return nil, fmt.Errorf("detecting from DataScienceCluster: %w", err)
	} else if found {
		ver, err := semver.Parse(versionStr)
		if err != nil {
			return nil, fmt.Errorf("parsing version %q: %w", versionStr, err)
		}

		if !ver.EQ(semver.Version{}) {
			return &ver, nil
		}
	}

	// Priority 2: DSCInitialization
	if versionStr, found, err := DetectFromDSCInitialization(ctx, c); err != nil {
		return nil, fmt.Errorf("detecting from DSCInitialization: %w", err)
	} else if found {
		ver, err := semver.Parse(versionStr)
		if err != nil {
			return nil, fmt.Errorf("parsing version %q: %w", versionStr, err)
		}

		if !ver.EQ(semver.Version{}) {
			return &ver, nil
		}
	}

	// Priority 3: OLM
	if versionStr, found, err := DetectFromOLM(ctx, c); err != nil {
		return nil, fmt.Errorf("detecting from OLM: %w", err)
	} else if found {
		ver, err := semver.Parse(versionStr)
		if err != nil {
			return nil, fmt.Errorf("parsing version %q: %w", versionStr, err)
		}

		if !ver.EQ(semver.Version{}) {
			return &ver, nil
		}
	}

	// No nonzero version found from any source (sources may exist but report a
	// placeholder 0.0.0 version, or not exist at all)
	return nil, errors.New("unable to detect cluster version: no nonzero version found from DataScienceCluster, DSCInitialization, or OLM resources")
}
