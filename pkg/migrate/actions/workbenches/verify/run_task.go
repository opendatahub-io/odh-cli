package verify

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
	action *VerifyMigrationAction
}

func (t *runTask) Validate(
	ctx context.Context,
	target action.Target,
) (*result.ActionResult, error) {
	return t.Execute(ctx, target)
}

func (t *runTask) Execute(
	ctx context.Context,
	target action.Target,
) (*result.ActionResult, error) {
	if err := t.action.Scope.Validate(); err != nil {
		return nil, fmt.Errorf("invalid flags: %w", err)
	}

	if err := t.action.validatePhase(); err != nil {
		return nil, err
	}

	recorder := action.NewVerboseRootRecorder(target.IO)

	discoverStep := recorder.Child(
		"discover-notebooks",
		"Discover notebooks for migration verification",
	)

	notebooks, err := t.action.Scope.ListNotebooks(ctx, target)
	if err != nil {
		discoverStep.Completef(result.StepFailed, "Failed to list Notebooks: %v", err)

		return recorder.Build(), nil
	}

	if len(notebooks) == 0 {
		discoverStep.Completef(result.StepCompleted,
			"No Notebook instances found")

		return recorder.Build(), nil
	}

	discoverStep.Completef(result.StepCompleted,
		"Found %d Notebook(s) to verify", len(notebooks))

	summary := newSummary()

	for _, nb := range notebooks {
		t.verifyNotebook(ctx, target, nb, recorder, summary)
	}

	t.buildSummaryStep(recorder, summary, len(notebooks))

	return recorder.Build(), nil
}

func (t *runTask) verifyNotebook(
	ctx context.Context,
	target action.Target,
	nb *unstructured.Unstructured,
	recorder action.RootRecorder,
	summary *verifySummary,
) {
	name := nb.GetName()
	namespace := nb.GetNamespace()

	step := recorder.Child(
		fmt.Sprintf("verify-%s-%s", namespace, name),
		fmt.Sprintf("Verify %s/%s", namespace, name),
	)

	status, detail := ClassifyNotebook(nb)
	summary.countStatus(status)

	runState, runDetail := GetRunningState(ctx, target, nb)
	summary.countRunState(runState)

	statusMsg := string(status)
	if detail != "" {
		statusMsg += " (" + detail + ")"
	}

	stateMsg := string(runState)
	if runDetail != "" {
		stateMsg += " (" + runDetail + ")"
	}

	classificationOK := status == StatusMigrated || status == StatusLegacy
	classificationStep := result.StepCompleted

	if !classificationOK {
		classificationStep = result.StepFailed
		summary.classificationFail++
	}

	step.Recordf("classification",
		"Status: %s | State: %s", classificationStep,
		statusMsg, stateMsg)

	allPassed := classificationOK

	if t.action.includeMigration() {
		passed, failures := workbenches.CheckMigrationState(nb)
		if passed {
			step.Recordf("migration-checks",
				"Migration checks: all passed", result.StepCompleted)
		} else {
			step.Recordf("migration-checks",
				"Migration checks failed: %s", result.StepFailed,
				strings.Join(failures, "; "))
			allPassed = false
		}

		if passed {
			summary.migrationPass++
		} else {
			summary.migrationFail++
		}
	}

	if t.action.includeCleanup() {
		passed, failures := CheckCleanupState(ctx, target, nb)
		if passed {
			step.Recordf("cleanup-checks",
				"Cleanup checks: all passed", result.StepCompleted)
		} else {
			step.Recordf("cleanup-checks",
				"Cleanup checks failed: %s", result.StepFailed,
				strings.Join(failures, "; "))
			allPassed = false
		}

		if passed {
			summary.cleanupPass++
		} else {
			summary.cleanupFail++
		}
	}

	if allPassed {
		step.Completef(result.StepCompleted,
			"All checks passed for %s/%s [%s]", namespace, name, statusMsg)
	} else {
		step.Completef(result.StepFailed,
			"Some checks failed for %s/%s [%s]", namespace, name, statusMsg)
	}
}

func (t *runTask) buildSummaryStep(
	recorder action.RootRecorder,
	summary *verifySummary,
	total int,
) {
	step := recorder.Child("summary", "Verification summary")

	step.AddDetail("total", total)
	step.AddDetail("byStatus", map[string]int{
		"legacy":       summary.legacy,
		"migrated":     summary.migrated,
		"unreconciled": summary.unreconciled,
		"invalid":      summary.invalid,
		"unknown":      summary.unknown,
	})
	step.AddDetail("byRunState", map[string]int{
		"running":  summary.running,
		"stopped":  summary.stopped,
		"starting": summary.starting,
		"other":    summary.otherState,
	})

	if t.action.includeMigration() {
		step.AddDetail("migrationPass", summary.migrationPass)
		step.AddDetail("migrationFail", summary.migrationFail)
	}

	if t.action.includeCleanup() {
		step.AddDetail("cleanupPass", summary.cleanupPass)
		step.AddDetail("cleanupFail", summary.cleanupFail)
	}

	if summary.classificationFail > 0 {
		step.AddDetail("classificationFail", summary.classificationFail)
	}

	totalFailed := summary.classificationFail + summary.migrationFail + summary.cleanupFail
	if totalFailed > 0 {
		step.Completef(result.StepFailed,
			"Verified %d Notebook(s): %d with failures", total, totalFailed)
	} else {
		step.Completef(result.StepCompleted,
			"Verified %d Notebook(s): all checks passed", total)
	}
}

type verifySummary struct {
	legacy       int
	migrated     int
	unreconciled int
	invalid      int
	unknown      int

	running    int
	stopped    int
	starting   int
	otherState int

	classificationFail int

	migrationPass int
	migrationFail int
	cleanupPass   int
	cleanupFail   int
}

func newSummary() *verifySummary {
	return &verifySummary{}
}

func (s *verifySummary) countStatus(status MigrationStatus) {
	switch status {
	case StatusLegacy:
		s.legacy++
	case StatusMigrated:
		s.migrated++
	case StatusUnreconciled:
		s.unreconciled++
	case StatusInvalid:
		s.invalid++
	case StatusUnknown:
		s.unknown++
	}
}

func (s *verifySummary) countRunState(state RunningState) {
	switch state {
	case StateRunning:
		s.running++
	case StateStopped:
		s.stopped++
	case StateStarting:
		s.starting++
	case StateError: // counted alongside unrecognized phases
		fallthrough
	default:
		s.otherState++
	}
}
