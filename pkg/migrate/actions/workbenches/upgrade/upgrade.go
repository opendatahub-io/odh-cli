package upgrade

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/pflag"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/opendatahub-io/odh-cli/pkg/backup"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/action"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/action/result"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/confirmation"
	"github.com/opendatahub-io/odh-cli/pkg/util/jq"
)

const (
	actionID          = "workbenches.upgrade-2x-to-3x"
	actionName        = "Upgrade workbenches from 2.x to 3.x"
	actionDescription = "Updates workbench notebook container names for 3.x compatibility"

	annotationKubeflowResourceStopped = "kubeflow-resource-stopped"
	annotationAcceleratorName         = "opendatahub.io/accelerator-name"
	annotationLastSizeSelection       = "notebooks.opendatahub.io/last-size-selection"

	oauthProxyContainerName = "oauth-proxy"
	oauthProxyImageSubstr   = "ose-oauth-proxy-rhel9"
)

// WorkbenchUpgradeAction implements the workbenches.upgrade-2x-to-3x migration action.
// It fixes container name mismatches in Dashboard-managed notebooks so that accelerator
// injection and size selection work correctly after upgrading to RHOAI 3.x.
type WorkbenchUpgradeAction struct {
	ForceNonStopped bool
}

func (a *WorkbenchUpgradeAction) ID() string          { return actionID }
func (a *WorkbenchUpgradeAction) Name() string        { return actionName }
func (a *WorkbenchUpgradeAction) Description() string { return actionDescription }

func (a *WorkbenchUpgradeAction) Group() action.ActionGroup {
	return action.GroupMigration
}

func (a *WorkbenchUpgradeAction) Phase() action.ActionPhase {
	return action.PhasePreUpgrade
}

func buildResult(recorder action.StepRecorder) (*result.ActionResult, error) {
	rootRecorder, ok := recorder.(action.RootRecorder)
	if !ok {
		return nil, errors.New("recorder is not a RootRecorder")
	}

	return rootRecorder.Build(), nil
}

func (a *WorkbenchUpgradeAction) AddFlags(fs *pflag.FlagSet) {
	fs.BoolVar(&a.ForceNonStopped, "force-non-stopped", false,
		"Include non-stopped workbenches in migration (unsafe: may cause data loss)")
}

func (a *WorkbenchUpgradeAction) CanApply(target action.Target) bool {
	if target.CurrentVersion == nil || target.TargetVersion == nil {
		return false
	}

	return target.CurrentVersion.Major == 2 &&
		target.CurrentVersion.Minor >= 16 &&
		target.TargetVersion.Major >= 3
}

func (a *WorkbenchUpgradeAction) Prepare() action.Task {
	return &prepareTask{action: a}
}

func (a *WorkbenchUpgradeAction) Run() action.Task {
	return &runTask{action: a}
}

// listNotebooks lists all Notebook CRs across all namespaces.
func (a *WorkbenchUpgradeAction) listNotebooks(
	ctx context.Context,
	target action.Target,
) ([]*unstructured.Unstructured, error) {
	nbs, err := target.Client.List(ctx, resources.Notebook)
	if err != nil {
		return nil, fmt.Errorf("listing notebooks: %w", err)
	}

	return nbs, nil
}

// isStopped returns true if the notebook has a non-empty kubeflow-resource-stopped annotation.
func isStopped(nb *unstructured.Unstructured) bool {
	annotations := nb.GetAnnotations()
	if annotations == nil {
		return false
	}

	stopped, ok := annotations[annotationKubeflowResourceStopped]

	return ok && stopped != ""
}

// hasDashboardAnnotation returns true if the notebook has Dashboard-managed annotations.
func hasDashboardAnnotation(nb *unstructured.Unstructured) bool {
	annotations := nb.GetAnnotations()
	if annotations == nil {
		return false
	}

	return annotations[annotationAcceleratorName] != "" ||
		annotations[annotationLastSizeSelection] != ""
}

// notebookContainer holds the parsed name and image of a container.
type notebookContainer struct {
	Name  string
	Image string
}

// isInfrastructureContainer returns true if the container is a known sidecar.
func isInfrastructureContainer(name string, image string) bool {
	return name == oauthProxyContainerName && strings.Contains(image, oauthProxyImageSubstr)
}

// extractWorkloadContainers extracts non-infrastructure containers from a notebook.
func extractWorkloadContainers(nb *unstructured.Unstructured) ([]notebookContainer, error) {
	rawContainers, err := jq.Query[[]any](nb, ".spec.template.spec.containers")
	if err != nil {
		return nil, fmt.Errorf("querying containers: %w", err)
	}

	var containers []notebookContainer

	for _, raw := range rawContainers {
		containerMap, ok := raw.(map[string]any)
		if !ok {
			continue
		}

		name, _ := containerMap["name"].(string)
		image, _ := containerMap["image"].(string)

		if isInfrastructureContainer(name, image) {
			continue
		}

		containers = append(containers, notebookContainer{
			Name:  name,
			Image: image,
		})
	}

	return containers, nil
}

// hasContainerNameMismatch returns true if the primary workload container name
// does not match the notebook CR name.
func hasContainerNameMismatch(nb *unstructured.Unstructured) (bool, error) {
	if !hasDashboardAnnotation(nb) {
		return false, nil
	}

	containers, err := extractWorkloadContainers(nb)
	if err != nil {
		return false, err
	}

	if len(containers) != 1 {
		return false, nil
	}

	return containers[0].Name != nb.GetName(), nil
}

