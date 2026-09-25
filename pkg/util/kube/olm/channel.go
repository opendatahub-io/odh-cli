package olm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"slices"
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
	channelNames := make([]string, 0, len(channels))
	for _, ch := range channels {
		chMap, ok := ch.(map[string]any)
		if !ok {
			continue
		}
		if name, ok := chMap["name"].(string); ok {
			channelNames = append(channelNames, name)
		}
	}

	defaultChannel, _ := jq.Query[string](pm, ".status.defaultChannel")
	channel, err := selectChannel(channelNames, defaultChannel)
	if err != nil {
		return "", fmt.Errorf("PackageManifest %s: %w", pm.GetName(), err)
	}

	return channel, nil
}

// ResolveCatalogChannel chooses a channel for a package from the OLM v1
// ClusterCatalog's JSON Lines content. It follows PackageManifest channel
// selection: highest stable-v* channel, then the package's default channel.
func ResolveCatalogChannel(content io.Reader, packageName string) (string, error) {
	var channels []string
	var defaultChannel string
	packageFound := false
	decoder := json.NewDecoder(content)
	for {
		var entry struct {
			Schema         string `json:"schema"`
			Name           string `json:"name"`
			Package        string `json:"package"`
			DefaultChannel string `json:"defaultChannel"`
		}
		if err := decoder.Decode(&entry); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}

			return "", fmt.Errorf("decode ClusterCatalog content: %w", err)
		}
		switch {
		case entry.Schema == "olm.package" && entry.Name == packageName:
			packageFound = true
			defaultChannel = entry.DefaultChannel
		case entry.Schema == "olm.channel" && entry.Package == packageName:
			channels = append(channels, entry.Name)
		}
	}
	if !packageFound {
		return "", fmt.Errorf("package %q not found in ClusterCatalog", packageName)
	}

	channel, err := selectChannel(channels, defaultChannel)
	if err != nil {
		return "", fmt.Errorf("package %q: %w", packageName, err)
	}
	if !slices.Contains(channels, channel) {
		return "", fmt.Errorf("channel %q for package %q not found in ClusterCatalog", channel, packageName)
	}

	return channel, nil
}

func selectChannel(channels []string, defaultChannel string) (string, error) {
	bestChannel := ""
	bestVersion := semver.Version{}

	for _, name := range channels {
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

	if strings.TrimSpace(defaultChannel) == "" {
		return "", errors.New("no stable-v* or defaultChannel found")
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
