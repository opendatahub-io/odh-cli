package datasciencepipelines

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/lburgazzoli/odh-cli/pkg/lint/check"
	"github.com/lburgazzoli/odh-cli/pkg/lint/check/result"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/shared/base"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/shared/components"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/shared/results"
	"github.com/lburgazzoli/odh-cli/pkg/util/version"
)

type RenamingCheck struct {
	base.BaseCheck
}

func NewRenamingCheck() *RenamingCheck {
	return &RenamingCheck{
		BaseCheck: base.BaseCheck{
			CheckGroup:       check.GroupComponent,
			Kind:             check.ComponentDataSciencePipelines,
			CheckType:        check.CheckTypeRenaming,
			CheckID:          "components.datasciencepipelines.renaming",
			CheckName:        "Components :: DataSciencePipelines :: Component Renaming (3.x)",
			CheckDescription: "Informs about DataSciencePipelines component renaming to AIPipelines in DSC v2 (RHOAI 3.x)",
		},
	}
}

// CanApply returns whether this check should run for the given target.
// This check only applies when upgrading FROM 2.x TO 3.x.
func (c *RenamingCheck) CanApply(target check.Target) bool {
	return version.IsUpgradeFrom2xTo3x(target.CurrentVersion, target.TargetVersion)
}

// Validate executes the check against the provided target.
func (c *RenamingCheck) Validate(
	ctx context.Context,
	target check.Target,
) (*result.DiagnosticResult, error) {
	dr := c.NewResult()

	// Get the DataScienceCluster singleton
	dsc, err := target.Client.GetDataScienceCluster(ctx)
	switch {
	case apierrors.IsNotFound(err):
		return results.DataScienceClusterNotFound(string(c.Group()), c.Kind, c.CheckType, c.Description()), nil
	case err != nil:
		return nil, fmt.Errorf("getting DataScienceCluster: %w", err)
	}

	// In DSC v1 (2.x): .spec.components.datasciencepipelines
	dspStateStr, configured, err := components.GetManagementState(dsc, "datasciencepipelines")
	if err != nil {
		return nil, fmt.Errorf("querying datasciencepipelines managementState: %w", err)
	}

	if !configured {
		results.SetComponentNotConfigured(dr, "DataSciencePipelines")

		return dr, nil
	}

	dr.Annotations[check.AnnotationComponentManagementState] = dspStateStr
	if target.TargetVersion != nil {
		dr.Annotations[check.AnnotationCheckTargetVersion] = target.TargetVersion.String()
	}

	// If component is configured (Managed or Unmanaged), provide warning about renaming
	if dspStateStr == check.ManagementStateManaged || dspStateStr == check.ManagementStateUnmanaged {
		// WARNING level - use ImpactAdvisory with Status=False
		results.SetCondition(dr, check.NewCondition(
			check.ConditionTypeCompatible,
			metav1.ConditionFalse,
			check.ReasonComponentRenamed,
			"DataSciencePipelines component (state: %s) will be renamed to AIPipelines in DSC v2 (RHOAI 3.x). The field path changes from '.spec.components.datasciencepipelines' to '.spec.components.aipipelines'",
			dspStateStr,
			check.WithImpact(result.ImpactAdvisory),
		))

		return dr, nil
	}

	// Component is Removed - no action needed
	results.SetCompatibilitySuccessf(dr,
		"DataSciencePipelines component is disabled (state: %s) - no action required for renaming in RHOAI 3.x",
		dspStateStr)

	return dr, nil
}
