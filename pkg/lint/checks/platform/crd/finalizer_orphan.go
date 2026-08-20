package crd

import (
	"context"
	"fmt"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/opendatahub-io/odh-cli/pkg/constants"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
	"github.com/opendatahub-io/odh-cli/pkg/util/jq"
	"github.com/opendatahub-io/odh-cli/pkg/util/version"
)

const (
	conditionTypeFinalizerHealth = "FinalizerHealth"
	operatorLabelSelector        = "operators.coreos.com/rhods-operator.redhat-ods-operator"

	msgFinalizersHealthy = "No finalizer health issues detected"
	msgNoFinalizersFound = "No finalizers found on DSC/DSCI resources"
	msgOperatorUnhealthy = "Finalizers present on %s but operator deployment is unhealthy: %s"
	msgOperatorNotFound  = "Finalizers present on %s but no operator deployment found"
)

// FinalizerOrphanCheck verifies that when DSC/DSCI have finalizers, the operator
// deployment is healthy and able to process them. Post-CRD-update: the operator is
// running (it updated the CRDs), so this is a health check, not an existence check.
type FinalizerOrphanCheck struct {
	check.BaseCheck
}

func NewFinalizerOrphanCheck() *FinalizerOrphanCheck {
	return &FinalizerOrphanCheck{
		BaseCheck: check.BaseCheck{
			CheckGroup:       check.GroupPlatform,
			Kind:             constants.PlatformCRD,
			Type:             check.CheckTypeFinalizerOrphan,
			CheckID:          "platform.crd.finalizer-orphan",
			CheckName:        "Platform :: CRD :: DSC/DSCI Finalizer Health (3.x)",
			CheckDescription: "Verifies operator is healthy when DSC/DSCI have finalizers",
			CheckRemediation: "Check operator pod logs and ensure the operator deployment is healthy. If the operator is crashlooping, investigate the cause before proceeding",
		},
	}
}

func (c *FinalizerOrphanCheck) CanApply(_ context.Context, target check.Target) (bool, error) {
	return version.IsUpgradeFrom2xTo3x(target.CurrentVersion, target.TargetVersion), nil
}

func (c *FinalizerOrphanCheck) Validate(
	ctx context.Context,
	target check.Target,
) (*result.DiagnosticResult, error) {
	dr := c.NewResult()

	if target.TargetVersion != nil {
		dr.Annotations[check.AnnotationCheckTargetVersion] = target.TargetVersion.String()
	}

	var resourcesWithFinalizers []string

	dsc, err := client.GetDataScienceCluster(ctx, target.Client)
	if err != nil && !apierrors.IsNotFound(err) {
		return nil, fmt.Errorf("getting DataScienceCluster: %w", err)
	}

	if dsc != nil && len(dsc.GetFinalizers()) > 0 {
		resourcesWithFinalizers = append(resourcesWithFinalizers, "DataScienceCluster")
	}

	dsci, err := client.GetDSCInitialization(ctx, target.Client)
	if err != nil && !apierrors.IsNotFound(err) {
		return nil, fmt.Errorf("getting DSCInitialization: %w", err)
	}

	if dsci != nil && len(dsci.GetFinalizers()) > 0 {
		resourcesWithFinalizers = append(resourcesWithFinalizers, "DSCInitialization")
	}

	if len(resourcesWithFinalizers) == 0 {
		dr.SetCondition(check.NewCondition(
			conditionTypeFinalizerHealth,
			metav1.ConditionTrue,
			check.WithReason(check.ReasonRequirementsMet),
			check.WithMessage(msgNoFinalizersFound),
		))

		return dr, nil
	}

	resourceNames := strings.Join(resourcesWithFinalizers, ", ")

	deployments, err := target.Client.List(ctx, resources.Deployment,
		client.WithLabelSelector(operatorLabelSelector))
	if err != nil {
		return nil, fmt.Errorf("listing operator deployments: %w", err)
	}

	if len(deployments) == 0 {
		dr.SetCondition(check.NewCondition(
			conditionTypeFinalizerHealth,
			metav1.ConditionFalse,
			check.WithReason(check.ReasonResourceNotFound),
			check.WithMessage(msgOperatorNotFound, resourceNames),
			check.WithImpact(result.ImpactAdvisory),
			check.WithRemediation(c.CheckRemediation),
		))

		return dr, nil
	}

	for _, deploy := range deployments {
		if !isDeploymentHealthy(deploy) {
			detail := getDeploymentUnhealthyDetail(deploy)

			dr.SetCondition(check.NewCondition(
				conditionTypeFinalizerHealth,
				metav1.ConditionFalse,
				check.WithReason(check.ReasonResourceUnavailable),
				check.WithMessage(msgOperatorUnhealthy, resourceNames, detail),
				check.WithImpact(result.ImpactAdvisory),
				check.WithRemediation(c.CheckRemediation),
			))

			return dr, nil
		}
	}

	dr.SetCondition(check.NewCondition(
		conditionTypeFinalizerHealth,
		metav1.ConditionTrue,
		check.WithReason(check.ReasonRequirementsMet),
		check.WithMessage(msgFinalizersHealthy),
	))

	return dr, nil
}

func isDeploymentHealthy(deploy *unstructured.Unstructured) bool {
	available, _ := jq.Query[int64](deploy, ".status.availableReplicas")

	return available > 0
}

func getDeploymentUnhealthyDetail(deploy *unstructured.Unstructured) string {
	available, _ := jq.Query[int64](deploy, ".status.availableReplicas")
	replicas, _ := jq.Query[int64](deploy, ".status.replicas")

	return fmt.Sprintf("%d/%d replicas available", available, replicas)
}
