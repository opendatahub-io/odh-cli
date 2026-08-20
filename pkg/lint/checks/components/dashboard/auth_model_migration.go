package dashboard

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/opendatahub-io/odh-cli/pkg/constants"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/validate"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
	"github.com/opendatahub-io/odh-cli/pkg/util/jq"
	"github.com/opendatahub-io/odh-cli/pkg/util/version"
)

const (
	containerOAuthProxy    = "oauth-proxy"
	containerKubeRBACProxy = "kube-rbac-proxy"

	conditionTypeAuthModel = "AuthModelMigration"

	msgAuthModelMigrated = "Dashboard pods are using kube-rbac-proxy auth model (3.x)"
	msgAuthModelLegacy   = "Found %d dashboard pod(s) still using oauth-proxy sidecar - operator will migrate automatically"
	msgAuthModelNoPods   = "No dashboard pods found"
)

// AuthModelMigrationCheck detects dashboard pods still using the legacy oauth-proxy sidecar.
type AuthModelMigrationCheck struct {
	check.BaseCheck
}

func NewAuthModelMigrationCheck() *AuthModelMigrationCheck {
	return &AuthModelMigrationCheck{
		BaseCheck: check.BaseCheck{
			CheckGroup:       check.GroupComponent,
			Kind:             constants.ComponentDashboard,
			Type:             check.CheckTypeAuthModelMigration,
			CheckID:          "components.dashboard.auth-model-migration",
			CheckName:        "Components :: Dashboard :: Auth Model Migration (3.x)",
			CheckDescription: "Detects dashboard pods using legacy oauth-proxy sidecar that will be migrated to kube-rbac-proxy",
			CheckRemediation: "The operator will automatically migrate the auth model during upgrade - no manual action required",
		},
	}
}

func (c *AuthModelMigrationCheck) CanApply(_ context.Context, target check.Target) (bool, error) {
	return version.IsUpgradeFrom2xTo3x(target.CurrentVersion, target.TargetVersion), nil
}

func (c *AuthModelMigrationCheck) Validate(
	ctx context.Context,
	target check.Target,
) (*result.DiagnosticResult, error) {
	return validate.Component(c, target).
		WithApplicationsNamespace().
		Run(ctx, c.checkAuthModel)
}

func (c *AuthModelMigrationCheck) checkAuthModel(
	ctx context.Context,
	req *validate.ComponentRequest,
) error {
	pods, err := req.Client.List(ctx, resources.Pod,
		client.WithNamespace(req.ApplicationsNamespace),
		client.WithLabelSelector("app=rhods-dashboard"))
	if err != nil {
		return fmt.Errorf("listing dashboard pods: %w", err)
	}

	if len(pods) == 0 {
		req.Result.SetCondition(check.NewCondition(
			conditionTypeAuthModel,
			metav1.ConditionTrue,
			check.WithReason(check.ReasonRequirementsMet),
			check.WithMessage(msgAuthModelNoPods),
		))

		return nil
	}

	legacyCount := 0

	for _, pod := range pods {
		if hasContainer(pod, containerOAuthProxy) {
			legacyCount++
		}
	}

	if legacyCount > 0 {
		req.Result.SetCondition(check.NewCondition(
			conditionTypeAuthModel,
			metav1.ConditionFalse,
			check.WithReason(check.ReasonMigrationPending),
			check.WithMessage(msgAuthModelLegacy, legacyCount),
			check.WithImpact(result.ImpactAdvisory),
			check.WithRemediation(c.CheckRemediation),
		))

		return nil
	}

	req.Result.SetCondition(check.NewCondition(
		conditionTypeAuthModel,
		metav1.ConditionTrue,
		check.WithReason(check.ReasonRequirementsMet),
		check.WithMessage(msgAuthModelMigrated),
	))

	return nil
}

func hasContainer(pod *unstructured.Unstructured, containerName string) bool {
	containers, err := jq.Query[[]any](pod, ".spec.containers")
	if err != nil {
		return false
	}

	for _, raw := range containers {
		containerMap, ok := raw.(map[string]any)
		if !ok {
			continue
		}

		name, _ := containerMap["name"].(string)
		if name == containerName {
			return true
		}
	}

	return false
}
