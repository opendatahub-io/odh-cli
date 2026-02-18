package ray

import (
	"context"
	"fmt"
	"io"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/validate"
	"github.com/opendatahub-io/odh-cli/pkg/util/version"
)

func (c *ImpactedWorkloadsCheck) newCodeFlareRayClusterCondition(
	_ context.Context,
	req *validate.WorkloadRequest[*metav1.PartialObjectMetadata],
) []result.Condition {
	total := len(req.Items)
	targetLabel := version.MajorMinorLabel(req.TargetVersion)
	withAnnotation := 0
	for i := range req.Items {
		if req.Items[i].Annotations[RayPreUpgradeBackupAnnotation] != "" {
			withAnnotation++
		}
	}

	if total == 0 {
		return []result.Condition{check.NewCondition(
			ConditionTypeCodeFlareRayClusterCompatible,
			metav1.ConditionTrue,
			check.WithReason(check.ReasonVersionCompatible),
			check.WithMessage("No CodeFlare-managed RayCluster(s) found - ready for RHOAI %s upgrade", targetLabel),
		)}
	}
	if withAnnotation == total {
		return []result.Condition{check.NewCondition(
			ConditionTypeCodeFlareRayClusterCompatible,
			metav1.ConditionTrue,
			check.WithReason(check.ReasonVersionCompatible),
			check.WithMessage("All %d CodeFlare-managed RayCluster(s) have completed pre-upgrade steps - ready for RHOAI %s", total, targetLabel),
		)}
	}
	withoutAnnotation := total - withAnnotation

	return []result.Condition{check.NewCondition(
		ConditionTypeCodeFlareRayClusterCompatible,
		metav1.ConditionFalse,
		check.WithReason(check.ReasonVersionIncompatible),
		check.WithMessage("Found %d CodeFlare-managed RayCluster(s) which have not had pre-upgrade steps completed, not ready for RHOAI %s upgrade", withoutAnnotation, targetLabel),
		check.WithImpact(result.ImpactAdvisory),
		check.WithRemediation(c.CheckRemediation),
	)}
}

// formatRayImpactedObjects renders each RayCluster with [WARNING] when pre-upgrade steps are not
// complete (annotation odh.ray.io/pre-upgrade-backup-taken missing), and [INFO] when completed.
func formatRayImpactedObjects(out io.Writer, objects []metav1.PartialObjectMetadata) {
	for i := range objects {
		obj := &objects[i]
		name := obj.Name
		if obj.Namespace != "" {
			name = obj.Namespace + "/" + name
		}
		kind := obj.Kind
		if kind == "" {
			kind = "RayCluster"
		}
		if obj.Annotations[RayPreUpgradeBackupAnnotation] != "" {
			_, _ = fmt.Fprintf(out, "    - [INFO] %s (%s) - pre-upgrade steps completed\n", name, kind)
		} else {
			_, _ = fmt.Fprintf(out, "    - [WARNING] %s (%s) - pre-upgrade steps not complete\n", name, kind)
		}
	}
}
