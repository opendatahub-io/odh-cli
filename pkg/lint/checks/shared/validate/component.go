// Package validate provides fluent builders for common lint check validation patterns.
// These builders eliminate boilerplate for fetching resources and handling errors.
package validate

import (
	"context"
	"fmt"
	"slices"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/lburgazzoli/odh-cli/pkg/lint/check"
	"github.com/lburgazzoli/odh-cli/pkg/lint/check/result"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/shared/components"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/shared/results"
	"github.com/lburgazzoli/odh-cli/pkg/util/client"
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
//	    InState(check.ManagementStateManaged, check.ManagementStateUnmanaged).
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

// InState specifies which management states trigger validation.
// If the component is not in any of the specified states, a "not configured" result is returned.
// If no states are specified (InState not called), validation runs for any configured state.
//
// Common patterns:
//   - InState(check.ManagementStateManaged) - only validate when component is managed
//   - InState(check.ManagementStateManaged, check.ManagementStateUnmanaged) - validate when enabled
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
		return results.DataScienceClusterNotFound(
			string(b.check.Group()),
			b.check.CheckKind(),
			b.check.CheckType(),
			b.check.Description(),
		), nil
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
		// Component not in required state - return "not configured" result
		dr := result.New(
			string(b.check.Group()),
			b.check.CheckKind(),
			b.check.CheckType(),
			b.check.Description(),
		)
		results.SetComponentNotConfigured(dr, b.componentName)

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
			results.SetDSCInitializationNotFound(dr)

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
