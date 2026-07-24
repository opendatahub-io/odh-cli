package servicemesh

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	operatorsv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
	"github.com/opendatahub-io/odh-cli/pkg/util/jq"
	"github.com/opendatahub-io/odh-cli/pkg/util/version"
)

const kind = "servicemesh-v3"

const displayName = "Red Hat Service Mesh v3"

const subscriptionName = "servicemeshoperator3"

// mirrorRemediationFmt is the shared remediation template for failures where the required
// servicemeshoperator3 CSV is not available in the cluster catalog.
const mirrorRemediationFmt = "Mirror %s into the '%s' channel of the redhat-operators catalog source in the openshift-marketplace namespace. See the pre-requisite instructions in the RHOAI 2.x to 3.x upgrade guide."

// Check validates that the required Service Mesh v3 version is available in the cluster's operator catalog.
type Check struct {
	check.BaseCheck
}

func NewCheck() *Check {
	return &Check{
		BaseCheck: check.BaseCheck{
			CheckGroup:       check.GroupDependency,
			Kind:             kind,
			Type:             check.CheckTypeInstalled,
			CheckID:          "dependencies.servicemesh.installed",
			CheckName:        "Dependencies :: Service Mesh v3 :: Installed",
			CheckDescription: "Validates that the required Service Mesh v3 version is available to install from the cluster's operator catalog",
		},
	}
}

func (c *Check) CanApply(_ context.Context, target check.Target) (bool, error) {
	return version.IsUpgradeFrom2xTo3x(target.CurrentVersion, target.TargetVersion), nil
}

// extractEnvVar extracts a named environment variable from the ingress-operator deployment.
// It returns the value, a diagnostic result if the variable is missing/empty, and an error.
// When the returned result is non-nil, the caller should return it immediately.
func extractEnvVar(deploy *unstructured.Unstructured, dr *result.DiagnosticResult, envName string) (string, *result.DiagnosticResult, error) {
	value, err := jq.Query[string](deploy,
		fmt.Sprintf(`[.spec.template.spec.containers[].env[]? | select(.name == "%s") | .value] | first`, envName))

	switch {
	case errors.Is(err, jq.ErrNotFound):
		dr.SetCondition(check.NewCondition(
			check.ConditionTypeAvailable,
			metav1.ConditionFalse,
			check.WithReason(check.ReasonDependencyUnavailable),
			check.WithMessage("%s env var not found on ingress-operator deployment", envName),
			check.WithRemediation(fmt.Sprintf("Verify the ingress-operator deployment in the openshift-ingress-operator namespace has the %s environment variable.", envName)),
			check.WithImpact(result.ImpactBlocking),
		))

		return "", dr, nil
	case err != nil:
		return "", nil, fmt.Errorf("querying %s: %w", envName, err)
	case value == "":
		dr.SetCondition(check.NewCondition(
			check.ConditionTypeAvailable,
			metav1.ConditionFalse,
			check.WithReason(check.ReasonDependencyUnavailable),
			check.WithMessage("%s env var is empty on ingress-operator deployment", envName),
			check.WithRemediation(fmt.Sprintf("Verify the ingress-operator deployment in the openshift-ingress-operator namespace has a non-empty %s environment variable.", envName)),
			check.WithImpact(result.ImpactBlocking),
		))

		return "", dr, nil
	}

	return value, nil, nil
}

func (c *Check) Validate(ctx context.Context, target check.Target) (*result.DiagnosticResult, error) {
	dr := c.NewResult()

	// Step 1: Get the ingress-operator deployment to determine the required version and channel.
	deploy, err := target.Client.GetResource(ctx, resources.Deployment, "ingress-operator",
		client.InNamespace("openshift-ingress-operator"))

	switch {
	case apierrors.IsNotFound(err):
		dr.SetCondition(check.NewCondition(
			check.ConditionTypeAvailable,
			metav1.ConditionFalse,
			check.WithReason(check.ReasonResourceNotFound),
			check.WithMessage("ingress-operator deployment not found in openshift-ingress-operator namespace; unable to determine required OSSM version (expected on HCP clusters)"),
			check.WithRemediation("If this is a Hosted Control Plane (HCP) cluster, this check can be safely ignored. Otherwise, verify that the openshift-ingress-operator namespace and ingress-operator deployment exist."),
			check.WithImpact(result.ImpactAdvisory),
		))

		return dr, nil
	case err != nil:
		return nil, fmt.Errorf("getting ingress-operator deployment: %w", err)
	case deploy == nil:
		dr.SetCondition(check.NewCondition(
			check.ConditionTypeAvailable,
			metav1.ConditionFalse,
			check.WithReason(check.ReasonInsufficientData),
			check.WithMessage("Unable to read ingress-operator deployment (insufficient permissions)"),
			check.WithRemediation("Grant read access to deployments in the openshift-ingress-operator namespace."),
			check.WithImpact(result.ImpactBlocking),
		))

		return dr, nil
	}

	// Step 2: Check for noOLM mode. On OCP 4.21.22+ / 4.22+, the Cluster Ingress Operator
	// deploys istiod directly (Sail Library) without an OSSM OLM subscription. When no
	// servicemeshoperator3 subscription exists, the PackageManifest validation is not applicable.
	// This must run before env var extraction: noOLM clusters may lack the GATEWAY_API_OPERATOR_*
	// env vars entirely, which would otherwise produce false-positive blocking failures.
	if isNoOLMMode(ctx, target) {
		dr.SetCondition(check.NewCondition(
			check.ConditionTypeAvailable,
			metav1.ConditionTrue,
			check.WithReason(check.ReasonRequirementsMet),
			check.WithMessage("Gateway API is managed by the Cluster Ingress Operator (noOLM mode); %s catalog entry not required", subscriptionName),
		))

		return dr, nil
	}

	// Step 3: Extract the required version from the GATEWAY_API_OPERATOR_VERSION env var.
	requiredVersion, earlyResult, err := extractEnvVar(deploy, dr, "GATEWAY_API_OPERATOR_VERSION")
	if earlyResult != nil || err != nil {
		return earlyResult, err
	}

	// Step 4: Extract the required channel from the GATEWAY_API_OPERATOR_CHANNEL env var.
	requiredChannel, earlyResult, err := extractEnvVar(deploy, dr, "GATEWAY_API_OPERATOR_CHANNEL")
	if earlyResult != nil || err != nil {
		return earlyResult, err
	}

	// Step 5: The env var contains the full CSV name (e.g. "servicemeshoperator3.v3.1.0").
	requiredCSV := requiredVersion
	displayVersion := strings.TrimPrefix(requiredCSV, "servicemeshoperator3.v")

	// Step 6: Validate that the required CSV is available in the OLM catalog.
	return validateCatalogCSV(ctx, target, dr, requiredCSV, requiredChannel, displayVersion)
}

