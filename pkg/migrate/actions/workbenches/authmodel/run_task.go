package authmodel

import (
	"context"
	"errors"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/opendatahub-io/odh-cli/pkg/migrate/action"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/action/result"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/actions/workbenches/cleanup"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/confirmation"
)

type runTask struct {
	action *PatchAuthModelAction
}

func (t *runTask) Validate(
	ctx context.Context,
	target action.Target,
) (*result.ActionResult, error) {
	if err := t.validateFlags(); err != nil {
		return nil, err
	}

	step := target.Recorder.Child(
		"validate-auth-model-patch",
		"Validate notebooks for auth model patch",
	)

	notebooks, err := t.action.Scope.ListNotebooks(ctx, target)
	if err != nil {
		step.Completef(result.StepFailed, "Failed to list Notebooks: %v", err)

		return action.BuildResult(target)
	}

	if len(notebooks) == 0 {
		step.Completef(result.StepCompleted, "No Notebook instances found")

		return action.BuildResult(target)
	}

	toPatch, alreadyPatched := classifyNotebooks(notebooks)

	if len(toPatch) == 0 {
		step.Completef(result.StepCompleted,
			"All %d Notebook(s) already patched for 3.x auth model", len(alreadyPatched))
	} else {
		step.Completef(result.StepCompleted,
			"Found %d Notebook(s) needing auth model patch, %d already patched",
			len(toPatch), len(alreadyPatched))
	}

	return action.BuildResult(target)
}

func (t *runTask) Execute(
	ctx context.Context,
	target action.Target,
) (*result.ActionResult, error) {
	if err := t.validateFlags(); err != nil {
		return nil, err
	}

	// Step 1: Discover notebooks
	discoverStep := target.Recorder.Child(
		"discover-notebooks",
		"Discover notebooks for auth model patch",
	)

	notebooks, err := t.action.Scope.ListNotebooks(ctx, target)
	if err != nil {
		discoverStep.Completef(result.StepFailed, "Failed to list Notebooks: %v", err)

		return action.BuildResult(target)
	}

	if len(notebooks) == 0 {
		discoverStep.Completef(result.StepCompleted,
			"No Notebook instances found, nothing to patch")

		return action.BuildResult(target)
	}

	// Step 2: Classify
	toPatch, alreadyPatched := classifyNotebooks(notebooks)

	discoverStep.Completef(result.StepCompleted,
		"Found %d Notebook(s) needing patch, %d already patched",
		len(toPatch), len(alreadyPatched))

	if len(toPatch) == 0 {
		return action.BuildResult(target)
	}

	// Step 3: Filter by stopped/running state and auto-stop
	var stoppedByAction []*unstructured.Unstructured

	toPatch, stoppedByAction = t.handleLifecycle(ctx, target, toPatch)

	if len(toPatch) == 0 {
		t.restartNotebooks(ctx, target, stoppedByAction)

		return action.BuildResult(target)
	}

	// Step 4: Dry-run reporting
	if target.DryRun {
		t.reportDryRun(target, toPatch)

		return action.BuildResult(target)
	}

	// Step 5: Confirmation
	if !t.promptBeforePatch(target, len(toPatch)) {
		target.Recorder.Recordf("patch-cancelled",
			"User cancelled auth model patch", result.StepSkipped)

		t.restartNotebooks(ctx, target, stoppedByAction)

		return action.BuildResult(target)
	}

	// Step 6: Patch notebooks
	patched := t.patchNotebooks(ctx, target, toPatch)

	// Step 7: Cleanup (if requested and notebooks were patched)
	if t.action.WithCleanup && len(patched) > 0 {
		t.runCleanup(ctx, target, patched)
	}

	// Step 8: Restart notebooks that were stopped by this action
	t.restartNotebooks(ctx, target, stoppedByAction)

	// Step 9: Check for Kueue terminating pods (when --skip-stop was used)
	if t.action.SkipStop && len(patched) > 0 {
		kueueStep := target.Recorder.Child(
			"check-kueue-terminating",
			"Check for Kueue finalizer conflicts",
		)

		if checkKueueTerminatingPods(ctx, target, patched, kueueStep) {
			kueueStep.Completef(result.StepFailed,
				"Kueue finalizer conflicts detected")
		} else {
			kueueStep.Completef(result.StepCompleted,
				"Kueue terminating pod check complete")
		}
	}

	return action.BuildResult(target)
}

func (t *runTask) validateFlags() error {
	if err := t.action.Scope.Validate(); err != nil {
		return fmt.Errorf("invalid flags: %w", err)
	}

	if t.action.SkipStop && t.action.OnlyStopped {
		return errors.New("--skip-stop and --only-stopped are mutually exclusive")
	}

	return nil
}

