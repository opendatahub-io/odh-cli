package kueue

import (
	"context"
	"fmt"

	"github.com/opendatahub-io/odh-cli/pkg/constants"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
	"github.com/opendatahub-io/odh-cli/pkg/util/components"
)

// Intermediate resource types used to build the ownership graph.
// These appear in ownership chains between top-level CRs and Pods.
//
//nolint:gochecknoglobals // Static configuration for intermediate resource types.
var intermediateTypes = []resources.ResourceType{
	resources.Deployment,
	resources.StatefulSet,
	resources.ReplicaSet,
	resources.DaemonSet,
	resources.Job,
	resources.CronJob,
	resources.Pod,
}

// Condition type for the consolidated data-integrity check.
const (
	conditionTypeKueueConsistency = "KueueConsistency"
)

// Remediation guidance for kueue consistency violations.
const (
	remediationConsistency = "Ensure kueue-managed namespaces and workload kueue.x-k8s.io/queue-name labels are consistent. " +
		"Add the kueue-managed or kueue.openshift.io/managed label to namespaces with kueue workloads, " +
		"or add the kueue.x-k8s.io/queue-name label to all workloads in kueue-enabled namespaces"
)

// Messages for the consolidated KueueConsistency condition.
const (
	msgConsistent           = "All monitored workloads are consistent with kueue namespace configuration"
	msgNoRelevantNamespaces = "No kueue-managed namespaces or kueue-labeled workloads found"
	msgInconsistent         = "Found %d kueue consistency violation(s) across monitored workloads"
)

// Messages for individual violation descriptions.
const (
	// Invariant 1: workload in kueue namespace missing queue-name label.
	msgInvariant1 = "%s %s/%s is in kueue-managed namespace %s but missing kueue.x-k8s.io/queue-name label"

	// Invariant 2: workload with queue-name label in non-kueue namespace.
	msgInvariant2 = "%s %s/%s has kueue.x-k8s.io/queue-name=%s but namespace is not kueue-managed"

	// Invariant 3: owner tree label disagreement.
	msgInvariant3Missing    = "%s %s/%s has kueue.x-k8s.io/queue-name=%s but descendant %s %s/%s is missing the label"
	msgInvariant3Unexpected = "%s %s/%s has kueue.x-k8s.io/queue-name=%s but ancestor %s %s/%s does not have the label"
	msgInvariant3Mismatch   = "%s %s/%s has kueue.x-k8s.io/queue-name=%s but root %s %s/%s has kueue.x-k8s.io/queue-name=%s"
)

// IsKueueUnmanaged returns true when Kueue managementState is Unmanaged on the DSC.
// Data integrity checks only apply when the user manages Kueue themselves (Unmanaged state).
func IsKueueUnmanaged(
	ctx context.Context,
	target check.Target,
) (bool, error) {
	dsc, err := client.GetDataScienceCluster(ctx, target.Client)
	if err != nil {
		return false, fmt.Errorf("getting DataScienceCluster: %w", err)
	}

	return components.HasManagementState(
		dsc, constants.ComponentKueue,
		constants.ManagementStateUnmanaged,
	), nil
}
