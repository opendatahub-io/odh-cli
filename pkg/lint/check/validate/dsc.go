package validate //nolint:dupl // DSCBuilder mirrors DSCIBuilder pattern for the DataScienceCluster resource

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

// DSCBuilder provides a fluent API for DataScienceCluster-based validation.
// It handles DSC fetching and annotation population automatically.
type DSCBuilder struct {
	check  check.Check
	target check.Target
}

// DSC creates a builder for DataScienceCluster-based validation.
// This is used by checks that need to read platform configuration from DSC.
//
// Example:
//
//	validate.DSC(c, target).
//	    Run(ctx, func(dr *result.DiagnosticResult, dsc *unstructured.Unstructured) error {
//	        // Validation logic here
//	        return nil
//	    })
func DSC(c check.Check, target check.Target) *DSCBuilder {
	return &DSCBuilder{check: c, target: target}
}

// DSCValidateFn is the validation function called after DSC is fetched.
// It receives an auto-created DiagnosticResult with pre-populated annotations and the fetched DSC.
type DSCValidateFn func(dr *result.DiagnosticResult, dsc *unstructured.Unstructured) error

// Run fetches the DSC, auto-populates annotations, and executes validation.
//
// The builder handles:
//   - DSC not found: returns a standard "not found" diagnostic result (not an error)
//   - DSC fetch error: returns wrapped error
//   - Annotation population: target version is automatically added
//
// Returns (*result.DiagnosticResult, error) following the standard lint check signature.
func (b *DSCBuilder) Run(
	ctx context.Context,
	fn DSCValidateFn,
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
	if err := fn(dr, dsc); err != nil {
		return nil, err
	}

	return dr, nil
}
