package notebooks

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/opendatahub-io/odh-cli/pkg/backup/dependencies"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
	"github.com/opendatahub-io/odh-cli/pkg/util/jq"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// Resolver resolves dependencies for Kubeflow Notebooks.
type Resolver struct{}

// NewResolver creates a new Notebook dependency resolver.
func NewResolver() *Resolver {
	return &Resolver{}
}

// CanHandle returns true for Kubeflow Notebook resources.
func (r *Resolver) CanHandle(gvr schema.GroupVersionResource) bool {
	return gvr.Group == resources.Notebook.Group && gvr.Resource == resources.Notebook.Resource
}

// Resolve finds all dependencies for a Notebook.
func (r *Resolver) Resolve(
	ctx context.Context,
	c client.Reader,
	obj *unstructured.Unstructured,
) ([]dependencies.Dependency, error) {
	namespace := obj.GetNamespace()

	volumes, err := r.extractVolumes(obj)
	if err != nil {
		return nil, err
	}

	containers, err := r.extractContainers(obj)
	if err != nil {
		return nil, err
	}

	// Combine volumes and containers for resolution
	sources := make([]any, 0, len(volumes)+len(containers))
	for _, v := range volumes {
		sources = append(sources, v)
	}
	for _, c := range containers {
		sources = append(sources, c)
	}

	var allDeps []dependencies.Dependency

	// Filter out trusted-ca-bundle ConfigMaps before resolving
	filteredSources := r.filterTrustedCABundleSources(sources)

	configMapDeps, err := dependencies.ResolveConfigMaps(ctx, c, namespace, filteredSources...)
	if err != nil {
		return nil, fmt.Errorf("resolving ConfigMaps: %w", err)
	}
	allDeps = append(allDeps, configMapDeps...)

	// Resolve Secrets unconditionally when dependencies are enabled.
	secretDeps, err := dependencies.ResolveSecrets(ctx, c, namespace, filteredSources...)
	if err != nil {
		return nil, fmt.Errorf("resolving Secrets: %w", err)
	}
	allDeps = append(allDeps, secretDeps...)

	// Pass volumes only for PVCs
	volumeSources := make([]any, 0, len(volumes))
	for _, v := range volumes {
		volumeSources = append(volumeSources, v)
	}

	pvcDeps, err := dependencies.ResolvePVCs(ctx, c, namespace, volumeSources...)
	if err != nil {
		return nil, fmt.Errorf("resolving PVCs: %w", err)
	}
	allDeps = append(allDeps, pvcDeps...)

	return allDeps, nil
}

func (r *Resolver) extractVolumes(obj *unstructured.Unstructured) ([]corev1.Volume, error) {
	volumes, err := jq.Query[[]corev1.Volume](obj, ".spec.template.spec.volumes // []")
	if err != nil && !errors.Is(err, jq.ErrNotFound) {
		return nil, fmt.Errorf("querying volumes: %w", err)
	}

	return volumes, nil
}

func (r *Resolver) extractContainers(obj *unstructured.Unstructured) ([]corev1.Container, error) {
	containers, err := jq.Query[[]corev1.Container](obj, ".spec.template.spec.containers // []")
	if err != nil && !errors.Is(err, jq.ErrNotFound) {
		return nil, fmt.Errorf("querying containers: %w", err)
	}

	return containers, nil
}

// filterTrustedCABundleSources filters out volumes and containers that reference ConfigMaps
// ending with "trusted-ca-bundle" (typically large cluster CA bundles that can be recreated).
func (r *Resolver) filterTrustedCABundleSources(sources []any) []any {
	filtered := make([]any, 0, len(sources))

	for _, source := range sources {
		switch v := source.(type) {
		case corev1.Volume:
			if v.ConfigMap != nil && strings.HasSuffix(v.ConfigMap.Name, "trusted-ca-bundle") {
				continue
			}
			filtered = append(filtered, v)
		case corev1.Container:
			filteredContainer := r.filterContainerConfigMapRefs(v)
			filtered = append(filtered, filteredContainer)
		default:
			filtered = append(filtered, source)
		}
	}

	return filtered
}

// filterContainerConfigMapRefs removes ConfigMap references ending with "trusted-ca-bundle"
// from container env and envFrom fields.
func (r *Resolver) filterContainerConfigMapRefs(container corev1.Container) corev1.Container {
	// Filter envFrom
	filteredEnvFrom := make([]corev1.EnvFromSource, 0, len(container.EnvFrom))
	for _, envFrom := range container.EnvFrom {
		if envFrom.ConfigMapRef != nil && strings.HasSuffix(envFrom.ConfigMapRef.Name, "trusted-ca-bundle") {
			continue
		}
		filteredEnvFrom = append(filteredEnvFrom, envFrom)
	}
	container.EnvFrom = filteredEnvFrom

	// Filter env (valueFrom.configMapKeyRef)
	filteredEnv := make([]corev1.EnvVar, 0, len(container.Env))
	for _, env := range container.Env {
		if env.ValueFrom != nil &&
			env.ValueFrom.ConfigMapKeyRef != nil &&
			strings.HasSuffix(env.ValueFrom.ConfigMapKeyRef.Name, "trusted-ca-bundle") {
			continue
		}
		filteredEnv = append(filteredEnv, env)
	}
	container.Env = filteredEnv

	return container
}
