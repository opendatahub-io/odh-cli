package rhbok

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/opendatahub-io/odh-cli/pkg/migrate/action"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/action/result"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
)

const (
	legacyCohortCRD     = "cohorts.kueue.x-k8s.io"
	legacyTopologyCRD   = "topologies.kueue.x-k8s.io"
	legacyCohortGroup   = "kueue.x-k8s.io"
	legacyCohortVersion = "v1alpha1"
	legacyCohortKind    = "Cohort"
	legacyTopologyKind  = "Topology"
)

//nolint:gochecknoglobals // static legacy CRD names for migration
var legacyCRDNames = []string{legacyCohortCRD, legacyTopologyCRD}

func (a *RHBOKMigrationAction) deleteLegacyCRDs(
	ctx context.Context,
	target action.Target,
) {
	step := target.Recorder.Child(
		"delete-legacy-crds",
		"Delete legacy v1alpha1 Kueue CRDs",
	)

	if target.DryRun {
		step.Completef(result.StepSkipped, "Would delete legacy CRDs: %v", legacyCRDNames)

		return
	}

	failed := 0

	for _, crdName := range legacyCRDNames {
		crdStep := step.Child("delete-"+crdName, "Delete CRD "+crdName)

		_, err := target.Client.APIExtensions().ApiextensionsV1().
			CustomResourceDefinitions().
			Get(ctx, crdName, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			crdStep.Completef(result.StepSkipped, "CRD not found")

			continue
		}

		if err != nil {
			crdStep.Completef(result.StepFailed, "Failed to get CRD: %v", err)

			failed++

			continue
		}

		if !a.ForceDeleteLegacyCRDs {
			if hasInstances, msg := a.legacyCRDHasInstances(ctx, target, crdName); hasInstances {
				crdStep.Completef(result.StepFailed, "%s", msg)

				failed++

				continue
			}
		}

		err = target.Client.APIExtensions().ApiextensionsV1().
			CustomResourceDefinitions().
			Delete(ctx, crdName, metav1.DeleteOptions{})
		if err != nil && !apierrors.IsNotFound(err) {
			crdStep.Completef(result.StepFailed, "Failed to delete CRD: %v", err)

			failed++

			continue
		}

		crdStep.Completef(result.StepCompleted, "Deleted CRD %s", crdName)
	}

	if failed > 0 {
		step.Completef(result.StepFailed, "One or more legacy CRD deletions failed")

		return
	}

	step.Completef(result.StepCompleted, "Legacy CRD deletion complete")
}

func (a *RHBOKMigrationAction) legacyCRDHasInstances(
	ctx context.Context,
	target action.Target,
	crdName string,
) (bool, string) {
	var gvr resources.ResourceType

	switch crdName {
	case legacyCohortCRD:
		gvr = resources.ResourceType{
			Group: legacyCohortGroup, Version: legacyCohortVersion,
			Kind: legacyCohortKind, Resource: "cohorts",
		}
	case legacyTopologyCRD:
		gvr = resources.ResourceType{
			Group: legacyCohortGroup, Version: legacyCohortVersion,
			Kind: legacyTopologyKind, Resource: "topologies",
		}
	default:
		return false, ""
	}

	items, err := target.Client.ListResources(ctx, gvr.GVR())
	if err != nil {
		if apierrors.IsNotFound(err) || client.IsResourceTypeNotFound(err) {
			return false, ""
		}

		return true, fmt.Sprintf("Could not list %s instances: %v", crdName, err)
	}

	if len(items) > 0 {
		return true, fmt.Sprintf(
			"CRD %s has %d instance(s); convert or delete them first, or use --force-delete-legacy-crds",
			crdName, len(items))
	}

	return false, ""
}
