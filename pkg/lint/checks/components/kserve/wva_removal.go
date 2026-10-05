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
)

var variantAutoscalingResource = resources.ResourceType{
	Group:    "llmd.ai",
	Version:  "v1alpha1",
	Kind:     "VariantAutoscaling",
	Resource: "variantautoscalings",
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
	if target.TargetVersion == nil || !version.IsVersionAtLeast(target.TargetVersion, 3, 6) {
		return false, nil
	}
	if target.CurrentVersion != nil && version.IsVersionAtLeast(target.CurrentVersion, 3, 6) {
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

func wvaUpgradeFindings(ctx context.Context, req *validate.ComponentRequest) (blockers, advisories []string, err error) {
	state, qerr := jq.Query[string](req.DSC, ".spec.components.kserve.wva.managementState")
	switch {
	case errors.Is(qerr, jq.ErrNotFound):
	case qerr != nil:
		return nil, nil, fmt.Errorf("querying kserve wva managementState: %w", qerr)
	case state == constants.ManagementStateManaged:
		blockers = append(blockers, "spec.components.kserve.wva.managementState is Managed")
	}

	ns := req.ApplicationsNamespace
	exists, err := namedResourceExists(ctx, req.Client, resources.Deployment.GVR(), wvaControllerDeployment, ns)
	if err != nil {
		return nil, nil, fmt.Errorf("checking WVA deployment: %w", err)
	}
	if exists {
		blockers = append(blockers, fmt.Sprintf("deployment %s/%s exists", ns, wvaControllerDeployment))
	}

	crdExists, err := namedResourceExists(ctx, req.Client, resources.CustomResourceDefinition.GVR(), variantAutoscalingCRD, "")
	if err != nil {
		return nil, nil, fmt.Errorf("checking VariantAutoscaling CRD: %w", err)
	}
	if crdExists {
		items, listErr := req.Client.List(ctx, variantAutoscalingResource)
		if listErr != nil && !client.IsResourceTypeNotFound(listErr) {
			return nil, nil, fmt.Errorf("listing VariantAutoscaling resources: %w", listErr)
		}
		if len(items) > 0 {
			names := make([]string, 0, len(items))
			for _, item := range items {
				names = append(names, fmt.Sprintf("%s/%s", item.GetNamespace(), item.GetName()))
			}
			blockers = append(blockers, "VariantAutoscaling resources exist: "+strings.Join(names, ", "))
		} else {
			advisories = append(advisories, "CRD "+variantAutoscalingCRD+" still exists")
		}
	}

	cm, err := req.Client.Get(ctx, resources.ConfigMap.GVR(), wvaConfigMapName, client.InNamespace(ns))
	if err != nil && !apierrors.IsNotFound(err) {
		return nil, nil, fmt.Errorf("checking inferenceservice-config: %w", err)
	}
	if cm != nil {
		_, keyErr := jq.Query[string](cm, `.data["`+wvaConfigMapKey+`"]`)
		switch {
		case errors.Is(keyErr, jq.ErrNotFound):
		case keyErr != nil:
			return nil, nil, fmt.Errorf("querying %s: %w", wvaConfigMapKey, keyErr)
		default:
			advisories = append(advisories, fmt.Sprintf("ConfigMap %s/%s still contains %s", ns, wvaConfigMapName, wvaConfigMapKey))
		}
	}

	return blockers, advisories, nil
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
		return false, err
	}

	return true, nil
}
