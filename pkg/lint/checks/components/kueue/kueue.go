package kueue

import (
	"context"
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/opendatahub-io/odh-cli/pkg/constants"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/validate"
	kueuediscovery "github.com/opendatahub-io/odh-cli/pkg/lint/checks/kueue/discovery"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
	"github.com/opendatahub-io/odh-cli/pkg/util/components"
	"github.com/opendatahub-io/odh-cli/pkg/util/version"
)

const (
	kind                     = "kueue"
	checkTypeManagementState = "management-state"

	// Version-parameterized message formats. The single %s is the target's major.minor label
	// (e.g. "3.5"), supplied via version.MajorMinorLabel(req.TargetVersion) at call time.
	msgManagedProhibitedFmt   = "The %s upgrade does not support the Kueue managementState of Managed. Migrate Kueue to Unmanaged with the Red Hat build of Kueue Operator before upgrading."
	msgManagedBlockingFmt     = "The %s upgrade currently only supports the Kueue managementState of Removed. The Kueue managementState is currently Managed but no workloads on the cluster are using Kueue. Set the Kueue managementState to Removed and then re-run this check to proceed with migration."
	msgUnmanagedCompatibleFmt = "The Kueue managementState is Unmanaged, which is supported for the %s upgrade."

	// remediationManagedMigrate advises migrating away from embedded (Managed) Kueue. Mirrors
	// operatorInstalledManagedRemediation in kueue_operator_installed.go for consistency.
	remediationManagedMigrate = "Migrate to the Red Hat build of Kueue operator following https://docs.redhat.com/en/documentation/red_hat_openshift_ai_self-managed/2.25/html/managing_openshift_ai/managing-workloads-with-kueue#migrating-to-the-rhbok-operator_kueue before upgrading"

	// msgDefaultLocalQueueFmt describes the "default" LocalQueue hazard. The single %s is the
	// semicolon-joined list of triggers that fired.
	msgDefaultLocalQueueFmt = "Potential \"default\" LocalQueue naming conflict detected (%s). When Kueue is Unmanaged, workloads in kueue-managed namespaces that are not explicitly assigned to a queue are implicitly placed on the LocalQueue named \"default\", which may route them to an unintended queue."

	// remediationDefaultLocalQueue advises resolving the "default" LocalQueue hazard.
	remediationDefaultLocalQueue = "Rename spec.components.kueue.defaultLocalQueueName to a value other than \"default\", drain workloads from the existing \"default\" LocalQueue, and delete it"
)

// ManagementStateCheck validates the Kueue managementState is compatible before upgrading to 3.x.
//
// The check distinguishes between clusters that are actively using Kueue (namespaces or workloads
// labeled for Kueue) and those that have Kueue enabled but are not actually using it:
//   - Managed + in use: prohibited — migrate to the Red Hat build of Kueue Operator (Unmanaged) first
//   - Managed + not in use: blocking (recoverable — set managementState to Removed)
//   - Unmanaged (either): pass — a supported target state; the RHBoK operator requirement is
//     enforced separately by the components.kueue.operator-installed check
//
// In addition, whenever the upgrade could proceed onto an Unmanaged state (Managed+in use, which
// requires migration to Unmanaged; or already Unmanaged) it emits an advisory warning if a
// potentially dangerous "default" LocalQueue configuration is detected. The advisory layers on top
// of the base condition without escalating overall severity, because GetImpact() returns the max
// impact across conditions.
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
			label := version.MajorMinorLabel(req.TargetVersion)

			switch req.ManagementState {
			case constants.ManagementStateManaged:
				return c.validateManaged(ctx, req, label)
			case constants.ManagementStateUnmanaged:
				return c.validateUnmanaged(ctx, req, label)
			default:
				return fmt.Errorf("unexpected management state %q for kueue", req.ManagementState)
			}
		})
}

// validateManaged handles the Managed state. When Kueue is in use there is no supported upgrade
// path from embedded Kueue, so the base condition is prohibited (with migration remediation) and a
// default-LocalQueue advisory is layered on since the migration target is Unmanaged. When Kueue is
// not in use the state is recoverable, so the base condition is blocking (set managementState to
// Removed) and the advisory is not relevant.
func (c *ManagementStateCheck) validateManaged(
	ctx context.Context,
	req *validate.ComponentRequest,
	label string,
) error {
	kueueInUse, err := isKueueInUse(ctx, req.Client)
	if err != nil {
		return fmt.Errorf("checking kueue usage: %w", err)
	}

	if !kueueInUse {
		req.Result.SetCondition(check.NewCondition(
			check.ConditionTypeCompatible,
			metav1.ConditionFalse,
			check.WithReason(check.ReasonVersionIncompatible),
			check.WithMessage(msgManagedBlockingFmt, label),
			check.WithImpact(result.ImpactBlocking),
		))

		return nil
	}

	req.Result.SetCondition(check.NewCondition(
		check.ConditionTypeCompatible,
		metav1.ConditionFalse,
		check.WithReason(check.ReasonVersionIncompatible),
		check.WithMessage(msgManagedProhibitedFmt, label),
		check.WithImpact(result.ImpactProhibited),
		check.WithRemediation(remediationManagedMigrate),
	))

	return c.addDefaultLocalQueueWarning(ctx, req)
}

// validateUnmanaged handles the Unmanaged state, which is a supported target for the upgrade. The
// base condition passes (compatibility confirmed); the RHBoK operator requirement is enforced by
// the separate components.kueue.operator-installed check. A default-LocalQueue advisory is layered
// on when the hazard is detected — it never blocks the upgrade.
func (c *ManagementStateCheck) validateUnmanaged(
	ctx context.Context,
	req *validate.ComponentRequest,
	label string,
) error {
	req.Result.SetCondition(check.NewCondition(
		check.ConditionTypeCompatible,
		metav1.ConditionTrue,
		check.WithReason(check.ReasonVersionCompatible),
		check.WithMessage(msgUnmanagedCompatibleFmt, label),
	))

	return c.addDefaultLocalQueueWarning(ctx, req)
}

// addDefaultLocalQueueWarning appends an advisory condition when a potentially dangerous "default"
// LocalQueue configuration is detected. It layers on top of the base condition already set on the
// result; because GetImpact() returns the max impact across conditions, the advisory surfaces the
// hazard without escalating overall severity.
func (c *ManagementStateCheck) addDefaultLocalQueueWarning(
	ctx context.Context,
	req *validate.ComponentRequest,
) error {
	triggers, impacted, err := detectDefaultLocalQueue(ctx, req.Client, req.DSC)
	if err != nil {
		return fmt.Errorf("detecting default LocalQueue: %w", err)
	}

	if len(triggers) == 0 {
		return nil
	}

	req.Result.SetCondition(check.NewCondition(
		check.ConditionTypeConfigured,
		metav1.ConditionFalse,
		check.WithReason(check.ReasonDefaultLocalQueueConflict),
		check.WithMessage(msgDefaultLocalQueueFmt, strings.Join(triggers, "; ")),
		check.WithImpact(result.ImpactAdvisory),
		check.WithRemediation(remediationDefaultLocalQueue),
	))

	if len(impacted) > 0 {
		req.Result.AddImpactedObjects(resources.LocalQueue, impacted)
	}

	return nil
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
