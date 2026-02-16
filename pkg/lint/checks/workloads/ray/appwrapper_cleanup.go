package ray

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/opendatahub-io/odh-cli/pkg/constants"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/validate"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
	"github.com/opendatahub-io/odh-cli/pkg/util/components"
	"github.com/opendatahub-io/odh-cli/pkg/util/version"
)

// dscComponent is the DSC spec key for the CodeFlare component.
// The user-facing kind is "ray" but the DSC still uses "codeflare".
const dscComponent = "codeflare"

const ConditionTypeAppWrapperCompatible = "AppWrapperCompatible" //nolint:gosec // Not a credential

// AppWrapperCleanupCheck lists AppWrappers that will be impacted when CodeFlare is removed in RHOAI 3.x.
type AppWrapperCleanupCheck struct {
	check.BaseCheck
}

func NewAppWrapperCleanupCheck() *AppWrapperCleanupCheck {
	return &AppWrapperCleanupCheck{
		BaseCheck: check.BaseCheck{
			CheckGroup:       check.GroupWorkload,
			Kind:             kind,
			Type:             check.CheckTypeImpactedWorkloads,
			CheckID:          "workloads.ray.appwrapper-cleanup",
			CheckName:        "Workloads :: Ray :: AppWrapper Cleanup (3.x)",
			CheckDescription: "Lists AppWrappers managed by CodeFlare that will be impacted in RHOAI 3.x",
			CheckRemediation: "Remove redundant AppWrapper CRs or install the AppWrapper controller separately before upgrading",
		},
	}
}

// CanApply returns whether this check should run for the given target.
// Only applies when upgrading from 2.x to 3.x and CodeFlare is Managed.
func (c *AppWrapperCleanupCheck) CanApply(ctx context.Context, target check.Target) (bool, error) {
	if !version.IsUpgradeFrom2xTo3x(target.CurrentVersion, target.TargetVersion) {
		return false, nil
	}

	dsc, err := client.GetDataScienceCluster(ctx, target.Client)
	if err != nil {
		return false, fmt.Errorf("getting DataScienceCluster: %w", err)
	}

	return components.HasManagementState(dsc, dscComponent, constants.ManagementStateManaged), nil
}

// Validate executes the check against the provided target.
func (c *AppWrapperCleanupCheck) Validate(
	ctx context.Context,
	target check.Target,
) (*result.DiagnosticResult, error) {
	return validate.WorkloadsMetadata(c, target, resources.AppWrapper).
		Complete(ctx, c.newAppWrapperCondition)
}

func (c *AppWrapperCleanupCheck) newAppWrapperCondition(
	_ context.Context,
	req *validate.WorkloadRequest[*metav1.PartialObjectMetadata],
) ([]result.Condition, error) {
	count := len(req.Items)

	if count > 0 {
		return []result.Condition{check.NewCondition(
			ConditionTypeAppWrapperCompatible,
			metav1.ConditionFalse,
			check.WithReason(check.ReasonVersionIncompatible),
			check.WithMessage("Found %d AppWrapper workload CRs. The AppWrapper controller has been removed from OpenShift AI as part of the broader CodeFlare Operator removal process. Please remove any redundant CRs or install AppWrapper separately", count),
			check.WithImpact(result.ImpactAdvisory),
			check.WithRemediation(c.CheckRemediation),
		)}, nil
	}

	return []result.Condition{check.NewCondition(
		ConditionTypeAppWrapperCompatible,
		metav1.ConditionTrue,
		check.WithReason(check.ReasonVersionCompatible),
		check.WithMessage("No AppWrapper(s) found - ready for RHOAI %s upgrade", version.MajorMinorLabel(req.TargetVersion)),
	)}, nil
}
