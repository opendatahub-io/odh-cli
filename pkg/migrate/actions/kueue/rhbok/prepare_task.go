package rhbok

import (
	"context"
	"errors"
	"path/filepath"

	"github.com/opendatahub-io/odh-cli/pkg/backup"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/action"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/action/result"
	"github.com/opendatahub-io/odh-cli/pkg/resources"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type prepareTask struct {
	action *RHBOKMigrationAction
}

func (t *prepareTask) Validate(
	ctx context.Context,
	target action.Target,
) (*result.ActionResult, error) {
	t.action.checkCurrentKueueState(ctx, target)
	t.action.verifyKueueResources(ctx, target)

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
	kueueManaged := t.action.checkKueueManaged(ctx, target)

	if kueueManaged {
		t.backupKueueResources(ctx, target)
	} else {
		step := target.Recorder.Child(
			"backup-skipped",
			"Backup skipped (Kueue not managed)",
		)
		step.Complete(result.StepSkipped, "Kueue is not managed by DataScienceCluster")
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
		step.Complete(result.StepSkipped, "Would backup ClusterQueues and ConfigMap to %s", target.OutputDir)

		return
	}

	t.backupClusterQueues(ctx, target, step)
	t.backupConfigMap(ctx, target, step)

	step.Complete(result.StepCompleted, "Backup complete in %s", target.OutputDir)
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
			step.Complete(result.StepSkipped, "No ClusterQueue CRD found")

			return
		}

		step.Complete(result.StepFailed, "Failed to list ClusterQueues: %v", err)

		return
	}

	if len(clusterQueues) == 0 {
		step.Complete(result.StepSkipped, "No ClusterQueues found")

		return
	}

	// Write cluster-scoped resources to root directory
	if err := backup.WriteResourcesToDir(target.OutputDir, resources.ClusterQueue.GVR(), clusterQueues); err != nil {
		step.Complete(result.StepFailed, "Failed to write ClusterQueues: %v", err)

		return
	}

	step.Complete(result.StepCompleted, "Backed up %d ClusterQueues to %s", len(clusterQueues), target.OutputDir)
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
			step.Complete(result.StepSkipped, "ConfigMap not found")

			return
		}

		step.Complete(result.StepFailed, "Failed to get ConfigMap: %v", err)

		return
	}

	// Write namespaced resources to namespace directory
	outputDir := filepath.Join(target.OutputDir, applicationsNamespace)

	if err := backup.WriteResourcesToDir(outputDir, resources.ConfigMap.GVR(), []*unstructured.Unstructured{obj}); err != nil {
		step.Complete(result.StepFailed, "Failed to write ConfigMap: %v", err)

		return
	}

	step.Complete(result.StepCompleted, "Backed up ConfigMap %s to %s", configMapName, outputDir)
}
