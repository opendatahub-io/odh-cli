package servicemeshoperator

import (
	"context"
	"fmt"

	"github.com/blang/semver/v4"

	"github.com/lburgazzoli/odh-cli/pkg/doctor/check"
	"github.com/lburgazzoli/odh-cli/pkg/resources"
	"github.com/lburgazzoli/odh-cli/pkg/util/jq"
)

const (
	checkID          = "dependencies.servicemeshoperator2.upgrade"
	checkName        = "Dependencies :: ServiceMeshOperator2 :: Upgrade (3.x)"
	checkDescription = "Validates that servicemeshoperator2 is not installed when upgrading to RHOAI 3.x (requires servicemeshoperator3)"
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

func (c *Check) CanApply(currentVersion *semver.Version, targetVersion *semver.Version) bool {
	if currentVersion == nil || targetVersion == nil {
		return false
	}

	return currentVersion.Major == 2 && targetVersion.Major >= 3
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

		if nameStr == "servicemeshoperator2" {
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
			Message: "servicemeshoperator2: Not installed (ready for RHOAI 3.x)",
			Details: map[string]any{
				"installed": false,
				"version":   "Not installed",
			},
		}, nil
	}

	severity := check.SeverityCritical

	return &check.DiagnosticResult{
		Status:   check.StatusFail,
		Severity: &severity,
		Message: fmt.Sprintf(
			"servicemeshoperator2 is installed (%s) but not supported in RHOAI 3.x, requires servicemeshoperator3",
			version,
		),
		Details: map[string]any{
			"installed":     true,
			"version":       version,
			"targetVersion": target.Version.Version,
		},
	}, nil
}

//nolint:gochecknoinits
func init() {
	check.MustRegisterCheck(&Check{})
}
