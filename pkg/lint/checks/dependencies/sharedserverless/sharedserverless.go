package sharedserverless

import (
	"context"
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/lint/checks/dependencies/shared"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
	"github.com/opendatahub-io/odh-cli/pkg/util/version"
)

const (
	checkKind = "shared-serverless"
	checkType = "shared-usage"
)

func managedNamespaces() []string {
	return []string{
		"istio-system",
		"knative-serving",
		"knative-eventing",
		"openshift-serverless",
		"openshift-operators",
	}
}

// Check detects Knative/Serverless resources (KnativeServing, KnativeEventing, KService) outside
// RHOAI-managed namespaces, indicating shared Serverless usage between AI and non-AI workloads.
type Check struct {
	check.BaseCheck
}

func NewCheck() *Check {
	return &Check{
		BaseCheck: check.BaseCheck{
			CheckGroup:       check.GroupDependency,
			Kind:             checkKind,
			Type:             checkType,
			CheckID:          "dependencies.shared-serverless.shared-usage",
			CheckName:        "Dependencies :: Shared Serverless :: Shared Usage Detection",
			CheckDescription: "Detects Knative/Serverless resources shared between RHOAI and non-AI workloads",
			CheckRemediation: "Review the identified Knative/Serverless resources before migration. Non-AI workloads using OpenShift Serverless may be impacted by the RHOAI 2.x to 3.x migration.",
		},
	}
}

func (c *Check) CanApply(_ context.Context, target check.Target) (bool, error) {
	return version.IsUpgradeFrom2xTo3x(target.CurrentVersion, target.TargetVersion), nil
}

func (c *Check) Validate(ctx context.Context, target check.Target) (*result.DiagnosticResult, error) {
	dr := c.NewResult()

	rhoaiNS := shared.RHOAIManagedNamespaces(ctx, target.Client, managedNamespaces())
	isNonRHOAI := shared.IsNonRHOAIFilter(rhoaiNS)

	kservices, err := client.List(ctx, target.Client, resources.KnativeService, isNonRHOAI)
	if err != nil {
		return nil, fmt.Errorf("listing Knative Services: %w", err)
	}

	servings, err := client.List(ctx, target.Client, resources.KnativeServing, isNonRHOAI)
	if err != nil {
		return nil, fmt.Errorf("listing KnativeServings: %w", err)
	}

	eventings, err := client.List(ctx, target.Client, resources.KnativeEventing, isNonRHOAI)
	if err != nil {
		return nil, fmt.Errorf("listing KnativeEventings: %w", err)
	}

	totalCount := len(kservices) + len(servings) + len(eventings)
	if totalCount == 0 {
		dr.SetCondition(check.NewCondition(
			check.ConditionTypeValidated,
			metav1.ConditionTrue,
			check.WithReason(check.ReasonRequirementsMet),
			check.WithMessage("No shared Serverless resources detected outside RHOAI-managed namespaces"),
		))

		return dr, nil
	}

	namespaces := shared.CollectNamespaces(kservices, servings, eventings)

	dr.SetCondition(check.NewCondition(
		check.ConditionTypeValidated,
		metav1.ConditionFalse,
		check.WithReason(check.ReasonWorkloadsImpacted),
		check.WithMessage(
			"Found %d Serverless resource(s) outside RHOAI-managed namespaces in: %s. These may be impacted by the RHOAI migration",
			totalCount,
			strings.Join(namespaces, ", "),
		),
		check.WithRemediation(c.CheckRemediation),
	))

	shared.AddAllImpactedObjects(dr,
		shared.ImpactedEntry{ResourceType: resources.KnativeService, Items: kservices},
		shared.ImpactedEntry{ResourceType: resources.KnativeServing, Items: servings},
		shared.ImpactedEntry{ResourceType: resources.KnativeEventing, Items: eventings},
	)

	return dr, nil
}
