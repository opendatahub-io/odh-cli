package servicemesh

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

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
			check.WithMessage("ingress-operator deployment not found in openshift-ingress-operator namespace"),
			check.WithRemediation("Check that the openshift-ingress-operator namespace and the ingress-operator deployment exist in the cluster."),
			check.WithImpact(result.ImpactBlocking),
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

	// Step 2: Extract the required version from the GATEWAY_API_OPERATOR_VERSION env var.
	requiredVersion, earlyResult, err := extractEnvVar(deploy, dr, "GATEWAY_API_OPERATOR_VERSION")
	if earlyResult != nil || err != nil {
		return earlyResult, err
	}

	// Step 3: Extract the required channel from the GATEWAY_API_OPERATOR_CHANNEL env var.
	requiredChannel, earlyResult, err := extractEnvVar(deploy, dr, "GATEWAY_API_OPERATOR_CHANNEL")
	if earlyResult != nil || err != nil {
		return earlyResult, err
	}

	// Step 4: The env var contains the full CSV name (e.g. "servicemeshoperator3.v3.1.0").
	requiredCSV := requiredVersion
	displayVersion := strings.TrimPrefix(requiredCSV, "servicemeshoperator3.v")

	// Step 5: Find the servicemeshoperator3 PackageManifest from redhat-operators in openshift-marketplace.
	// Multiple catalog sources can produce PackageManifests with the same name, so we list all
	// PackageManifests and filter by .status.catalogSource. A direct get-by-name would return a
	// non-deterministic result when multiple catalog sources provide the same package.
	manifests, err := client.List[*unstructured.Unstructured](ctx, target.Client, resources.PackageManifest,
		func(pm *unstructured.Unstructured) (bool, error) {
			if pm.GetName() != "servicemeshoperator3" || pm.GetNamespace() != "openshift-marketplace" {
				return false, nil
			}

			catalogSource, err := jq.Query[string](pm, ".status.catalogSource")
			if err != nil {
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

	// Step 6: Extract available CSVs from the required channel.
	availableCSVs, err := jq.Query[[]string](pm,
		fmt.Sprintf(`[.status.channels[]? | select(.name == "%s") | .entries[]?.name]`, requiredChannel))
	if err != nil {
		return nil, fmt.Errorf("querying available CSVs: %w", err)
	}

	// Step 7: Check if the required CSV is available.
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
