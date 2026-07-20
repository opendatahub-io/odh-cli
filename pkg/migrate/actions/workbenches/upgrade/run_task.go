package upgrade

import (
	"context"
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/opendatahub-io/odh-cli/pkg/migrate/action"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/action/result"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/actions/workbenches"
)

type runTask struct {
	action *WorkbenchUpgradeAction
}

func (t *runTask) Validate(
	ctx context.Context,
	target action.Target,
) (*result.ActionResult, error) {
	step := target.Recorder.Child(
		"validate-prerequisites",
		"Validate migration prerequisites",
	)

	notebooks, err := t.action.listNotebooks(ctx, target)
	if err != nil {
		step.Completef(result.StepFailed, "Failed to list Notebooks: %v", err)

		return buildResult(target.Recorder)
	}

	if len(notebooks) == 0 {
		step.Completef(result.StepCompleted, "No Notebook instances found, nothing to migrate")

		return buildResult(target.Recorder)
	}

	stopped := t.action.handleNonStoppedNotebooks(target, notebooks, step)

	if len(stopped) == 0 {
		step.Completef(result.StepSkipped,
			"No eligible Notebook(s) to validate (all non-stopped)")

		return buildResult(target.Recorder)
	}

	mismatchCount := 0
	checkErrCount := 0

	for _, nb := range stopped {
		hasMismatch, checkErr := hasContainerNameMismatch(nb)
		if checkErr != nil {
			step.Recordf("check-error",
				fmt.Sprintf("Failed to inspect Notebook %s/%s: %v", nb.GetNamespace(), nb.GetName(), checkErr),
				result.StepFailed)
			checkErrCount++

			continue
		}

		if hasMismatch {
			mismatchCount++
		}
	}

	if checkErrCount > 0 {
		step.Completef(result.StepFailed,
			"Failed to inspect %d Notebook(s); resolve validation errors first", checkErrCount)
	} else if mismatchCount > 0 {
		step.Completef(result.StepCompleted,
			"Found %d Notebook(s) with container name mismatches requiring update", mismatchCount)
	} else {
		step.Completef(result.StepCompleted,
			"All %d stopped Notebook(s) have correct container names", len(stopped))
	}

	t.verifyAuthModel(target, stopped)

	return buildResult(target.Recorder)
}

func (t *runTask) Execute(
	ctx context.Context,
	target action.Target,
) (*result.ActionResult, error) {
	listStep := target.Recorder.Child(
		"list-notebooks",
		"List Notebook resources",
	)

	notebooks, err := t.action.listNotebooks(ctx, target)
	if err != nil {
		listStep.Completef(result.StepFailed, "Failed to list Notebooks: %v", err)

		return buildResult(target.Recorder)
	}

	if len(notebooks) == 0 {
		listStep.Completef(result.StepCompleted, "No Notebook instances found, nothing to migrate")

		return buildResult(target.Recorder)
	}

	listStep.Completef(result.StepCompleted, "Found %d Notebook(s)", len(notebooks))

	stopped := t.action.handleNonStoppedNotebooks(target, notebooks, target.Recorder)

	if len(stopped) == 0 {
		target.Recorder.Recordf("no-eligible-notebooks",
			"No eligible Notebook(s) to migrate (all non-stopped)", result.StepSkipped)

		return buildResult(target.Recorder)
	}

	t.fixContainerNames(ctx, target, stopped)

	t.verifyAuthModel(target, stopped)

	return buildResult(target.Recorder)
}

