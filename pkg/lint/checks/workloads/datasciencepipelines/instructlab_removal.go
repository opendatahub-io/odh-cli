package datasciencepipelines

import (
	"context"
	"fmt"
	"strconv"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"

	"github.com/lburgazzoli/odh-cli/pkg/constants"
	"github.com/lburgazzoli/odh-cli/pkg/lint/check"
	"github.com/lburgazzoli/odh-cli/pkg/lint/check/result"
	"github.com/lburgazzoli/odh-cli/pkg/lint/check/validate"
	"github.com/lburgazzoli/odh-cli/pkg/resources"
	"github.com/lburgazzoli/odh-cli/pkg/util/client"
	"github.com/lburgazzoli/odh-cli/pkg/util/components"
	"github.com/lburgazzoli/odh-cli/pkg/util/inspect"
	"github.com/lburgazzoli/odh-cli/pkg/util/version"
)

const (
	kind                        = "datasciencepipelines"
	checkTypeInstructLabRemoval = "instructlab-removal"
)

type InstructLabRemovalCheck struct {
	check.BaseCheck
}

func NewInstructLabRemovalCheck() *InstructLabRemovalCheck {
	return &InstructLabRemovalCheck{
		BaseCheck: check.BaseCheck{
			CheckGroup:       check.GroupWorkload,
			Kind:             kind,
			Type:             checkTypeInstructLabRemoval,
			CheckID:          "workloads.datasciencepipelines.instructlab-removal",
			CheckName:        "Workloads :: DataSciencePipelines :: InstructLab ManagedPipelines Removal (3.x)",
			CheckDescription: "Validates that DSPA objects do not use the removed InstructLab managedPipelines field before upgrading to RHOAI 3.x",
			CheckRemediation: "Remove the '.spec.apiServer.managedPipelines.instructLab' field from affected DSPA objects before upgrading",
		},
	}
}

// CanApply returns whether this check should run for the given target.
// This check only applies when upgrading FROM 2.x TO 3.x and DataSciencePipelines is Managed.
func (c *InstructLabRemovalCheck) CanApply(ctx context.Context, target check.Target) (bool, error) {
	if !version.IsUpgradeFrom2xTo3x(target.CurrentVersion, target.TargetVersion) {
		return false, nil
	}

	dsc, err := client.GetDataScienceCluster(ctx, target.Client)
	if err != nil {
		return false, fmt.Errorf("getting DataScienceCluster: %w", err)
	}

	return components.HasManagementState(dsc, kind, constants.ManagementStateManaged), nil
}

func (c *InstructLabRemovalCheck) Validate(ctx context.Context, target check.Target) (*result.DiagnosticResult, error) {
	return validate.Component(c, target).
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
				req.Result.SetCondition(check.NewCondition(
					check.ConditionTypeCompatible,
					metav1.ConditionFalse,
					check.WithReason(check.ReasonFeatureRemoved),
					check.WithMessage("Found %d DataSciencePipelinesApplication(s) with deprecated '.spec.apiServer.managedPipelines.instructLab' field - InstructLab feature was removed in RHOAI 3.x", len(impactedDSPAs)),
					check.WithImpact(result.ImpactAdvisory),
					check.WithRemediation(c.CheckRemediation),
				))

				req.Result.SetImpactedObjects(usedResourceType, impactedDSPAs)

				return nil
			}

			req.Result.SetCondition(check.NewCondition(
				check.ConditionTypeCompatible,
				metav1.ConditionTrue,
				check.WithReason(check.ReasonVersionCompatible),
				check.WithMessage("No DataSciencePipelinesApplications found using deprecated 'managedPipelines.instructLab' field - ready for RHOAI 3.x upgrade"),
			))

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
