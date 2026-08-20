package dashboard

import (
	"context"
	"fmt"
	"slices"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/opendatahub-io/odh-cli/pkg/constants"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/validate"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
	"github.com/opendatahub-io/odh-cli/pkg/util/version"
)

//nolint:gochecknoglobals // Constant-like list of legacy route names
var legacyDashboardRouteNames = []string{
	"rhods-dashboard",
	"odh-dashboard",
}

const (
	conditionTypeRouteMigration = "RouteMigration"

	msgRouteWillChange = "Dashboard route %q found - URL will change after upgrade to a gateway-based route"
	msgNoLegacyRoute   = "No legacy dashboard routes found"
)

// RouteMigrationCheck detects legacy dashboard routes that will change during upgrade.
type RouteMigrationCheck struct {
	check.BaseCheck
}

func NewRouteMigrationCheck() *RouteMigrationCheck {
	return &RouteMigrationCheck{
		BaseCheck: check.BaseCheck{
			CheckGroup:       check.GroupComponent,
			Kind:             constants.ComponentDashboard,
			Type:             check.CheckTypeRouteMigration,
			CheckID:          "components.dashboard.route-migration",
			CheckName:        "Components :: Dashboard :: Route Migration (3.x)",
			CheckDescription: "Detects legacy dashboard routes that will change during upgrade to gateway-based routing",
			CheckRemediation: "Update any bookmarks, scripts, or integrations referencing the old dashboard URL after upgrade",
		},
	}
}

func (c *RouteMigrationCheck) CanApply(_ context.Context, target check.Target) (bool, error) {
	return version.IsUpgradeFrom2xTo3x(target.CurrentVersion, target.TargetVersion), nil
}

func (c *RouteMigrationCheck) Validate(
	ctx context.Context,
	target check.Target,
) (*result.DiagnosticResult, error) {
	return validate.Component(c, target).
		WithApplicationsNamespace().
		Run(ctx, c.checkRoute)
}

func (c *RouteMigrationCheck) checkRoute(
	ctx context.Context,
	req *validate.ComponentRequest,
) error {
	routes, err := req.Client.List(ctx, resources.Route,
		client.WithNamespace(req.ApplicationsNamespace))
	if err != nil {
		return fmt.Errorf("listing routes: %w", err)
	}

	for _, route := range routes {
		name := route.GetName()

		if slices.Contains(legacyDashboardRouteNames, name) {
			req.Result.SetCondition(check.NewCondition(
				conditionTypeRouteMigration,
				metav1.ConditionFalse,
				check.WithReason(check.ReasonMigrationPending),
				check.WithMessage(msgRouteWillChange, name),
				check.WithImpact(result.ImpactAdvisory),
				check.WithRemediation(c.CheckRemediation),
			))

			return nil
		}
	}

	req.Result.SetCondition(check.NewCondition(
		conditionTypeRouteMigration,
		metav1.ConditionTrue,
		check.WithReason(check.ReasonRequirementsMet),
		check.WithMessage(msgNoLegacyRoute),
	))

	return nil
}
