package authmodel

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/opendatahub-io/odh-cli/pkg/util/jq"
)

const (
	annotationInjectAuth     = "notebooks.opendatahub.io/inject-auth"
	annotationInjectOAuth    = "notebooks.opendatahub.io/inject-oauth"
	annotationOAuthLogoutURL = "notebooks.opendatahub.io/oauth-logout-url"

	containerOAuthProxy = "oauth-proxy"

	finalizerOAuthClient = "notebook-oauth-client-finalizer.opendatahub.io"

	envNotebookArgs = "NOTEBOOK_ARGS"
)

// isOAuthVolumeName returns true if name is one of the volumes injected by the
// 2.x OAuth-proxy sidecar.
func isOAuthVolumeName(name string) bool {
	switch name {
	case "oauth-config", "oauth-client", "tls-certificates":
		return true
	default:
		return false
	}
}

const tornadoSettingsPrefix = "--ServerApp.tornado_settings="

func isArgWhitespace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r'
}

// findBraceEnd finds the end of a brace-delimited block by counting {/} depth.
// It does not account for braces inside JSON string literals, which is safe for
// known tornado_settings values (CSP headers) but not arbitrary JSON.
func findBraceEnd(s string, start int) int {
	depth := 0

	for i := start; i < len(s); i++ {
		switch s[i] {
		case '{':
			depth++
		case '}':
			depth--
		}

		if depth == 0 {
			return i + 1
		}
	}

	return len(s)
}

// removeTornadoArg strips all --ServerApp.tornado_settings=... arguments
// (and their leading whitespace) from s while preserving neighbouring args.
// If the value starts with '{', brace-depth counting is used to find the end,
// which correctly handles JSON that contains embedded spaces.
func removeTornadoArg(s string) string {
	for {
		idx := strings.Index(s, tornadoSettingsPrefix)
		if idx < 0 {
			return s
		}

		start := idx
		for start > 0 && isArgWhitespace(s[start-1]) {
			start--
		}

		end := idx + len(tornadoSettingsPrefix)
		if end < len(s) && s[end] == '{' {
			end = findBraceEnd(s, end)
		} else {
			for end < len(s) && !isArgWhitespace(s[end]) {
				end++
			}
		}

		s = s[:start] + s[end:]
	}
}

// needsAuthModelPatch returns true if the notebook still has any 2.x OAuth
// artifacts that must be removed before 3.x.
func needsAuthModelPatch(nb *unstructured.Unstructured) bool {
	return hasOAuthAnnotation(nb) ||
		hasOAuthProxyContainer(nb) ||
		hasOAuthFinalizer(nb) ||
		hasOAuthVolumes(nb) ||
		hasTornadoSettings(nb)
}

// hasOAuthAnnotation returns true if inject-oauth or oauth-logout-url is present.
func hasOAuthAnnotation(nb *unstructured.Unstructured) bool {
	annotations := nb.GetAnnotations()
	if annotations == nil {
		return false
	}

	if _, ok := annotations[annotationInjectOAuth]; ok {
		return true
	}

	_, ok := annotations[annotationOAuthLogoutURL]

	return ok
}

// hasOAuthProxyContainer returns true if there is an "oauth-proxy" container.
func hasOAuthProxyContainer(nb *unstructured.Unstructured) bool {
	containers, err := jq.Query[[]any](nb, ".spec.template.spec.containers")
	if err != nil {
		return false
	}

	for _, raw := range containers {
		m, ok := raw.(map[string]any)
		if !ok {
			continue
		}

		if name, _ := m["name"].(string); name == containerOAuthProxy {
			return true
		}
	}

	return false
}

// hasOAuthFinalizer returns true if the OAuth client finalizer is present.
func hasOAuthFinalizer(nb *unstructured.Unstructured) bool {
	return slices.Contains(nb.GetFinalizers(), finalizerOAuthClient)
}

// hasOAuthVolumes returns true if any of the OAuth volumes are present.
func hasOAuthVolumes(nb *unstructured.Unstructured) bool {
	volumes, err := jq.Query[[]any](nb, ".spec.template.spec.volumes")
	if err != nil {
		return false
	}

	for _, raw := range volumes {
		m, ok := raw.(map[string]any)
		if !ok {
			continue
		}

		if name, _ := m["name"].(string); isOAuthVolumeName(name) {
			return true
		}
	}

	return false
}

// hasTornadoSettings returns true if any container has a NOTEBOOK_ARGS env var
// that contains --ServerApp.tornado_settings=.
func hasTornadoSettings(nb *unstructured.Unstructured) bool {
	containers, err := jq.Query[[]any](nb, ".spec.template.spec.containers")
	if err != nil {
		return false
	}

	for _, raw := range containers {
		m, ok := raw.(map[string]any)
		if !ok {
			continue
		}

		envVars, ok := m["env"].([]any)
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

			if name == envNotebookArgs && strings.Contains(value, "--ServerApp.tornado_settings=") {
				return true
			}
		}
	}

	return false
}

