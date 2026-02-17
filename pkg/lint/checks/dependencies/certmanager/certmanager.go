package certmanager

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/validate"
)

const kind = "certmanager"

// Check validates cert-manager operator installation.
type Check struct {
	check.BaseCheck
}

func NewCheck() *Check {
	return &Check{
		BaseCheck: check.BaseCheck{
			CheckGroup:       check.GroupDependency,
			Kind:             kind,
			Type:             check.CheckTypeInstalled,
			CheckID:          "dependencies.certmanager.installed",
			CheckName:        "Dependencies :: CertManager :: Installed",
			CheckDescription: "Reports the cert-manager operator installation status and version",
		},
	}
}

func (c *Check) CanApply(_ context.Context, _ check.Target) (bool, error) {
	return true, nil
}

func (c *Check) Validate(ctx context.Context, target check.Target) (*result.DiagnosticResult, error) {
	return validate.Operator(c, target).
		WithNames("cert-manager", "openshift-cert-manager-operator").
		WithConditionBuilder(func(found bool, version string) result.Condition {
			if !found {
				return check.NewCondition(
					check.ConditionTypeAvailable,
					metav1.ConditionFalse,
					check.WithReason(check.ReasonResourceNotFound),
					check.WithMessage("%s operator is not installed", kind),
					check.WithImpact(result.ImpactBlocking),
				)
			}

			return check.NewCondition(
				check.ConditionTypeAvailable,
				metav1.ConditionTrue,
				check.WithReason(check.ReasonResourceFound),
				check.WithMessage("%s operator installed: %s", kind, version),
			)
		}).
		Run(ctx)
}
