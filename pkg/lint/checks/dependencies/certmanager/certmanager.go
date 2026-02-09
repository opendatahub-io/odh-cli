package certmanager

import (
	"context"

	"github.com/lburgazzoli/odh-cli/pkg/lint/check"
	"github.com/lburgazzoli/odh-cli/pkg/lint/check/result"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/shared/base"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/shared/validate"
)

const kind = "certmanager"

// Check validates cert-manager operator installation.
type Check struct {
	base.BaseCheck
}

func NewCheck() *Check {
	return &Check{
		BaseCheck: base.BaseCheck{
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
		Run(ctx)
}