func (t *runTask) fixContainerNames(
	ctx context.Context,
	target action.Target,
	notebooks []*unstructured.Unstructured,
) {
	step := target.Recorder.Child(
		"fix-container-names",
		"Fix container name mismatches",
	)

	var toFix []*unstructured.Unstructured

	checkErrCount := 0

	for _, nb := range notebooks {
		hasMismatch, err := hasContainerNameMismatch(nb)
		if err != nil {
			step.Recordf("check-error",
				fmt.Sprintf("Error checking %s/%s: %v", nb.GetNamespace(), nb.GetName(), err),
				result.StepFailed)
			checkErrCount++

			continue
		}

		if hasMismatch {
			toFix = append(toFix, nb)
		}
	}

	if len(toFix) == 0 && checkErrCount > 0 {
		step.Completef(result.StepFailed,
			"Failed to inspect %d Notebook(s), no fixable mismatches found", checkErrCount)

		return
	}

	if len(toFix) == 0 {
		step.Completef(result.StepCompleted,
			"All %d Notebook(s) have correct container names, no changes needed", len(notebooks))

		return
	}

	if target.DryRun {
		for _, nb := range toFix {
			containers, _ := extractWorkloadContainers(nb)
			if len(containers) > 0 {
				step.Recordf("would-fix",
					fmt.Sprintf("Would rename container %q to %q in %s/%s",
						containers[0].Name, nb.GetName(), nb.GetNamespace(), nb.GetName()),
					result.StepSkipped)
			}
		}

		if checkErrCount > 0 {
			step.Completef(result.StepFailed,
				"Would fix %d Notebook(s), but failed to inspect %d",
				len(toFix), checkErrCount)
		} else {
			step.Completef(result.StepSkipped,
				"Would fix container names in %d Notebook(s)", len(toFix))
		}

		return
	}

	if !t.action.promptBeforeModification(target, len(toFix)) {
		step.Completef(result.StepSkipped, "User cancelled modification")

		return
	}

	successCount, failCount := t.applyContainerFixes(ctx, target, toFix, step)

	if failCount > 0 || checkErrCount > 0 {
		step.Completef(result.StepFailed,
			"Fixed %d/%d Notebook(s), %d update failure(s), %d inspection failure(s)",
			successCount, len(toFix), failCount, checkErrCount)
	} else {
		step.Completef(result.StepCompleted,
			"Fixed container names in %d/%d Notebook(s)", successCount, len(toFix))
	}
}

func (t *runTask) verifyAuthModel(
	target action.Target,
	notebooks []*unstructured.Unstructured,
) {
	step := target.Recorder.Child(
		"verify-auth-model",
		"Verify workbench auth model migration",
	)

	needsPatchCount := 0

	for _, nb := range notebooks {
		passed, failures := workbenches.CheckMigrationState(nb)
		if !passed {
			step.Recordf(
				fmt.Sprintf("auth-model-%s-%s", nb.GetNamespace(), nb.GetName()),
				"%s/%s needs auth model patch: %s",
				result.StepFailed,
				nb.GetNamespace(), nb.GetName(), strings.Join(failures, "; "),
			)

			needsPatchCount++
		}
	}

	if needsPatchCount > 0 {
		step.Completef(result.StepFailed,
			"%d stopped Notebook(s) still have legacy OAuth-proxy sidecar — run workbenches.patch-auth-model to complete migration",
			needsPatchCount)
	} else {
		step.Completef(result.StepCompleted,
			"All %d stopped Notebook(s) have correct auth model", len(notebooks))
	}
}

func (t *runTask) applyContainerFixes(
	ctx context.Context,
	target action.Target,
	toFix []*unstructured.Unstructured,
	parentStep action.StepRecorder,
) (int, int) {
	successCount := 0
	failCount := 0

	for _, nb := range toFix {
		nbStep := parentStep.Child(
			fmt.Sprintf("fix-%s-%s", nb.GetNamespace(), nb.GetName()),
			fmt.Sprintf("Fix container name in %s/%s", nb.GetNamespace(), nb.GetName()),
		)

		patched := nb.DeepCopy()

		oldName, err := fixContainerName(patched)
		if err != nil {
			nbStep.Completef(result.StepFailed, "Failed to fix container name: %v", err)
			failCount++

			continue
		}

		if err := t.action.updateNotebook(ctx, target, patched); err != nil {
			nbStep.Completef(result.StepFailed,
				"Failed to update Notebook %s/%s: %v", nb.GetNamespace(), nb.GetName(), err)
			failCount++

			continue
		}

		nbStep.Completef(result.StepCompleted,
			"Renamed container %q to %q", oldName, nb.GetName())
		successCount++
	}

	return successCount, failCount
}