func classifyNotebooks(
	notebooks []*unstructured.Unstructured,
) ([]*unstructured.Unstructured, []*unstructured.Unstructured) {
	var toPatch, alreadyPatched []*unstructured.Unstructured

	for _, nb := range notebooks {
		if needsAuthModelPatch(nb) {
			toPatch = append(toPatch, nb)
		} else {
			alreadyPatched = append(alreadyPatched, nb)
		}
	}

	return toPatch, alreadyPatched
}

// handleLifecycle manages stopping/filtering running notebooks before patching.
// Returns the filtered list of notebooks to patch and a list of notebooks
// that were stopped by this action (for later restart).
func (t *runTask) handleLifecycle(
	ctx context.Context,
	target action.Target,
	notebooks []*unstructured.Unstructured,
) ([]*unstructured.Unstructured, []*unstructured.Unstructured) {
	var toPatch, stoppedByAction []*unstructured.Unstructured

	if t.action.OnlyStopped {
		step := target.Recorder.Child(
			"filter-stopped",
			"Filter to stopped workbenches only",
		)

		var stopped, running []*unstructured.Unstructured

		for _, nb := range notebooks {
			if isStopped(nb) {
				stopped = append(stopped, nb)
			} else {
				running = append(running, nb)
			}
		}

		if len(running) > 0 {
			step.Completef(result.StepCompleted,
				"Skipping %d running Notebook(s), proceeding with %d stopped",
				len(running), len(stopped))
		} else {
			step.Completef(result.StepCompleted,
				"All %d Notebook(s) are stopped", len(stopped))
		}

		return stopped, nil
	}

	if t.action.SkipStop {
		return notebooks, nil
	}

	// Default: auto-stop running notebooks
	step := target.Recorder.Child(
		"auto-stop-workbenches",
		"Stop running workbenches before patching",
	)

	for _, nb := range notebooks {
		if isStopped(nb) {
			toPatch = append(toPatch, nb)

			continue
		}

		if target.DryRun {
			step.Recordf(
				fmt.Sprintf("would-stop-%s-%s", nb.GetNamespace(), nb.GetName()),
				"Would stop %s/%s before patching",
				result.StepSkipped,
				nb.GetNamespace(), nb.GetName(),
			)

			toPatch = append(toPatch, nb)

			continue
		}

		if err := stopWorkbench(ctx, target, nb, step); err != nil {
			step.Recordf(
				fmt.Sprintf("stop-failed-%s-%s", nb.GetNamespace(), nb.GetName()),
				"Failed to stop %s/%s, skipping: %v",
				result.StepFailed,
				nb.GetNamespace(), nb.GetName(), err,
			)

			continue
		}

		// Re-fetch after stop to get the current resourceVersion;
		// stopWorkbench's MergePatch advanced it on the server.
		refreshed, err := target.Client.Dynamic().Resource(resources.Notebook.GVR()).
			Namespace(nb.GetNamespace()).
			Get(ctx, nb.GetName(), metav1.GetOptions{})
		if err != nil {
			step.Recordf(
				fmt.Sprintf("refetch-failed-%s-%s", nb.GetNamespace(), nb.GetName()),
				"Failed to re-fetch %s/%s after stop, skipping patch: %v",
				result.StepFailed,
				nb.GetNamespace(), nb.GetName(), err,
			)

			stoppedByAction = append(stoppedByAction, nb)

			continue
		}

		stoppedByAction = append(stoppedByAction, refreshed)
		toPatch = append(toPatch, refreshed)
	}

	if len(stoppedByAction) > 0 {
		step.Completef(result.StepCompleted,
			"Stopped %d running Notebook(s)", len(stoppedByAction))
	} else {
		step.Completef(result.StepCompleted,
			"No running Notebooks required stopping")
	}

	return toPatch, stoppedByAction
}

func (t *runTask) reportDryRun(
	target action.Target,
	toPatch []*unstructured.Unstructured,
) {
	step := target.Recorder.Child(
		"dry-run-report",
		"Dry-run: planned operations",
	)

	for _, nb := range toPatch {
		step.Recordf(
			fmt.Sprintf("would-patch-%s-%s", nb.GetNamespace(), nb.GetName()),
			"Would patch %s/%s: set inject-auth, remove oauth-proxy/volumes/finalizer/annotations/tornado_settings, delete StatefulSet",
			result.StepSkipped,
			nb.GetNamespace(), nb.GetName(),
		)
	}

	step.Completef(result.StepSkipped,
		"Would patch %d Notebook(s)", len(toPatch))
}

