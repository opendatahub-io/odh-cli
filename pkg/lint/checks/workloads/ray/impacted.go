package ray

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/blang/semver/v4"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/lburgazzoli/odh-cli/pkg/lint/check"
	"github.com/lburgazzoli/odh-cli/pkg/lint/check/result"
	"github.com/lburgazzoli/odh-cli/pkg/resources"
)

const (
	checkID          = "workloads.ray.impacted-workloads"
	checkName        = "Workloads :: Ray :: Impacted Workloads (3.x)"
	checkDescription = "Lists RayClusters managed by CodeFlare that will be impacted in RHOAI 3.x (CodeFlare not available)"

	finalizerCodeFlareOAuth = "ray.openshift.ai/oauth-finalizer"
)

type impactedResource struct {
	namespace string
	name      string
}

// ImpactedWorkloadsCheck lists RayClusters managed by CodeFlare.
type ImpactedWorkloadsCheck struct{}

// ID returns the unique identifier for this check.
func (c *ImpactedWorkloadsCheck) ID() string {
	return checkID
}

// Name returns the human-readable check name.
func (c *ImpactedWorkloadsCheck) Name() string {
	return checkName
}

// Description returns what this check validates.
func (c *ImpactedWorkloadsCheck) Description() string {
	return checkDescription
}

// Group returns the check group.
func (c *ImpactedWorkloadsCheck) Group() check.CheckGroup {
	return check.GroupWorkload
}

// CanApply returns whether this check should run for the given versions.
// This check only applies when upgrading FROM 2.x TO 3.x.
func (c *ImpactedWorkloadsCheck) CanApply(
	currentVersion *semver.Version,
	targetVersion *semver.Version,
) bool {
	if currentVersion == nil || targetVersion == nil {
		return false
	}

	return currentVersion.Major == 2 && targetVersion.Major >= 3
}

// Validate executes the check against the provided target.
func (c *ImpactedWorkloadsCheck) Validate(
	ctx context.Context,
	target *check.CheckTarget,
) (*result.DiagnosticResult, error) {
	dr := result.New(
		string(check.GroupWorkload),
		check.ComponentRay,
		check.CheckTypeImpactedWorkloads,
		checkDescription,
	)

	if target.Version != nil {
		dr.Annotations[check.AnnotationCheckTargetVersion] = target.Version.Version
	}

	// Find impacted RayClusters
	impactedClusters, err := c.findImpactedRayClusters(ctx, target)
	if err != nil {
		return nil, err
	}

	totalImpacted := len(impactedClusters)
	dr.Annotations[check.AnnotationImpactedWorkloadCount] = strconv.Itoa(totalImpacted)

	if totalImpacted == 0 {
		dr.Status.Conditions = []metav1.Condition{
			check.NewCondition(
				check.ConditionTypeCompatible,
				metav1.ConditionTrue,
				check.ReasonVersionCompatible,
				"No CodeFlare-managed RayClusters found - ready for RHOAI 3.x upgrade",
			),
		}

		return dr, nil
	}

	message := c.buildImpactMessage(impactedClusters)

	dr.Status.Conditions = []metav1.Condition{
		check.NewCondition(
			check.ConditionTypeCompatible,
			metav1.ConditionFalse,
			check.ReasonVersionIncompatible,
			message,
		),
	}

	return dr, nil
}

func (c *ImpactedWorkloadsCheck) findImpactedRayClusters(
	ctx context.Context,
	target *check.CheckTarget,
) ([]impactedResource, error) {
	rayClusters, err := target.Client.List(ctx, resources.RayCluster)
	if err != nil {
		return nil, fmt.Errorf("listing RayClusters: %w", err)
	}

	var impacted []impactedResource

	for i := range rayClusters {
		cluster := &rayClusters[i]
		finalizers := cluster.GetFinalizers()

		if slices.Contains(finalizers, finalizerCodeFlareOAuth) {
			impacted = append(impacted, impactedResource{
				namespace: cluster.GetNamespace(),
				name:      cluster.GetName(),
			})
		}
	}

	return impacted, nil
}

func (c *ImpactedWorkloadsCheck) buildImpactMessage(
	impactedClusters []impactedResource,
) string {
	resourceStrs := make([]string, len(impactedClusters))
	for i, r := range impactedClusters {
		resourceStrs[i] = fmt.Sprintf("%s/%s (CodeFlare-managed)", r.namespace, r.name)
	}

	return fmt.Sprintf(
		"Found %d CodeFlare-managed RayCluster(s) that will be impacted (CodeFlare not available in RHOAI 3.x): %s",
		len(impactedClusters),
		strings.Join(resourceStrs, ", "),
	)
}

// Register the check in the global registry.
//
//nolint:gochecknoinits // Required for auto-registration pattern
func init() {
	check.MustRegisterCheck(&ImpactedWorkloadsCheck{})
}
