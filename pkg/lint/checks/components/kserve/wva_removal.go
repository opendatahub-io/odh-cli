package kserve

import (
	"context"
	"errors"
	"fmt"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/opendatahub-io/odh-cli/pkg/constants"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/validate"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
	"github.com/opendatahub-io/odh-cli/pkg/util/jq"
	"github.com/opendatahub-io/odh-cli/pkg/util/version"
)

const wvaRemovalCheckType = "wva-removal"

const (
	wvaControllerDeployment = "workload-variant-autoscaler-controller-manager"
	wvaConfigMapName        = "inferenceservice-config"
	wvaConfigMapKey         = "autoscaling-wva-controller-config"
	variantAutoscalingCRD   = "variantautoscalings.llmd.ai"
	wvaRemovedMajor         = 3
	wvaRemovedMinor         = 6
)

func variantAutoscalingType() resources.ResourceType {
	return resources.ResourceType{
		Group:    "llmd.ai",
		Version:  "v1alpha1",
		Kind:     "VariantAutoscaling",
		Resource: "variantautoscalings",
	}
}

// WVARemovalCheck stops a 3.6 upgrade while WVA is still enabled or its
// controller and custom resources are still on the cluster.
type WVARemovalCheck struct {
	check.BaseCheck
}

func NewWVARemovalCheck() *WVARemovalCheck {
	return &WVARemovalCheck{
		BaseCheck: check.BaseCheck{
			CheckGroup:       check.GroupComponent,
			Kind:             constants.ComponentKServe,
			Type:             wvaRemovalCheckType,
			CheckID:          "components.kserve.wva-removal",
			CheckName:        "Components :: KServe :: WVA Removal (3.6)",
			CheckDescription: "Validates that Workload Variant Autoscaler is turned off and its controller and custom resources are gone before upgrading to RHOAI 3.6",
			CheckRemediation: "Set spec.components.kserve.wva.managementState to Removed, delete leftover VariantAutoscaling resources, and delete the workload-variant-autoscaler-controller-manager deployment before upgrading. A leftover variantautoscalings.llmd.ai CRD is removed by kserve-module after upgrade.",
		},
	}
}

// CanApply reports whether this check should run. It runs only for an upgrade
// into 3.6 from an older release.
func (c *WVARemovalCheck) CanApply(_ context.Context, target check.Target) (bool, error) {
	if target.TargetVersion == nil || !version.IsVersionAtLeast(target.TargetVersion, wvaRemovedMajor, wvaRemovedMinor) {
		return false, nil
	}
	if target.CurrentVersion != nil && version.IsVersionAtLeast(target.CurrentVersion, wvaRemovedMajor, wvaRemovedMinor) {
		return false, nil
	}

	return true, nil
}

func (c *WVARemovalCheck) Validate(ctx context.Context, target check.Target) (*result.DiagnosticResult, error) {
	return validate.Component(c, target).
		WithApplicationsNamespace().
		Run(ctx, func(ctx context.Context, req *validate.ComponentRequest) error {
			blockers, advisories, err := wvaUpgradeFindings(ctx, req)
			if err != nil {
				return err
			}

			switch {
			case len(blockers) > 0:
				req.Result.SetCondition(check.NewCondition(
					check.ConditionTypeCompatible,
					metav1.ConditionFalse,
					check.WithReason(check.ReasonVersionIncompatible),
					check.WithMessage("WVA is still present and blocks the RHOAI %s upgrade: %s", version.MajorMinorLabel(req.TargetVersion), strings.Join(blockers, "; ")),
					check.WithImpact(result.ImpactBlocking),
					check.WithRemediation(c.CheckRemediation),
				))
			case len(advisories) > 0:
				req.Result.SetCondition(check.NewCondition(
					check.ConditionTypeCompatible,
					metav1.ConditionFalse,
					check.WithReason(check.ReasonVersionIncompatible),
					check.WithMessage("WVA leftovers will be removed during the RHOAI %s upgrade: %s", version.MajorMinorLabel(req.TargetVersion), strings.Join(advisories, "; ")),
					check.WithImpact(result.ImpactAdvisory),
					check.WithRemediation(c.CheckRemediation),
				))
			default:
				req.Result.SetCondition(check.NewCondition(
					check.ConditionTypeCompatible,
					metav1.ConditionTrue,
					check.WithReason(check.ReasonVersionCompatible),
					check.WithMessage("WVA is not enabled and no WVA controller or custom resources were found"),
				))
			}

			return nil
		})
}

