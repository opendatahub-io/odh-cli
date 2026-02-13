package validate

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
)

// DSCIBuilder provides a fluent API for DSCInitialization-based validation.
// It handles DSCI fetching and annotation population automatically.
type DSCIBuilder struct {
	check  check.Check
	target check.Target
}

// DSCI creates a builder for DSCInitialization-based validation.
// This is used by service checks that need to read platform configuration from DSCI.
//
// Example:
//
//	validate.DSCI(c, target).
//	    Run(ctx, func(dr *result.DiagnosticResult, dsci *unstructured.Unstructured) error {
//	        // Validation logic here
//	        return nil
//	    })
func DSCI(c check.Check, target check.Target) *DSCIBuilder {
	return &DSCIBuilder{check: c, target: target}
}

// DSCIValidateFn is the validation function called after DSCI is fetched.
// It receives an auto-created DiagnosticResult with pre-populated annotations and the fetched DSCI.
type DSCIValidateFn func(dr *result.DiagnosticResult, dsci *unstructured.Unstructured) error

// Run fetches the DSCI, auto-populates annotations, and executes validation.
//
// The builder handles:
//   - DSCI not found: returns a standard "not found" diagnostic result (not an error)
//   - DSCI fetch error: returns wrapped error
//   - Annotation population: target version is automatically added
//
// Returns (*result.DiagnosticResult, error) following the standard lint check signature.
func (b *DSCIBuilder) Run(
	ctx context.Context,
	fn DSCIValidateFn,
) (*result.DiagnosticResult, error) {
	// Fetch the DSCInitialization singleton
	dsci, err := client.GetDSCInitialization(ctx, b.target.Client)
	switch {
	case apierrors.IsNotFound(err):
		dr := result.New(string(b.check.Group()), b.check.CheckKind(), b.check.CheckType(), b.check.Description())
		dr.Status.Conditions = []result.Condition{
			check.NewCondition(
				check.ConditionTypeAvailable,
				metav1.ConditionFalse,
				check.WithReason(check.ReasonResourceNotFound),
				check.WithMessage("No DSCInitialization found"),
			),
		}

		return dr, nil
	case err != nil:
		return nil, fmt.Errorf("getting DSCInitialization: %w", err)
	}

	// Create result with auto-populated annotations
	dr := result.New(
		string(b.check.Group()),
		b.check.CheckKind(),
		b.check.CheckType(),
		b.check.Description(),
	)

	if b.target.TargetVersion != nil {
		dr.Annotations[check.AnnotationCheckTargetVersion] = b.target.TargetVersion.String()
	}

	// Execute the validation function
	if err := fn(dr, dsci); err != nil {
		return nil, err
	}

	return dr, nil
}
