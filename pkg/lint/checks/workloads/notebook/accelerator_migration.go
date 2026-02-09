package notebook

import (
	"context"
	"fmt"
	"strconv"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/lburgazzoli/odh-cli/pkg/lint/check"
	"github.com/lburgazzoli/odh-cli/pkg/lint/check/result"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/shared/base"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/shared/results"
	"github.com/lburgazzoli/odh-cli/pkg/resources"
	"github.com/lburgazzoli/odh-cli/pkg/util/client"
	"github.com/lburgazzoli/odh-cli/pkg/util/kube"
	"github.com/lburgazzoli/odh-cli/pkg/util/version"
)

const (
	ConditionTypeAcceleratorProfileCompatible = "AcceleratorProfileCompatible"

	// Annotations used by workbenches to reference AcceleratorProfiles.
	annotationAcceleratorName      = "opendatahub.io/accelerator-name"
	annotationAcceleratorNamespace = "opendatahub.io/accelerator-profile-namespace"
)

// AcceleratorMigrationCheck detects Notebook (workbench) CRs referencing AcceleratorProfiles
// that need to be migrated to HardwareProfiles in RHOAI 3.x.
type AcceleratorMigrationCheck struct {
	base.BaseCheck
}

func NewAcceleratorMigrationCheck() *AcceleratorMigrationCheck {
	return &AcceleratorMigrationCheck{
		BaseCheck: base.BaseCheck{
			CheckGroup:       check.GroupWorkload,
			Kind:             kind,
			Type:             check.CheckTypeImpactedWorkloads,
			CheckID:          "workloads.notebook.accelerator-migration",
			CheckName:        "Workloads :: Notebook :: AcceleratorProfile Migration (3.x)",
			CheckDescription: "Detects Notebook (workbench) CRs referencing AcceleratorProfiles that need migration to HardwareProfiles",
			CheckRemediation: "Migrate AcceleratorProfiles to HardwareProfiles before upgrading to RHOAI 3.x",
		},
	}
}

// CanApply returns whether this check should run for the given target.
// Only applies when upgrading from 2.x to 3.x.
func (c *AcceleratorMigrationCheck) CanApply(_ context.Context, target check.Target) bool {
	return version.IsUpgradeFrom2xTo3x(target.CurrentVersion, target.TargetVersion)
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

	// Find notebooks with accelerator profile references and check if the profiles exist
	impacted, missingCount, err := c.findNotebooksWithAcceleratorProfiles(ctx, target)
	if err != nil {
		return nil, err
	}

	totalImpacted := len(impacted)
	dr.Annotations[check.AnnotationImpactedWorkloadCount] = strconv.Itoa(totalImpacted)

	// Add condition based on findings
	dr.Status.Conditions = append(
		dr.Status.Conditions,
		c.newAcceleratorMigrationCondition(totalImpacted, missingCount),
	)

	// Populate ImpactedObjects if any notebooks found
	if totalImpacted > 0 {
		results.PopulateImpactedObjects(dr, resources.Notebook, impacted)
	}

	return dr, nil
}

func (c *AcceleratorMigrationCheck) findNotebooksWithAcceleratorProfiles(
	ctx context.Context,
	target check.Target,
) ([]types.NamespacedName, int, error) {
	// Use ListMetadata since we only need annotations
	notebooks, err := target.Client.ListMetadata(ctx, resources.Notebook)
	if err != nil {
		if client.IsResourceTypeNotFound(err) {
			return nil, 0, nil
		}

		return nil, 0, fmt.Errorf("listing Notebooks: %w", err)
	}

	// Resolve the applications namespace for AcceleratorProfile lookups.
	// AcceleratorProfiles live in the applications namespace, but notebooks may not
	// have the namespace annotation set, so we need a proper default.
	appNS, err := client.GetApplicationsNamespace(ctx, target.Client)
	if err != nil {
		return nil, 0, fmt.Errorf("getting applications namespace: %w", err)
	}

	// Build a cache of existing AcceleratorProfiles
	profileCache, err := c.buildAcceleratorProfileCache(ctx, target)
	if err != nil {
		return nil, 0, err
	}

	var impacted []types.NamespacedName
	missingCount := 0

	for _, nb := range notebooks {
		profileRef := types.NamespacedName{
			Namespace: kube.GetAnnotation(nb, annotationAcceleratorNamespace),
			Name:      kube.GetAnnotation(nb, annotationAcceleratorName),
		}

		if profileRef.Name == "" {
			continue
		}
		if profileRef.Namespace == "" {
			profileRef.Namespace = appNS
		}

		// Track this notebook as impacted
		impacted = append(impacted, types.NamespacedName{
			Namespace: nb.GetNamespace(),
			Name:      nb.GetName(),
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

func (c *AcceleratorMigrationCheck) newAcceleratorMigrationCondition(
	totalImpacted int,
	totalMissing int,
) result.Condition {
	if totalImpacted == 0 {
		return check.NewCondition(
			ConditionTypeAcceleratorProfileCompatible,
			metav1.ConditionTrue,
			check.ReasonVersionCompatible,
			"No Notebooks found using AcceleratorProfiles - no migration needed",
		)
	}

	// If there are missing profiles, this is a blocking issue
	if totalMissing > 0 {
		return check.NewCondition(
			ConditionTypeAcceleratorProfileCompatible,
			metav1.ConditionFalse,
			check.ReasonResourceNotFound,
			"Found %d Notebook(s) referencing AcceleratorProfiles (%d missing) - ensure AcceleratorProfiles exist and migrate to HardwareProfiles",
			totalImpacted,
			totalMissing,
			check.WithImpact(result.ImpactAdvisory),
			check.WithRemediation(c.CheckRemediation),
		)
	}

	// All referenced profiles exist - advisory only
	return check.NewCondition(
		ConditionTypeAcceleratorProfileCompatible,
		metav1.ConditionFalse,
		check.ReasonConfigurationInvalid,
		"Found %d Notebook(s) using AcceleratorProfiles - migrate to HardwareProfiles before upgrading",
		totalImpacted,
		check.WithImpact(result.ImpactAdvisory),
		check.WithRemediation(c.CheckRemediation),
	)
}
