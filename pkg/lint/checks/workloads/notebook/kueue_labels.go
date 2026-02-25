package notebook

import (
	"context"
	"fmt"
	"strconv"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/validate"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
)

// Kueue-specific label keys.
const (
	LabelKueueManaged          = "kueue-managed"
	LabelKueueOpenshiftManaged = "kueue.openshift.io/managed"
	LabelKueueQueueName        = "kueue.x-k8s.io/queue-name"
)

// KueueLabelsCheck verifies that Notebooks in kueue-enabled namespaces have the
// required kueue queue label for workload scheduling.
type KueueLabelsCheck struct {
	check.BaseCheck
}

func NewKueueLabelsCheck() *KueueLabelsCheck {
	return &KueueLabelsCheck{
		BaseCheck: check.BaseCheck{
			CheckGroup:       check.GroupWorkload,
			Kind:             kind,
			Type:             check.CheckTypeDataIntegrity,
			CheckID:          "workloads.notebook.kueue-labels",
			CheckName:        "Workloads :: Notebook :: Kueue Labels",
			CheckDescription: "Verifies that Notebooks in kueue-enabled namespaces have the required kueue queue label for workload scheduling",
			CheckRemediation: "Add the label kueue.x-k8s.io/queue-name: default to the affected Notebooks, or remove kueue labels from the namespace if kueue integration is not intended",
		},
	}
}

// CanApply returns whether this check should run for the given target.
// Applies whenever Workbenches is Managed, regardless of version.
func (c *KueueLabelsCheck) CanApply(ctx context.Context, target check.Target) (bool, error) {
	return isWorkbenchesManaged(ctx, target)
}

// Validate lists Notebooks and checks that those in kueue-enabled namespaces have
// the required kueue queue label.
func (c *KueueLabelsCheck) Validate(
	ctx context.Context,
	target check.Target,
) (*result.DiagnosticResult, error) {
	return validate.WorkloadsMetadata(c, target, resources.Notebook).
		Run(ctx, c.checkKueueLabels)
}

// checkKueueLabels cross-references notebook namespaces against kueue-enabled namespaces
// and verifies that notebooks in those namespaces have the required queue label.
func (c *KueueLabelsCheck) checkKueueLabels(
	ctx context.Context,
	req *validate.WorkloadRequest[*metav1.PartialObjectMetadata],
) error {
	dr := req.Result

	// Collect unique namespaces from notebooks.
	notebookNamespaces := sets.New[string]()
	for _, nb := range req.Items {
		notebookNamespaces.Insert(nb.GetNamespace())
	}

	// Build kueue-enabled namespace set by fetching metadata for each namespace.
	kueueNamespaces := sets.New[string]()

	for ns := range notebookNamespaces {
		nsMeta, err := req.Client.GetResourceMetadata(ctx, resources.Namespace, ns)
		if err != nil {
			if client.IsResourceTypeNotFound(err) {
				continue
			}

			return fmt.Errorf("getting namespace %s metadata: %w", ns, err)
		}

		if nsMeta == nil {
			continue
		}

		labels := nsMeta.GetLabels()
		if labels[LabelKueueManaged] == "true" || labels[LabelKueueOpenshiftManaged] == "true" {
			kueueNamespaces.Insert(ns)
		}
	}

	// Check notebooks in kueue-enabled namespaces for the required queue label.
	impacted := make([]types.NamespacedName, 0)

	for _, nb := range req.Items {
		if !kueueNamespaces.Has(nb.GetNamespace()) {
			continue
		}

		labels := nb.GetLabels()
		queueName, ok := labels[LabelKueueQueueName]
		if !ok || queueName == "" {
			impacted = append(impacted, types.NamespacedName{
				Namespace: nb.GetNamespace(),
				Name:      nb.GetName(),
			})
		}
	}

	totalImpacted := len(impacted)
	dr.Annotations[check.AnnotationImpactedWorkloadCount] = strconv.Itoa(totalImpacted)

	dr.Status.Conditions = append(dr.Status.Conditions, c.newCondition(totalImpacted))
	dr.SetImpactedObjects(resources.Notebook, impacted)

	return nil
}

func (c *KueueLabelsCheck) newCondition(totalImpacted int) result.Condition {
	if totalImpacted == 0 {
		return check.NewCondition(
			ConditionTypeKueueLabels,
			metav1.ConditionTrue,
			check.WithReason(check.ReasonRequirementsMet),
			check.WithMessage(MsgAllKueueLabelsValid),
		)
	}

	return check.NewCondition(
		ConditionTypeKueueLabels,
		metav1.ConditionFalse,
		check.WithReason(check.ReasonConfigurationInvalid),
		check.WithMessage(MsgKueueLabelsMissing, totalImpacted),
		check.WithImpact(result.ImpactBlocking),
		check.WithRemediation(c.CheckRemediation),
	)
}
