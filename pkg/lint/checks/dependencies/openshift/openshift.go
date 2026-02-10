package openshift

import (
	"context"

	"github.com/blang/semver/v4"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/lburgazzoli/odh-cli/pkg/lint/check"
	"github.com/lburgazzoli/odh-cli/pkg/lint/check/result"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/shared/base"
	"github.com/lburgazzoli/odh-cli/pkg/util/version"
)

const (
	kind      = "openshift-platform"
	checkType = "version-requirement"
)

//nolint:gochecknoglobals
var minVersion = semver.MustParse("4.19.9")

// Check validates OpenShift version requirements for RHOAI 3.x upgrades.
type Check struct {
	base.BaseCheck
}

// NewCheck creates a new OpenShift version requirement check.
func NewCheck() *Check {
	return &Check{
		BaseCheck: base.BaseCheck{
			CheckGroup:       check.GroupDependency,
			Kind:             kind,
			Type:             checkType,
			CheckID:          "dependencies.openshift.version-requirement",
			CheckName:        "Dependencies :: OpenShift :: Version Requirement (3.x)",
			CheckDescription: "Validates that OpenShift is at least version 4.19.9 when upgrading to RHOAI 3.x",
		},
	}
}

func (c *Check) CanApply(_ context.Context, target check.Target) (bool, error) {
	return version.IsUpgradeFrom2xTo3x(target.CurrentVersion, target.TargetVersion), nil
}

func (c *Check) Validate(
	ctx context.Context,
	target check.Target,
) (*result.DiagnosticResult, error) {
	dr := c.NewResult()

	ver, err := version.DetectOpenShiftVersion(ctx, target.Client)

	switch {
	case err != nil:
		dr.SetCondition(check.NewCondition(
			check.ConditionTypeCompatible,
			metav1.ConditionFalse,
			check.WithReason(check.ReasonInsufficientData),
			check.WithMessage("Unable to detect OpenShift version: %s. RHOAI 3.x requires OpenShift %s or later", err.Error(), minVersion.String()),
			check.WithImpact(result.ImpactBlocking),
		))
	case ver.GTE(minVersion):
		dr.SetCondition(check.NewCondition(
			check.ConditionTypeCompatible,
			metav1.ConditionTrue,
			check.WithReason(check.ReasonVersionCompatible),
			check.WithMessage("OpenShift %s meets RHOAI 3.x minimum version requirement (%s+)", ver.String(), minVersion.String()),
		))
	default:
		dr.SetCondition(check.NewCondition(
			check.ConditionTypeCompatible,
			metav1.ConditionFalse,
			check.WithReason(check.ReasonVersionIncompatible),
			check.WithMessage("OpenShift %s does not meet RHOAI 3.x minimum version requirement (%s+). Upgrade OpenShift to %s or later before upgrading RHOAI",
				ver.String(), minVersion.String(), minVersion.String()),
			check.WithImpact(result.ImpactBlocking),
		))
	}

	return dr, nil
}
