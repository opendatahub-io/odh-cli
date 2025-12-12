package operators

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/lburgazzoli/odh-cli/pkg/lint/check"
	"github.com/lburgazzoli/odh-cli/pkg/lint/check/result"
	"github.com/lburgazzoli/odh-cli/pkg/resources"
	"github.com/lburgazzoli/odh-cli/pkg/util/client"
	"github.com/lburgazzoli/odh-cli/pkg/util/jq"
)

// Operator represents an installed operator with its name and version.
type Operator struct {
	Name    string
	Version string
}

// GetOperator extracts the operator name and version from a subscription.
func GetOperator(subscription *unstructured.Unstructured) (*Operator, error) {
	name, err := jq.Query(subscription, ".metadata.name")
	if err != nil || name == nil {
		return nil, fmt.Errorf("failed to get subscription name: %w", err)
	}

	nameStr, ok := name.(string)
	if !ok {
		return nil, fmt.Errorf("subscription name is not a string: %T", name)
	}

	op := &Operator{Name: nameStr}

	// Get version from installedCSV
	installedCSV, err := jq.Query(subscription, ".status.installedCSV")
	if err == nil && installedCSV != nil {
		if csvStr, ok := installedCSV.(string); ok {
			op.Version = csvStr
		}
	}

	return op, nil
}

// ConditionBuilder is a function that creates a condition based on operator presence and version.
type ConditionBuilder func(found bool, version string) metav1.Condition

// SubscriptionMatcher is a predicate function that determines if a subscription matches the desired operator.
// It receives the entire subscription for maximum flexibility.
type SubscriptionMatcher func(subscription *unstructured.Unstructured) bool

// CheckConfig holds configuration for operator presence checks.
type CheckConfig struct {
	Group            string
	Kind             string
	Name             string
	Description      string
	Matcher          SubscriptionMatcher
	ConditionBuilder ConditionBuilder
}

// Option is a functional option for configuring operator presence checks.
type Option func(*CheckConfig)

// WithGroup sets the diagnostic group.
func WithGroup(group string) Option {
	return func(c *CheckConfig) {
		c.Group = group
	}
}

// WithName sets the diagnostic name.
func WithName(name string) Option {
	return func(c *CheckConfig) {
		c.Name = name
	}
}

// WithDescription sets the check description.
func WithDescription(description string) Option {
	return func(c *CheckConfig) {
		c.Description = description
	}
}

// WithMatcher sets the subscription matcher function.
func WithMatcher(matcher SubscriptionMatcher) Option {
	return func(c *CheckConfig) {
		c.Matcher = matcher
	}
}

// WithConditionBuilder sets the condition builder function.
func WithConditionBuilder(builder ConditionBuilder) Option {
	return func(c *CheckConfig) {
		c.ConditionBuilder = builder
	}
}

// CheckOperatorPresence checks if an operator is installed using functional options.
// It provides sensible defaults and allows customization through options.
//
// Default behavior:
// - Group: "dependency"
// - Name: "installed"
// - Matcher: exact match on operator name
// - ConditionBuilder: standard Available condition (True if found, False if not)
//
// Example usage:
//
//	operators.CheckOperatorPresence(ctx, client, "certmanager",
//	    operators.WithDescription("Checks cert-manager installation"),
//	    operators.WithMatcher(func(name string) bool {
//	        return name == "cert-manager" || name == "openshift-cert-manager-operator"
//	    }),
//	)
func CheckOperatorPresence(
	ctx context.Context,
	k8sClient *client.Client,
	operatorKind string,
	opts ...Option,
) (*result.DiagnosticResult, error) {
	// Initialize config with defaults
	config := &CheckConfig{
		Group:       "dependency",
		Kind:        operatorKind,
		Name:        "installed",
		Description: fmt.Sprintf("Reports the %s operator installation status and version", operatorKind),
		// Default matcher: exact match on operator name
		Matcher: func(subscription *unstructured.Unstructured) bool {
			op, err := GetOperator(subscription)
			if err != nil {
				return false
			}

			return op.Name == operatorKind
		},
		// Default condition builder: standard Available condition
		ConditionBuilder: func(found bool, version string) metav1.Condition {
			if !found {
				return check.NewCondition(
					check.ConditionTypeAvailable,
					metav1.ConditionFalse,
					check.ReasonResourceNotFound,
					operatorKind+" operator is not installed",
				)
			}

			return check.NewCondition(
				check.ConditionTypeAvailable,
				metav1.ConditionTrue,
				check.ReasonResourceFound,
				fmt.Sprintf("%s operator installed: %s", operatorKind, version),
			)
		},
	}

	// Apply options
	for _, opt := range opts {
		opt(config)
	}

	// Create diagnostic result
	dr := result.New(config.Group, config.Kind, config.Name, config.Description)

	// List subscriptions
	subscriptions, err := k8sClient.List(ctx, resources.Subscription)
	if err != nil {
		return nil, fmt.Errorf("listing subscriptions: %w", err)
	}

	// Find matching operator
	var version string
	for _, sub := range subscriptions {
		if config.Matcher(&sub) {
			// Found the operator - get version
			op, err := GetOperator(&sub)
			if err == nil {
				version = op.Version
			}

			break
		}
	}

	// Build condition
	condition := config.ConditionBuilder(version != "", version)
	dr.Status.Conditions = []metav1.Condition{condition}

	// Store version in annotations if found
	if version != "" {
		dr.Metadata.Annotations["operator.opendatahub.io/installed-version"] = version
	}

	return dr, nil
}
