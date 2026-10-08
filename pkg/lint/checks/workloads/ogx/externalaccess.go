package ogx

import (
	"context"
	"errors"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/opendatahub-io/odh-cli/pkg/constants"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/validate"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
	"github.com/opendatahub-io/odh-cli/pkg/util/components"
	"github.com/opendatahub-io/odh-cli/pkg/util/jq"
	"github.com/opendatahub-io/odh-cli/pkg/util/version"
)

const (
	conditionTypeExternalAccessTLSConfigured = "ExternalAccessTLSConfigured"

	msgNoImpactedOGXServers = "No OGXServer(s) with external access enabled and a missing hostname or TLS Secret found" +
		" - ready for OpenShift AI 3.6"
	msgImpactedOGXServers = "Found %d OGXServer(s) with external access enabled and a missing hostname or TLS Secret." +
		" OpenShift AI 3.6 requires both when external access is enabled, and rejects changes to these" +
		" OGXServer(s) that leave external access enabled without both"
)

// ExternalAccessTLSCheck reports OGXServers that enable external access without a hostname or a
// TLS Secret, which OpenShift AI 3.6 requires. The 3.5 API has no externalAccess.tls field, so
// affected OGXServers cannot be fixed before the upgrade and the check is advisory.
type ExternalAccessTLSCheck struct {
	check.BaseCheck
}

func NewExternalAccessTLSCheck() *ExternalAccessTLSCheck {
	return &ExternalAccessTLSCheck{
		BaseCheck: check.BaseCheck{
			CheckGroup: check.GroupWorkload,
			Kind:       constants.ComponentOGX,
			Type:       check.CheckTypeImpactedWorkloads,
			CheckID:    "workloads.ogx.externalaccess-tls",
			CheckName:  "Workloads :: OGX :: External Access TLS (3.6+)",
			CheckDescription: "Detects OGXServers with external access enabled and no hostname or TLS Secret, " +
				"which OpenShift AI 3.6 requires",
			CheckRemediation: "After upgrading, set spec.network.externalAccess.hostname and " +
				"spec.network.externalAccess.tls.secretName on each listed OGXServer. The Secret must be a TLS " +
				"Secret for that hostname in the OGXServer's namespace. If an OGXServer does not need external " +
				"access, set spec.network.externalAccess.enabled to false before upgrading instead.",
		},
	}
}

// CanApply applies to upgrades from before 3.6 to 3.6 or newer when OGX is managed.
func (c *ExternalAccessTLSCheck) CanApply(ctx context.Context, target check.Target) (bool, error) {
	//nolint:mnd // Version numbers 3.6
	if !version.IsVersionAtLeast(target.TargetVersion, 3, 6) || version.IsVersionAtLeast(target.CurrentVersion, 3, 6) {
		return false, nil
	}

	dsc, err := client.GetDataScienceCluster(ctx, target.Client)
	if err != nil {
		return false, fmt.Errorf("getting DataScienceCluster: %w", err)
	}

	return components.HasManagementState(dsc, constants.ComponentOGX, constants.ManagementStateManaged), nil
}

func (c *ExternalAccessTLSCheck) Validate(
	ctx context.Context,
	target check.Target,
) (*result.DiagnosticResult, error) {
	return validate.Workloads(c, target, resources.OGXServer).
		ForComponent(constants.ComponentOGX).
		Filter(lacksExternalAccessTLS).
		Complete(ctx, c.newExternalAccessTLSCondition)
}

// lacksExternalAccessTLS mirrors the OpenShift AI 3.6 operator's validation: when external access
// is enabled, both a hostname and a TLS Secret name must be non-empty.
func lacksExternalAccessTLS(obj *unstructured.Unstructured) (bool, error) {
	enabled, err := jq.Query[bool](obj, ".spec.network.externalAccess.enabled")
	if errors.Is(err, jq.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !enabled {
		return false, nil
	}

	hostname, err := queryOptionalString(obj, ".spec.network.externalAccess.hostname")
	if err != nil {
		return false, err
	}

	secretName, err := queryOptionalString(obj, ".spec.network.externalAccess.tls.secretName")
	if err != nil {
		return false, err
	}

	return hostname == "" || secretName == "", nil
}

func queryOptionalString(obj *unstructured.Unstructured, query string) (string, error) {
	value, err := jq.Query[string](obj, query)
	if errors.Is(err, jq.ErrNotFound) {
		return "", nil
	}

	return value, err
}

func (c *ExternalAccessTLSCheck) newExternalAccessTLSCondition(
	_ context.Context,
	req *validate.WorkloadRequest[*unstructured.Unstructured],
) ([]result.Condition, error) {
	if len(req.Items) == 0 {
		return []result.Condition{check.NewCondition(
			conditionTypeExternalAccessTLSConfigured,
			metav1.ConditionTrue,
			check.WithReason(check.ReasonVersionCompatible),
			check.WithMessage(msgNoImpactedOGXServers),
		)}, nil
	}

	return []result.Condition{check.NewCondition(
		conditionTypeExternalAccessTLSConfigured,
		metav1.ConditionFalse,
		check.WithReason(check.ReasonWorkloadsImpacted),
		check.WithMessage(msgImpactedOGXServers, len(req.Items)),
		check.WithImpact(result.ImpactAdvisory),
		check.WithRemediation(c.CheckRemediation),
	)}, nil
}
