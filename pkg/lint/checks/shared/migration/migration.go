package migration

import (
	"context"
	"fmt"
	"strconv"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/lburgazzoli/odh-cli/pkg/lint/check"
	"github.com/lburgazzoli/odh-cli/pkg/lint/check/result"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/shared/results"
	"github.com/lburgazzoli/odh-cli/pkg/resources"
	"github.com/lburgazzoli/odh-cli/pkg/util/client"
	"github.com/lburgazzoli/odh-cli/pkg/util/kube"
)

// Config holds the resource-specific configuration for a migration check.
type Config struct {
	// ResourceType is the Kubernetes resource type to discover.
	ResourceType resources.ResourceType

	// ResourceLabel is the human-readable name used in condition messages (e.g., "AcceleratorProfile").
	ResourceLabel string

	// NoMigrationMessage is the message when no resources are found.
	NoMigrationMessage string

	// MigrationPendingMessage is the printf format for the message when resources are found.
	// Must contain a single %d placeholder for the count.
	MigrationPendingMessage string

	// Remediation provides actionable guidance on how to resolve migration findings.
	Remediation string
}

// ValidateResources discovers resources and populates a DiagnosticResult with migration conditions.
// This eliminates duplication between migration checks that follow the same pattern:
// list resources, report count as advisory if found, report no-migration if empty.
func ValidateResources(
	ctx context.Context,
	target check.Target,
	dr *result.DiagnosticResult,
	cfg Config,
) error {
	if target.TargetVersion != nil {
		dr.Annotations[check.AnnotationCheckTargetVersion] = target.TargetVersion.String()
	}

	profileNames, err := listResources(ctx, target, cfg)
	if err != nil {
		return err
	}

	totalCount := len(profileNames)
	dr.Annotations[check.AnnotationImpactedWorkloadCount] = strconv.Itoa(totalCount)

	// Add condition based on findings.
	if totalCount == 0 {
		results.SetCondition(dr, check.NewCondition(
			check.ConditionTypeMigrationRequired,
			metav1.ConditionTrue,
			check.ReasonNoMigrationRequired,
			cfg.NoMigrationMessage,
		))

		return nil
	}

	// Resources found - advisory notice about auto-migration.
	// Use Status=False (not yet migrated) with advisory impact since this is informational.
	conditionOpts := []any{
		totalCount,
		check.WithImpact(result.ImpactAdvisory),
	}

	if cfg.Remediation != "" {
		conditionOpts = append(conditionOpts, check.WithRemediation(cfg.Remediation))
	}

	results.SetCondition(dr, check.NewCondition(
		check.ConditionTypeMigrationRequired,
		metav1.ConditionFalse,
		check.ReasonMigrationPending,
		cfg.MigrationPendingMessage,
		conditionOpts...,
	))

	// Populate ImpactedObjects.
	results.PopulateImpactedObjects(dr, cfg.ResourceType, profileNames)

	return nil
}

// listResources retrieves all resources of the configured type using metadata-only retrieval.
func listResources(
	ctx context.Context,
	target check.Target,
	cfg Config,
) ([]types.NamespacedName, error) {
	profiles, err := target.Client.ListMetadata(ctx, cfg.ResourceType)
	if err != nil {
		if client.IsResourceTypeNotFound(err) {
			// CRD doesn't exist - nothing to migrate.
			return nil, nil
		}

		return nil, fmt.Errorf("listing %ss: %w", cfg.ResourceLabel, err)
	}

	return kube.ToNamespacedNames(profiles), nil
}
