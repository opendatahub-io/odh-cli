package cleanup

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/pflag"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/opendatahub-io/odh-cli/pkg/migrate/action"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/action/result"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/actions/workbenches"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/confirmation"
)

const (
	actionID          = "workbenches.cleanup-oauth"
	actionName        = "Clean up legacy OAuth resources from workbenches"
	actionDescription = "Removes stale OAuth-proxy resources (Route, Service, Secrets, OAuthClient) " +
		"left behind after migrating workbenches from 2.x to 3.x"

	minTargetMajorVersion = 3
)

// CleanupOAuthAction implements the workbenches.cleanup-oauth migration action.
// It removes legacy OAuth-proxy resources (Route, Service, Secrets, OAuthClient)
// for notebooks that have been migrated to kube-rbac-proxy.
type CleanupOAuthAction struct {
	Scope *workbenches.SharedScopeOptions
}

func (a *CleanupOAuthAction) ID() string          { return actionID }
func (a *CleanupOAuthAction) Name() string        { return actionName }
func (a *CleanupOAuthAction) Description() string { return actionDescription }

func (a *CleanupOAuthAction) Group() action.ActionGroup {
	return action.GroupMigration
}

func (a *CleanupOAuthAction) Phase() action.ActionPhase {
	return action.PhasePostUpgrade
}

func (a *CleanupOAuthAction) AddFlags(fs *pflag.FlagSet) {
	workbenches.AddScopeFlags(a.Scope, fs)
}

func (a *CleanupOAuthAction) CanApply(target action.Target) bool {
	if target.TargetVersion == nil {
		return false
	}

	return target.TargetVersion.Major >= minTargetMajorVersion
}

func (a *CleanupOAuthAction) Prepare() action.Task {
	return nil
}

func (a *CleanupOAuthAction) Run() action.Task {
	return &runTask{action: a}
}

// CheckMigrationState delegates to the shared workbenches.CheckMigrationState.
func CheckMigrationState(nb *unstructured.Unstructured) (bool, []string) {
	return workbenches.CheckMigrationState(nb)
}

// DeleteResourceIfPresent performs an idempotent deletion: if the resource
// exists it is deleted; if it is already absent, no error is raised.
// For cluster-scoped resources, pass an empty namespace.
// Returns true if the resource was deleted or already absent; false on error.
func DeleteResourceIfPresent(
	ctx context.Context,
	target action.Target,
	gvr schema.GroupVersionResource,
	name string,
	namespace string,
	step action.StepRecorder,
) bool {
	label := resourceLabel(gvr, name, namespace)
	stepID := fmt.Sprintf("delete-%s-%s", gvr.Resource, name)

	if target.DryRun {
		step.Recordf(stepID, "Would delete %s", result.StepSkipped, label)

		return true
	}

	ri := target.Client.Dynamic().Resource(gvr)

	var err error
	if namespace != "" {
		err = ri.Namespace(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	} else {
		err = ri.Delete(ctx, name, metav1.DeleteOptions{})
	}

	if apierrors.IsNotFound(err) {
		step.Recordf(stepID, "Already absent: %s", result.StepCompleted, label)

		return true
	}

	if err != nil {
		step.Recordf(stepID, "Failed to delete %s: %v", result.StepFailed, label, err)

		return false
	}

	step.Recordf(stepID, "Deleted %s", result.StepCompleted, label)

	return true
}

func resourceLabel(gvr schema.GroupVersionResource, name, namespace string) string {
	if namespace != "" {
		return fmt.Sprintf("%s/%s in namespace %q", gvr.Resource, name, namespace)
	}

	return fmt.Sprintf("%s/%s (cluster-scoped)", gvr.Resource, name)
}

// CleanupResult indicates the outcome of a single notebook cleanup.
type CleanupResult int

const (
	CleanupResultCleaned CleanupResult = iota
	CleanupResultSkipped
	CleanupResultFailed
)

// CleanupNotebook deletes the legacy OAuth resources for a single notebook.
// It runs the pre-check and prompts for confirmation if the check fails.
func CleanupNotebook(
	ctx context.Context,
	target action.Target,
	nb *unstructured.Unstructured,
	parentStep action.StepRecorder,
) CleanupResult {
	name := nb.GetName()
	namespace := nb.GetNamespace()

	step := parentStep.Child(
		fmt.Sprintf("cleanup-%s-%s", namespace, name),
		fmt.Sprintf("Clean up OAuth resources for %s/%s", namespace, name),
	)

	passed, failures := CheckMigrationState(nb)
	if !passed {
		step.Recordf("precheck",
			"Pre-check failed: %s",
			result.StepFailed,
			strings.Join(failures, "; "))

		if !target.SkipConfirm {
			target.IO.Fprintln()
			target.IO.Errorf("Pre-checks failed for %s/%s:", namespace, name)

			for _, f := range failures {
				target.IO.Errorf("  - %s", f)
			}

			if !confirmation.Prompt(target.IO,
				fmt.Sprintf("Continue cleanup for %s/%s despite failed pre-checks?", namespace, name)) {
				step.Completef(result.StepSkipped,
					"Skipped cleanup for %s/%s (pre-check failed)", namespace, name)

				return CleanupResultSkipped
			}
		}

		step.Recordf("precheck-override",
			"Continuing cleanup despite failed pre-checks", result.StepCompleted)
	}

	hasFailed := !DeleteResourceIfPresent(ctx, target,
		resources.Route.GVR(), name, namespace, step)

	if !DeleteResourceIfPresent(ctx, target,
		resources.Service.GVR(), name+"-tls", namespace, step) {
		hasFailed = true
	}

	if !DeleteResourceIfPresent(ctx, target,
		resources.Secret.GVR(), name+"-oauth-client", namespace, step) {
		hasFailed = true
	}

	if !DeleteResourceIfPresent(ctx, target,
		resources.Secret.GVR(), name+"-oauth-config", namespace, step) {
		hasFailed = true
	}

	if !DeleteResourceIfPresent(ctx, target,
		resources.Secret.GVR(), name+"-tls", namespace, step) {
		hasFailed = true
	}

	oauthClientName := fmt.Sprintf("%s-%s-oauth-client", name, namespace)
	if !DeleteResourceIfPresent(ctx, target,
		resources.OAuthClient.GVR(), oauthClientName, "", step) {
		hasFailed = true
	}

	if hasFailed {
		step.Completef(result.StepFailed,
			"Cleanup partially failed for %s/%s", namespace, name)

		return CleanupResultFailed
	}

	if target.DryRun {
		step.Completef(result.StepSkipped,
			"Would clean up OAuth resources for %s/%s", namespace, name)
	} else {
		step.Completef(result.StepCompleted,
			"Cleaned up OAuth resources for %s/%s", namespace, name)
	}

	return CleanupResultCleaned
}

func (a *CleanupOAuthAction) promptBeforeCleanup(
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
