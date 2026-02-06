package operators

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/lburgazzoli/odh-cli/pkg/lint/check/result"
	"github.com/lburgazzoli/odh-cli/pkg/util/client"
)

// SubscriptionInfo contains the subscription fields relevant for matching.
type SubscriptionInfo struct {
	Name    string
	Channel string
	Version string
}

// ConditionBuilder is a function that creates a condition based on operator presence and version.
type ConditionBuilder func(found bool, version string) result.Condition

// SubscriptionMatcher is a predicate function that determines if a subscription matches the desired operator.
type SubscriptionMatcher func(sub *SubscriptionInfo) bool

// FindResult contains the result of searching for an operator subscription.
type FindResult struct {
	Found   bool
	Version string
}

// FindOperator searches OLM subscriptions for an operator matching the given predicate.
// Returns a FindResult indicating whether the operator was found and its version.
// Returns an error only for infrastructure failures (listing subscriptions).
func FindOperator(
	ctx context.Context,
	k8sClient client.Reader,
	matcher SubscriptionMatcher,
) (*FindResult, error) {
	subscriptions, err := k8sClient.OLM().Subscriptions("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing subscriptions: %w", err)
	}

	for i := range subscriptions.Items {
		sub := &subscriptions.Items[i]

		var channel string
		if sub.Spec != nil {
			channel = sub.Spec.Channel
		}

		info := &SubscriptionInfo{
			Name:    sub.Name,
			Channel: channel,
			Version: sub.Status.InstalledCSV,
		}

		if matcher(info) {
			return &FindResult{
				Found:   true,
				Version: info.Version,
			}, nil
		}
	}

	return &FindResult{Found: false}, nil
}
