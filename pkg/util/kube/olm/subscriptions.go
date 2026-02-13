package olm

import (
	"context"
	"fmt"

	"github.com/opendatahub-io/odh-cli/pkg/util/client"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SubscriptionInfo contains the subscription fields relevant for matching.
type SubscriptionInfo struct {
	Name    string
	Channel string
	Version string
}

// Found returns true (always true for a non-nil receiver; nil-safe: returns false for nil).
func (s *SubscriptionInfo) Found() bool {
	return s != nil
}

// GetVersion returns the installed CSV version, or empty string if the receiver is nil.
func (s *SubscriptionInfo) GetVersion() string {
	if s == nil {
		return ""
	}

	return s.Version
}

// SubscriptionMatcher is a predicate function that determines if a subscription matches the desired operator.
type SubscriptionMatcher func(sub *SubscriptionInfo) bool

// FindOperator searches OLM subscriptions for an operator matching the given predicate.
// Returns the matching SubscriptionInfo if found, or nil if no match.
// Returns an error only for infrastructure failures (listing subscriptions).
func FindOperator(
	ctx context.Context,
	k8sClient client.Reader,
	matcher SubscriptionMatcher,
) (*SubscriptionInfo, error) {
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
			return info, nil
		}
	}

	return nil, nil
}
