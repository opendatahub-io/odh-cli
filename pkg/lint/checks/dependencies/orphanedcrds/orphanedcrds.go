package orphanedcrds

import (
	"context"
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/lint/checks/dependencies/shared"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
	"github.com/opendatahub-io/odh-cli/pkg/util/jq"
	"github.com/opendatahub-io/odh-cli/pkg/util/version"
)

const (
	checkKind = "orphaned-sm2-crds"

	labelMaistraVersion = "maistra-version"
	crdSuffixIstio      = ".istio.io"
	packageSM2Operator  = "servicemeshoperator"
)

const (
	msgNoCRDs                   = "No orphaned Service Mesh v2 *.istio.io CRDs detected"
	msgSM2StillInstalled        = "Service Mesh v2 operator subscription found; *.istio.io CRDs with label %q are managed by the operator, not orphaned"
	msgOrphanedCRDs             = "Found %d orphaned *.istio.io CRD(s) from Service Mesh v2 (label %q): %s. These stale CRDs block the Data Science Gateway post-upgrade"
	msgActiveResources          = "Found %d orphaned *.istio.io CRD(s) from Service Mesh v2 with %d active custom resource instance(s) across them. Manual review required before deletion"
	msgResourceInspectionFailed = "Found %d orphaned *.istio.io CRD(s) from Service Mesh v2 but unable to verify whether active custom resources exist: %v. Manual inspection required before deletion"
)

const (
	remediationOrphaned         = "Delete the orphaned CRDs before upgrading: oc get crd -l maistra-version -o name | grep -E '\\.istio\\.io$' | xargs oc delete crd"
	remediationActiveResources  = "Review and remove active custom resources before deleting the orphaned CRDs. Run: for crd in $(oc get crd -l maistra-version -o custom-columns=NAME:.metadata.name --no-headers | grep -E '\\.istio\\.io$'); do echo \"$crd: $(oc get $crd -A --no-headers 2>/dev/null | wc -l) instances\"; done"
	remediationInspectionFailed = "Verify RBAC permissions allow listing custom resources, then re-run the check. Do not delete CRDs until active resource count is confirmed to be zero"
)

// Check detects orphaned *.istio.io CRDs from Service Mesh v2 that block the Data Science Gateway.
type Check struct {
	check.BaseCheck
}

// NewCheck creates a new orphaned SM2 CRDs check.
func NewCheck() *Check {
	return &Check{
		BaseCheck: check.BaseCheck{
			CheckGroup:       check.GroupDependency,
			Kind:             checkKind,
			Type:             check.CheckTypeReadiness,
			CheckID:          "dependencies.orphaned-sm2-crds.readiness",
			CheckName:        "Dependencies :: Orphaned SM2 CRDs :: Readiness",
			CheckDescription: "Detects orphaned *.istio.io CRDs from Service Mesh v2 that block Data Science Gateway post-upgrade",
			CheckRemediation: remediationOrphaned,
		},
	}
}

// CanApply returns true for 2.x to 3.x upgrades.
func (c *Check) CanApply(_ context.Context, target check.Target) (bool, error) {
	return version.IsUpgradeFrom2xTo3x(target.CurrentVersion, target.TargetVersion), nil
}

