package version_test

import (
	"testing"

	"github.com/blang/semver/v4"
	"github.com/opendatahub-io/odh-cli/pkg/util/version"

	. "github.com/onsi/gomega"
)

func TestIsUpgradeFrom2xTo3x(t *testing.T) {
	tests := []struct {
		name           string
		from           *semver.Version
		to             *semver.Version
		expectedResult bool
	}{
		{
			name:           "nil from version returns false",
			from:           nil,
			to:             toVersionPtr("3.0.0"),
			expectedResult: false,
		},
		{
			name:           "nil to version returns false",
			from:           toVersionPtr("2.15.0"),
			to:             nil,
			expectedResult: false,
		},
		{
			name:           "both nil returns false",
			from:           nil,
			to:             nil,
			expectedResult: false,
		},
		{
			name:           "upgrade from 2.x to 3.x returns true",
			from:           toVersionPtr("2.15.0"),
			to:             toVersionPtr("3.0.0"),
			expectedResult: true,
		},
		{
			name:           "upgrade from 2.x to 3.1 returns true",
			from:           toVersionPtr("2.15.0"),
			to:             toVersionPtr("3.1.0"),
			expectedResult: true,
		},
		{
			name:           "upgrade from 2.x to 4.x returns false",
			from:           toVersionPtr("2.15.0"),
			to:             toVersionPtr("4.0.0"),
			expectedResult: false,
		},
		{
			name:           "upgrade from 1.x to 3.x returns false",
			from:           toVersionPtr("1.0.0"),
			to:             toVersionPtr("3.0.0"),
			expectedResult: false,
		},
		{
			name:           "upgrade from 3.x to 3.x returns false",
			from:           toVersionPtr("3.0.0"),
			to:             toVersionPtr("3.1.0"),
			expectedResult: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			result := version.IsUpgradeFrom2xTo3x(tt.from, tt.to)

			g.Expect(result).To(Equal(tt.expectedResult))
		})
	}
}

func TestIsVersion3x(t *testing.T) {
	tests := []struct {
		name           string
		version        *semver.Version
		expectedResult bool
	}{
		{
			name:           "nil version returns false",
			version:        nil,
			expectedResult: false,
		},
		{
			name:           "2.x version returns false",
			version:        toVersionPtr("2.17.0"),
			expectedResult: false,
		},
		{
			name:           "3.0.0 returns true",
			version:        toVersionPtr("3.0.0"),
			expectedResult: true,
		},
		{
			name:           "3.1.0 returns true",
			version:        toVersionPtr("3.1.0"),
			expectedResult: true,
		},
		{
			name:           "4.x version returns false",
			version:        toVersionPtr("4.0.0"),
			expectedResult: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			result := version.IsVersion3x(tt.version)

			g.Expect(result).To(Equal(tt.expectedResult))
		})
	}
}

func TestIsVersionAtLeast(t *testing.T) {
	tests := []struct {
		name           string
		version        *semver.Version
		major          uint64
		minor          uint64
		expectedResult bool
	}{
		{
			name:           "nil version returns false",
			version:        nil,
			major:          3,
			minor:          3,
			expectedResult: false,
		},
		{
			name:           "exact version match returns true",
			version:        toVersionPtr("3.3.0"),
			major:          3,
			minor:          3,
			expectedResult: true,
		},
		{
			name:           "higher minor version returns true",
			version:        toVersionPtr("3.5.0"),
			major:          3,
			minor:          3,
			expectedResult: true,
		},
		{
			name:           "higher major version returns true",
			version:        toVersionPtr("4.0.0"),
			major:          3,
			minor:          3,
			expectedResult: true,
		},
		{
			name:           "lower minor version returns false",
			version:        toVersionPtr("3.2.0"),
			major:          3,
			minor:          3,
			expectedResult: false,
		},
		{
			name:           "lower major version returns false",
			version:        toVersionPtr("2.15.0"),
			major:          3,
			minor:          3,
			expectedResult: false,
		},
		{
			name:           "patch version doesn't affect result",
			version:        toVersionPtr("3.3.99"),
			major:          3,
			minor:          3,
			expectedResult: true,
		},
		{
			name:           "major version 0 exact match",
			version:        toVersionPtr("0.1.0"),
			major:          0,
			minor:          1,
			expectedResult: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			result := version.IsVersionAtLeast(tt.version, tt.major, tt.minor)

			g.Expect(result).To(Equal(tt.expectedResult))
		})
	}
}

func toVersionPtr(versionStr string) *semver.Version {
	v := semver.MustParse(versionStr)

	return &v
}
