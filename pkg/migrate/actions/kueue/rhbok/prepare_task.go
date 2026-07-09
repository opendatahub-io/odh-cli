package rhbok

import (
	"context"
	"errors"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/opendatahub-io/odh-cli/pkg/backup"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/action"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/action/result"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
)

type prepareTask struct {
	action *RHBOKMigrationAction
}

func (t *prepareTask) Validate(
	ctx context.Context,
	target action.Target,
) (*result.ActionResult, error) {
	t.action.verifyRBAC(ctx, target, preparePermissions())
	t.action.checkCertManager(ctx, target)
	t.action.checkCurrentKueueState(ctx, target)
	t.action.verifyKueueResources(ctx, target)
	t.action.reportLabelingPlan(ctx, target)

	rootRecorder, ok := target.Recorder.(action.RootRecorder)
	if !ok {
		return nil, errors.New("recorder is not a RootRecorder")
	}

	return rootRecorder.Build(), nil
}

func (t *prepareTask) Execute(
	ctx context.Context,
	target action.Target,
) (*result.ActionResult, error) {
	validationResult, err := t.Validate(ctx, target)
	if err != nil {
		return nil, err
	}

	if hasFailedStep(validationResult.Status.Steps) {
		return validationResult, errPreflightFailed
	}

	kueueManaged := t.action.checkKueueManaged(ctx, target)

	if kueueManaged {
		t.backupKueueResources(ctx, target)
	} else {
		step := target.Recorder.Child(
			"backup-skipped",
			"Backup skipped (Kueue not managed)",
		)
		step.Completef(result.StepSkipped, "Kueue is not managed by DataScienceCluster")
	}

	rootRecorder, ok := target.Recorder.(action.RootRecorder)
	if !ok {
		return nil, errors.New("recorder is not a RootRecorder")
	}

	return rootRecorder.Build(), nil
}

func (t *prepareTask) backupKueueResources(
	ctx context.Context,
	target action.Target,
) {
	step := target.Recorder.Child(
		"backup-kueue-resources",
		"Backup Kueue resources",
	)

	if target.DryRun {
		step.Completef(result.StepSkipped, "Would backup ClusterQueues and ConfigMap to %s", target.OutputDir)

		return
	}

	t.backupClusterQueues(ctx, target, step)
	t.backupConfigMap(ctx, target, step)

	step.Completef(result.StepCompleted, "Backup complete in %s", target.OutputDir)
}

func (t *prepareTask) backupClusterQueues(
	ctx context.Context,
	target action.Target,
	parentStep action.StepRecorder,
) {
	step := parentStep.Child(
		"backup-clusterqueues",
		"Backup ClusterQueues",
	)

	clusterQueues, err := target.Client.ListResources(ctx, resources.ClusterQueue.GVR())
	if err != nil {
		if apierrors.IsNotFound(err) {
			step.Completef(result.StepSkipped, "No ClusterQueue CRD found")

			return
		}

		step.Completef(result.StepFailed, "Failed to list ClusterQueues: %v", err)

		return
	}

	if len(clusterQueues) == 0 {
		step.Completef(result.StepSkipped, "No ClusterQueues found")

		return
	}

	// Write cluster-scoped resources to root directory
	if err := backup.WriteResourcesToDir(target.OutputDir, resources.ClusterQueue.GVR(), clusterQueues); err != nil {
		step.Completef(result.StepFailed, "Failed to write ClusterQueues: %v", err)

		return
	}

	step.Completef(result.StepCompleted, "Backed up %d ClusterQueues to %s", len(clusterQueues), target.OutputDir)
}

func (t *prepareTask) backupConfigMap(
	ctx context.Context,
	target action.Target,
	parentStep action.StepRecorder,
) {
	step := parentStep.Child(
		"backup-configmap",
		"Backup ConfigMap "+configMapName,
	)

	obj, err := target.Client.Dynamic().Resource(resources.ConfigMap.GVR()).
		Namespace(applicationsNamespace).
		Get(ctx, configMapName, metav1.GetOptions{})

	if err != nil {
		if apierrors.IsNotFound(err) {
			step.Completef(result.StepSkipped, "ConfigMap not found")

			return
		}

		step.Completef(result.StepFailed, "Failed to get ConfigMap: %v", err)

		return
	}

	if err := backup.WriteResourcesToDir(target.OutputDir, resources.ConfigMap.GVR(), []*unstructured.Unstructured{obj}); err != nil {
		step.Completef(result.StepFailed, "Failed to write ConfigMap: %v", err)

		return
	}

	step.Completef(result.StepCompleted, "Backed up ConfigMap %s to %s", configMapName, target.OutputDir)
}
