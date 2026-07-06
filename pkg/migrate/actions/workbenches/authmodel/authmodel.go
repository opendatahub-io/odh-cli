package authmodel

import (
	"github.com/spf13/pflag"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/opendatahub-io/odh-cli/pkg/migrate/action"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/actions/workbenches"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/actions/workbenches/cleanup"
)

const (
	actionID          = "workbenches.patch-auth-model"
	actionName        = "Patch workbench auth model from OAuth-proxy to kube-rbac-proxy"
	actionDescription = "Migrates Notebook CRs from oauth-proxy (2.x) to kube-rbac-proxy (3.x) auth " +
		"model by removing OAuth sidecar, annotations, volumes, finalizers, and tornado_settings"

	annotationKubeflowResourceStopped = "kubeflow-resource-stopped"

	minCurrentMajorVersion = 2
	minCurrentMinorVersion = 16
	minTargetMajorVersion  = 3
)

// PatchAuthModelAction implements the workbenches.patch-auth-model migration action.
// It patches Notebook CRs to switch from OAuth-proxy (2.x) to kube-rbac-proxy (3.x).
type PatchAuthModelAction struct {
	Scope       *workbenches.SharedScopeOptions
	SkipStop    bool
	OnlyStopped bool
	WithCleanup bool

	CleanupAction *cleanup.CleanupOAuthAction
}

func (a *PatchAuthModelAction) ID() string          { return actionID }
func (a *PatchAuthModelAction) Name() string        { return actionName }
func (a *PatchAuthModelAction) Description() string { return actionDescription }

func (a *PatchAuthModelAction) Group() action.ActionGroup {
	return action.GroupMigration
}

func (a *PatchAuthModelAction) Phase() action.ActionPhase {
	return action.PhasePreUpgrade
}

func (a *PatchAuthModelAction) AddFlags(fs *pflag.FlagSet) {
	workbenches.AddScopeFlags(a.Scope, fs)

	fs.BoolVar(&a.SkipStop, "skip-stop", false,
		"Skip auto stop/restart of running workbenches before patching")
	fs.BoolVar(&a.OnlyStopped, "only-stopped", false,
		"Only patch stopped workbenches, skip running ones")
	fs.BoolVar(&a.WithCleanup, "with-cleanup", false,
		"Delete legacy OAuth resources (Route, Service, Secrets, OAuthClient) after patching")
}

func (a *PatchAuthModelAction) CanApply(target action.Target) bool {
	if target.CurrentVersion == nil || target.TargetVersion == nil {
		return false
	}

	return target.CurrentVersion.Major == minCurrentMajorVersion &&
		target.CurrentVersion.Minor >= minCurrentMinorVersion &&
		target.TargetVersion.Major >= minTargetMajorVersion
}

func (a *PatchAuthModelAction) Prepare() action.Task {
	return &prepareTask{action: a}
}

func (a *PatchAuthModelAction) Run() action.Task {
	return &runTask{action: a}
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
