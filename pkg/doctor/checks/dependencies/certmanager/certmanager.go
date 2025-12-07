package certmanager

import (
	"context"
	"fmt"

	"github.com/blang/semver/v4"

	"github.com/lburgazzoli/odh-cli/pkg/doctor/check"
	"github.com/lburgazzoli/odh-cli/pkg/resources"
	"github.com/lburgazzoli/odh-cli/pkg/util/jq"
)

const (
	checkID          = "dependencies.certmanager.installed"
	checkName        = "Dependencies :: CertManager :: Installed"
	checkDescription = "Reports the cert-manager operator installation status and version"
)

type Check struct{}

func (c *Check) ID() string {
	return checkID
}

func (c *Check) Name() string {
	return checkName
}

func (c *Check) Description() string {
	return checkDescription
}

func (c *Check) Category() check.CheckCategory {
	return check.CategoryDependency
}

func (c *Check) CanApply(_ *semver.Version, _ *semver.Version) bool {
	return true
}

func (c *Check) Validate(ctx context.Context, target *check.CheckTarget) (*check.DiagnosticResult, error) {
	subscriptions, err := target.Client.List(ctx, resources.Subscription)
	if err != nil {
		return nil, fmt.Errorf("listing subscriptions: %w", err)
	}

	var version string
	for _, sub := range subscriptions {
		name, err := jq.Query(&sub, ".metadata.name")
		if err != nil {
			continue
		}

		nameStr, ok := name.(string)
		if !ok {
			continue
		}

		if nameStr == "cert-manager" || nameStr == "openshift-cert-manager-operator" {
			installedCSV, err := jq.Query(&sub, ".status.installedCSV")
			if err == nil && installedCSV != nil {
				if csvStr, ok := installedCSV.(string); ok {
					version = csvStr

					break
				}
			}
		}
	}

	if version == "" {
		return &check.DiagnosticResult{
			Status:  check.StatusPass,
			Message: "cert-manager operator: Not installed",
			Details: map[string]any{
				"installed": false,
				"version":   "Not installed",
			},
		}, nil
	}

	return &check.DiagnosticResult{
		Status:  check.StatusPass,
		Message: "cert-manager operator: " + version,
		Details: map[string]any{
			"installed": true,
			"version":   version,
		},
	}, nil
}

//nolint:gochecknoinits
func init() {
	check.MustRegisterCheck(&Check{})
}
