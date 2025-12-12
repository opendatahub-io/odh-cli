package servicemeshoperator

import (
	"context"
	"fmt"

	"github.com/blang/semver/v4"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/lburgazzoli/odh-cli/pkg/lint/check"
	"github.com/lburgazzoli/odh-cli/pkg/lint/check/result"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/shared/operators"
	"github.com/lburgazzoli/odh-cli/pkg/util/jq"
)

const (
	checkID          = "dependencies.servicemeshoperator2.upgrade"
	checkName        = "Dependencies :: ServiceMeshOperator2 :: Upgrade (3.x)"
	checkDescription = "Validates that servicemeshoperator2 is not installed when upgrading to RHOAI 3.x (requires servicemeshoperator3)"
)

// Check validates that Service Mesh Operator v2 is not installed when upgrading to 3.x.
type Check struct{}

func (c *Check) ID() string {
	return checkID
}

func (c *Check) Name() string {
	return checkName
}

func (c *Check) Description() string {
	return checkDescription
}

func (c *Check) Group() check.CheckGroup {
	return check.GroupDependency
}

func (c *Check) CanApply(currentVersion *semver.Version, targetVersion *semver.Version) bool {
	if currentVersion == nil || targetVersion == nil {
		return false
	}

	return currentVersion.Major == 2 && targetVersion.Major >= 3
}

func (c *Check) Validate(ctx context.Context, target *check.CheckTarget) (*result.DiagnosticResult, error) {
	res, err := operators.CheckOperatorPresence(
		ctx,
		target.Client,
		"servicemesh-operator-v2",
		operators.WithDescription(checkDescription),
		operators.WithMatcher(func(subscription *unstructured.Unstructured) bool {
			// Check if this is servicemeshoperator on v2.x channel
			op, err := operators.GetOperator(subscription)
			if err != nil || op.Name != "servicemeshoperator" {
				return false
			}

			// Check if it's on v2.x channel (stable or v2.x)
			channelStr, err := jq.Query[string](subscription, ".spec.channel")
			if err != nil || channelStr == "" {
				return false
			}

			return channelStr == "stable" || channelStr == "v2.x"
		}),
		operators.WithConditionBuilder(func(found bool, version string) metav1.Condition {
			// Inverted logic: NOT finding the operator is good
			if !found {
				return check.NewCondition(
					check.ConditionTypeCompatible,
					metav1.ConditionTrue,
					check.ReasonVersionCompatible,
					"Service Mesh Operator v2 is not installed - ready for RHOAI 3.x upgrade",
				)
			}

			return check.NewCondition(
				check.ConditionTypeCompatible,
				metav1.ConditionFalse,
				check.ReasonVersionIncompatible,
				fmt.Sprintf("Service Mesh Operator v2 (%s) is installed but RHOAI 3.x requires v3", version),
			)
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("checking servicemesh-operator v2 presence: %w", err)
	}

	return res, nil
}

//nolint:gochecknoinits
func init() {
	check.MustRegisterCheck(&Check{})
}