// validateCatalogCSV validates that the required servicemeshoperator3 CSV is available
// in the redhat-operators catalog. Reached only when a subscription exists (OLM mode).
func validateCatalogCSV(
	ctx context.Context,
	target check.Target,
	dr *result.DiagnosticResult,
	requiredCSV, requiredChannel, displayVersion string,
) (*result.DiagnosticResult, error) {
	manifests, err := client.List[*unstructured.Unstructured](ctx, target.Client, resources.PackageManifest,
		func(pm *unstructured.Unstructured) (bool, error) {
			if pm.GetName() != "servicemeshoperator3" || pm.GetNamespace() != "openshift-marketplace" {
				return false, nil
			}

			catalogSource, queryErr := jq.Query[string](pm, ".status.catalogSource")
			if queryErr != nil {
				return false, nil
			}

			return catalogSource == "redhat-operators", nil
		})
	if err != nil {
		return nil, fmt.Errorf("listing PackageManifests: %w", err)
	}

	if len(manifests) == 0 {
		dr.SetCondition(check.NewCondition(
			check.ConditionTypeAvailable,
			metav1.ConditionFalse,
			check.WithReason(check.ReasonResourceNotFound),
			check.WithMessage("servicemeshoperator3 PackageManifest not found in redhat-operators catalog (required: %s in '%s' channel)", requiredCSV, requiredChannel),
			check.WithRemediation(fmt.Sprintf(mirrorRemediationFmt, requiredCSV, requiredChannel)),
			check.WithImpact(result.ImpactBlocking),
		))

		return dr, nil
	}

	pm := manifests[0]

	availableCSVs, err := jq.Query[[]string](pm,
		fmt.Sprintf(`[.status.channels[]? | select(.name == "%s") | .entries[]?.name]`, requiredChannel))
	if err != nil {
		return nil, fmt.Errorf("querying available CSVs: %w", err)
	}

	if slices.Contains(availableCSVs, requiredCSV) {
		dr.SetCondition(check.NewCondition(
			check.ConditionTypeAvailable,
			metav1.ConditionTrue,
			check.WithReason(check.ReasonResourceFound),
			check.WithMessage("%s (%s) is available in the '%s' channel of the 'redhat-operators' cluster catalog", displayName, requiredCSV, requiredChannel),
		))
	} else {
		dr.SetCondition(check.NewCondition(
			check.ConditionTypeAvailable,
			metav1.ConditionFalse,
			check.WithReason(check.ReasonDependencyUnavailable),
			check.WithMessage("%s version %s is not available in the '%s' channel of the cluster catalog", displayName, displayVersion, requiredChannel),
			check.WithRemediation(fmt.Sprintf(mirrorRemediationFmt, requiredCSV, requiredChannel)),
			check.WithImpact(result.ImpactBlocking),
		))
	}

	return dr, nil
}

// isNoOLMMode checks whether the cluster is running in noOLM Gateway API mode.
// Returns true when OLM is unavailable or no servicemeshoperator3 subscription exists.
func isNoOLMMode(ctx context.Context, target check.Target) bool {
	if !target.Client.OLM().Available() {
		return true
	}

	subscriptions, err := target.Client.OLM().Subscriptions("").List(ctx, metav1.ListOptions{})
	if err != nil {
		// Cannot determine subscription state (e.g. RBAC restriction);
		// fall through to OLM catalog validation rather than failing the check.
		return false
	}

	return findSubscription(subscriptions) == nil
}

func findSubscription(list *operatorsv1alpha1.SubscriptionList) *operatorsv1alpha1.Subscription {
	for i := range list.Items {
		if list.Items[i].Name == subscriptionName {
			return &list.Items[i]
		}
	}

	return nil
}
