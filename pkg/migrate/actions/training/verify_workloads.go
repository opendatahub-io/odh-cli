package training

import (
	"context"
	"fmt"
	"strings"

	"github.com/opendatahub-io/odh-cli/pkg/constants"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/action"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/action/result"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
	"github.com/opendatahub-io/odh-cli/pkg/util/components"
	"github.com/opendatahub-io/odh-cli/pkg/util/version"
)

const (
	targetMajorVersion                  = 3
	trainingOperatorRemovalMinorVersion = 6
)

// VerifyWorkloadsAction enumerates Kubeflow v1 training workloads as a pre-upgrade check,
// categorizing them by migration readiness for Trainer v2 TrainJob.
type VerifyWorkloadsAction struct{}

func (a *VerifyWorkloadsAction) ID() string          { return verifyWorkloadsID }
func (a *VerifyWorkloadsAction) Name() string        { return verifyWorkloadsName }
func (a *VerifyWorkloadsAction) Description() string { return verifyWorkloadsDescription }

func (a *VerifyWorkloadsAction) Group() action.ActionGroup { return action.GroupValidation }
func (a *VerifyWorkloadsAction) Phase() action.ActionPhase { return action.PhasePostUpgrade }

func (a *VerifyWorkloadsAction) CanApply(target action.Target) bool {
	return version.IsVersion3x(target.TargetVersion)
}

func (a *VerifyWorkloadsAction) Prepare() action.Task { return nil }
func (a *VerifyWorkloadsAction) Run() action.Task     { return &verifyWorkloadsTask{} }

type verifyWorkloadsTask struct{}

func (t *verifyWorkloadsTask) Validate(ctx context.Context, target action.Target) (*result.ActionResult, error) {
	return t.Execute(ctx, target)
}

func (t *verifyWorkloadsTask) Execute(ctx context.Context, target action.Target) (*result.ActionResult, error) {
	recorder := action.NewVerboseRootRecorder(target.IO)
	removalTarget := version.IsVersionAtLeast(target.TargetVersion, targetMajorVersion, trainingOperatorRemovalMinorVersion)

	trainjobReady, err := t.checkTrainJobCRD(ctx, target, recorder)
	if err != nil {
		return recorder.Build(), err
	}

	allEntries, err := t.enumerateV1Workloads(ctx, target, removalTarget, recorder)
	if err != nil {
		return recorder.Build(), err
	}

	if removalTarget {
		if err := t.checkTrainingOperatorDSC(ctx, target, recorder); err != nil {
			return recorder.Build(), err
		}

		t.buildSummary(allEntries, trainjobReady, removalTarget, recorder)

		if err := t.checkTrainingOperatorCR(ctx, target, recorder); err != nil {
			return recorder.Build(), err
		}
	} else {
		t.assessMigrationReadiness(allEntries, recorder)
		t.buildSummary(allEntries, trainjobReady, removalTarget, recorder)
	}

	return recorder.Build(), nil
}

func (t *verifyWorkloadsTask) checkTrainJobCRD(
	ctx context.Context,
	target action.Target,
	recorder action.RootRecorder,
) (bool, error) {
	step := recorder.Child("trainjob-crd", "Check TrainJob v2 CRD readiness")

	_, err := target.Client.List(ctx, resources.TrainJob)
	if err != nil {
		if client.IsResourceTypeNotFound(err) {
			step.AddDetail("trainjobCRDInstalled", false)
			step.Completef(result.StepCompleted, "TrainJob CRD not installed — v2 API not yet available on this cluster")

			return false, nil
		}

		step.AddDetail("trainjobCRDInstalled", false)
		step.Completef(result.StepFailed, "Failed to check TrainJob CRD: %v", err)

		return false, fmt.Errorf("checking TrainJob CRD: %w", err)
	}

	step.AddDetail("trainjobCRDInstalled", true)
	step.Completef(result.StepCompleted, "TrainJob CRD installed — v2 API available")

	return true, nil
}

