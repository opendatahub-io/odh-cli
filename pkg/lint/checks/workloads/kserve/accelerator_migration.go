package kserve

import (
	"context"
	"fmt"
	"strconv"

	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/lburgazzoli/odh-cli/pkg/lint/check"
	"github.com/lburgazzoli/odh-cli/pkg/lint/check/result"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/shared/base"
	"github.com/lburgazzoli/odh-cli/pkg/resources"
	"github.com/lburgazzoli/odh-cli/pkg/util/client"
	"github.com/lburgazzoli/odh-cli/pkg/util/kube"
	"github.com/lburgazzoli/odh-cli/pkg/util/version"
)

const (
	ConditionTypeISVCAcceleratorProfileCompatible = "AcceleratorProfileCompatible"

	// Annotations used by InferenceServices to reference AcceleratorProfiles.
	annotationAcceleratorName      = "opendatahub.io/accelerator-name"
	annotationAcceleratorNamespace = "opendatahub.io/accelerator-profile-namespace"

	// minAcceleratorMigrationMajorVersion is the minimum major version for this check to apply.
	minAcceleratorMigrationMajorVersion = 3
)

// AcceleratorMigrationCheck detects InferenceService CRs referencing AcceleratorProfiles
// that need to be migrated to HardwareProfiles in RHOAI 3.x.
type AcceleratorMigrationCheck struct {
	base.BaseCheck
}

func NewAcceleratorMigrationCheck() *AcceleratorMigrationCheck {
	return &AcceleratorMigrationCheck{
		BaseCheck: base.BaseCheck{
			CheckGroup:       check.GroupWorkload,
			Kind:             check.ComponentKServe,
			CheckType:        check.CheckTypeImpactedWorkloads,
			CheckID:          "workloads.kserve.accelerator-migration",
			CheckName:        "Workloads :: KServe :: AcceleratorProfile Migration (3.x)",
			CheckDescription: "Detects InferenceService CRs referencing AcceleratorProfiles that need migration to HardwareProfiles",
			CheckRemediation: "Migrate AcceleratorProfiles to HardwareProfiles before upgrading to RHOAI 3.x",
		},
	}
}

// CanApply returns whether this check should run for the given target.
// Only applies when upgrading to 3.x or later.
func (c *AcceleratorMigrationCheck) CanApply(target check.Target) bool {
	return version.IsVersionAtLeast(target.TargetVersion, minAcceleratorMigrationMajorVersion, 0)
}

// Validate executes the check against the provided target.
func (c *AcceleratorMigrationCheck) Validate(
	ctx context.Context,
	target check.Target,
) (*result.DiagnosticResult, error) {
	dr := c.NewResult()

	if target.TargetVersion != nil {
		dr.Annotations[check.AnnotationCheckTargetVersion] = target.TargetVersion.String()
	}

	// Find InferenceServices with accelerator profile references and check if the profiles exist
	impacted, missingCount, err := c.findInferenceServicesWithAcceleratorProfiles(ctx, target)
	if err != nil {
		return nil, err
	}

	totalImpacted := len(impacted)
	dr.Annotations[check.AnnotationImpactedWorkloadCount] = strconv.Itoa(totalImpacted)

	// Add condition based on findings
	dr.Status.Conditions = append(
		dr.Status.Conditions,
		newISVCAcceleratorMigrationCondition(totalImpacted, missingCount),
	)

	// Populate ImpactedObjects if any InferenceServices found
	if totalImpacted > 0 {
		populateISVCAcceleratorImpactedObjects(dr, impacted)
	}

	return dr, nil
}

func (c *AcceleratorMigrationCheck) findInferenceServicesWithAcceleratorProfiles(
	ctx context.Context,
	target check.Target,
) ([]types.NamespacedName, int, error) {
	// Use ListMetadata since we only need annotations
	inferenceServices, err := target.Client.ListMetadata(ctx, resources.InferenceService)
	if err != nil {
		if client.IsResourceTypeNotFound(err) {
			return nil, 0, nil
		}

		return nil, 0, fmt.Errorf("listing InferenceServices: %w", err)
	}

	// Build a cache of existing AcceleratorProfiles
	profileCache, err := c.buildAcceleratorProfileCache(ctx, target)
	if err != nil {
		return nil, 0, err
	}

	var impacted []types.NamespacedName
	missingCount := 0

	for _, isvc := range inferenceServices {
		profileRef := types.NamespacedName{
			Namespace: kube.GetAnnotation(isvc, annotationAcceleratorNamespace),
			Name:      kube.GetAnnotation(isvc, annotationAcceleratorName),
		}

		if profileRef.Name == "" {
			continue
		}
		if profileRef.Namespace == "" {
			profileRef.Namespace = isvc.GetNamespace()
		}

		// Track this InferenceService as impacted
		impacted = append(impacted, types.NamespacedName{
			Namespace: isvc.GetNamespace(),
			Name:      isvc.GetName(),
		})

		// Check if the referenced AcceleratorProfile exists
		if !profileCache.Has(profileRef) {
			missingCount++
		}
	}

	return impacted, missingCount, nil
}

func (c *AcceleratorMigrationCheck) buildAcceleratorProfileCache(
	ctx context.Context,
	target check.Target,
) (sets.Set[types.NamespacedName], error) {
	// Use ListMetadata since we only need namespace/name
	profiles, err := target.Client.ListMetadata(ctx, resources.AcceleratorProfile)
	if err != nil {
		if client.IsResourceTypeNotFound(err) {
			// AcceleratorProfile CRD doesn't exist - all references are missing
			return sets.New[types.NamespacedName](), nil
		}

		return nil, fmt.Errorf("listing AcceleratorProfiles: %w", err)
	}

	cache := sets.New[types.NamespacedName]()

	for _, profile := range profiles {
		cache.Insert(types.NamespacedName{
			Namespace: profile.GetNamespace(),
			Name:      profile.GetName(),
		})
	}

	return cache, nil
}
