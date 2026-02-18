package ray

import (
	"context"
	"fmt"
	"io"
	"slices"

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

const (
	kind                    = "ray"
	finalizerCodeFlareOAuth = "ray.openshift.ai/oauth-finalizer"
	// RayPreUpgradeBackupAnnotation is set on RayClusters after the pre-upgrade backup is taken (value: RFC3339 UTC timestamp).
	RayPreUpgradeBackupAnnotation = "odh.ray.io/pre-upgrade-backup-taken"
)

const (
	ConditionTypeCodeFlareRayClusterCompatible = "CodeFlareRayClustersCompatible"
)

// ImpactedWorkloadsCheck lists RayClusters managed by CodeFlare.
type ImpactedWorkloadsCheck struct {
	check.BaseCheck
}

func NewImpactedWorkloadsCheck() *ImpactedWorkloadsCheck {
	return &ImpactedWorkloadsCheck{
		BaseCheck: check.BaseCheck{
			CheckGroup:       check.GroupWorkload,
			Kind:             kind,
			Type:             check.CheckTypeImpactedWorkloads,
			CheckID:          "workloads.ray.impacted-workloads",
			CheckName:        "Workloads :: Ray :: Impacted Workloads (3.x)",
			CheckDescription: "Lists RayClusters managed by CodeFlare that will be impacted in RHOAI 3.x (CodeFlare not available)",
			CheckRemediation: "Delete or back up CodeFlare-managed RayClusters before upgrading, as CodeFlare will not be available in RHOAI 3.x",
		},
	}
}

// CanApply returns whether this check should run for the given target.
// Only applies when upgrading FROM 2.x TO 3.x and Ray is Managed.
func (c *ImpactedWorkloadsCheck) CanApply(ctx context.Context, target check.Target) (bool, error) {
	if !version.IsUpgradeFrom2xTo3x(target.CurrentVersion, target.TargetVersion) {
		return false, nil
	}

	dsc, err := client.GetDataScienceCluster(ctx, target.Client)
	if err != nil {
		return false, fmt.Errorf("getting DataScienceCluster: %w", err)
	}

	return components.HasManagementState(dsc, kind, constants.ManagementStateManaged), nil
}

// validateRayClustersWithBackupStatus sets conditions and ImpactedObjects from full metadata
// so the pre-upgrade backup annotation (odh.ray.io/pre-upgrade-backup-taken) can be displayed per cluster.
func (c *ImpactedWorkloadsCheck) validateRayClustersWithBackupStatus(
	ctx context.Context,
	req *validate.WorkloadRequest[*metav1.PartialObjectMetadata],
) error {
	conditions := c.newCodeFlareRayClusterCondition(ctx, req)
	for _, cond := range conditions {
		req.Result.SetCondition(cond)
	}

	// Preserve full metadata (including annotations) so the group renderer can show warning vs info.
	req.Result.ImpactedObjects = make([]metav1.PartialObjectMetadata, 0, len(req.Items))
	for i := range req.Items {
		obj := req.Items[i].DeepCopy()
		if obj.APIVersion == "" {
			obj.APIVersion = resources.RayCluster.APIVersion()
		}
		if obj.Kind == "" {
			obj.Kind = resources.RayCluster.Kind
		}
		req.Result.ImpactedObjects = append(req.Result.ImpactedObjects, *obj)
	}

	return nil
}

// Validate executes the check against the provided target.
// ImpactedObjects are set from full metadata so the pre-upgrade backup annotation can be shown (warning vs info).
func (c *ImpactedWorkloadsCheck) Validate(
	ctx context.Context,
	target check.Target,
) (*result.DiagnosticResult, error) {
	return validate.WorkloadsMetadata(c, target, resources.RayCluster).
		Filter(func(cluster *metav1.PartialObjectMetadata) (bool, error) {
			return slices.Contains(cluster.GetFinalizers(), finalizerCodeFlareOAuth), nil
		}).
		Run(ctx, c.validateRayClustersWithBackupStatus)
}

// FormatVerboseOutput implements check.VerboseOutputFormatter.
// Renders each RayCluster with [WARNING] when the pre-upgrade backup annotation is missing,
// and [INFO] when present (odh.ray.io/pre-upgrade-backup-taken).
func (c *ImpactedWorkloadsCheck) FormatVerboseOutput(out io.Writer, dr *result.DiagnosticResult) {
	formatRayImpactedObjects(out, dr.ImpactedObjects)
}
