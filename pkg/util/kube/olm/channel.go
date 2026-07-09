package olm

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/blang/semver/v4"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
	"github.com/opendatahub-io/odh-cli/pkg/util/jq"
)

const (
	// FallbackKueueOperatorChannel is used when PackageManifest lookup fails.
	FallbackKueueOperatorChannel = "stable-v1.2"
)

var stableChannelPattern = regexp.MustCompile(`^stable-v(\d+(?:\.\d+)*)$`)

// PackageQuery identifies an operator package in a catalog source.
type PackageQuery struct {
	PackageName     string
	CatalogSource   string
	SourceNamespace string
}

// ResolveOperatorChannel returns the highest stable-v* channel from the operator's PackageManifest.
// If no stable-v* channel exists, it falls back to status.defaultChannel.
func ResolveOperatorChannel(
	ctx context.Context,
	r client.Reader,
	query PackageQuery,
) (string, error) {
	manifests, err := client.List[*unstructured.Unstructured](ctx, r, resources.PackageManifest,
		func(pm *unstructured.Unstructured) (bool, error) {
			if pm.GetName() != query.PackageName {
				return false, nil
			}

			if query.SourceNamespace != "" && pm.GetNamespace() != query.SourceNamespace {
				return false, nil
			}

			if query.CatalogSource == "" {
				return true, nil
			}

			catalogSource, err := jq.Query[string](pm, ".status.catalogSource")
			if err != nil {
				return false, nil
			}

			return catalogSource == query.CatalogSource, nil
		})
	if err != nil {
		return "", fmt.Errorf("listing PackageManifests for %s: %w", query.PackageName, err)
	}

	if len(manifests) == 0 {
		return "", fmt.Errorf("PackageManifest %q not found in catalog %q", query.PackageName, query.CatalogSource)
	}

	return resolveChannelFromManifest(manifests[0])
}

func resolveChannelFromManifest(pm *unstructured.Unstructured) (string, error) {
	channels, err := jq.Query[[]any](pm, ".status.channels")
	if err != nil {
		return "", fmt.Errorf("querying channels: %w", err)
	}

	bestChannel := ""
	bestVersion := semver.Version{}

	for _, ch := range channels {
		chMap, ok := ch.(map[string]any)
		if !ok {
			continue
		}

		name, _ := chMap["name"].(string)
		if name == "" {
			continue
		}

		matches := stableChannelPattern.FindStringSubmatch(name)
		if len(matches) != 2 {
			continue
		}

		v, err := parseChannelVersion(matches[1])
		if err != nil {
			continue
		}

		if bestChannel == "" || v.GT(bestVersion) {
			bestChannel = name
			bestVersion = v
		}
	}

	if bestChannel != "" {
		return bestChannel, nil
	}

	defaultChannel, err := jq.Query[string](pm, ".status.defaultChannel")
	if err != nil || strings.TrimSpace(defaultChannel) == "" {
		return "", fmt.Errorf("no stable-v* or defaultChannel found in PackageManifest %s", pm.GetName())
	}

	return defaultChannel, nil
}

func parseChannelVersion(version string) (semver.Version, error) {
	parts := strings.Split(version, ".")
	for len(parts) < 3 {
		parts = append(parts, "0")
	}

	return semver.Parse(strings.Join(parts[:3], ".")) //nolint:wrapcheck // version padding is local; parse error is self-descriptive
}
