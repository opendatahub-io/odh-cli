package validate

import (
	"context"
	"fmt"
	"strconv"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
	"github.com/opendatahub-io/odh-cli/pkg/util/iostreams"
	"github.com/opendatahub-io/odh-cli/pkg/util/kube"
)

// WorkloadRequest contains the pre-fetched data passed to the workload validation function.
type WorkloadRequest[T any] struct {
	// Result is the pre-created DiagnosticResult with auto-populated annotations.
	Result *result.DiagnosticResult

	// Items contains the (optionally filtered) workload items.
	Items []T

	// Client provides read-only access to the Kubernetes API.
	Client client.Reader

	// IO provides access to input/output streams for verbose logging.
	// Use IO.Errorf() for debug output that appears only when --verbose is set.
	// May be nil if no IO was provided to the check target.
	IO iostreams.Interface

	// Debug indicates whether detailed diagnostic logging is enabled.
	// When true, checks should emit internal processing logs for troubleshooting.
	// When false, only user-facing summary information should be logged via IO.
	Debug bool
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
	check        check.Check
	target       check.Target
	resourceType resources.ResourceType
	listFn       func(ctx context.Context) ([]T, error)
	filterFn     func(T) (bool, error)
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
		Result: dr,
		Items:  items,
		Client: b.target.Client,
		IO:     b.target.IO,
		Debug:  b.target.Debug,
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
