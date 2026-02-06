package client

import (
	"context"
	"errors"
	"fmt"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/lburgazzoli/odh-cli/pkg/resources"
	"github.com/lburgazzoli/odh-cli/pkg/util"
	"github.com/lburgazzoli/odh-cli/pkg/util/jq"
)

// DiscoverGVRsConfig configures CRD discovery.
type DiscoverGVRsConfig struct {
	LabelSelector string
}

// DiscoverGVRsOption is an option for configuring DiscoverGVRs.
type DiscoverGVRsOption = util.Option[DiscoverGVRsConfig]

// WithCRDLabelSelector filters CRDs by label selector.
func WithCRDLabelSelector(selector string) DiscoverGVRsOption {
	return util.FunctionalOption[DiscoverGVRsConfig](func(c *DiscoverGVRsConfig) {
		c.LabelSelector = selector
	})
}

// DiscoverGVRs discovers custom resources and returns their GVRs.
// Requires full Client access because it uses the APIExtensions client.
func DiscoverGVRs(ctx context.Context, c Client, opts ...DiscoverGVRsOption) ([]schema.GroupVersionResource, error) {
	cfg := &DiscoverGVRsConfig{
		LabelSelector: "platform.opendatahub.io/part-of", // default for workloads
	}
	util.ApplyOptions(cfg, opts...)

	crdList, err := c.APIExtensions().ApiextensionsV1().CustomResourceDefinitions().List(ctx, metav1.ListOptions{
		LabelSelector: cfg.LabelSelector,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list CRDs: %w", err)
	}

	gvrs := make([]schema.GroupVersionResource, 0, len(crdList.Items))
	for i := range crdList.Items {
		crd := &crdList.Items[i]

		// Skip non-established CRDs
		if !isCRDEstablished(crd) {
			continue
		}

		// Extract GVR using storage version
		gvr := crdToGVR(crd)
		gvrs = append(gvrs, gvr)
	}

	return gvrs, nil
}

// ListResourcesConfig configures resource listing.
type ListResourcesConfig struct {
	Namespace     string
	LabelSelector string
	FieldSelector string
}

// ListResourcesOption is an option for configuring ListResources.
type ListResourcesOption = util.Option[ListResourcesConfig]

// WithNamespace filters resources to a specific namespace.
func WithNamespace(ns string) ListResourcesOption {
	return util.FunctionalOption[ListResourcesConfig](func(c *ListResourcesConfig) {
		c.Namespace = ns
	})
}

// WithLabelSelector filters resources by label selector.
func WithLabelSelector(selector string) ListResourcesOption {
	return util.FunctionalOption[ListResourcesConfig](func(c *ListResourcesConfig) {
		c.LabelSelector = selector
	})
}

// WithFieldSelector filters resources by field selector.
func WithFieldSelector(selector string) ListResourcesOption {
	return util.FunctionalOption[ListResourcesConfig](func(c *ListResourcesConfig) {
		c.FieldSelector = selector
	})
}

// ListResources lists all instances of a resource type handling pagination automatically.
// Returns pointers to avoid copying large objects.
func (c *defaultClient) ListResources(ctx context.Context, gvr schema.GroupVersionResource, opts ...ListResourcesOption) ([]*unstructured.Unstructured, error) {
	cfg := &ListResourcesConfig{}
	util.ApplyOptions(cfg, opts...)

	var allItems []*unstructured.Unstructured
	continueToken := ""

	for {
		listOpts := metav1.ListOptions{
			LabelSelector: cfg.LabelSelector,
			FieldSelector: cfg.FieldSelector,
			Continue:      continueToken,
		}

		var list *unstructured.UnstructuredList
		var err error

		if cfg.Namespace != "" {
			list, err = c.dynamic.Resource(gvr).Namespace(cfg.Namespace).List(ctx, listOpts)
		} else {
			list, err = c.dynamic.Resource(gvr).List(ctx, listOpts)
		}

		if err != nil {
			// Permission errors are non-fatal - return empty list
			if IsPermissionError(err) {
				return []*unstructured.Unstructured{}, nil
			}

			return nil, fmt.Errorf("listing resources: %w", err)
		}

		// Append results (convert to pointers)
		for i := range list.Items {
			allItems = append(allItems, &list.Items[i])
		}

		// Check if more pages exist
		if list.GetContinue() == "" {
			break
		}
		continueToken = list.GetContinue()
	}

	return allItems, nil
}

// List lists all instances of a resource type handling pagination automatically.
// Returns pointers to avoid copying large objects.
// This is a convenience wrapper around ListResources that accepts ResourceType.
func (c *defaultClient) List(ctx context.Context, resourceType resources.ResourceType, opts ...ListResourcesOption) ([]*unstructured.Unstructured, error) {
	return c.ListResources(ctx, resourceType.GVR(), opts...)
}

// ListMetadata lists all instances of a resource type returning only metadata.
// Handles pagination automatically. Returns pointers to avoid copying.
// This is more efficient than List when only metadata fields (name, namespace, labels, annotations) are needed.
func (c *defaultClient) ListMetadata(ctx context.Context, resourceType resources.ResourceType, opts ...ListResourcesOption) ([]*metav1.PartialObjectMetadata, error) {
	cfg := &ListResourcesConfig{}
	util.ApplyOptions(cfg, opts...)

	var allItems []*metav1.PartialObjectMetadata
	continueToken := ""

	gvr := resourceType.GVR()

	for {
		listOpts := metav1.ListOptions{
			LabelSelector: cfg.LabelSelector,
			FieldSelector: cfg.FieldSelector,
			Continue:      continueToken,
		}

		var list *metav1.PartialObjectMetadataList
		var err error

		if cfg.Namespace != "" {
			list, err = c.metadata.Resource(gvr).Namespace(cfg.Namespace).List(ctx, listOpts)
		} else {
			list, err = c.metadata.Resource(gvr).List(ctx, listOpts)
		}

		if err != nil {
			// Permission errors are non-fatal - return empty list
			if IsPermissionError(err) {
				return []*metav1.PartialObjectMetadata{}, nil
			}

			return nil, fmt.Errorf("listing metadata for resources: %w", err)
		}

		// Append results (convert to pointers)
		for i := range list.Items {
			allItems = append(allItems, &list.Items[i])
		}

		// Check if more pages exist
		if list.GetContinue() == "" {
			break
		}
		continueToken = list.GetContinue()
	}

	return allItems, nil
}

// GetResource is a convenience wrapper around Get that accepts ResourceType.
func (c *defaultClient) GetResource(ctx context.Context, resourceType resources.ResourceType, name string, opts ...GetOption) (*unstructured.Unstructured, error) {
	return c.Get(ctx, resourceType.GVR(), name, opts...)
}

// GetResourceMetadata retrieves only the metadata of a single resource.
// Use this when you only need name, namespace, labels, or annotations.
// More efficient than GetResource when spec/status fields are not needed.
// Falls back to GetResource when metadata client is not available (e.g., in tests).
func (c *defaultClient) GetResourceMetadata(ctx context.Context, resourceType resources.ResourceType, name string, opts ...GetOption) (*metav1.PartialObjectMetadata, error) {
	cfg := &GetConfig{}
	util.ApplyOptions(cfg, opts...)

	gvr := resourceType.GVR()

	// Fall back to dynamic client if metadata client is not available
	if c.metadata == nil {
		obj, err := c.GetResource(ctx, resourceType, name, opts...)
		if err != nil {
			return nil, err
		}

		return &metav1.PartialObjectMetadata{
			TypeMeta: resourceType.TypeMeta(),
			ObjectMeta: metav1.ObjectMeta{
				Name:            obj.GetName(),
				Namespace:       obj.GetNamespace(),
				UID:             obj.GetUID(),
				ResourceVersion: obj.GetResourceVersion(),
				Labels:          obj.GetLabels(),
				Annotations:     obj.GetAnnotations(),
			},
		}, nil
	}

	var resource *metav1.PartialObjectMetadata
	var err error

	if cfg.Namespace != "" {
		resource, err = c.metadata.Resource(gvr).Namespace(cfg.Namespace).Get(ctx, name, metav1.GetOptions{})
	} else {
		resource, err = c.metadata.Resource(gvr).Get(ctx, name, metav1.GetOptions{})
	}

	if err != nil {
		return nil, fmt.Errorf("getting resource metadata: %w", err)
	}

	return resource, nil
}

// GetConfig holds options for customizing Get operations (e.g., namespace scope).
type GetConfig struct {
	Namespace string
}

// GetOption is a functional option for configuring Get operations.
type GetOption = util.Option[GetConfig]

// InNamespace specifies the namespace for the resource (optional for cluster-scoped).
func InNamespace(ns string) GetOption {
	return util.FunctionalOption[GetConfig](func(c *GetConfig) {
		c.Namespace = ns
	})
}

// Get retrieves a single resource by name, automatically handling namespace vs cluster-scoped resources.
func (c *defaultClient) Get(ctx context.Context, gvr schema.GroupVersionResource, name string, opts ...GetOption) (*unstructured.Unstructured, error) {
	cfg := &GetConfig{}
	util.ApplyOptions(cfg, opts...)

	var resource *unstructured.Unstructured
	var err error

	if cfg.Namespace != "" {
		resource, err = c.dynamic.Resource(gvr).Namespace(cfg.Namespace).Get(ctx, name, metav1.GetOptions{})
	} else {
		// Cluster-scoped resource
		resource, err = c.dynamic.Resource(gvr).Get(ctx, name, metav1.GetOptions{})
	}

	if err != nil {
		// Permission errors are non-fatal - return nil resource
		if IsPermissionError(err) {
			return nil, nil
		}

		return nil, fmt.Errorf("getting resource: %w", err)
	}

	return resource, nil
}

// --- Standalone helper functions that accept Reader ---

// GetSingleton expects exactly one instance of the resource type to exist.
// Returns error if zero or multiple instances found.
func GetSingleton(ctx context.Context, r Reader, resourceType resources.ResourceType) (*unstructured.Unstructured, error) {
	items, err := r.List(ctx, resourceType)
	if err != nil {
		return nil, fmt.Errorf("listing %s resources: %w", resourceType.Kind, err)
	}
	if len(items) == 0 {
		return nil, apierrors.NewNotFound(
			schema.GroupResource{Group: resourceType.Group, Resource: resourceType.Resource},
			"",
		)
	}
	if len(items) > 1 {
		return nil, fmt.Errorf("expected single %s resource, found %d", resourceType.Kind, len(items))
	}

	return items[0], nil
}

// GetDataScienceCluster retrieves the cluster's DataScienceCluster singleton resource.
func GetDataScienceCluster(ctx context.Context, r Reader) (*unstructured.Unstructured, error) {
	return GetSingleton(ctx, r, resources.DataScienceCluster)
}

// GetDSCInitialization retrieves the cluster's DSCInitialization singleton resource.
func GetDSCInitialization(ctx context.Context, r Reader) (*unstructured.Unstructured, error) {
	return GetSingleton(ctx, r, resources.DSCInitialization)
}

// GetApplicationsNamespace retrieves the applications namespace from DSCInitialization.
// Returns the namespace string and nil error if found. Returns empty string and NotFound
// error if DSCI doesn't exist or if applicationsNamespace is not set or empty. Returns
// empty string and wrapped error for other failures.
func GetApplicationsNamespace(ctx context.Context, r Reader) (string, error) {
	dsci, err := GetDSCInitialization(ctx, r)
	if err != nil {
		return "", err
	}

	namespace, err := jq.Query[string](dsci, ".spec.applicationsNamespace")
	if err != nil {
		if errors.Is(err, jq.ErrNotFound) {
			return "", apierrors.NewNotFound(
				schema.GroupResource{Resource: "applicationsNamespace"},
				"spec.applicationsNamespace",
			)
		}

		return "", fmt.Errorf("querying applicationsNamespace: %w", err)
	}

	if namespace == "" {
		return "", apierrors.NewNotFound(
			schema.GroupResource{Resource: "applicationsNamespace"},
			"spec.applicationsNamespace",
		)
	}

	return namespace, nil
}

// Helper functions for CRD discovery

func crdToGVR(crd *apiextensionsv1.CustomResourceDefinition) schema.GroupVersionResource {
	// Find storage version
	version := ""
	for _, v := range crd.Spec.Versions {
		if v.Storage {
			version = v.Name

			break
		}
	}

	return schema.GroupVersionResource{
		Group:    crd.Spec.Group,
		Version:  version,
		Resource: crd.Spec.Names.Plural,
	}
}

func isCRDEstablished(crd *apiextensionsv1.CustomResourceDefinition) bool {
	for _, condition := range crd.Status.Conditions {
		if condition.Type == apiextensionsv1.Established {
			return condition.Status == apiextensionsv1.ConditionTrue
		}
	}

	return false
}
