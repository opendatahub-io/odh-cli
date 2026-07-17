package ossm34

import (
	"context"
	"fmt"
	"strings"

	"github.com/blang/semver/v4"
	operatorsv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/util/version"
)

const (
	checkKind = "ossm-v3-compatibility"
	checkType = "compatibility"

	subscriptionName = "servicemeshoperator3"

	csvPrefix = "servicemeshoperator3.v"
)

var minAffectedVersion = semver.Version{Major: 3, Minor: 4, Patch: 0} //nolint:gochecknoglobals,mnd // constant-like semver

var minFixedOCPVersion = semver.Version{Major: 4, Minor: 21, Patch: 22} //nolint:gochecknoglobals,mnd // OCP 4.21.22 includes Sail Library fix

// Check detects servicemeshoperator3 subscription drift to v3.4.0+, which causes
// GatewayConfig to get stuck NotReady on OCP 4.19-4.21 due to Istio v1.26.2
// being rejected as end-of-life by the stricter version gate in OSSM 3.4.
type Check struct {
	check.BaseCheck
}

func NewCheck() *Check {
	return &Check{
		BaseCheck: check.BaseCheck{
			CheckGroup:       check.GroupDependency,
			Kind:             checkKind,
			Type:             checkType,
			CheckID:          "dependencies.ossm-v3-compatibility.compatibility",
			CheckName:        "Dependencies :: OSSM v3 Compatibility :: Version Compatibility",
			CheckDescription: "Detects servicemeshoperator3 subscription drift to v3.4.0+ which causes GatewayConfig failures on unpatched OCP 4.19-4.21",
			CheckRemediation: "Do not approve servicemeshoperator3 InstallPlans beyond v3.3.x on OCP 4.19-4.21. " +
				"Upgrade to OpenShift Container Platform 4.21.22 or higher to resolve via the Sail Library (no OLM dependency). " +
				"See https://access.redhat.com/solutions/7145505 for details.",
		},
	}
}

func (c *Check) CanApply(_ context.Context, target check.Target) (bool, error) {
	return version.IsVersion3x(target.TargetVersion), nil
}

func (c *Check) Validate(ctx context.Context, target check.Target) (*result.DiagnosticResult, error) {
	dr := c.NewResult()

	if !target.Client.OLM().Available() {
		dr.SetCondition(check.NewCondition(
			check.ConditionTypeCompatible,
			metav1.ConditionTrue,
			check.WithReason(check.ReasonCheckSkipped),
			check.WithMessage("OLM client not available; skipping OSSM v3 compatibility check"),
		))

		return dr, nil
	}

	subscriptions, err := target.Client.OLM().Subscriptions("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing subscriptions: %w", err)
	}

	sub := findSubscription(subscriptions)
	if sub == nil {
		dr.SetCondition(check.NewCondition(
			check.ConditionTypeCompatible,
			metav1.ConditionTrue,
			check.WithReason(check.ReasonRequirementsMet),
			check.WithMessage("servicemeshoperator3 subscription not found; OSSM v3 is not installed via OLM"),
		))

		return dr, nil
	}

	installedCSV := sub.Status.InstalledCSV
	if installedCSV == "" {
		dr.SetCondition(check.NewCondition(
			check.ConditionTypeCompatible,
			metav1.ConditionTrue,
			check.WithReason(check.ReasonRequirementsMet),
			check.WithMessage("servicemeshoperator3 subscription has no installed CSV; operator may be pending installation"),
		))

		return dr, nil
	}

	installedVersion, err := parseCSVVersion(installedCSV)
	if err != nil {
		dr.SetCondition(check.NewCondition(
			check.ConditionTypeCompatible,
			metav1.ConditionUnknown,
			check.WithReason(check.ReasonInsufficientData),
			check.WithMessage("Unable to parse version from installed CSV %q: %v", installedCSV, err),
		))

		return dr, nil
	}

	if installedVersion.GTE(minAffectedVersion) {
		ocpVersion, ocpErr := version.DetectOpenShiftVersion(ctx, target.Client)
		if ocpErr == nil && ocpVersion.GTE(minFixedOCPVersion) {
			dr.SetCondition(check.NewCondition(
				check.ConditionTypeCompatible,
				metav1.ConditionTrue,
				check.WithReason(check.ReasonVersionCompatible),
				check.WithMessage(
					"servicemeshoperator3 is at %s (affected), but OCP %s includes the fix (Sail Library); no action required",
					installedVersion, ocpVersion,
				),
			))

			return dr, nil
		}

		var startingCSV string
		if sub.Spec != nil {
			startingCSV = sub.Spec.StartingCSV
		}

		dr.SetCondition(check.NewCondition(
			check.ConditionTypeCompatible,
			metav1.ConditionFalse,
			check.WithReason(check.ReasonVersionIncompatible),
			check.WithMessage("%s", buildDriftMessage(installedVersion, installedCSV, startingCSV)),
			check.WithRemediation(c.CheckRemediation),
			check.WithImpact(result.ImpactBlocking),
		))

		return dr, nil
	}

	dr.SetCondition(check.NewCondition(
		check.ConditionTypeCompatible,
		metav1.ConditionTrue,
		check.WithReason(check.ReasonVersionCompatible),
		check.WithMessage("servicemeshoperator3 is installed at %s, which is not affected by the OSSM v3.4 compatibility issue", installedVersion),
	))

	return dr, nil
}

func findSubscription(list *operatorsv1alpha1.SubscriptionList) *operatorsv1alpha1.Subscription {
	for i := range list.Items {
		if list.Items[i].Name == subscriptionName {
			return &list.Items[i]
		}
	}

	return nil
}

func buildDriftMessage(installedVersion semver.Version, installedCSV, startingCSV string) string {
	message := fmt.Sprintf(
		"servicemeshoperator3 is installed at %s (CSV: %s), which is affected by a known issue: "+
			"OSSM v3.4.0+ rejects Istio v1.26.2 as end-of-life on OCP 4.19-4.21, "+
			"causing GatewayConfig to get stuck NotReady",
		installedVersion, installedCSV,
	)

	if startingCSV != "" && startingCSV != installedCSV {
		message += ". Subscription has drifted from pinned version " + startingCSV
	}

	return message
}

func parseCSVVersion(csv string) (semver.Version, error) {
	versionStr := strings.TrimPrefix(csv, csvPrefix)
	if versionStr == csv {
		return semver.Version{}, fmt.Errorf("CSV %q does not start with expected prefix %q", csv, csvPrefix)
	}

	v, err := semver.Parse(versionStr)
	if err != nil {
		return semver.Version{}, fmt.Errorf("parsing version %q: %w", versionStr, err)
	}

	return v, nil
}
