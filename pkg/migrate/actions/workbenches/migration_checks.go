package workbenches

import (
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/opendatahub-io/odh-cli/pkg/util/jq"
)

const (
	AnnotationInjectAuth  = "notebooks.opendatahub.io/inject-auth"
	AnnotationInjectOAuth = "notebooks.opendatahub.io/inject-oauth"

	ContainerKubeRBACProxy = "kube-rbac-proxy"
	ContainerOAuthProxy    = "oauth-proxy"

	EnvNotebookArgs       = "NOTEBOOK_ARGS"
	TornadoSettingsPrefix = "--ServerApp.tornado_settings="
)

// CheckMigrationState verifies that a notebook has been successfully migrated
// from OAuth-proxy to kube-rbac-proxy. Returns true if all checks pass, along
// with a list of failure messages for any checks that did not pass.
func CheckMigrationState(nb *unstructured.Unstructured) (bool, []string) {
	var failures []string

	annotations := nb.GetAnnotations()

	if annotations[AnnotationInjectAuth] != "true" {
		failures = append(failures,
			fmt.Sprintf("inject-auth annotation missing or not 'true' (found: %q)",
				annotations[AnnotationInjectAuth]))
	}

	containers, err := jq.Query[[]any](nb, ".spec.template.spec.containers")
	if err != nil {
		failures = append(failures, fmt.Sprintf("could not read containers: %v", err))

		return false, failures
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
		case ContainerKubeRBACProxy:
			hasKubeRBACProxy = true
		case ContainerOAuthProxy:
			hasOAuthProxy = true
		}
	}

	if !hasKubeRBACProxy {
		failures = append(failures, "kube-rbac-proxy sidecar container missing")
	}

	if hasOAuthProxy {
		failures = append(failures, "legacy oauth-proxy sidecar still present")
	}

	if injectOAuth, exists := annotations[AnnotationInjectOAuth]; exists {
		if !hasKubeRBACProxy || hasOAuthProxy {
			failures = append(failures,
				fmt.Sprintf("legacy inject-oauth annotation still exists: %q", injectOAuth))
		}
	}

	if HasTornadoSettings(containers) {
		failures = append(failures,
			"--ServerApp.tornado_settings still present in NOTEBOOK_ARGS")
	}

	return len(failures) == 0, failures
}

// HasTornadoSettings checks whether any container has --ServerApp.tornado_settings
// in its NOTEBOOK_ARGS environment variable.
func HasTornadoSettings(containers []any) bool {
	for _, raw := range containers {
		containerMap, ok := raw.(map[string]any)
		if !ok {
			continue
		}

		envVars, ok := containerMap["env"].([]any)
		if !ok {
			continue
		}

		for _, envRaw := range envVars {
			envMap, ok := envRaw.(map[string]any)
			if !ok {
				continue
			}

			name, _ := envMap["name"].(string)
			value, _ := envMap["value"].(string)

			if name == EnvNotebookArgs && strings.Contains(value, TornadoSettingsPrefix) {
				return true
			}
		}
	}

	return false
}
