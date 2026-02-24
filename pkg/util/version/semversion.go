package version

import (
	"fmt"

	"github.com/blang/semver/v4"
)

// SemVersion is a pflag.Value-compatible type that stores both the raw version
// string and its parsed semver representation. Validation happens at Set time,
// giving fail-fast behavior when used as a CLI flag.
type SemVersion struct {
	raw    string
	parsed *semver.Version
}

func (v *SemVersion) String() string {
	return v.raw
}

// Set parses and validates the version string using semver.ParseTolerant,
// accepting partial versions like "3.0" (normalized to "3.0.0").
func (v *SemVersion) Set(val string) error {
	parsed, err := semver.ParseTolerant(val)
	if err != nil {
		return fmt.Errorf("invalid version %q: %w", val, err)
	}

	v.raw = val
	v.parsed = &parsed

	return nil
}

func (v *SemVersion) Type() string {
	return "version"
}

// IsSet returns true if a version has been successfully parsed.
func (v *SemVersion) IsSet() bool {
	return v.parsed != nil
}

// Version returns the parsed semver, or nil if not set.
func (v *SemVersion) Version() *semver.Version {
	return v.parsed
}
