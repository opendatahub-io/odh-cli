package verify

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/opendatahub-io/odh-cli/pkg/migrate/action"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/actions/workbenches"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/jq"
)

// MigrationStatus classifies a notebook's migration state.
type MigrationStatus string

const (
	StatusLegacy       MigrationStatus = "LEGACY"
	StatusMigrated     MigrationStatus = "MIGRATED"
	StatusUnreconciled MigrationStatus = "UNRECONCILED"
	StatusInvalid      MigrationStatus = "INVALID"
	StatusUnknown      MigrationStatus = "UNKNOWN"
)

// RunningState describes whether a workbench is running, stopped, or starting.
type RunningState string

const (
	StateRunning  RunningState = "Running"
	StateStopped  RunningState = "Stopped"
	StateStarting RunningState = "Starting"
	StateError    RunningState = "Error"
)

// ClassifyNotebook determines migration status from sidecar and annotation state.
// Returns the status and a human-readable detail string.
func ClassifyNotebook(nb *unstructured.Unstructured) (MigrationStatus, string) {
	annotations := nb.GetAnnotations()
	injectAuth := annotations[workbenches.AnnotationInjectAuth]
	injectOAuth := annotations[workbenches.AnnotationInjectOAuth]

	containers, err := jq.Query[[]any](nb, ".spec.template.spec.containers")
	if err != nil {
		return StatusUnknown, "could not read containers"
	}

	hasKubeRBACProxy := false
	hasOAuthProxy := false

	for _, raw := range containers {
		containerMap, ok := raw.(map[string]any)
		if !ok {
			continue
		}

		name, _ := containerMap["name"].(string)

		switch name {
		case workbenches.ContainerKubeRBACProxy:
			hasKubeRBACProxy = true
		case workbenches.ContainerOAuthProxy:
			hasOAuthProxy = true
		}
	}

	switch {
	case hasKubeRBACProxy && hasOAuthProxy:
		return StatusInvalid, "both kube-rbac-proxy and oauth-proxy sidecars present"

	case hasKubeRBACProxy:
		if injectOAuth == "true" {
			return StatusMigrated, "leftover inject-oauth annotation"
		}

		return StatusMigrated, ""

	case hasOAuthProxy:
		if injectAuth == "true" {
			return StatusUnreconciled,
				"inject-auth is set but oauth-proxy sidecar still present"
		}

		return StatusLegacy, "needs migration to 3.x"

	default:
		return StatusUnknown, "no auth sidecars found"
	}
}

// GetRunningState determines whether a workbench is running, stopped, or starting
// by checking the kubeflow-resource-stopped annotation and querying the pod.
func GetRunningState(
	ctx context.Context,
	target action.Target,
	nb *unstructured.Unstructured,
) (RunningState, string) {
	annotations := nb.GetAnnotations()

	if stopped, ok := annotations["kubeflow-resource-stopped"]; ok && stopped != "" {
		return StateStopped, "since " + stopped
	}

	podName := nb.GetName() + "-0"
	namespace := nb.GetNamespace()

	pod, err := target.Client.Dynamic().Resource(resources.Pod.GVR()).
		Namespace(namespace).Get(ctx, podName, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return StateStarting, "no pod yet"
		}

		return StateError, fmt.Sprintf("error checking pod %q: %v", podName, err)
	}

	phase, _, _ := unstructured.NestedString(pod.Object, "status", "phase")

	switch phase {
	case "Running":
		return StateRunning, ""
	case "Pending":
		return StateStarting, "pod pending"
	default:
		return RunningState(phase), ""
	}
}

// CheckCleanupState verifies that legacy OAuth resources have been removed.
// Returns true if all resources are absent, along with a list of failure
// messages for any resources that still exist.
func CheckCleanupState(
	ctx context.Context,
	target action.Target,
	nb *unstructured.Unstructured,
) (bool, []string) {
	name := nb.GetName()
	namespace := nb.GetNamespace()

	var failures []string

	checks := []struct {
		gvr       resources.ResourceType
		resName   string
		namespace string
		label     string
	}{
		{resources.Route, name, namespace,
			fmt.Sprintf("Route '%s'", name)},
		{resources.Service, name + "-tls", namespace,
			fmt.Sprintf("Service '%s-tls'", name)},
		{resources.Secret, name + "-oauth-client", namespace,
			fmt.Sprintf("Secret '%s-oauth-client'", name)},
		{resources.Secret, name + "-oauth-config", namespace,
			fmt.Sprintf("Secret '%s-oauth-config'", name)},
		{resources.Secret, name + "-tls", namespace,
			fmt.Sprintf("Secret '%s-tls'", name)},
		{resources.OAuthClient, fmt.Sprintf("%s-%s-oauth-client", name, namespace), "",
			fmt.Sprintf("OAuthClient '%s-%s-oauth-client'", name, namespace)},
	}

	for _, check := range checks {
		exists, err := resourceExists(ctx, target, check.gvr, check.resName, check.namespace)
		if err != nil {
			failures = append(failures,
				fmt.Sprintf("error checking %s: %v", check.label, err))

			continue
		}

		if exists {
			failures = append(failures, check.label+" still exists")
		}
	}

	return len(failures) == 0, failures
}

func resourceExists(
	ctx context.Context,
	target action.Target,
	rt resources.ResourceType,
	name string,
	namespace string,
) (bool, error) {
	ri := target.Client.Dynamic().Resource(rt.GVR())

	var err error
	if namespace != "" {
		_, err = ri.Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	} else {
		_, err = ri.Get(ctx, name, metav1.GetOptions{})
	}

	if err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}

		return false, fmt.Errorf("getting %s %q: %w", rt.Resource, name, err)
	}

	return true, nil
}