// addInjectAuthAnnotation sets the inject-auth annotation to "true".
func addInjectAuthAnnotation(nb *unstructured.Unstructured) {
	annotations := nb.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}

	annotations[annotationInjectAuth] = "true"
	nb.SetAnnotations(annotations)
}

// removeAnnotation deletes a single annotation key.
func removeAnnotation(nb *unstructured.Unstructured, key string) {
	annotations := nb.GetAnnotations()
	if annotations == nil {
		return
	}

	delete(annotations, key)
	nb.SetAnnotations(annotations)
}

// removeOAuthProxyContainer filters out the oauth-proxy container.
func removeOAuthProxyContainer(nb *unstructured.Unstructured) error {
	containers, err := jq.Query[[]any](nb, ".spec.template.spec.containers")
	if err != nil {
		return fmt.Errorf("querying containers: %w", err)
	}

	filtered := make([]any, 0, len(containers))

	for _, raw := range containers {
		m, ok := raw.(map[string]any)
		if !ok {
			filtered = append(filtered, raw)

			continue
		}

		if name, _ := m["name"].(string); name == containerOAuthProxy {
			continue
		}

		filtered = append(filtered, raw)
	}

	data, err := json.Marshal(filtered)
	if err != nil {
		return fmt.Errorf("marshaling containers: %w", err)
	}

	if err := jq.Transform(nb, ".spec.template.spec.containers = %s", data); err != nil {
		return fmt.Errorf("setting containers: %w", err)
	}

	return nil
}

// removeOAuthFinalizer removes the notebook-oauth-client-finalizer.
func removeOAuthFinalizer(nb *unstructured.Unstructured) {
	finalizers := nb.GetFinalizers()
	if len(finalizers) == 0 {
		return
	}

	var filtered []string

	for _, f := range finalizers {
		if f != finalizerOAuthClient {
			filtered = append(filtered, f)
		}
	}

	nb.SetFinalizers(filtered)
}

// removeOAuthVolumes filters out the three OAuth volumes.
func removeOAuthVolumes(nb *unstructured.Unstructured) error {
	volumes, err := jq.Query[[]any](nb, ".spec.template.spec.volumes")
	if err != nil {
		return fmt.Errorf("querying volumes: %w", err)
	}

	filtered := make([]any, 0, len(volumes))

	for _, raw := range volumes {
		m, ok := raw.(map[string]any)
		if !ok {
			filtered = append(filtered, raw)

			continue
		}

		if name, _ := m["name"].(string); isOAuthVolumeName(name) {
			continue
		}

		filtered = append(filtered, raw)
	}

	data, err := json.Marshal(filtered)
	if err != nil {
		return fmt.Errorf("marshaling volumes: %w", err)
	}

	if err := jq.Transform(nb, ".spec.template.spec.volumes = %s", data); err != nil {
		return fmt.Errorf("setting volumes: %w", err)
	}

	return nil
}

// stripTornadoSettings removes --ServerApp.tornado_settings=... from the
// NOTEBOOK_ARGS env var in all containers.
func stripTornadoSettings(nb *unstructured.Unstructured) error {
	containers, err := jq.Query[[]any](nb, ".spec.template.spec.containers")
	if err != nil {
		return fmt.Errorf("querying containers: %w", err)
	}

	modified := false

	for _, raw := range containers {
		m, ok := raw.(map[string]any)
		if !ok {
			continue
		}

		envVars, ok := m["env"].([]any)
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

			if name == envNotebookArgs && strings.Contains(value, "--ServerApp.tornado_settings=") {
				envMap["value"] = strings.TrimSpace(removeTornadoArg(value))
				modified = true
			}
		}
	}

	if !modified {
		return nil
	}

	data, err := json.Marshal(containers)
	if err != nil {
		return fmt.Errorf("marshaling containers: %w", err)
	}

	if err := jq.Transform(nb, ".spec.template.spec.containers = %s", data); err != nil {
		return fmt.Errorf("setting containers: %w", err)
	}

	return nil
}

// applyAllPatches applies all 7 patch operations to a notebook (must be a DeepCopy):
// addInjectAuthAnnotation, removeAnnotation (×2), removeOAuthProxyContainer,
// removeOAuthFinalizer, removeOAuthVolumes, and stripTornadoSettings.
func applyAllPatches(nb *unstructured.Unstructured) error {
	addInjectAuthAnnotation(nb)
	removeAnnotation(nb, annotationInjectOAuth)
	removeAnnotation(nb, annotationOAuthLogoutURL)

	if err := removeOAuthProxyContainer(nb); err != nil {
		return fmt.Errorf("removing oauth-proxy container: %w", err)
	}

	removeOAuthFinalizer(nb)

	if err := removeOAuthVolumes(nb); err != nil {
		return fmt.Errorf("removing OAuth volumes: %w", err)
	}

	if err := stripTornadoSettings(nb); err != nil {
		return fmt.Errorf("stripping tornado_settings: %w", err)
	}

	return nil
}