func (t *runTask) patchNotebooks(
	ctx context.Context,
	target action.Target,
	toPatch []*unstructured.Unstructured,
) []*unstructured.Unstructured {
	step := target.Recorder.Child(
		"patch-auth-model",
		"Patch notebooks for kube-rbac-proxy auth model",
	)

	var patched []*unstructured.Unstructured

	successCount := 0
	failCount := 0

	for _, nb := range toPatch {
		name := nb.GetName()
		namespace := nb.GetNamespace()

		nbStep := step.Child(
			fmt.Sprintf("patch-%s-%s", namespace, name),
			fmt.Sprintf("Patch %s/%s", namespace, name),
		)

		modified := nb.DeepCopy()

		if err := applyAllPatches(modified); err != nil {
			nbStep.Completef(result.StepFailed,
				"Failed to apply patches to %s/%s: %v", namespace, name, err)
			failCount++

			continue
		}

		_, err := target.Client.Dynamic().Resource(resources.Notebook.GVR()).
			Namespace(namespace).
			Update(ctx, modified, metav1.UpdateOptions{})
		if err != nil {
			nbStep.Completef(result.StepFailed,
				"Failed to update %s/%s: %v", namespace, name, err)
			failCount++

			continue
		}

		nbStep.Recordf("patch-applied",
			"Auth model patch applied to %s/%s",
			result.StepCompleted, namespace, name)

		stsDeleted := deleteStatefulSet(ctx, target, name, namespace, nbStep)

		if !stsDeleted {
			nbStep.Completef(result.StepFailed,
				"Patched %s/%s but StatefulSet deletion failed, may reconcile with old spec until controller catches up",
				namespace, name)
			failCount++

			continue
		}

		nbStep.Completef(result.StepCompleted,
			"Patched %s/%s", namespace, name)

		patched = append(patched, modified)
		successCount++
	}

	if failCount > 0 {
		step.Completef(result.StepFailed,
			"Patched %d/%d Notebook(s), %d failure(s)",
			successCount, len(toPatch), failCount)
	} else {
		step.Completef(result.StepCompleted,
			"Patched %d Notebook(s)", successCount)
	}

	return patched
}

func (t *runTask) runCleanup(
	ctx context.Context,
	target action.Target,
	patched []*unstructured.Unstructured,
) {
	if !target.DryRun && !t.promptBeforeCleanup(target, len(patched)) {
		target.Recorder.Recordf("cleanup-cancelled",
			"User cancelled OAuth cleanup", result.StepSkipped)

		return
	}

	cleanupStep := target.Recorder.Child(
		"cleanup-oauth-resources",
		"Clean up legacy OAuth resources",
	)

	successCount := 0
	failCount := 0

	for _, nb := range patched {
		name := nb.GetName()
		namespace := nb.GetNamespace()

		// Re-fetch the notebook to get the reconciled state
		refreshed, err := target.Client.Dynamic().Resource(resources.Notebook.GVR()).
			Namespace(namespace).
			Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			cleanupStep.Recordf(
				fmt.Sprintf("refetch-failed-%s-%s", namespace, name),
				"Failed to re-fetch %s/%s for cleanup: %v",
				result.StepFailed, namespace, name, err)
			failCount++

			continue
		}

		if cleanup.CleanupNotebookAfterPatch(ctx, target, refreshed, cleanupStep) == cleanup.CleanupResultCleaned {
			successCount++
		} else {
			failCount++
		}
	}

	if failCount > 0 {
		cleanupStep.Completef(result.StepFailed,
			"Cleaned %d/%d Notebook(s), %d failure(s)",
			successCount, len(patched), failCount)
	} else if target.DryRun {
		cleanupStep.Completef(result.StepSkipped,
			"Would clean up OAuth resources for %d Notebook(s)", len(patched))
	} else {
		cleanupStep.Completef(result.StepCompleted,
			"Cleaned up OAuth resources for %d Notebook(s)", successCount)
	}
}

func (t *runTask) restartNotebooks(
	ctx context.Context,
	target action.Target,
	stoppedByAction []*unstructured.Unstructured,
) {
	if len(stoppedByAction) == 0 {
		return
	}

	step := target.Recorder.Child(
		"restart-workbenches",
		"Restart workbenches stopped by this action",
	)

	for _, nb := range stoppedByAction {
		restartWorkbench(ctx, target, nb, step)
	}

	step.Completef(result.StepCompleted,
		"Restart complete for %d Notebook(s)", len(stoppedByAction))
}

func (t *runTask) promptBeforePatch(
	target action.Target,
	count int,
) bool {
	if target.SkipConfirm {
		return true
	}

	target.IO.Fprintln()
	target.IO.Errorf("About to patch %d Notebook(s) for kube-rbac-proxy auth model", count)

	return confirmation.Prompt(target.IO, "Proceed with auth model patch?")
}

func (t *runTask) promptBeforeCleanup(
	target action.Target,
	count int,
) bool {
	if target.SkipConfirm {
		return true
	}

	target.IO.Fprintln()
	target.IO.Errorf("About to delete legacy OAuth resources for %d Notebook(s)", count)

	return confirmation.Prompt(target.IO, "Proceed with OAuth resource cleanup?")
}
