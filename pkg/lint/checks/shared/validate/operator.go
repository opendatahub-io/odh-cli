package validate

import (
	"context"
	"fmt"
	"slices"

	"github.com/lburgazzoli/odh-cli/pkg/lint/check"
	"github.com/lburgazzoli/odh-cli/pkg/lint/check/result"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/shared/operators"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/shared/results"
)

// OperatorBuilder provides a fluent API for OLM operator presence validation.
// It handles OLM availability checking, subscription matching, and annotation population automatically.
type OperatorBuilder struct {
	check            check.Check
	target           check.Target
	names            []string
	channels         []string
	conditionBuilder operators.ConditionBuilder
}

// Operator creates a builder for OLM operator presence validation.
// The operator kind is derived from c.CheckKind(), which should match the dependency constant
// (e.g. "certmanager", "kueueoperator").
//
// Default behavior:
//   - Matcher: exact match on subscription name == c.CheckKind()
//   - ConditionBuilder: standard Available condition (True if found, False if not)
//
// Example:
//
//	validate.Operator(c, target).
//	    WithNames("cert-manager", "openshift-cert-manager-operator").
//	    Run(ctx)
func Operator(c check.Check, target check.Target) *OperatorBuilder {
	kind := c.CheckKind()

	return &OperatorBuilder{
		check:  c,
		target: target,
		// Default condition builder: standard Available condition.
		conditionBuilder: func(found bool, version string) result.Condition {
			if !found {
				return results.NewAvailabilityFailure("%s operator is not installed", kind)
			}

			return results.NewAvailabilitySuccess("%s operator installed: %s", kind, version)
		},
	}
}

// WithNames overrides the default subscription name matching.
// When set, a subscription matches if its name equals any of the provided names (OR semantics).
// When not set, the default matches on subscription name == c.CheckKind().
func (b *OperatorBuilder) WithNames(names ...string) *OperatorBuilder {
	b.names = names

	return b
}

// WithChannels restricts matching to subscriptions on specific channels (OR semantics).
// A subscription must match both a name AND one of the channels.
// Subscriptions with an empty channel never match when channels are specified.
func (b *OperatorBuilder) WithChannels(channels ...string) *OperatorBuilder {
	b.channels = channels

	return b
}

// WithConditionBuilder overrides the default condition builder.
// Use this when the default Available condition semantics are not appropriate
// (e.g. inverted logic where NOT finding the operator is the success case).
func (b *OperatorBuilder) WithConditionBuilder(builder operators.ConditionBuilder) *OperatorBuilder {
	b.conditionBuilder = builder

	return b
}

// Run executes the operator presence check.
//
// The builder handles:
//   - OLM unavailable: returns a result with "OLM client not available" message
//   - Subscription listing errors: returns wrapped error
//   - Annotation population: target version and operator version are automatically added
//
// Returns (*result.DiagnosticResult, error) following the standard lint check signature.
func (b *OperatorBuilder) Run(ctx context.Context) (*result.DiagnosticResult, error) {
	dr := result.New(
		string(b.check.Group()),
		b.check.CheckKind(),
		b.check.CheckType(),
		b.check.Description(),
	)

	if b.target.TargetVersion != nil {
		dr.Annotations[check.AnnotationCheckTargetVersion] = b.target.TargetVersion.String()
	}

	// Check if OLM client is available.
	if !b.target.Client.OLM().Available() {
		condition := b.conditionBuilder(false, "")
		condition.Message = "OLM client not available"
		dr.Status.Conditions = []result.Condition{condition}

		return dr, nil
	}

	// Build matcher from names and channels.
	matcher := b.buildMatcher()

	// Find the operator via OLM subscriptions.
	found, err := operators.FindOperator(ctx, b.target.Client, matcher)
	if err != nil {
		return nil, fmt.Errorf("checking %s operator presence: %w", b.check.CheckKind(), err)
	}

	// Build condition from find result.
	condition := b.conditionBuilder(found.Found, found.Version)
	dr.Status.Conditions = []result.Condition{condition}

	// Store version in annotations if found.
	if found.Version != "" {
		dr.Annotations[check.AnnotationOperatorInstalledVersion] = found.Version
	}

	return dr, nil
}

// buildMatcher constructs a SubscriptionMatcher from the configured names and channels.
func (b *OperatorBuilder) buildMatcher() operators.SubscriptionMatcher {
	names := b.names
	if len(names) == 0 {
		// Default: match by check kind.
		names = []string{b.check.CheckKind()}
	}

	return func(sub *operators.SubscriptionInfo) bool {
		if !slices.Contains(names, sub.Name) {
			return false
		}

		// If channels are specified, subscription must match one.
		if len(b.channels) > 0 {
			if sub.Channel == "" {
				return false
			}

			return slices.Contains(b.channels, sub.Channel)
		}

		return true
	}
}
