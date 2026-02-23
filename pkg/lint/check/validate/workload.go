package validate

import (
	"context"
	"fmt"
	"strconv"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/opendatahub-io/odh-cli/pkg/constants"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
	"github.com/opendatahub-io/odh-cli/pkg/util/components"
	"github.com/opendatahub-io/odh-cli/pkg/util/kube"
)

// WorkloadRequest contains the pre-fetched data passed to the workload validation function.
//
// check.Target is embedded, so fields like Client, IO, Debug, TargetVersion, and CurrentVersion
// are directly accessible (e.g. req.Client, req.IO, req.Debug, req.TargetVersion).
type WorkloadRequest[T any] struct {
	check.Target

	// Result is the pre-created DiagnosticResult with auto-populated annotations.
	Result *result.DiagnosticResult

	// Items contains the (optionally filtered) workload items.
	Items []T
}

// WorkloadValidateFn is the callback invoked by WorkloadBuilder.Run after listing and filtering.
type WorkloadValidateFn[T any] func(ctx context.Context, req *WorkloadRequest[T]) error

// WorkloadConditionFn maps a workload request to conditions to set on the result.
// Use with Complete as a higher-level alternative to Run when the callback only needs to set conditions.
type WorkloadConditionFn[T any] func(ctx context.Context, req *WorkloadRequest[T]) ([]result.Condition, error)

// WorkloadBuilder provides a fluent API for workload-based lint checks.
// It handles resource listing, CRD-not-found handling, filtering, annotation population,
// and auto-populating ImpactedObjects.
type WorkloadBuilder[T kube.NamespacedNamer] struct {
	check          check.Check
	target         check.Target
	resourceType   resources.ResourceType
	listFn         func(ctx context.Context) ([]T, error)
	filterFn       func(T) (bool, error)
	componentNames []string
}

// Workloads creates a WorkloadBuilder that lists full unstructured objects.
// Use this when the validation function needs access to spec or status fields.
func Workloads(
	c check.Check,
	target check.Target,
	resourceType resources.ResourceType,
) *WorkloadBuilder[*unstructured.Unstructured] {
	return &WorkloadBuilder[*unstructured.Unstructured]{
		check:        c,
		target:       target,
		resourceType: resourceType,
		listFn: func(ctx context.Context) ([]*unstructured.Unstructured, error) {
			return target.Client.List(ctx, resourceType)
		},
	}
}

// WorkloadsMetadata creates a WorkloadBuilder that lists metadata-only objects.
// Use this when only name, namespace, labels, annotations, or finalizers are needed.
func WorkloadsMetadata(
	c check.Check,
	target check.Target,
	resourceType resources.ResourceType,
) *WorkloadBuilder[*metav1.PartialObjectMetadata] {
	return &WorkloadBuilder[*metav1.PartialObjectMetadata]{
		check:        c,
		target:       target,
		resourceType: resourceType,
		listFn: func(ctx context.Context) ([]*metav1.PartialObjectMetadata, error) {
			return target.Client.ListMetadata(ctx, resourceType)
		},
	}
}

// Filter adds an optional predicate to select only matching items.
// Items for which fn returns false are excluded before the validation function is called.
// If fn returns an error, Run stops and propagates it.
func (b *WorkloadBuilder[T]) Filter(fn func(T) (bool, error)) *WorkloadBuilder[T] {
	b.filterFn = fn

	return b
}

// ForComponent specifies the DSC component(s) this workload check requires.
// If set, Run() verifies at least one component is not in "Removed" state
// before listing resources. If all components are Removed (or DSC is not found),
// a passing result is returned indicating no validation is needed.
// Multiple names use OR semantics (at least one must be active).
func (b *WorkloadBuilder[T]) ForComponent(names ...string) *WorkloadBuilder[T] {
	b.componentNames = names

	return b
}

