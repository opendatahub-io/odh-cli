package kube

import (
	"context"
	"encoding/json"
	"fmt"

	"golang.org/x/sync/errgroup"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/lburgazzoli/odh-cli/pkg/resources"
	"github.com/lburgazzoli/odh-cli/pkg/util/client"
)

// FetchResourcesByName fetches resources by name from the cluster (parallel version).
func FetchResourcesByName(
	ctx context.Context,
	c *client.Client,
	namespace string,
	resourceType resources.ResourceType,
	names []string,
) ([]*unstructured.Unstructured, error) {
	items, _, err := FetchResourcesByNameWithMissing(ctx, c, namespace, resourceType, names)
	return items, err
}

// FetchResourcesByNameWithMissing fetches resources and returns both found items and missing names.
func FetchResourcesByNameWithMissing(
	ctx context.Context,
	c *client.Client,
	namespace string,
	resourceType resources.ResourceType,
	names []string,
) ([]*unstructured.Unstructured, []string, error) {
	if len(names) == 0 {
		return nil, nil, nil
	}

	gvr := resourceType.GVR()

	// Use errgroup for concurrent fetches
	g, ctx := errgroup.WithContext(ctx)
	items := make([]*unstructured.Unstructured, len(names))
	notFound := make([]bool, len(names))

	for i, name := range names {
		g.Go(func() error {
			resource, err := c.Get(ctx, gvr, name, client.InNamespace(namespace))
			if err != nil {
				// Mark as not found
				notFound[i] = true
				return nil
			}
			items[i] = resource

			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return nil, nil, fmt.Errorf("waiting for parallel fetches: %w", err)
	}

	// Separate found items and missing names
	result := make([]*unstructured.Unstructured, 0, len(items))
	missing := make([]string, 0)
	for i, item := range items {
		if item != nil {
			result = append(result, item)
		} else if notFound[i] {
			missing = append(missing, names[i])
		}
	}

	return result, missing, nil
}

// ExtractConfigMapRefsFromVolumes extracts ConfigMap names from volume definitions.
func ExtractConfigMapRefsFromVolumes(volumes []corev1.Volume) []string {
	names := sets.New[string]()
	for _, vol := range volumes {
		if ref := ExtractConfigMapRefsFromVolume(vol); ref != "" {
			names.Insert(ref)
		}
	}

	return sets.List(names)
}

// ExtractSecretRefsFromVolumes extracts Secret names from volume definitions.
func ExtractSecretRefsFromVolumes(volumes []corev1.Volume) []string {
	names := sets.New[string]()
	for _, vol := range volumes {
		if ref := ExtractSecretRefsFromVolume(vol); ref != "" {
			names.Insert(ref)
		}
	}

	return sets.List(names)
}

// ExtractPVCRefsFromVolumes extracts PVC names from volume definitions.
func ExtractPVCRefsFromVolumes(volumes []corev1.Volume) []string {
	names := sets.New[string]()
	for _, vol := range volumes {
		if ref := ExtractPVCRefsFromVolume(vol); ref != "" {
			names.Insert(ref)
		}
	}

	return sets.List(names)
}

// ExtractConfigMapRefsFromVolume extracts ConfigMap name from a single volume definition.
func ExtractConfigMapRefsFromVolume(volume corev1.Volume) string {
	if volume.ConfigMap != nil && volume.ConfigMap.Name != "" {
		return volume.ConfigMap.Name
	}

	return ""
}

// ExtractSecretRefsFromVolume extracts Secret name from a single volume definition.
func ExtractSecretRefsFromVolume(volume corev1.Volume) string {
	if volume.Secret != nil && volume.Secret.SecretName != "" {
		return volume.Secret.SecretName
	}

	return ""
}

// ExtractPVCRefsFromVolume extracts PVC name from a single volume definition.
func ExtractPVCRefsFromVolume(volume corev1.Volume) string {
	if volume.PersistentVolumeClaim != nil && volume.PersistentVolumeClaim.ClaimName != "" {
		return volume.PersistentVolumeClaim.ClaimName
	}

	return ""
}

// ExtractConfigMapRefFromEnvFromSource extracts ConfigMap name from a single EnvFromSource.
func ExtractConfigMapRefFromEnvFromSource(envFrom corev1.EnvFromSource) string {
	if envFrom.ConfigMapRef != nil && envFrom.ConfigMapRef.Name != "" {
		return envFrom.ConfigMapRef.Name
	}

	return ""
}

// ExtractSecretRefFromEnvFromSource extracts Secret name from a single EnvFromSource.
func ExtractSecretRefFromEnvFromSource(envFrom corev1.EnvFromSource) string {
	if envFrom.SecretRef != nil && envFrom.SecretRef.Name != "" {
		return envFrom.SecretRef.Name
	}

	return ""
}

// ExtractConfigMapRefFromEnvVar extracts ConfigMap name from a single EnvVar's ValueFrom.
func ExtractConfigMapRefFromEnvVar(env corev1.EnvVar) string {
	if env.ValueFrom != nil && env.ValueFrom.ConfigMapKeyRef != nil && env.ValueFrom.ConfigMapKeyRef.Name != "" {
		return env.ValueFrom.ConfigMapKeyRef.Name
	}

	return ""
}

// ExtractSecretRefFromEnvVar extracts Secret name from a single EnvVar's ValueFrom.
func ExtractSecretRefFromEnvVar(env corev1.EnvVar) string {
	if env.ValueFrom != nil && env.ValueFrom.SecretKeyRef != nil && env.ValueFrom.SecretKeyRef.Name != "" {
		return env.ValueFrom.SecretKeyRef.Name
	}

	return ""
}

// ExtractConfigMapRefs extracts all ConfigMap names referenced by a container.
func ExtractConfigMapRefs(container corev1.Container) []string {
	names := sets.New[string]()

	for _, envFrom := range container.EnvFrom {
		if ref := ExtractConfigMapRefFromEnvFromSource(envFrom); ref != "" {
			names.Insert(ref)
		}
	}

	for _, env := range container.Env {
		if ref := ExtractConfigMapRefFromEnvVar(env); ref != "" {
			names.Insert(ref)
		}
	}

	return sets.List(names)
}

// ExtractSecretRefs extracts all Secret names referenced by a container.
func ExtractSecretRefs(container corev1.Container) []string {
	names := sets.New[string]()

	for _, envFrom := range container.EnvFrom {
		if ref := ExtractSecretRefFromEnvFromSource(envFrom); ref != "" {
			names.Insert(ref)
		}
	}

	for _, env := range container.Env {
		if ref := ExtractSecretRefFromEnvVar(env); ref != "" {
			names.Insert(ref)
		}
	}

	return sets.List(names)
}

// ExtractConfigMapRefsFromSources extracts ConfigMap references from various source types.
// Supported types: corev1.Volume, corev1.Container.
func ExtractConfigMapRefsFromSources(sources []any) ([]string, error) {
	names := sets.New[string]()

	for _, source := range sources {
		switch v := source.(type) {
		case corev1.Volume:
			if ref := ExtractConfigMapRefsFromVolume(v); ref != "" {
				names.Insert(ref)
			}
		case corev1.Container:
			names.Insert(ExtractConfigMapRefs(v)...)
		default:
			return nil, fmt.Errorf("unsupported source type for ConfigMap extraction: %T", source)
		}
	}

	return sets.List(names), nil
}

// ExtractSecretRefsFromSources extracts Secret references from various source types.
// Supported types: corev1.Volume, corev1.Container.
func ExtractSecretRefsFromSources(sources []any) ([]string, error) {
	names := sets.New[string]()

	for _, source := range sources {
		switch v := source.(type) {
		case corev1.Volume:
			if ref := ExtractSecretRefsFromVolume(v); ref != "" {
				names.Insert(ref)
			}
		case corev1.Container:
			names.Insert(ExtractSecretRefs(v)...)
		default:
			return nil, fmt.Errorf("unsupported source type for Secret extraction: %T", source)
		}
	}

	return sets.List(names), nil
}

// ExtractPVCRefsFromSources extracts PVC references from various source types.
// Supported types: corev1.Volume.
func ExtractPVCRefsFromSources(sources []any) ([]string, error) {
	names := sets.New[string]()

	for _, source := range sources {
		switch v := source.(type) {
		case corev1.Volume:
			if ref := ExtractPVCRefsFromVolume(v); ref != "" {
				names.Insert(ref)
			}
		default:
			return nil, fmt.Errorf("unsupported source type for PVC extraction: %T", source)
		}
	}

	return sets.List(names), nil
}

// ConvertToTyped converts unstructured data to a typed value.
func ConvertToTyped[T any](raw any, typeName string) (T, error) {
	var zero T
	if raw == nil {
		return zero, nil
	}

	data, err := json.Marshal(raw)
	if err != nil {
		return zero, fmt.Errorf("marshaling %s: %w", typeName, err)
	}

	var result T
	if err := json.Unmarshal(data, &result); err != nil {
		return zero, fmt.Errorf("unmarshaling %s: %w", typeName, err)
	}

	return result, nil
}
