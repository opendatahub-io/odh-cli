package datasciencepipelines

import (
	"context"
	"fmt"
	"strconv"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"

	"github.com/lburgazzoli/odh-cli/pkg/lint/check"
	"github.com/lburgazzoli/odh-cli/pkg/lint/check/result"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/shared/base"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/shared/results"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/shared/validate"
	"github.com/lburgazzoli/odh-cli/pkg/resources"
	"github.com/lburgazzoli/odh-cli/pkg/util/client"
	"github.com/lburgazzoli/odh-cli/pkg/util/inspect"
	"github.com/lburgazzoli/odh-cli/pkg/util/version"
)

type InstructLabRemovalCheck struct {
	base.BaseCheck
}

func NewInstructLabRemovalCheck() *InstructLabRemovalCheck {
	return &InstructLabRemovalCheck{
		BaseCheck: base.BaseCheck{
			CheckGroup:       check.GroupComponent,
			Kind:             check.ComponentDataSciencePipelines,
			Type:             check.CheckTypeInstructLabRemoval,
			CheckID:          "components.datasciencepipelines.instructlab-removal",
			CheckName:        "Components :: DataSciencePipelines :: InstructLab ManagedPipelines Removal (3.x)",
			CheckDescription: "Validates that DSPA objects do not use the removed InstructLab managedPipelines field before upgrading to RHOAI 3.x",
		},
	}
}

// CanApply returns whether this check should run for the given target.
// This check only applies when upgrading FROM 2.x TO 3.x.
func (c *InstructLabRemovalCheck) CanApply(_ context.Context, target check.Target) bool {
	return version.IsUpgradeFrom2xTo3x(target.CurrentVersion, target.TargetVersion)
}

func (c *InstructLabRemovalCheck) Validate(ctx context.Context, target check.Target) (*result.DiagnosticResult, error) {
	return validate.Component(c, target).
		InState(check.ManagementStateManaged).
		Run(ctx, func(ctx context.Context, req *validate.ComponentRequest) error {
			dspas, usedResourceType, err := c.listDSPAs(ctx, req.Client)
			if err != nil {
				return err
			}

			impactedDSPAs := make([]types.NamespacedName, 0)

			for i := range dspas {
				dspa := dspas[i]

				found, err := inspect.HasFields(dspa, ".spec.apiServer.managedPipelines.instructLab")
				if err != nil {
					return fmt.Errorf("querying managedPipelines.instructLab for DSPA %s/%s: %w",
						dspa.GetNamespace(), dspa.GetName(), err)
				}

				if len(found) == 0 {
					continue
				}

				impactedDSPAs = append(impactedDSPAs, types.NamespacedName{
					Namespace: dspa.GetNamespace(),
					Name:      dspa.GetName(),
				})
			}

			req.Result.Annotations[check.AnnotationImpactedWorkloadCount] = strconv.Itoa(len(impactedDSPAs))

			if len(impactedDSPAs) > 0 {
				results.SetCondition(req.Result, check.NewCondition(
					check.ConditionTypeCompatible,
					metav1.ConditionFalse,
					check.ReasonFeatureRemoved,
					"Found %d DataSciencePipelinesApplication(s) with deprecated '.spec.apiServer.managedPipelines.instructLab' field - InstructLab feature was removed in RHOAI 3.x",
					len(impactedDSPAs),
				))

				results.PopulateImpactedObjects(req.Result, usedResourceType, impactedDSPAs)

				return nil
			}

			results.SetCompatibilitySuccessf(req.Result,
				"No DataSciencePipelinesApplications found using deprecated 'managedPipelines.instructLab' field - ready for RHOAI 3.x upgrade")

			return nil
		})
}

// listDSPAs attempts to list DSPAs using v1 first, falling back to v1alpha1 if v1 is not available.
// Returns the list of DSPAs and the ResourceType that was successfully used.
func (c *InstructLabRemovalCheck) listDSPAs(
	ctx context.Context,
	r client.Reader,
) ([]*unstructured.Unstructured, resources.ResourceType, error) {
	// Try v1 first
	dspasV1, err := r.List(ctx, resources.DataSciencePipelinesApplicationV1)
	if err == nil {
		// v1 exists, use it
		return dspasV1, resources.DataSciencePipelinesApplicationV1, nil
	}

	if !apierrors.IsNotFound(err) {
		// Not a NotFound error - something else went wrong
		return nil, resources.ResourceType{}, fmt.Errorf("listing DataSciencePipelinesApplications v1: %w", err)
	}

	// v1 not found, try v1alpha1
	dspasV1Alpha1, err := r.List(ctx, resources.DataSciencePipelinesApplicationV1Alpha1)
	if err != nil {
		return nil, resources.ResourceType{}, fmt.Errorf("listing DataSciencePipelinesApplications v1alpha1: %w", err)
	}

	return dspasV1Alpha1, resources.DataSciencePipelinesApplicationV1Alpha1, nil
}
