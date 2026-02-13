package datasciencepipelines

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/jq"
	"github.com/opendatahub-io/odh-cli/pkg/util/version"
)

const (
	checkTypeStoredVersionRemoval = "stored-version-removal"

	// CRD name for DataSciencePipelinesApplication.
	dspaCRDName = "datasciencepipelinesapplications.datasciencepipelinesapplications.opendatahub.io"

	// The deprecated stored version that will be removed in 3.x.
	deprecatedStoredVersion = "v1alpha1"

	msgStoredVersionFound    = "Some DataSciencePipelinesApplication resources still use the deprecated %s API version which will be removed in RHOAI 3.x"
	msgStoredVersionNotFound = "No DataSciencePipelinesApplication resources using deprecated %s API version - ready for RHOAI 3.x upgrade"
	msgCRDNotFound           = "DataSciencePipelinesApplication CRD not found - DataSciencePipelines may not be installed"
)

// StoredVersionRemovalCheck validates that the DataSciencePipelinesApplication CRD
// does not have v1alpha1 among its status.storedVersions, since v1alpha1 will be
// removed in RHOAI 3.x.
type StoredVersionRemovalCheck struct {
	check.BaseCheck
}

// NewStoredVersionRemovalCheck creates a new StoredVersionRemovalCheck.
func NewStoredVersionRemovalCheck() *StoredVersionRemovalCheck {
	return &StoredVersionRemovalCheck{
		BaseCheck: check.BaseCheck{
			CheckGroup:       check.GroupWorkload,
			Kind:             kind,
			Type:             checkTypeStoredVersionRemoval,
			CheckID:          "workloads.datasciencepipelines.stored-version-removal",
			CheckName:        "Workloads :: DataSciencePipelines :: v1alpha1 StoredVersion Removal (3.x)",
			CheckDescription: "Validates that the DataSciencePipelinesApplication CRD does not have v1alpha1 in status.storedVersions before upgrading to RHOAI 3.x",
			CheckRemediation: "Migrate all DataSciencePipelinesApplication resources from v1alpha1 to v1",
		},
	}
}

// CanApply returns whether this check should run for the given target.
// This check applies when upgrading FROM 2.x TO 3.x.
func (c *StoredVersionRemovalCheck) CanApply(_ context.Context, target check.Target) (bool, error) {
	return version.IsUpgradeFrom2xTo3x(target.CurrentVersion, target.TargetVersion), nil
}

// Validate checks the DSPA CRD status.storedVersions for the deprecated v1alpha1 version.
func (c *StoredVersionRemovalCheck) Validate(
	ctx context.Context,
	target check.Target,
) (*result.DiagnosticResult, error) {
	dr := c.NewResult()

	if target.TargetVersion != nil {
		dr.Annotations[check.AnnotationCheckTargetVersion] = target.TargetVersion.String()
	}

	crd, err := target.Client.GetResource(ctx, resources.CustomResourceDefinition, dspaCRDName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			dr.SetCondition(check.NewCondition(
				check.ConditionTypeCompatible,
				metav1.ConditionTrue,
				check.WithReason(check.ReasonResourceNotFound),
				check.WithMessage(msgCRDNotFound),
			))

			return dr, nil
		}

		return nil, fmt.Errorf("getting CRD %s: %w", dspaCRDName, err)
	}

	// CRD not returned (permission error returns nil)
	if crd == nil {
		dr.SetCondition(check.NewCondition(
			check.ConditionTypeCompatible,
			metav1.ConditionUnknown,
			check.WithReason(check.ReasonAPIAccessDenied),
			check.WithMessage("Unable to access CRD %s - insufficient permissions", dspaCRDName),
		))

		return dr, nil
	}

	storedVersions, err := jq.Query[[]any](crd, ".status.storedVersions")
	if err != nil {
		return nil, fmt.Errorf("querying status.storedVersions for CRD %s: %w", dspaCRDName, err)
	}

	if hasDeprecatedVersion(storedVersions) {
		dr.SetCondition(check.NewCondition(
			check.ConditionTypeCompatible,
			metav1.ConditionFalse,
			check.WithReason(check.ReasonVersionIncompatible),
			check.WithMessage(msgStoredVersionFound, deprecatedStoredVersion),
			check.WithImpact(result.ImpactBlocking),
			check.WithRemediation(c.CheckRemediation),
		))

		return dr, nil
	}

	dr.SetCondition(check.NewCondition(
		check.ConditionTypeCompatible,
		metav1.ConditionTrue,
		check.WithReason(check.ReasonVersionCompatible),
		check.WithMessage(msgStoredVersionNotFound, deprecatedStoredVersion),
	))

	return dr, nil
}

func hasDeprecatedVersion(storedVersions []any) bool {
	for _, v := range storedVersions {
		if s, ok := v.(string); ok && s == deprecatedStoredVersion {
			return true
		}
	}

	return false
}