// fixContainerName renames the primary workload container to match the notebook CR name.
// Preserves all other container properties (env vars, volume mounts, resources, etc.).
func fixContainerName(nb *unstructured.Unstructured) (string, error) {
	workloads, err := extractWorkloadContainers(nb)
	if err != nil {
		return "", err
	}

	if len(workloads) != 1 {
		return "", fmt.Errorf("expected exactly 1 workload container, found %d", len(workloads))
	}

	rawContainers, err := jq.Query[[]any](nb, ".spec.template.spec.containers")
	if err != nil {
		return "", fmt.Errorf("querying containers: %w", err)
	}

	crName := nb.GetName()
	var oldName string

	for _, raw := range rawContainers {
		containerMap, ok := raw.(map[string]any)
		if !ok {
			continue
		}

		name, _ := containerMap["name"].(string)
		image, _ := containerMap["image"].(string)

		if isInfrastructureContainer(name, image) {
			continue
		}

		oldName = name
		containerMap["name"] = crName

		break
	}

	containersJSON, err := json.Marshal(rawContainers)
	if err != nil {
		return "", fmt.Errorf("marshaling containers: %w", err)
	}

	if err := jq.Transform(nb, ".spec.template.spec.containers = %s", containersJSON); err != nil {
		return "", fmt.Errorf("setting containers: %w", err)
	}

	return oldName, nil
}

// backupNotebooks writes notebook resources to the output directory, grouped by namespace.
// Returns an error if any backup write fails — callers must not proceed with mutations.
func (a *WorkbenchUpgradeAction) backupNotebooks(
	_ context.Context,
	target action.Target,
	notebooks []*unstructured.Unstructured,
	parentStep action.StepRecorder,
) error {
	step := parentStep.Child(
		"backup-notebooks",
		"Backup Notebook resources",
	)

	if target.DryRun {
		step.Completef(result.StepSkipped, "Would backup %d Notebook(s) to %s", len(notebooks), target.OutputDir)

		return nil
	}

	byNamespace := make(map[string][]*unstructured.Unstructured)
	for _, nb := range notebooks {
		ns := nb.GetNamespace()
		byNamespace[ns] = append(byNamespace[ns], nb)
	}

	totalBacked := 0

	for ns, nbs := range byNamespace {
		outputDir := filepath.Join(target.OutputDir, ns)

		if err := backup.WriteResourcesToDir(outputDir, resources.Notebook.GVR(), nbs); err != nil {
			step.Completef(result.StepFailed, "Failed to write Notebooks in namespace %s: %v", ns, err)

			return fmt.Errorf("backup failed for namespace %s: %w", ns, err)
		}

		totalBacked += len(nbs)
	}

	step.Completef(result.StepCompleted, "Backed up %d Notebook(s) across %d namespace(s) to %s", totalBacked, len(byNamespace), target.OutputDir)

	return nil
}

// handleNonStoppedNotebooks checks for non-stopped notebooks and returns only the stopped ones.
// When --force-non-stopped is set, includes all notebooks regardless of stopped state.
func (a *WorkbenchUpgradeAction) handleNonStoppedNotebooks(
	target action.Target,
	notebooks []*unstructured.Unstructured,
	parentStep action.StepRecorder,
) []*unstructured.Unstructured {
	step := parentStep.Child(
		"check-non-stopped",
		"Check for non-stopped workbenches",
	)

	var stopped, nonStopped []*unstructured.Unstructured

	for _, nb := range notebooks {
		if isStopped(nb) {
			stopped = append(stopped, nb)
		} else {
			nonStopped = append(nonStopped, nb)
		}
	}

	if len(nonStopped) == 0 {
		step.Completef(result.StepCompleted, "All %d Notebook(s) are stopped", len(notebooks))

		return stopped
	}

	var names []string
	for _, nb := range nonStopped {
		names = append(names, fmt.Sprintf("%s/%s", nb.GetNamespace(), nb.GetName()))
	}

	step.AddDetail("non_stopped_notebooks", names)

	if a.ForceNonStopped {
		step.Completef(result.StepCompleted,
			"Including %d non-stopped Notebook(s) (--force-non-stopped): %s",
			len(nonStopped), strings.Join(names, ", "))

		return notebooks
	}

	if target.DryRun {
		step.Completef(result.StepSkipped,
			"Found %d non-stopped Notebook(s) (would skip in dry-run): %s",
			len(nonStopped), strings.Join(names, ", "))

		return stopped
	}

	if !target.SkipConfirm {
		target.IO.Fprintln()
		target.IO.Errorf("Found %d non-stopped Notebook(s):", len(nonStopped))

		for _, name := range names {
			target.IO.Errorf("  - %s", name)
		}

		target.IO.Errorf("Non-stopped notebooks will be skipped. Stop them before upgrading.")
		target.IO.Errorf("Use --force-non-stopped to include them (unsafe).")
	}

	step.Completef(result.StepCompleted,
		"Skipping %d non-stopped Notebook(s), proceeding with %d stopped Notebook(s)",
		len(nonStopped), len(stopped))

	return stopped
}

// updateNotebook applies changes to a notebook on the cluster.
func (a *WorkbenchUpgradeAction) updateNotebook(
	ctx context.Context,
	target action.Target,
	nb *unstructured.Unstructured,
) error {
	_, err := target.Client.Dynamic().Resource(resources.Notebook.GVR()).
		Namespace(nb.GetNamespace()).
		Update(ctx, nb, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("updating notebook %s/%s: %w", nb.GetNamespace(), nb.GetName(), err)
	}

	return nil
}

// promptBeforeModification asks the user for confirmation before modifying notebooks.
func (a *WorkbenchUpgradeAction) promptBeforeModification(
	target action.Target,
	count int,
) bool {
	if target.SkipConfirm {
		return true
	}

	target.IO.Fprintln()
	target.IO.Errorf("About to modify %d Notebook(s) to fix container name mismatches", count)

	return confirmation.Prompt(target.IO, "Proceed with workbench modifications?")
}
