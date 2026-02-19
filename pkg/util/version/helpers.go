package version

import (
	"fmt"

	"github.com/blang/semver/v4"
)

// MajorMinorLabel formats a semver version as "major.minor" for use in user-facing messages.
// Returns "unknown" if version is nil.
func MajorMinorLabel(v *semver.Version) string {
	if v == nil {
		return "unknown"
	}

	return fmt.Sprintf("%d.%d", v.Major, v.Minor)
}

// IsUpgradeFrom2xTo3x checks if the versions represent an upgrade from 2.x to 3.x specifically.
// Future major versions (4.x+) may have different compatibility requirements.
// Returns false if either version is nil.
func IsUpgradeFrom2xTo3x(from *semver.Version, to *semver.Version) bool {
	if from == nil || to == nil {
		return false
	}

	return from.Major == 2 && to.Major == 3
}

// IsVersion3x checks if the given version has major version 3.
// Returns false if version is nil.
func IsVersion3x(v *semver.Version) bool {
	if v == nil {
		return false
	}

	return v.Major == 3 //nolint:mnd
}

// SameMajorMinor checks if two versions share the same major and minor version.
// Patch version is ignored. Returns false if either version is nil.
func SameMajorMinor(a *semver.Version, b *semver.Version) bool {
	if a == nil || b == nil {
		return false
	}

	return a.Major == b.Major && a.Minor == b.Minor
}

// IsVersionAtLeast checks if the given version is at least the specified major.minor version.
// Patch version is ignored in the comparison.
// Returns false if version is nil.
func IsVersionAtLeast(
	version *semver.Version,
	major uint64,
	minor uint64,
) bool {
	if version == nil {
		return false
	}

	if version.Major > major {
		return true
	}

	return version.Major == major && version.Minor >= minor
}
