package datasciencecluster

import (
	"context"
	"errors"
	"fmt"
	"slices"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/opendatahub-io/odh-cli/pkg/migrate/action"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/action/result"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/confirmation"
)

const msgInvalidStoredVersions = "DataScienceCluster CRD has missing or invalid storedVersions"

const confirmPrompt = "Request DataScienceCluster storage migration and prune obsolete storage history?"

type storageVersionTask struct{}

func (t *storageVersionTask) Validate(ctx context.Context, target action.Target) (*result.ActionResult, error) {
	step := target.Recorder.Child("check-storage-version", "Read DataScienceCluster CRD storage state")
	crd, storage, err := readStorage(ctx, target)
	switch {
	case err != nil:
		step.Completef(result.StepFailed, "%v", err)
	default:
		step.Completef(result.StepCompleted, "Storage version %s; storedVersions %v", storage, crd.Status.StoredVersions)
	}

	return action.BuildResult(target)
}

func (t *storageVersionTask) Execute(ctx context.Context, target action.Target) (*result.ActionResult, error) {
	step := target.Recorder.Child("migrate-storage-version", "Migrate DataScienceCluster storage")
	crd, storage, err := readStorage(ctx, target)
	switch {
	case err != nil:
		step.Completef(result.StepFailed, "%v", err)

		dr, buildErr := action.BuildResult(target)

		return dr, errors.Join(err, buildErr)
	// Nothing older than the storage version is recorded, so nothing to rewrite.
	case slices.Equal(crd.Status.StoredVersions, []string{storage}):
		step.Completef(result.StepCompleted, "No migration needed: storedVersions is [%s]", storage)

		return action.BuildResult(target)
	case target.DryRun:
		step.Completef(
			result.StepSkipped,
			"Would request DataScienceCluster storage migration to %s and prune storedVersions %v",
			storage,
			crd.Status.StoredVersions,
		)

		return action.BuildResult(target)
	case !target.SkipConfirm && !confirmation.Prompt(target.IO, confirmPrompt):
		step.Completef(result.StepSkipped, "User cancelled storage migration")

		return action.BuildResult(target)
	}

	m := &storageVersionMigration{
		target:  target,
		step:    step,
		storage: storage,
	}

	err = m.run(ctx)
	switch {
	case err != nil:
		step.Completef(result.StepFailed, "%v", err)
	default:
		step.Completef(result.StepCompleted, "DataScienceCluster storage migration completed")
	}

	dr, buildErr := action.BuildResult(target)

	return dr, errors.Join(err, buildErr)
}

func readStorage(
	ctx context.Context,
	target action.Target,
) (*apiextensionsv1.CustomResourceDefinition, string, error) {
	definitions := target.Client.APIExtensions().ApiextensionsV1().CustomResourceDefinitions()

	crd, err := definitions.Get(ctx, resources.DataScienceCluster.CRDFQN(), metav1.GetOptions{})

	switch {
	case err != nil:
		return nil, "", fmt.Errorf("reading DataScienceCluster CRD: %w", err)
	case crd == nil:
		return nil, "", errors.New(msgInvalidStoredVersions)
	case len(crd.Status.StoredVersions) == 0:
		return nil, "", errors.New(msgInvalidStoredVersions)
	case slices.Contains(crd.Status.StoredVersions, ""):
		return nil, "", errors.New(msgInvalidStoredVersions)
	}

	stored := crd.Status.StoredVersions

	storage := ""
	for _, v := range crd.Spec.Versions {
		switch {
		case !v.Storage:
			continue
		case storage != "" || !v.Served || v.Name == "":
			return nil, "", errors.New("DataScienceCluster CRD has invalid storage configuration")
		default:
			storage = v.Name
		}
	}

	if storage == "" || !slices.Contains(stored, storage) {
		return nil, "", errors.New("DataScienceCluster CRD storage version is missing from storedVersions")
	}

	return crd, storage, nil
}
