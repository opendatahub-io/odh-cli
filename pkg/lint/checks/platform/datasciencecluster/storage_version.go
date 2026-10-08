package datasciencecluster

import (
	"context"
	"fmt"
	"slices"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/opendatahub-io/odh-cli/pkg/constants"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/jq"
	"github.com/opendatahub-io/odh-cli/pkg/util/version"
)

const storageVersionRemediation = "Run odh migrate run --migration dsc.storage-version.migrate --target-version 3.6.0 before removing v1 from the DataScienceCluster CRD."

const (
	storageVersionTargetMajor = 3
	storageVersionTargetMinor = 6
)

// StorageVersionCheck prevents retiring v1 while it remains in the CRD storage history.
type StorageVersionCheck struct {
	check.BaseCheck
}

// NewStorageVersionCheck creates the CRD-only DataScienceCluster storage check.
func NewStorageVersionCheck() *StorageVersionCheck {
	return &StorageVersionCheck{BaseCheck: check.BaseCheck{
		CheckGroup:       check.GroupPlatform,
		Kind:             constants.PlatformDSC,
		Type:             check.CheckType("storage-version"),
		CheckID:          "platform.dsc.storage-version",
		CheckName:        "Platform :: DSC :: Storage Version",
		CheckDescription: "Validates that DataScienceCluster v1 is absent from CRD storedVersions",
		CheckRemediation: storageVersionRemediation,
	}}
}

// CanApply applies to every source release when the destination is at least 3.6.
func (c StorageVersionCheck) CanApply(_ context.Context, target check.Target) (bool, error) {
	return version.IsVersionAtLeast(target.TargetVersion, storageVersionTargetMajor, storageVersionTargetMinor), nil
}

// Validate reads storage history, rather than the converted custom-resource API.
func (c StorageVersionCheck) Validate(ctx context.Context, target check.Target) (*result.DiagnosticResult, error) {
	name := resources.DataScienceCluster.CRDFQN()
	crd, err := target.Client.GetResource(ctx, resources.CustomResourceDefinition, name)
	if err != nil {
		return nil, fmt.Errorf("reading DataScienceCluster CRD: %w", err)
	}
	if crd == nil {
		return nil, fmt.Errorf("reading DataScienceCluster CRD %s: no resource returned", name)
	}

	storedVersions, err := jq.Query[[]string](crd, ".status.storedVersions")
	if err != nil {
		return nil, fmt.Errorf("reading DataScienceCluster storedVersions: %w", err)
	}
	if len(storedVersions) == 0 || slices.Contains(storedVersions, "") {
		return nil, fmt.Errorf("DataScienceCluster CRD %s has invalid storedVersions", name)
	}

	dr := c.NewResult()
	if target.TargetVersion != nil {
		dr.Annotations[check.AnnotationCheckTargetVersion] = target.TargetVersion.String()
	}

	switch {
	case slices.Contains(storedVersions, "v1"):
		dr.SetCondition(check.NewCondition(check.ConditionTypeMigrationRequired, metav1.ConditionFalse,
			check.WithReason(check.ReasonMigrationPending),
			check.WithMessage("DataScienceCluster CRD storedVersions contains v1; migrate stored objects before retiring v1"),
			check.WithImpact(result.ImpactBlocking),
			check.WithRemediation(storageVersionRemediation),
		))
	default:
		dr.SetCondition(check.NewCondition(check.ConditionTypeMigrationRequired, metav1.ConditionTrue,
			check.WithReason(check.ReasonNoMigrationRequired),
			check.WithMessage("DataScienceCluster CRD storedVersions does not contain v1"),
		))
	}

	return dr, nil
}
