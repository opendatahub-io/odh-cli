package kueue

import (
	"context"
	"fmt"
	"strconv"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/opendatahub-io/odh-cli/pkg/constants"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/validate"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
	"github.com/opendatahub-io/odh-cli/pkg/util/components"
	"github.com/opendatahub-io/odh-cli/pkg/util/kube"
)

// Messages for the "labeled in non-kueue namespace" condition.
const (
	MsgNoWorkloads        = "No %s instances found"
	MsgNoLabeledWorkloads = "No %s(s) found with the kueue.x-k8s.io/queue-name label"
	MsgAllValid           = "All %d %s(s) with the kueue.x-k8s.io/queue-name label are in kueue-enabled namespaces"
	MsgNsNotKueueEnabled  = "Found %d %s(s) with the kueue.x-k8s.io/queue-name label in namespaces not enabled for kueue"
)

// Messages for the "missing label in kueue namespace" condition.
const (
	MsgNoWorkloadsInKueueNs  = "No %s(s) found in kueue-enabled namespaces"
	MsgAllInKueueNsLabeled   = "All %d %s(s) in kueue-enabled namespaces have the kueue.x-k8s.io/queue-name label"
	MsgMissingLabelInKueueNs = "Found %d %s(s) in kueue-enabled namespaces without the kueue.x-k8s.io/queue-name label"
)

// IsComponentAndKueueActive returns true when the given component is Managed AND Kueue is active
// (Managed or Unmanaged) on the DSC.
func IsComponentAndKueueActive(
	ctx context.Context,
	target check.Target,
	componentName string,
) (bool, error) {
	dsc, err := client.GetDataScienceCluster(ctx, target.Client)
	if err != nil {
		return false, fmt.Errorf("getting DataScienceCluster: %w", err)
	}

	if !components.HasManagementState(
		dsc, componentName,
		constants.ManagementStateManaged,
	) {
		return false, nil
	}

	if !components.HasManagementState(
		dsc, constants.ComponentKueue,
		constants.ManagementStateManaged, constants.ManagementStateUnmanaged,
	) {
		return false, nil
	}

	return true, nil
}

// ValidateFn returns a validation function that classifies workloads into four categories
// based on kueue label presence and namespace enablement, then emits two conditions:
//   - labeled workloads in non-kueue namespaces (conditionType)
//   - unlabeled workloads in kueue-enabled namespaces (missingLabelConditionType)
func ValidateFn(
	resourceType resources.ResourceType,
	conditionType string,
	missingLabelConditionType string,
	kindLabel string,
) validate.WorkloadValidateFn[*metav1.PartialObjectMetadata] {
	return func(ctx context.Context, req *validate.WorkloadRequest[*metav1.PartialObjectMetadata]) error {
		dr := req.Result

		if len(req.Items) == 0 {
			dr.Annotations[check.AnnotationImpactedWorkloadCount] = "0"
			dr.SetCondition(newLabeledInNonKueueCondition(conditionType, kindLabel, len(req.Items), 0, 0))
			dr.SetCondition(newMissingLabelCondition(missingLabelConditionType, kindLabel, 0, 0))
			dr.SetImpactedObjects(resourceType, nil)

			return nil
		}

		allKueueNs, err := kueueEnabledNamespaces(ctx, req.Client)
		if err != nil {
			return err
		}

		// Classify workloads into four categories based on label presence and namespace.
		var (
			labeled             []types.NamespacedName
			labeledInNonKueueNs []types.NamespacedName
			inKueueNs           []types.NamespacedName
			unlabeledInKueueNs  []types.NamespacedName
		)

		for _, item := range req.Items {
			nn := types.NamespacedName{
				Namespace: item.GetNamespace(),
				Name:      item.GetName(),
			}

			hasLabel := kube.ContainsLabel(item, constants.LabelKueueQueueName)
			inKueue := allKueueNs.Has(item.GetNamespace())

			if hasLabel {
				labeled = append(labeled, nn)
			}

			if inKueue {
				inKueueNs = append(inKueueNs, nn)
			}

			switch {
			case hasLabel && !inKueue:
				labeledInNonKueueNs = append(labeledInNonKueueNs, nn)
			case !hasLabel && inKueue:
				unlabeledInKueueNs = append(unlabeledInKueueNs, nn)
			}
		}

		totalImpacted := len(labeledInNonKueueNs) + len(unlabeledInKueueNs)
		dr.Annotations[check.AnnotationImpactedWorkloadCount] = strconv.Itoa(totalImpacted)

		dr.SetCondition(newLabeledInNonKueueCondition(
			conditionType, kindLabel,
			len(req.Items), len(labeled), len(labeledInNonKueueNs),
		))
		dr.SetCondition(newMissingLabelCondition(
			missingLabelConditionType, kindLabel,
			len(inKueueNs), len(unlabeledInKueueNs),
		))

		dr.SetImpactedObjects(resourceType, labeledInNonKueueNs)
		dr.AddImpactedObjects(resourceType, unlabeledInKueueNs)

		return nil
	}
}