// Run lists resources, applies the filter, populates annotations, calls the validation function,
// and auto-populates ImpactedObjects if the mapper did not set them.
func (b *WorkloadBuilder[T]) Run(
	ctx context.Context,
	fn WorkloadValidateFn[T],
) (*result.DiagnosticResult, error) {
	dr := result.New(
		string(b.check.Group()),
		b.check.CheckKind(),
		b.check.CheckType(),
		b.check.Description(),
	)

	if b.target.TargetVersion != nil {
		dr.Annotations[check.AnnotationCheckTargetVersion] = b.target.TargetVersion.String()
	}

	// Check component state precondition if ForComponent was called.
	if len(b.componentNames) > 0 {
		earlyResult, err := b.checkComponentState(ctx, dr)
		if err != nil {
			return nil, err
		}

		if earlyResult != nil {
			return earlyResult, nil
		}
	}

	// List resources; treat CRD-not-found as empty list.
	items, err := b.listFn(ctx)
	if err != nil && !client.IsResourceTypeNotFound(err) {
		return nil, fmt.Errorf("listing %s resources: %w", b.resourceType.Kind, err)
	}

	// Apply filter if set.
	if b.filterFn != nil {
		filtered := make([]T, 0, len(items))

		for _, item := range items {
			match, err := b.filterFn(item)
			if err != nil {
				return nil, fmt.Errorf("filtering %s resources: %w", b.resourceType.Kind, err)
			}

			if match {
				filtered = append(filtered, item)
			}
		}

		items = filtered
	}

	dr.Annotations[check.AnnotationImpactedWorkloadCount] = strconv.Itoa(len(items))

	// Call the validation function.
	req := &WorkloadRequest[T]{
		Target: b.target,
		Result: dr,
		Items:  items,
	}

	if err := fn(ctx, req); err != nil {
		return nil, err
	}

	// Auto-populate ImpactedObjects if the mapper did not set them.
	if dr.ImpactedObjects == nil && len(items) > 0 {
		dr.SetImpactedObjects(b.resourceType, kube.ToNamespacedNames(items))
	}

	return dr, nil
}

// checkComponentState verifies at least one component is not in Removed state.
// Returns (result, nil) if validation should short-circuit, or (nil, nil) to continue.
func (b *WorkloadBuilder[T]) checkComponentState(
	ctx context.Context,
	dr *result.DiagnosticResult,
) (*result.DiagnosticResult, error) {
	dsc, err := client.GetDataScienceCluster(ctx, b.target.Client)
	switch {
	case apierrors.IsNotFound(err):
		dr.SetCondition(check.NewCondition(
			check.ConditionTypeAvailable,
			metav1.ConditionFalse,
			check.WithReason(check.ReasonResourceNotFound),
			check.WithMessage("No DataScienceCluster found"),
		))

		return dr, nil
	case err != nil:
		return nil, fmt.Errorf("getting DataScienceCluster: %w", err)
	}

	// Check if at least one component is active (not Removed).
	for _, name := range b.componentNames {
		if !components.HasManagementState(dsc, name, constants.ManagementStateRemoved) {
			return nil, nil
		}
	}

	// All components are Removed - skip workload validation.
	dr.SetCondition(check.NewCondition(
		check.ConditionTypeConfigured,
		metav1.ConditionTrue,
		check.WithReason(check.ReasonRequirementsMet),
	))

	return dr, nil
}

// Complete is a convenience alternative to Run for checks that only need to set conditions.
// It calls fn to obtain conditions, sets each on the result, and returns.
func (b *WorkloadBuilder[T]) Complete(
	ctx context.Context,
	fn WorkloadConditionFn[T],
) (*result.DiagnosticResult, error) {
	return b.Run(ctx, func(ctx context.Context, req *WorkloadRequest[T]) error {
		conditions, err := fn(ctx, req)
		if err != nil {
			return err
		}

		for _, c := range conditions {
			req.Result.SetCondition(c)
		}

		return nil
	})
}
