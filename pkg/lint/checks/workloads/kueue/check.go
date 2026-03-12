package kueue

import (
	"context"
	"fmt"
	"strconv"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/opendatahub-io/odh-cli/pkg/constants"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
)

// DataIntegrityCheck verifies that the cluster is in a consistent state
// with respect to kueue labels. It checks three invariants:
//  1. Every workload in a kueue-managed namespace has the queue-name label
//  2. Every workload with the queue-name label is in a kueue-managed namespace
//  3. Within each top-level CR's ownership tree, all resources agree on the queue-name label
type DataIntegrityCheck struct {
	check.BaseCheck
	check.EnhancedVerboseFormatter
}

func NewDataIntegrityCheck() *DataIntegrityCheck {
	return &DataIntegrityCheck{
		BaseCheck: check.BaseCheck{
			CheckGroup:       check.GroupWorkload,
			Kind:             constants.ComponentKueue,
			Type:             check.CheckTypeDataIntegrity,
			CheckID:          "workloads.kueue.data-integrity",
			CheckName:        "Workloads :: Kueue :: Data Integrity",
			CheckDescription: "Verifies that kueue namespace labels and workload queue-name labels are consistent across the cluster",
			CheckRemediation: remediationConsistency,
		},
	}
}

func (c *DataIntegrityCheck) CanApply(ctx context.Context, target check.Target) (bool, error) {
	ok, err := IsKueueActive(ctx, target)
	if err != nil {
		return false, fmt.Errorf("checking kueue state: %w", err)
	}

	return ok, nil
}

// Validate does not use the existing validate.Workloads or validate.WorkloadsMetadata builders
// because this check spans multiple resource types, performs namespace-level grouping with a
// shared ownership graph, and emits a single cluster-wide condition rather than per-resource
// conditions. If more cluster-wide consistency checks emerge, a validate.ClusterWide builder
// should be introduced to capture this pattern.
func (c *DataIntegrityCheck) Validate(
	ctx context.Context,
	target check.Target,
) (*result.DiagnosticResult, error) {
	dr := c.NewResult()

	if target.TargetVersion != nil {
		dr.Annotations[check.AnnotationCheckTargetVersion] = target.TargetVersion.String()
	}

	// Phase 1: determine relevant namespaces.
	kueueNamespaces, err := kueueEnabledNamespaces(ctx, target.Client)
	if err != nil {
		return nil, fmt.Errorf("finding kueue-enabled namespaces: %w", err)
	}

	workloadNamespaces, err := workloadLabeledNamespaces(ctx, target.Client)
	if err != nil {
		return nil, fmt.Errorf("finding workload-labeled namespaces: %w", err)
	}

	relevantNamespaces := kueueNamespaces.Union(workloadNamespaces)

	if relevantNamespaces.Len() == 0 {
		dr.SetCondition(check.NewCondition(
			conditionTypeKueueConsistency,
			metav1.ConditionTrue,
			check.WithReason(check.ReasonRequirementsMet),
			check.WithMessage(msgNoRelevantNamespaces),
		))
		dr.Annotations[check.AnnotationImpactedWorkloadCount] = "0"

		return dr, nil
	}

	// Phase 2 & 3: check invariants per namespace.
	var violations []violation

	for _, namespace := range sets.List(relevantNamespaces) {
		namespaceViolations, err := c.checkNamespace(ctx, target.Client, namespace, kueueNamespaces)
		if err != nil {
			return nil, fmt.Errorf("checking namespace %s: %w", namespace, err)
		}

		violations = append(violations, namespaceViolations...)
	}

	// Phase 4: emit result.
	impacted := uniqueResources(violations)
	dr.Annotations[check.AnnotationImpactedWorkloadCount] = strconv.Itoa(len(impacted))

	if len(violations) == 0 {
		dr.SetCondition(check.NewCondition(
			conditionTypeKueueConsistency,
			metav1.ConditionTrue,
			check.WithReason(check.ReasonRequirementsMet),
			check.WithMessage(msgConsistent),
		))

		return dr, nil
	}

	dr.SetCondition(check.NewCondition(
		conditionTypeKueueConsistency,
		metav1.ConditionFalse,
		check.WithReason(check.ReasonConfigurationInvalid),
		check.WithMessage(msgInconsistent, len(violations)),
		check.WithImpact(result.ImpactProhibited),
		check.WithRemediation(c.CheckRemediation),
	))

	// Populate impacted objects — only top-level CRs.
	populateImpactedObjects(dr, impacted)

	return dr, nil
}

// checkNamespace checks all three invariants for workloads in a single namespace.
func (c *DataIntegrityCheck) checkNamespace(
	ctx context.Context,
	r client.Reader,
	namespace string,
	kueueNamespaces sets.Set[string],
) ([]violation, error) {
	// List all top-level CRs in this namespace (metadata-only).
	workloads, err := listWorkloadsInNamespace(ctx, r, namespace)
	if err != nil {
		return nil, err
	}

	if len(workloads) == 0 {
		return nil, nil
	}

	// Build ownership graph for invariant 3.
	graph, err := buildGraph(ctx, r, namespace)
	if err != nil {
		return nil, fmt.Errorf("building ownership graph: %w", err)
	}

	var violations []violation

	for _, cr := range workloads {
		// Invariant 1: namespace → workload.
		if v := checkNamespaceToWorkload(cr, kueueNamespaces); v != nil {
			violations = append(violations, *v)

			continue
		}

		// Invariant 2: workload → namespace.
		if v := checkWorkloadToNamespace(cr, kueueNamespaces); v != nil {
			violations = append(violations, *v)

			continue
		}

		// Invariant 3: owner tree consistency.
		if v := checkOwnerTreeConsistency(cr, graph); v != nil {
			violations = append(violations, *v)
		}
	}

	return violations, nil
}