func (t *verifyWorkloadsTask) checkTrainingOperatorDSC(
	ctx context.Context,
	target action.Target,
	recorder action.RootRecorder,
) error {
	step := recorder.Child("trainingoperator-dsc", "Check TrainingOperator DSC state")

	dsc, err := client.GetDataScienceCluster(ctx, target.Client)
	if err != nil {
		step.Completef(result.StepFailed, "Failed to get DataScienceCluster: %v", err)

		return fmt.Errorf("getting DataScienceCluster: %w", err)
	}

	state, err := components.GetManagementState(dsc, constants.ComponentTrainingOperator)
	if err != nil {
		step.Completef(result.StepFailed, "Failed to check TrainingOperator DSC state: %v", err)

		return fmt.Errorf("checking TrainingOperator DSC state: %w", err)
	}

	step.AddDetail("state", state)

	if state == constants.ManagementStateManaged {
		step.Completef(result.StepSkipped,
			"TrainingOperator is still managed by RHOAI (state: %s) — set it to Removed", state)

		return nil
	}

	step.Completef(result.StepCompleted,
		"No TrainingOperator installed or managed by RHOAI (state: %s)", state)

	return nil
}

func (t *verifyWorkloadsTask) enumerateV1Workloads(ctx context.Context, target action.Target, removalTarget bool, recorder action.RootRecorder) ([]WorkloadEntry, error) {
	entries, failures, err := EnumerateWorkloads(ctx, target.Client)

	failuresByKind := make(map[string]ListFailure, len(failures))
	for _, f := range failures {
		failuresByKind[f.Kind] = f
	}

	entriesByKind := make(map[string][]WorkloadEntry)
	for _, e := range entries {
		entriesByKind[e.Kind] = append(entriesByKind[e.Kind], e)
	}

	for _, rt := range TrainingJobTypes {
		step := recorder.Child(rt.Resource, fmt.Sprintf("List %s workloads", rt.Kind))

		if f, ok := failuresByKind[rt.Kind]; ok {
			step.Completef(result.StepFailed, "Failed to list: %v", f.Err)

			continue
		}

		kindEntries := entriesByKind[rt.Kind]
		if len(kindEntries) == 0 {
			step.Completef(result.StepCompleted, "No %s found", rt.Kind)

			continue
		}

		for _, entry := range kindEntries {
			status := result.StepCompleted
			message := "%s/%s — %s (age: %s)"

			if removalTarget {
				status = result.StepSkipped
				message = "%s/%s — %s (age: %s) is no longer managed by RHOAI, use Trainer v2"
			}

			step.Recordf(
				entry.Name,
				message,
				status,
				entry.Namespace, entry.Name, entry.Status, entry.Age,
			)
		}

		if removalTarget {
			step.Completef(result.StepSkipped,
				"Found %d %s(s) — no longer managed by RHOAI, use Trainer v2",
				len(kindEntries), rt.Kind)
		} else {
			step.Completef(result.StepCompleted, "Found %d %s(s)", len(kindEntries), rt.Kind)
		}
	}

	if err != nil {
		return entries, fmt.Errorf("enumerating v1 workloads: %w", err)
	}

	return entries, nil
}

func (t *verifyWorkloadsTask) assessMigrationReadiness(
	entries []WorkloadEntry,
	recorder action.RootRecorder,
) {
	step := recorder.Child("migration-readiness", "Assess migration readiness")

	var blockers []WorkloadEntry

	for _, e := range entries {
		if isActiveStatus(e.Status) {
			blockers = append(blockers, e)
		}
	}

	step.AddDetail("blockers", len(blockers))
	step.AddDetail("completable", len(entries)-len(blockers))

	if len(entries) == 0 {
		step.Completef(result.StepCompleted, "No v1 training workloads found — nothing to migrate")

		return
	}

	if len(blockers) == 0 {
		step.Completef(result.StepCompleted, "All %d v1 training job(s) are completed — safe to proceed with migration", len(entries))

		return
	}

	for _, b := range blockers {
		step.Recordf(
			b.Name,
			"%s/%s is %s — must complete or stop before migration",
			result.StepFailed,
			b.Namespace, b.Name, b.Status,
		)
	}

	step.Completef(result.StepFailed,
		"[BLOCKER] %d active v1 job(s) must complete or be stopped before migration", len(blockers))
}

