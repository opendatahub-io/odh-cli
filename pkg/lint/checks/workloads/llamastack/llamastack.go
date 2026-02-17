package llamastack

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/opendatahub-io/odh-cli/pkg/constants"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/validate"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
	"github.com/opendatahub-io/odh-cli/pkg/util/components"
	"github.com/opendatahub-io/odh-cli/pkg/util/version"
)

const (
	kind = "llamastackdistribution"

	ConditionTypeRequiresRecreation = "RequiresRecreation"

	numConditions = 1 // Number of conditions in buildConditions
)

// ConfigCheck validates LlamaStackDistribution resources for 3.3 upgrade compatibility.
type ConfigCheck struct {
	check.BaseCheck
}

func NewConfigCheck() *ConfigCheck {
	return &ConfigCheck{
		BaseCheck: check.BaseCheck{
			CheckGroup:       check.GroupWorkload,
			Kind:             kind,
			Type:             "config",
			CheckID:          "workloads.llamastack.config",
			CheckName:        "Workloads :: LlamaStack :: Upgrade Preparation (2.x to 3.3)",
			CheckDescription: "Identifies LlamaStackDistribution resources that require deletion and recreation for RHOAI 3.3 upgrade",
			CheckRemediation: "Back up configurations using the backup script from rhoai-upgrade-helpers repository, coordinate with owners, then delete and recreate LlamaStackDistributions in RHOAI 3.3.0 following the migration guide",
		},
	}
}

// CanApply returns whether this check should run for the given target.
func (c *ConfigCheck) CanApply(ctx context.Context, target check.Target) (bool, error) {
	if !version.IsUpgradeFrom2xTo3x(target.CurrentVersion, target.TargetVersion) {
		return false, nil
	}

	dsc, err := client.GetDataScienceCluster(ctx, target.Client)
	if err != nil {
		return false, fmt.Errorf("getting DataScienceCluster: %w", err)
	}

	return components.HasManagementState(dsc, "llamastackoperator", constants.ManagementStateManaged), nil
}

// Validate executes the check against the provided target.
func (c *ConfigCheck) Validate(
	ctx context.Context,
	target check.Target,
) (*result.DiagnosticResult, error) {
	return validate.Workloads(c, target, resources.LlamaStackDistribution).
		Run(ctx, c.validateDistributions)
}

func (c *ConfigCheck) validateDistributions(
	_ context.Context,
	req *validate.WorkloadRequest[*unstructured.Unstructured],
) error {
	count := len(req.Items)

	if count == 0 {
		// No LlamaStackDistributions found - upgrade can proceed
		req.Result.SetCondition(newNoWorkloadsCondition())

		return nil
	}

	// ALL LlamaStackDistributions require recreation
	// Create conditions
	conditions := c.buildConditions(count)

	for _, cond := range conditions {
		req.Result.SetCondition(cond)
	}

	// Populate impacted objects - ALL LLSDs are impacted
	req.Result.ImpactedObjects = make([]metav1.PartialObjectMetadata, 0, len(req.Items))

	for _, llsd := range req.Items {
		namespace := llsd.GetNamespace()
		name := llsd.GetName()

		req.Result.ImpactedObjects = append(req.Result.ImpactedObjects, metav1.PartialObjectMetadata{
			TypeMeta: resources.LlamaStackDistribution.TypeMeta(),
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      name,
				Annotations: map[string]string{
					"upgrade.action": "requires-recreation",
				},
			},
		})
	}

	return nil
}

func newNoWorkloadsCondition() result.Condition {
	return check.NewCondition(
		ConditionTypeRequiresRecreation,
		metav1.ConditionTrue,
		check.WithReason(check.ReasonResourceNotFound),
		check.WithMessage("No LlamaStackDistribution resources found - upgrade can proceed without LlamaStack-specific actions"),
	)
}

func (c *ConfigCheck) buildConditions(totalCount int) []result.Condition {
	conditions := make([]result.Condition, 0, numConditions)

	// ALL LlamaStackDistributions require recreation (BLOCKING)
	conditions = append(conditions, check.NewCondition(
		ConditionTypeRequiresRecreation,
		metav1.ConditionFalse,
		check.WithReason("ArchitecturalIncompatibility"),
		check.WithMessage("Found %d LlamaStackDistribution(s) that must be deleted and recreated after RHOAI 3.3 upgrade. In-place upgrade is not supported. ALL DATA WILL BE LOST - Archive data before upgrade.", totalCount),
		check.WithImpact(result.ImpactBlocking),
		check.WithRemediation("1. Run backup script: llamastack/backup-all-llamastack.sh from rhoai-upgrade-helpers repo to archive existing configurations. 2. Coordinate with LLSD owners about data loss and recreation requirements. 3. After RHOAI 3.3 upgrade, delete old LLSDs and create new ones following RHOAI 3.3 documentation."),
	))

	return conditions
}