// listWorkloadsInNamespace lists all monitored workload types in the given namespace.
func listWorkloadsInNamespace(
	ctx context.Context,
	r client.Reader,
	namespace string,
) ([]*metav1.PartialObjectMetadata, error) {
	var all []*metav1.PartialObjectMetadata

	for _, rt := range monitoredWorkloadTypes {
		items, err := r.ListMetadata(ctx, rt, client.WithNamespace(namespace))
		if err != nil {
			// A missing CRD means the resource type is not installed on this cluster,
			// so there are zero instances. Ideally ListMetadata would handle this
			// the same way it handles permission errors (return empty list).
			if client.IsResourceTypeNotFound(err) {
				continue
			}

			return nil, fmt.Errorf("listing %s in namespace %s: %w", rt.Kind, namespace, err)
		}

		// ListMetadata returns PartialObjectMetadata items whose Kind and APIVersion
		// reflect the metadata wrapper ("PartialObjectMetadata" / "meta.k8s.io/v1")
		// rather than the actual resource type. Copy each item with the correct
		// TypeMeta so violation messages reference the real kind (e.g. "Notebook")
		// and we avoid mutating pointers owned by the caller.
		for _, item := range items {
			obj := &metav1.PartialObjectMetadata{
				TypeMeta:   rt.TypeMeta(),
				ObjectMeta: *item.ObjectMeta.DeepCopy(),
			}
			all = append(all, obj)
		}
	}

	return all, nil
}

// impactedResource holds the identity, type, and violation context of a top-level CR
// that failed one or more kueue consistency invariants.
type impactedResource struct {
	Namespace  string
	Name       string
	Kind       string
	APIVersion string
	Message    string
}

// uniqueResources deduplicates violations by resource identity (kind, apiVersion, namespace, name),
// returning the unique set of impacted top-level CRs.
func uniqueResources(violations []violation) []impactedResource {
	type resourceKey struct {
		Kind       string
		APIVersion string
		Namespace  string
		Name       string
	}

	seen := make(map[resourceKey]struct{})

	var unique []impactedResource

	for i := range violations {
		key := resourceKey{
			Kind:       violations[i].Kind,
			APIVersion: violations[i].APIVersion,
			Namespace:  violations[i].Resource.Namespace,
			Name:       violations[i].Resource.Name,
		}

		if _, exists := seen[key]; !exists {
			seen[key] = struct{}{}
			unique = append(unique, impactedResource{
				Namespace:  violations[i].Resource.Namespace,
				Name:       violations[i].Resource.Name,
				Kind:       violations[i].Kind,
				APIVersion: violations[i].APIVersion,
				Message:    violations[i].Message,
			})
		}
	}

	return unique
}

// populateImpactedObjects sets impacted objects on the diagnostic result.
// Only top-level CRs from the monitored list appear as impacted objects.
// Each object carries per-object annotations for context and CRD FQN.
func populateImpactedObjects(
	dr *result.DiagnosticResult,
	impacted []impactedResource,
) {
	// Build lookup from APIVersion+Kind to authoritative CRD FQN
	// so the verbose formatter doesn't need to derive plurals naively.
	crdfqnLookup := buildCRDFQNLookup()

	dr.ImpactedObjects = make([]metav1.PartialObjectMetadata, 0, len(impacted))

	for _, r := range impacted {
		annotations := make(map[string]string)

		if r.Message != "" {
			annotations[result.AnnotationObjectContext] = r.Message
		}

		if fqn, ok := crdfqnLookup[r.APIVersion+"/"+r.Kind]; ok {
			annotations[result.AnnotationObjectCRDName] = fqn
		}

		obj := metav1.PartialObjectMetadata{
			TypeMeta: metav1.TypeMeta{
				Kind:       r.Kind,
				APIVersion: r.APIVersion,
			},
			ObjectMeta: metav1.ObjectMeta{
				Namespace:   r.Namespace,
				Name:        r.Name,
				Annotations: annotations,
			},
		}

		dr.ImpactedObjects = append(dr.ImpactedObjects, obj)
	}
}

// buildCRDFQNLookup builds a map from "apiVersion/kind" to CRD FQN
// using the authoritative ResourceType definitions from monitoredWorkloadTypes.
func buildCRDFQNLookup() map[string]string {
	lookup := make(map[string]string, len(monitoredWorkloadTypes))

	for _, rt := range monitoredWorkloadTypes {
		key := rt.APIVersion() + "/" + rt.Kind
		lookup[key] = rt.CRDFQN()
	}

	return lookup
}