func (t *verifyWorkloadsTask) buildSummary(
	entries []WorkloadEntry,
	trainjobReady bool,
	removalTarget bool,
	recorder action.RootRecorder,
) {
	step := recorder.Child("summary", "Migration summary")

	report := BuildReport(entries)

	step.AddDetail("total", report.Summary.Total)
	step.AddDetail("byKind", report.Summary.ByKind)
	step.AddDetail("byStatus", report.Summary.ByStatus)
	step.AddDetail("trainjobCRDInstalled", trainjobReady)

	migrationMap := buildMigrationMap(report.Summary.ByKind)
	step.AddDetail("migrationMap", migrationMap)

	var activeCount int

	for _, e := range entries {
		if isActiveStatus(e.Status) {
			activeCount++
		}
	}

	if removalTarget {
		status := result.StepCompleted
		if activeCount > 0 {
			status = result.StepSkipped
		}
		step.AddDetail("active", activeCount)
		step.AddDetail("terminal", report.Summary.Total-activeCount)
		step.Completef(status,
			"Found %d v1 training workload(s): %d active, %d terminal — none are managed by RHOAI",
			report.Summary.Total, activeCount, report.Summary.Total-activeCount)

		return
	}

	step.AddDetail("blockers", activeCount)
	step.AddDetail("completable", report.Summary.Total-activeCount)
	step.Completef(result.StepCompleted,
		"Found %d v1 training workload(s): %d blocking migration, %d completable",
		report.Summary.Total, activeCount, report.Summary.Total-activeCount)
}

func (t *verifyWorkloadsTask) checkTrainingOperatorCR(
	ctx context.Context,
	target action.Target,
	recorder action.RootRecorder,
) error {
	step := recorder.Child("trainingoperator-cr", "Check TrainingOperator component CR")

	resourceType := resources.GetComponentCR(constants.ComponentTrainingOperator)
	items, err := target.Client.List(ctx, *resourceType)
	if err != nil {
		if client.IsResourceTypeNotFound(err) {
			step.AddDetail("present", false)
			step.Completef(result.StepCompleted, "TrainingOperator component CR not found")

			return nil
		}

		step.Completef(result.StepFailed, "Failed to list TrainingOperator component CRs: %v", err)

		return fmt.Errorf("listing TrainingOperator component CRs: %w", err)
	}

	step.AddDetail("present", len(items) > 0)
	step.AddDetail("count", len(items))

	if len(items) == 0 {
		step.Completef(result.StepCompleted, "TrainingOperator component CR not found")

		return nil
	}

	for _, item := range items {
		step.Recordf(item.GetName(), "%s is no longer managed by RHOAI — use Trainer v2", result.StepSkipped, item.GetName())
	}

	step.Completef(result.StepSkipped,
		"Found %d TrainingOperator component CR(s) — no longer managed by RHOAI, use Trainer v2",
		len(items))

	return nil
}

func isActiveStatus(status string) bool {
	lower := strings.ToLower(status)

	return lower == "running" || lower == "created"
}

func buildMigrationMap(byKind map[string]int) map[string]string {
	migrationMap := make(map[string]string)

	for kind := range byKind {
		if runtime, ok := v1KindToV2Runtime[kind]; ok {
			migrationMap[kind] = fmt.Sprintf("TrainJob with runtime %q", runtime)
		}
	}

	return migrationMap
}
