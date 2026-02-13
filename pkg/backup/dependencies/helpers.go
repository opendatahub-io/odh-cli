package dependencies

import (
	"context"
	"fmt"

	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
	"github.com/opendatahub-io/odh-cli/pkg/util/kube"
)

// ResolveConfigMaps collects ConfigMap references from various sources,
// fetches them from the cluster, and returns them as dependencies.
//
// Supported source types:
//   - corev1.Volume: Extracts ConfigMap references from volume definitions
//   - corev1.Container: Extracts ConfigMap references from envFrom and env.valueFrom fields
//
// Use spread operator to pass slices: ResolveConfigMaps(ctx, c, ns, volumes...)
//
// Returns an error if an unsupported source type is provided.
func ResolveConfigMaps(
	ctx context.Context,
	c client.Reader,
	namespace string,
	sources ...any,
) ([]Dependency, error) {
	names, err := kube.ExtractConfigMapRefsFromSources(sources)
	if err != nil {
		return nil, fmt.Errorf("extracting ConfigMap references: %w", err)
	}

	items, err := kube.FetchResourcesByName(ctx, c, namespace, resources.ConfigMap, names)
	if err != nil {
		return nil, fmt.Errorf("fetching ConfigMaps: %w", err)
	}

	deps := make([]Dependency, 0, len(items))
	for _, res := range items {
		deps = append(deps, Dependency{
			GVR:      resources.ConfigMap.GVR(),
			Resource: res,
			Name:     res.GetName(),
			Error:    nil,
		})
	}

	return deps, nil
}

// ResolveSecrets collects Secret references from various sources,
// fetches them from the cluster, and returns them as dependencies.
//
// Supported source types:
//   - corev1.Volume: Extracts Secret references from volume definitions
//   - corev1.Container: Extracts Secret references from envFrom and env.valueFrom fields
//
// Use spread operator to pass slices: ResolveSecrets(ctx, c, ns, volumes...)
//
// Returns an error if an unsupported source type is provided.
func ResolveSecrets(
	ctx context.Context,
	c client.Reader,
	namespace string,
	sources ...any,
) ([]Dependency, error) {
	names, err := kube.ExtractSecretRefsFromSources(sources)
	if err != nil {
		return nil, fmt.Errorf("extracting Secret references: %w", err)
	}

	items, errors, err := kube.FetchResourcesByNameWithErrors(ctx, c, namespace, resources.Secret, names)
	if err != nil {
		return nil, fmt.Errorf("fetching Secrets: %w", err)
	}

	// Create dependencies for both found and missing resources
	deps := make([]Dependency, 0, len(names))

	// Add found resources
	for _, res := range items {
		deps = append(deps, Dependency{
			GVR:      resources.Secret.GVR(),
			Resource: res,
			Name:     res.GetName(),
			Error:    nil,
		})
	}

	// Add missing resources with error info
	for name, fetchErr := range errors {
		deps = append(deps, Dependency{
			GVR:      resources.Secret.GVR(),
			Resource: nil,
			Name:     name,
			Error:    fetchErr,
		})
	}

	return deps, nil
}

// ResolvePVCs collects PVC references from various sources,
// fetches them from the cluster, and returns them as dependencies.
//
// Supported source types:
//   - corev1.Volume: Extracts PVC references from volume definitions
//
// Use spread operator to pass slices: ResolvePVCs(ctx, c, ns, volumes...)
//
// Returns an error if an unsupported source type is provided.
func ResolvePVCs(
	ctx context.Context,
	c client.Reader,
	namespace string,
	sources ...any,
) ([]Dependency, error) {
	names, err := kube.ExtractPVCRefsFromSources(sources)
	if err != nil {
		return nil, fmt.Errorf("extracting PVC references: %w", err)
	}

	items, err := kube.FetchResourcesByName(ctx, c, namespace, resources.PersistentVolumeClaim, names)
	if err != nil {
		return nil, fmt.Errorf("fetching PVCs: %w", err)
	}

	deps := make([]Dependency, 0, len(items))
	for _, res := range items {
		deps = append(deps, Dependency{
			GVR:      resources.PersistentVolumeClaim.GVR(),
			Resource: res,
			Name:     res.GetName(),
			Error:    nil,
		})
	}

	return deps, nil
}