// newLabeledInNonKueueCondition builds the condition for workloads that have the
// kueue queue label but are in namespaces not enabled for kueue.
func newLabeledInNonKueueCondition(
	conditionType string,
	kindLabel string,
	totalWorkloads int,
	labeledCount int,
	impactedCount int,
) result.Condition {
	switch {
	case totalWorkloads == 0:
		return check.NewCondition(
			conditionType,
			metav1.ConditionTrue,
			check.WithReason(check.ReasonRequirementsMet),
			check.WithMessage(MsgNoWorkloads, kindLabel),
		)
	case labeledCount == 0:
		return check.NewCondition(
			conditionType,
			metav1.ConditionTrue,
			check.WithReason(check.ReasonRequirementsMet),
			check.WithMessage(MsgNoLabeledWorkloads, kindLabel),
		)
	case impactedCount == 0:
		return check.NewCondition(
			conditionType,
			metav1.ConditionTrue,
			check.WithReason(check.ReasonRequirementsMet),
			check.WithMessage(MsgAllValid, labeledCount, kindLabel),
		)
	default:
		return check.NewCondition(
			conditionType,
			metav1.ConditionFalse,
			check.WithReason(check.ReasonConfigurationInvalid),
			check.WithMessage(MsgNsNotKueueEnabled, impactedCount, kindLabel),
			check.WithImpact(result.ImpactBlocking),
			check.WithRemediation(remediationLabeledInNonKueueNs),
		)
	}
}

// newMissingLabelCondition builds the condition for workloads in kueue-enabled
// namespaces that are missing the kueue queue label.
func newMissingLabelCondition(
	conditionType string,
	kindLabel string,
	totalInKueueNs int,
	missingCount int,
) result.Condition {
	switch {
	case totalInKueueNs == 0:
		return check.NewCondition(
			conditionType,
			metav1.ConditionTrue,
			check.WithReason(check.ReasonRequirementsMet),
			check.WithMessage(MsgNoWorkloadsInKueueNs, kindLabel),
		)
	case missingCount == 0:
		return check.NewCondition(
			conditionType,
			metav1.ConditionTrue,
			check.WithReason(check.ReasonRequirementsMet),
			check.WithMessage(MsgAllInKueueNsLabeled, totalInKueueNs, kindLabel),
		)
	default:
		return check.NewCondition(
			conditionType,
			metav1.ConditionFalse,
			check.WithReason(check.ReasonConfigurationInvalid),
			check.WithMessage(MsgMissingLabelInKueueNs, missingCount, kindLabel),
			check.WithImpact(result.ImpactBlocking),
			check.WithRemediation(remediationMissingLabelInKueueNs),
		)
	}
}

// kueueEnabledNamespaces returns the set of namespaces that have a kueue-managed label.
// Uses two ListMetadata calls with label selectors for server-side filtering,
// giving a fixed cost regardless of how many namespaces exist.
func kueueEnabledNamespaces(
	ctx context.Context,
	r client.Reader,
) (sets.Set[string], error) {
	enabled := sets.New[string]()

	for _, selector := range []string{
		constants.LabelKueueManaged + "=true",
		constants.LabelKueueOpenshiftManaged + "=true",
	} {
		items, err := r.ListMetadata(ctx, resources.Namespace,
			client.WithLabelSelector(selector))
		if err != nil {
			return nil, fmt.Errorf("listing kueue-enabled namespaces: %w", err)
		}

		for _, ns := range items {
			enabled.Insert(ns.GetName())
		}
	}

	return enabled, nil
}
