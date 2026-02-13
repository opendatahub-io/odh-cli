// Package validate provides fluent builders for common lint check validation patterns.
// These builders eliminate boilerplate for fetching resources and handling errors.
package validate

import (
	"context"
	"fmt"
	"slices"

	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
	"github.com/opendatahub-io/odh-cli/pkg/util/components"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// ComponentBuilder provides a fluent API for component-based validation.
// It handles DSC fetching, component state filtering, and annotation population automatically.
type ComponentBuilder struct {
	check                     check.Check
	componentName             string
	target                    check.Target
	requiredStates            []string
	loadApplicationsNamespace bool
}

// Component creates a builder for component validation.
// The component name is derived from c.CheckKind(), which should be the lowercase key
// under spec.components (e.g. "kueue", "kserve", "codeflare").
//
// Example:
//
//	validate.Component(c, target).
//	    InState(constants.ManagementStateManaged, constants.ManagementStateUnmanaged).
//	    Run(ctx, func(ctx context.Context, req *ComponentRequest) error {
//	        // Validation logic here
//	        return nil
//	    })
func Component(c check.Check, target check.Target) *ComponentBuilder {
	return &ComponentBuilder{
		check:         c,
		componentName: c.CheckKind(),
		target:        target,
	}
}

// ComponentRequest contains pre-fetched data for component validation.
// It provides convenient access to commonly needed data without requiring
// callbacks to parse annotations or fetch additional resources.
type ComponentRequest struct {
	// Result is the pre-created DiagnosticResult with auto-populated annotations.
	Result *result.DiagnosticResult

	// DSC is the fetched DataScienceCluster (for JQ queries if needed).
	DSC *unstructured.Unstructured

	// ManagementState is the component's management state string.
	ManagementState string

	// Client provides read-only access to the Kubernetes API.
	Client client.Reader

	// ApplicationsNamespace is populated when WithApplicationsNamespace() is used.
	// Empty string if not requested. If DSCI is not found, Run() returns early
	// with a "not found" diagnostic result before calling the validation function.
	ApplicationsNamespace string
}

// ComponentValidateFn is the validation function called after DSC is fetched and state is verified.
// It receives context and a ComponentRequest with pre-populated data.
type ComponentValidateFn func(ctx context.Context, req *ComponentRequest) error

// ComponentConditionFn maps a component request to conditions to set on the result.
// Use with Complete as a higher-level alternative to Run when the callback only needs to set conditions.
type ComponentConditionFn func(ctx context.Context, req *ComponentRequest) ([]result.Condition, error)

// InState specifies which management states trigger validation.
// If the component is not in any of the specified states, a "not configured" result is returned.
// If no states are specified (InState not called), validation runs for any configured state.
//
// Common patterns:
//   - InState(constants.ManagementStateManaged) - only validate when component is managed
//   - InState(constants.ManagementStateManaged, constants.ManagementStateUnmanaged) - validate when enabled
func (b *ComponentBuilder) InState(states ...string) *ComponentBuilder {
	b.requiredStates = states

	return b
}

// WithApplicationsNamespace requests that ApplicationsNamespace be populated in the ComponentRequest.
// This fetches the applications namespace from DSCI before calling the validation function.
// If DSCI is not found, Run() returns early with a "not found" diagnostic result.
// If not called, ApplicationsNamespace will be empty in the request.
func (b *ComponentBuilder) WithApplicationsNamespace() *ComponentBuilder {
	b.loadApplicationsNamespace = true

	return b
}

// Removal returns a ComponentValidateFn that sets a compatibility failure condition.
// ManagementState is automatically prepended as the first format argument.
//
// Example:
//
//	validate.Component(c, target).
//	    InState(constants.ManagementStateManaged).
//	    Run(ctx, validate.Removal("CodeFlare is enabled (state: %s) but will be removed in RHOAI 3.x"))
func Removal(format string, opts ...check.ConditionOption) ComponentValidateFn {
	return func(_ context.Context, req *ComponentRequest) error {
		allOpts := append([]check.ConditionOption{
			check.WithReason(check.ReasonVersionIncompatible),
			check.WithMessage(format, req.ManagementState),
		}, opts...)
		req.Result.SetCondition(check.NewCondition(
			check.ConditionTypeCompatible,
			metav1.ConditionFalse,
			allOpts...,
		))

		return nil
	}
}

// Run fetches the DSC, checks component state, auto-populates annotations, and executes validation.
//
// The builder handles:
//   - DSC not found: returns a standard "not found" diagnostic result (not an error)
//   - DSC fetch error: returns wrapped error
//   - Component not in required state: returns a "not configured" diagnostic result
//   - Annotation population: management state and target version are automatically added
//
// Returns (*result.DiagnosticResult, error) following the standard lint check signature.
func (b *ComponentBuilder) Run(
	ctx context.Context,
	fn ComponentValidateFn,
) (*result.DiagnosticResult, error) {
	// Fetch the DataScienceCluster singleton
	dsc, err := client.GetDataScienceCluster(ctx, b.target.Client)
	switch {
	case apierrors.IsNotFound(err):
		dr := result.New(string(b.check.Group()), b.check.CheckKind(), b.check.CheckType(), b.check.Description())
		dr.Status.Conditions = []result.Condition{
			check.NewCondition(
				check.ConditionTypeAvailable,
				metav1.ConditionFalse,
				check.WithReason(check.ReasonResourceNotFound),
				check.WithMessage("No DataScienceCluster found"),
			),
		}

		return dr, nil
	case err != nil:
		return nil, fmt.Errorf("getting DataScienceCluster: %w", err)
	}

	// Get component management state
	state, err := components.GetManagementState(dsc, b.componentName)
	if err != nil {
		return nil, fmt.Errorf("querying %s managementState: %w", b.componentName, err)
	}

	// Check state precondition if states are specified
	if len(b.requiredStates) > 0 && !slices.Contains(b.requiredStates, state) {
		// Component not in required state - check doesn't apply, return passing result
		dr := result.New(
			string(b.check.Group()),
			b.check.CheckKind(),
			b.check.CheckType(),
			b.check.Description(),
		)
		dr.SetCondition(check.NewCondition(
			check.ConditionTypeConfigured,
			metav1.ConditionTrue,
			check.WithReason(check.ReasonRequirementsMet),
		))

		return dr, nil
	}

	// Create result with auto-populated annotations
	dr := result.New(
		string(b.check.Group()),
		b.check.CheckKind(),
		b.check.CheckType(),
		b.check.Description(),
	)

	dr.Annotations[check.AnnotationComponentManagementState] = state
	if b.target.TargetVersion != nil {
		dr.Annotations[check.AnnotationCheckTargetVersion] = b.target.TargetVersion.String()
	}

	// Create the request with pre-populated data
	req := &ComponentRequest{
		Result:          dr,
		DSC:             dsc,
		ManagementState: state,
		Client:          b.target.Client,
	}

	// Load applications namespace if requested
	if b.loadApplicationsNamespace {
		ns, nsErr := client.GetApplicationsNamespace(ctx, b.target.Client)
		switch {
		case apierrors.IsNotFound(nsErr):
			dr.SetCondition(check.NewCondition(
				check.ConditionTypeAvailable,
				metav1.ConditionFalse,
				check.WithReason(check.ReasonResourceNotFound),
				check.WithMessage("No DSCInitialization found"),
			))

			return dr, nil
		case nsErr != nil:
			return nil, fmt.Errorf("getting applications namespace: %w", nsErr)
		}

		req.ApplicationsNamespace = ns
	}

	// Execute the validation function
	if err := fn(ctx, req); err != nil {
		return nil, err
	}

	return dr, nil
}

// Complete is a convenience alternative to Run for checks that only need to set conditions.
// It calls fn to obtain conditions, sets each on the result, and returns.
func (b *ComponentBuilder) Complete(
	ctx context.Context,
	fn ComponentConditionFn,
) (*result.DiagnosticResult, error) {
	return b.Run(ctx, func(ctx context.Context, req *ComponentRequest) error {
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
