package datasciencepipelines

import (
	"context"
	"errors"
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
	"github.com/lburgazzoli/odh-cli/pkg/resources"
	"github.com/lburgazzoli/odh-cli/pkg/util/jq"
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
			CheckType:        check.CheckTypeInstructLabRemoval,
			CheckID:          "components.datasciencepipelines.instructlab-removal",
			CheckName:        "Components :: DataSciencePipelines :: InstructLab ManagedPipelines Removal (3.x)",
			CheckDescription: "Validates that DSPA objects do not use the removed InstructLab managedPipelines field before upgrading to RHOAI 3.x",
		},
	}
}

// CanApply returns whether this check should run for the given target.
// This check only applies when upgrading FROM 2.x TO 3.x.
func (c *InstructLabRemovalCheck) CanApply(target check.Target) bool {
	return version.IsUpgradeFrom2xTo3x(target.CurrentVersion, target.TargetVersion)
}

// Validate executes the check against the provided target.
func (c *InstructLabRemovalCheck) Validate(
	ctx context.Context,
	target check.Target,
) (*result.DiagnosticResult, error) {
	dr := c.NewResult()

	if target.TargetVersion != nil {
		dr.Annotations[check.AnnotationCheckTargetVersion] = target.TargetVersion.String()
	}

	// Determine which DSPA API version is available at runtime
	dspas, usedResourceType, err := c.listDSPAs(ctx, target)
	if err != nil {
		return nil, err
	}

	// Find DSPAs with managedPipelines.instructLab field set
	impactedDSPAs := make([]types.NamespacedName, 0)

	for i := range dspas {
		dspa := dspas[i]

		// Check if .spec.apiServer.managedPipelines.instructLab exists
		_, err := jq.Query[any](&dspa, ".spec.apiServer.managedPipelines.instructLab")
		if err != nil {
			if errors.Is(err, jq.ErrNotFound) {
				// Field doesn't exist - this DSPA is not impacted
				continue
			}
			// Other error - propagate
			return nil, fmt.Errorf("querying managedPipelines.instructLab for DSPA %s/%s: %w",
				dspa.GetNamespace(), dspa.GetName(), err)
		}

		// If we get here, field exists - add to impacted list
		impactedDSPAs = append(impactedDSPAs, types.NamespacedName{
			Namespace: dspa.GetNamespace(),
			Name:      dspa.GetName(),
		})
	}

	// Set impacted count annotation
	dr.Annotations[check.AnnotationImpactedWorkloadCount] = strconv.Itoa(len(impactedDSPAs))

	// Create condition based on findings
	if len(impactedDSPAs) > 0 {
		// FAILURE - blocking upgrade
		results.SetCondition(dr, check.NewCondition(
			check.ConditionTypeCompatible,
			metav1.ConditionFalse,
			check.ReasonFeatureRemoved,
			"Found %d DataSciencePipelinesApplication(s) with deprecated '.spec.apiServer.managedPipelines.instructLab' field - InstructLab feature was removed in RHOAI 3.x",
			len(impactedDSPAs),
		))

		// Populate ImpactedObjects
		populateImpactedDSPAs(dr, impactedDSPAs, usedResourceType)

		return dr, nil
	}

	// Success - no DSPAs using managedPipelines.instructLab
	results.SetCompatibilitySuccessf(dr,
		"No DataSciencePipelinesApplications found using deprecated 'managedPipelines.instructLab' field - ready for RHOAI 3.x upgrade")

	return dr, nil
}

// listDSPAs attempts to list DSPAs using v1 first, falling back to v1alpha1 if v1 is not available.
// Returns the list of DSPAs and the ResourceType that was successfully used.
func (c *InstructLabRemovalCheck) listDSPAs(
	ctx context.Context,
	target check.Target,
) ([]unstructured.Unstructured, resources.ResourceType, error) {
	// Try v1 first
	dspasV1, err := target.Client.List(ctx, resources.DataSciencePipelinesApplicationV1)
	if err == nil {
		// v1 exists, use it
		return dspasV1, resources.DataSciencePipelinesApplicationV1, nil
	}

	if !apierrors.IsNotFound(err) {
		// Not a NotFound error - something else went wrong
		return nil, resources.ResourceType{}, fmt.Errorf("listing DataSciencePipelinesApplications v1: %w", err)
	}

	// v1 not found, try v1alpha1
	dspasV1Alpha1, err := target.Client.List(ctx, resources.DataSciencePipelinesApplicationV1Alpha1)
	if err != nil {
		return nil, resources.ResourceType{}, fmt.Errorf("listing DataSciencePipelinesApplications v1alpha1: %w", err)
	}

	return dspasV1Alpha1, resources.DataSciencePipelinesApplicationV1Alpha1, nil
}

// populateImpactedDSPAs adds impacted DSPA objects to the diagnostic result.
func populateImpactedDSPAs(
	dr *result.DiagnosticResult,
	impactedDSPAs []types.NamespacedName,
	resourceType resources.ResourceType,
) {
	dr.ImpactedObjects = make([]metav1.PartialObjectMetadata, 0, len(impactedDSPAs))

	for _, dspa := range impactedDSPAs {
		obj := metav1.PartialObjectMetadata{
			TypeMeta: resourceType.TypeMeta(),
			ObjectMeta: metav1.ObjectMeta{
				Namespace: dspa.Namespace,
				Name:      dspa.Name,
			},
		}
		dr.ImpactedObjects = append(dr.ImpactedObjects, obj)
	}
}

//nolint:gochecknoinits
func init() {
	check.MustRegisterCheck(NewInstructLabRemovalCheck())
}