// Validate executes the check against the provided target.
func (c *Check) Validate(ctx context.Context, target check.Target) (*result.DiagnosticResult, error) {
	dr := c.NewResult()

	crds, err := target.Client.List(ctx, resources.CustomResourceDefinition,
		client.WithLabelSelector(labelMaistraVersion))
	if err != nil {
		return nil, fmt.Errorf("listing CRDs with label %q: %w", labelMaistraVersion, err)
	}

	istioCRDs := filterIstioCRDs(crds)
	if len(istioCRDs) == 0 {
		dr.SetCondition(check.NewCondition(
			check.ConditionTypeAvailable,
			metav1.ConditionTrue,
			check.WithReason(check.ReasonRequirementsMet),
			check.WithMessage(msgNoCRDs),
		))

		return dr, nil
	}

	sm2Installed, err := isSM2OperatorInstalled(ctx, target)
	if err != nil {
		return nil, fmt.Errorf("checking SM2 operator subscription: %w", err)
	}

	if sm2Installed {
		dr.SetCondition(check.NewCondition(
			check.ConditionTypeAvailable,
			metav1.ConditionTrue,
			check.WithReason(check.ReasonRequirementsMet),
			check.WithMessage(msgSM2StillInstalled, labelMaistraVersion),
		))

		return dr, nil
	}

	activeCount, err := countActiveResources(ctx, target, istioCRDs)
	if err != nil {
		dr.SetCondition(check.NewCondition(
			check.ConditionTypeAvailable,
			metav1.ConditionUnknown,
			check.WithReason(check.ReasonInsufficientData),
			check.WithMessage(msgResourceInspectionFailed, len(istioCRDs), err),
			check.WithRemediation(remediationInspectionFailed),
			check.WithImpact(result.ImpactBlocking),
		))

		populateImpactedObjects(dr, istioCRDs)

		return dr, nil
	}

	switch {
	case activeCount > 0:
		dr.SetCondition(check.NewCondition(
			check.ConditionTypeAvailable,
			metav1.ConditionFalse,
			check.WithReason(check.ReasonDependencyUnavailable),
			check.WithMessage(msgActiveResources, len(istioCRDs), activeCount),
			check.WithRemediation(remediationActiveResources),
			check.WithImpact(result.ImpactBlocking),
		))
	default:
		crdNames := collectCRDNames(istioCRDs)

		dr.SetCondition(check.NewCondition(
			check.ConditionTypeAvailable,
			metav1.ConditionFalse,
			check.WithReason(check.ReasonDependencyUnavailable),
			check.WithMessage(msgOrphanedCRDs, len(istioCRDs), labelMaistraVersion, strings.Join(crdNames, ", ")),
			check.WithRemediation(remediationOrphaned),
			check.WithImpact(result.ImpactBlocking),
		))
	}

	populateImpactedObjects(dr, istioCRDs)

	return dr, nil
}

func filterIstioCRDs(crds []*unstructured.Unstructured) []*unstructured.Unstructured {
	var filtered []*unstructured.Unstructured

	for _, crd := range crds {
		if strings.HasSuffix(crd.GetName(), crdSuffixIstio) {
			filtered = append(filtered, crd)
		}
	}

	return filtered
}

func isSM2OperatorInstalled(ctx context.Context, target check.Target) (bool, error) {
	if !target.Client.OLM().Available() {
		return false, nil
	}

	subscriptions, err := target.Client.OLM().Subscriptions("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return false, fmt.Errorf("listing subscriptions: %w", err)
	}

	for i := range subscriptions.Items {
		sub := &subscriptions.Items[i]
		if sub.Spec != nil && sub.Spec.Package == packageSM2Operator {
			return true, nil
		}
	}

	return false, nil
}

// countActiveResources counts custom resource instances across all orphaned CRDs.
// Returns an error if any CRD's resources cannot be listed, since an incomplete
// count could lead to recommending deletion when active resources exist.
func countActiveResources(
	ctx context.Context,
	target check.Target,
	crds []*unstructured.Unstructured,
) (int, error) {
	total := 0

	for _, crd := range crds {
		gvr, ok := extractGVR(crd)
		if !ok {
			continue
		}

		items, err := target.Client.ListResources(ctx, gvr)
		if err != nil {
			return 0, fmt.Errorf("listing resources for CRD %q: %w", crd.GetName(), err)
		}

		total += len(items)
	}

	return total, nil
}

// extractGVR derives the GVR from a CRD's spec fields using JQ.
func extractGVR(crd *unstructured.Unstructured) (schema.GroupVersionResource, bool) {
	group, err := jq.Query[string](crd, ".spec.group")
	if err != nil {
		return schema.GroupVersionResource{}, false
	}

	resource, err := jq.Query[string](crd, ".spec.names.plural")
	if err != nil {
		return schema.GroupVersionResource{}, false
	}

	// Use the first served version
	ver, err := jq.Query[string](crd, "[.spec.versions[]? | select(.served == true) | .name] | first")
	if err != nil || ver == "" {
		return schema.GroupVersionResource{}, false
	}

	return schema.GroupVersionResource{
		Group:    group,
		Version:  ver,
		Resource: resource,
	}, true
}

func collectCRDNames(crds []*unstructured.Unstructured) []string {
	names := make([]string, 0, len(crds))

	for _, crd := range crds {
		names = append(names, crd.GetName())
	}

	return names
}

func populateImpactedObjects(dr *result.DiagnosticResult, crds []*unstructured.Unstructured) {
	shared.AddAllImpactedObjects(dr, shared.ImpactedEntry{
		ResourceType: resources.CustomResourceDefinition,
		Items:        crds,
	})
}
