package kueue

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/opendatahub-io/odh-cli/pkg/constants"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/validate"
	kueuediscovery "github.com/opendatahub-io/odh-cli/pkg/lint/checks/kueue/discovery"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
	"github.com/opendatahub-io/odh-cli/pkg/util/components"
	"github.com/opendatahub-io/odh-cli/pkg/util/version"
)

const (
	kind                     = "kueue"
	checkTypeManagementState = "management-state"

	// Deferred: parameterize hardcoded version references using ComponentRequest.TargetVersion.
	msgManagedProhibited   = "The 3.3.2 upgrade currently only supports the Kueue managementState of Removed. A future 3.3.x release might allow an upgrade when you have migrated to the Red Hat build of Kueue Operator and the Kueue managementState is Unmanaged."
	msgUnmanagedProhibited = "The 3.3.2 upgrade currently only supports the Kueue managementState of Removed. A future 3.3.x release might allow an upgrade when the Kueue managementState is Unmanaged."
	msgManagedBlocking     = "The 3.3.2 upgrade currently only supports the Kueue managementState of Removed. The Kueue managementState is currently Managed but no workloads on the cluster are using Kueue. Set the Kueue managementState to Removed and then re-run this script to proceed with migration."
	msgUnmanagedBlocking   = "The 3.3.2 upgrade currently only supports the Kueue managementState of Removed. The Kueue managementState is currently Unmanaged but no workloads on the cluster are using Kueue. Set the Kueue managementState to Removed and then re-run this script to proceed with migration."
)

// ManagementStateCheck validates that Kueue managementState is Removed before upgrading to 3.x.
// In RHOAI 3.3.2, only the Removed state is supported. A future 3.3.x release will support
// Unmanaged with the Red Hat build of Kueue Operator.
//
// The check distinguishes between clusters that are actively using Kueue (namespaces or workloads
// labeled for Kueue) and those that have Kueue enabled but are not actually using it:
//   - Managed/Unmanaged + in use: prohibited (upgrade not possible)
//   - Managed/Unmanaged + not in use: blocking (recoverable — set managementState to Removed)
type ManagementStateCheck struct {
	check.BaseCheck
}

func NewManagementStateCheck() *ManagementStateCheck {
	return &ManagementStateCheck{
		BaseCheck: check.BaseCheck{
			CheckGroup:       check.GroupComponent,
			Kind:             kind,
			Type:             checkTypeManagementState,
			CheckID:          "components.kueue.management-state",
			CheckName:        "Components :: Kueue :: Management State (3.x)",
			CheckDescription: "Validates that Kueue managementState is Removed before upgrading to RHOAI 3.x",
		},
	}
}

// CanApply returns whether this check should run for the given target.
// This check only applies when upgrading FROM 2.x TO 3.x and Kueue is Managed or Unmanaged.
func (c *ManagementStateCheck) CanApply(ctx context.Context, target check.Target) (bool, error) {
	if !version.IsUpgradeFrom2xTo3x(target.CurrentVersion, target.TargetVersion) {
		return false, nil
	}

	dsc, err := client.GetDataScienceCluster(ctx, target.Client)
	if err != nil {
		return false, fmt.Errorf("getting DataScienceCluster: %w", err)
	}

	return components.HasManagementState(
		dsc, "kueue",
		constants.ManagementStateManaged, constants.ManagementStateUnmanaged,
	), nil
}

func (c *ManagementStateCheck) Validate(ctx context.Context, target check.Target) (*result.DiagnosticResult, error) {
	return validate.Component(c, target).
		Run(ctx, func(ctx context.Context, req *validate.ComponentRequest) error {
			kueueInUse, err := isKueueInUse(ctx, req.Client)
			if err != nil {
				return fmt.Errorf("checking kueue usage: %w", err)
			}

			setCondition := func(msg string, impact result.Impact) {
				req.Result.SetCondition(check.NewCondition(
					check.ConditionTypeCompatible,
					metav1.ConditionFalse,
					check.WithReason(check.ReasonVersionIncompatible),
					check.WithMessage("%s", msg),
					check.WithImpact(impact),
				))
			}

			switch {
			case req.ManagementState == constants.ManagementStateManaged && kueueInUse:
				setCondition(msgManagedProhibited, result.ImpactProhibited)
			case req.ManagementState == constants.ManagementStateManaged && !kueueInUse:
				setCondition(msgManagedBlocking, result.ImpactBlocking)
			case req.ManagementState == constants.ManagementStateUnmanaged && kueueInUse:
				setCondition(msgUnmanagedProhibited, result.ImpactProhibited)
			case req.ManagementState == constants.ManagementStateUnmanaged && !kueueInUse:
				setCondition(msgUnmanagedBlocking, result.ImpactBlocking)
			default:
				return fmt.Errorf("unexpected management state %q for kueue", req.ManagementState)
			}

			return nil
		})
}

// isKueueInUse returns true if at least one namespace is labeled for Kueue management
// or at least one monitored workload has the kueue.x-k8s.io/queue-name label.
func isKueueInUse(ctx context.Context, r client.Reader) (bool, error) {
	kueueNamespaces, err := kueuediscovery.KueueEnabledNamespaces(ctx, r)
	if err != nil {
		return false, fmt.Errorf("finding kueue-enabled namespaces: %w", err)
	}

	if kueueNamespaces.Len() > 0 {
		return true, nil
	}

	workloadNamespaces, err := kueuediscovery.WorkloadLabeledNamespaces(ctx, r)
	if err != nil {
		return false, fmt.Errorf("finding workload-labeled namespaces: %w", err)
	}

	return workloadNamespaces.Len() > 0, nil
}