func wvaUpgradeFindings(ctx context.Context, req *validate.ComponentRequest) ([]string, []string, error) {
	var blockers, advisories []string

	blocker, err := wvaManagementStateBlocker(req)
	if err != nil {
		return nil, nil, err
	}
	if blocker != "" {
		blockers = append(blockers, blocker)
	}

	blocker, err = wvaDeploymentBlocker(ctx, req)
	if err != nil {
		return nil, nil, err
	}
	if blocker != "" {
		blockers = append(blockers, blocker)
	}

	crdBlockers, crdAdvisories, err := wvaCustomResourceFindings(ctx, req)
	if err != nil {
		return nil, nil, err
	}
	blockers = append(blockers, crdBlockers...)
	advisories = append(advisories, crdAdvisories...)

	advisory, err := wvaConfigMapAdvisory(ctx, req)
	if err != nil {
		return nil, nil, err
	}
	if advisory != "" {
		advisories = append(advisories, advisory)
	}

	return blockers, advisories, nil
}

func wvaManagementStateBlocker(req *validate.ComponentRequest) (string, error) {
	state, err := jq.Query[string](req.DSC, ".spec.components.kserve.wva.managementState")
	switch {
	case errors.Is(err, jq.ErrNotFound):
		return "", nil
	case err != nil:
		return "", fmt.Errorf("querying kserve wva managementState: %w", err)
	case state == constants.ManagementStateManaged:
		return "spec.components.kserve.wva.managementState is Managed", nil
	default:
		return "", nil
	}
}

func wvaDeploymentBlocker(ctx context.Context, req *validate.ComponentRequest) (string, error) {
	ns := req.ApplicationsNamespace
	exists, err := namedResourceExists(ctx, req.Client, resources.Deployment.GVR(), wvaControllerDeployment, ns)
	if err != nil {
		return "", fmt.Errorf("checking WVA deployment: %w", err)
	}
	if !exists {
		return "", nil
	}

	return fmt.Sprintf("deployment %s/%s exists", ns, wvaControllerDeployment), nil
}

func wvaCustomResourceFindings(ctx context.Context, req *validate.ComponentRequest) ([]string, []string, error) {
	crdExists, err := namedResourceExists(ctx, req.Client, resources.CustomResourceDefinition.GVR(), variantAutoscalingCRD, "")
	if err != nil {
		return nil, nil, fmt.Errorf("checking VariantAutoscaling CRD: %w", err)
	}
	if !crdExists {
		return nil, nil, nil
	}

	items, listErr := req.Client.List(ctx, variantAutoscalingType())
	if listErr != nil && !client.IsResourceTypeNotFound(listErr) {
		return nil, nil, fmt.Errorf("listing VariantAutoscaling resources: %w", listErr)
	}
	if len(items) == 0 {
		return nil, []string{"CRD " + variantAutoscalingCRD + " still exists"}, nil
	}

	names := make([]string, 0, len(items))
	for _, item := range items {
		names = append(names, fmt.Sprintf("%s/%s", item.GetNamespace(), item.GetName()))
	}

	return []string{"VariantAutoscaling resources exist: " + strings.Join(names, ", ")}, nil, nil
}

func wvaConfigMapAdvisory(ctx context.Context, req *validate.ComponentRequest) (string, error) {
	ns := req.ApplicationsNamespace
	cm, err := req.Client.Get(ctx, resources.ConfigMap.GVR(), wvaConfigMapName, client.InNamespace(ns))
	if apierrors.IsNotFound(err) || (cm == nil && err == nil) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("checking inferenceservice-config: %w", err)
	}

	_, keyErr := jq.Query[string](cm, `.data["`+wvaConfigMapKey+`"]`)
	switch {
	case errors.Is(keyErr, jq.ErrNotFound):
		return "", nil
	case keyErr != nil:
		return "", fmt.Errorf("querying %s: %w", wvaConfigMapKey, keyErr)
	default:
		return fmt.Sprintf("ConfigMap %s/%s still contains %s", ns, wvaConfigMapName, wvaConfigMapKey), nil
	}
}

func namedResourceExists(ctx context.Context, r client.Reader, gvr schema.GroupVersionResource, name, namespace string) (bool, error) {
	opts := []client.GetOption{}
	if namespace != "" {
		opts = append(opts, client.InNamespace(namespace))
	}

	obj, err := r.Get(ctx, gvr, name, opts...)
	if apierrors.IsNotFound(err) || (obj == nil && err == nil) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("getting %s %s: %w", gvr.Resource, name, err)
	}

	return true, nil
}
