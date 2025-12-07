package version

import (
	"fmt"
	"strings"
)

const (
	// minVersionParts is the minimum number of parts required in a version string (major.minor).
	minVersionParts = 2
)

// VersionSource indicates where the version information was obtained.
type VersionSource string

const (
	SourceDataScienceCluster VersionSource = "DataScienceCluster"
	SourceDSCInitialization  VersionSource = "DSCInitialization"
	SourceOLM                VersionSource = "OLM"
	SourceManual             VersionSource = "Manual" // User-specified target version
	SourceUnknown            VersionSource = "Unknown"
)

// String returns the string representation of the version source.
func (s VersionSource) String() string {
	return string(s)
}

// VersionConfidence indicates the reliability of the version detection.
type VersionConfidence string

const (
	ConfidenceHigh   VersionConfidence = "High"   // From DataScienceCluster or DSCInitialization
	ConfidenceMedium VersionConfidence = "Medium" // From OLM ClusterServiceVersion
	ConfidenceLow    VersionConfidence = "Low"    // Fallback or uncertain
)

// String returns the string representation of the confidence level.
func (c VersionConfidence) String() string {
	return string(c)
}

// ClusterVersion represents detected OpenShift AI cluster version.
type ClusterVersion struct {
	// Version is the semver version string (e.g., "2.17.0")
	Version string

	// Source indicates where the version was detected from
	Source VersionSource

	// Confidence indicates the reliability of the detection
	Confidence VersionConfidence

	// Branch is the corresponding Git branch for this version (e.g., "stable-2.17")
	Branch string
}

// VersionToBranch maps a version string to the corresponding Git branch
// Version format: X.Y.Z
// Branch mapping:
//   - 2.x.x → stable-2.x
//   - 3.x.x → main
//   - Unknown → ""
func VersionToBranch(version string) (string, error) {
	parts := strings.Split(version, ".")
	if len(parts) < minVersionParts {
		return "", fmt.Errorf("invalid version format: %s", version)
	}

	major := parts[0]
	minor := parts[1]

	switch major {
	case "2":
		return fmt.Sprintf("stable-%s.%s", major, minor), nil
	case "3":
		return "main", nil
	default:
		return "", fmt.Errorf("unsupported version: %s", version)
	}
}

// String returns a human-readable representation of the cluster version.
func (v *ClusterVersion) String() string {
	return fmt.Sprintf("%s (source: %s, confidence: %s, branch: %s)",
		v.Version, v.Source, v.Confidence, v.Branch)
}
