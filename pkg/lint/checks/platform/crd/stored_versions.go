package crd

import (
	"context"
	"fmt"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/opendatahub-io/odh-cli/pkg/constants"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/jq"
	"github.com/opendatahub-io/odh-cli/pkg/util/version"
)

//nolint:gochecknoglobals // Constant-like list of CRD names to check
var storedVersionCRDs = []string{
	"datascienceclusters.datasciencecluster.opendatahub.io",
	"dscinitializations.dscinitialization.opendatahub.io",
	"hardwareprofiles.infrastructure.opendatahub.io",
}

const (
	conditionTypeStoredVersions = "StoredVersionsCompatible"

	msgStoredVersionsOK       = "All CRD storedVersions are compatible with served versions"
	msgStoredVersionsMismatch = "CRD %s has storedVersion(s) [%s] not served by current spec.versions [%s]"
	msgNoCRDsFound            = "No target CRDs found"
)

// StoredVersionsCheck validates that CRD status.storedVersions only contains versions
// present in spec.versions. Stale storedVersions cause OLM install plan failures.
type StoredVersionsCheck struct {
	check.BaseCheck
}

func NewStoredVersionsCheck() *StoredVersionsCheck {
	return &StoredVersionsCheck{
		BaseCheck: check.BaseCheck{
			CheckGroup:       check.GroupPlatform,
			Kind:             constants.PlatformCRD,
			Type:             check.CheckTypeStoredVersions,
			CheckID:          "platform.crd.stored-versions",
			CheckName:        "Platform :: CRD :: StoredVersions Compatibility (3.x)",
			CheckDescription: "Validates that CRD storedVersions are compatible with served versions",
			CheckRemediation: "Patch CRD status.storedVersions to remove stale versions: kubectl patch crd <name> --subresource=status --type=json -p='[{\"op\":\"replace\",\"path\":\"/status/storedVersions\",\"value\":[\"<served-versions>\"]}]'",
		},
	}
}

func (c *StoredVersionsCheck) CanApply(_ context.Context, target check.Target) (bool, error) {
	return version.IsUpgradeFrom2xTo3x(target.CurrentVersion, target.TargetVersion), nil
}

func (c *StoredVersionsCheck) Validate(
	ctx context.Context,
	target check.Target,
) (*result.DiagnosticResult, error) {
	dr := c.NewResult()

	if target.TargetVersion != nil {
		dr.Annotations[check.AnnotationCheckTargetVersion] = target.TargetVersion.String()
	}

	var issues []string
	found := 0

	for _, crdName := range storedVersionCRDs {
		issue, err := checkCRDStoredVersions(ctx, target, crdName)
		if err != nil {
			return nil, err
		}

		if issue == "" {
			continue
		}

		found++

		if issue != "ok" {
			issues = append(issues, issue)
		}
	}

	if found == 0 {
		dr.SetCondition(check.NewCondition(
			conditionTypeStoredVersions,
			metav1.ConditionTrue,
			check.WithReason(check.ReasonResourceNotFound),
			check.WithMessage(msgNoCRDsFound),
		))

		return dr, nil
	}

	if len(issues) > 0 {
		dr.SetCondition(check.NewCondition(
			conditionTypeStoredVersions,
			metav1.ConditionFalse,
			check.WithReason(check.ReasonVersionIncompatible),
			check.WithMessage("%s", strings.Join(issues, "; ")),
			check.WithImpact(result.ImpactBlocking),
			check.WithRemediation(c.CheckRemediation),
		))

		return dr, nil
	}

	dr.SetCondition(check.NewCondition(
		conditionTypeStoredVersions,
		metav1.ConditionTrue,
		check.WithReason(check.ReasonVersionCompatible),
		check.WithMessage(msgStoredVersionsOK),
	))

	return dr, nil
}

// checkCRDStoredVersions checks a single CRD for stale storedVersions.
// Returns "" if CRD not found, "ok" if compatible, or a description of the issue.
func checkCRDStoredVersions(
	ctx context.Context,
	target check.Target,
	crdName string,
) (string, error) {
	crd, err := target.Client.GetResource(ctx, resources.CustomResourceDefinition, crdName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return "", nil
		}

		return "", fmt.Errorf("getting CRD %s: %w", crdName, err)
	}

	if crd == nil {
		return "", nil
	}

	storedVersions, err := jq.Query[[]any](crd, ".status.storedVersions")
	if err != nil {
		return "", fmt.Errorf("querying status.storedVersions for CRD %s: %w", crdName, err)
	}

	servedVersions, err := jq.Query[[]any](crd, "[.spec.versions[].name]")
	if err != nil {
		return "", fmt.Errorf("querying spec.versions for CRD %s: %w", crdName, err)
	}

	servedSet := make(map[string]bool, len(servedVersions))
	servedNames := make([]string, 0, len(servedVersions))

	for _, v := range servedVersions {
		if s, ok := v.(string); ok {
			servedSet[s] = true
			servedNames = append(servedNames, s)
		}
	}

	var stale []string

	for _, v := range storedVersions {
		if s, ok := v.(string); ok && !servedSet[s] {
			stale = append(stale, s)
		}
	}

	if len(stale) > 0 {
		return fmt.Sprintf(msgStoredVersionsMismatch,
			crdName, strings.Join(stale, ", "), strings.Join(servedNames, ", ")), nil
	}

	return "ok", nil
}
