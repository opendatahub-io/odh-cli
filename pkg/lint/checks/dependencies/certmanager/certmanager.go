package certmanager

import (
	"context"
	"fmt"

	"github.com/blang/semver/v4"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/lburgazzoli/odh-cli/pkg/lint/check"
	"github.com/lburgazzoli/odh-cli/pkg/lint/check/result"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/shared/operators"
)

const (
	checkID          = "dependencies.certmanager.installed"
	checkName        = "Dependencies :: CertManager :: Installed"
	checkDescription = "Reports the cert-manager operator installation status and version"
)

// Check validates cert-manager operator installation.
type Check struct {
}

func (c *Check) ID() string {
	return checkID
}

func (c *Check) Name() string {
	return checkName
}

func (c *Check) Description() string {
	return checkDescription
}

func (c *Check) Group() check.CheckGroup {
	return check.GroupDependency
}

func (c *Check) CanApply(_ *semver.Version, _ *semver.Version) bool {
	return true
}

func (c *Check) Validate(ctx context.Context, target *check.CheckTarget) (*result.DiagnosticResult, error) {
	res, err := operators.CheckOperatorPresence(
		ctx,
		target.Client,
		"cert-manager",
		operators.WithDescription(checkDescription),
		operators.WithMatcher(func(subscription *unstructured.Unstructured) bool {
			op, err := operators.GetOperator(subscription)
			if err != nil {
				return false
			}

			return op.Name == "cert-manager" || op.Name == "openshift-cert-manager-operator"
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("checking cert-manager operator presence: %w", err)
	}

	return res, nil
}

//nolint:gochecknoinits
func init() {
	check.MustRegisterCheck(&Check{})
}
