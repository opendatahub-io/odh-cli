package client

import (
	"context"

	operatorsv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	olmclientset "github.com/operator-framework/operator-lifecycle-manager/pkg/api/client/clientset/versioned"

	apiextensionsclientset "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/metadata"

	"github.com/opendatahub-io/odh-cli/pkg/resources"
)

// Reader provides read-only access to Kubernetes resources.
// Used by lint checks to enforce that no write operations can occur.
type Reader interface {
	// List lists all instances of a resource type handling pagination automatically.
	List(
		ctx context.Context,
		resourceType resources.ResourceType,
		opts ...ListResourcesOption,
	) ([]*unstructured.Unstructured, error)

	// ListMetadata lists all instances of a resource type returning only metadata.
	ListMetadata(
		ctx context.Context,
		resourceType resources.ResourceType,
		opts ...ListResourcesOption,
	) ([]*metav1.PartialObjectMetadata, error)

	// ListResources lists all instances of a resource by GVR handling pagination automatically.
	ListResources(
		ctx context.Context,
		gvr schema.GroupVersionResource,
		opts ...ListResourcesOption,
	) ([]*unstructured.Unstructured, error)

	// Get retrieves a single resource by GVR and name.
	Get(
		ctx context.Context,
		gvr schema.GroupVersionResource,
		name string,
		opts ...GetOption,
	) (*unstructured.Unstructured, error)

	// GetResource retrieves a single resource by ResourceType and name.
	GetResource(
		ctx context.Context,
		resourceType resources.ResourceType,
		name string,
		opts ...GetOption,
	) (*unstructured.Unstructured, error)

	// GetResourceMetadata retrieves only the metadata of a single resource.
	// Use this when you only need name, namespace, labels, or annotations.
	GetResourceMetadata(
		ctx context.Context,
		resourceType resources.ResourceType,
		name string,
		opts ...GetOption,
	) (*metav1.PartialObjectMetadata, error)

	// OLM returns a read-only accessor for OLM resources (subscriptions, CSVs).
	OLM() OLMReader
}

// Writer provides write access to Kubernetes resources.
// Currently empty -- write operations will be added as needed.
type Writer any

// Client provides full access to Kubernetes resources.
// Embeds Reader and Writer, and exposes the underlying clientsets
// for callers that need low-level or write access.
type Client interface {
	Reader
	Writer

	// Dynamic returns the dynamic Kubernetes client.
	Dynamic() dynamic.Interface

	// Discovery returns the Kubernetes discovery client.
	Discovery() discovery.DiscoveryInterface

	// APIExtensions returns the API extensions client for CRD operations.
	APIExtensions() apiextensionsclientset.Interface

	// Metadata returns the metadata-only client for efficient resource listing.
	Metadata() metadata.Interface

	// RESTMapper returns the REST mapper for GVK/GVR resolution.
	RESTMapper() meta.RESTMapper

	// OLMClient returns the full OLM clientset for write operations (subscriptions, CSVs).
	// Use OLM() from Reader for read-only access.
	OLMClient() olmclientset.Interface
}

// OLMReader provides read-only access to OLM resources.
type OLMReader interface {
	// Available returns true if OLM is available in the cluster.
	Available() bool

	// Subscriptions returns a read-only accessor for OLM subscriptions in the given namespace.
	// Use empty string for all namespaces.
	Subscriptions(namespace string) SubscriptionReader

	// ClusterServiceVersions returns a read-only accessor for CSVs in the given namespace.
	// Use empty string for all namespaces.
	ClusterServiceVersions(namespace string) CSVReader
}

// SubscriptionReader provides read-only access to OLM Subscription resources.
type SubscriptionReader interface {
	List(ctx context.Context, opts metav1.ListOptions) (*operatorsv1alpha1.SubscriptionList, error)
	Get(ctx context.Context, name string, opts metav1.GetOptions) (*operatorsv1alpha1.Subscription, error)
}

// CSVReader provides read-only access to OLM ClusterServiceVersion resources.
type CSVReader interface {
	List(ctx context.Context, opts metav1.ListOptions) (*operatorsv1alpha1.ClusterServiceVersionList, error)
	Get(ctx context.Context, name string, opts metav1.GetOptions) (*operatorsv1alpha1.ClusterServiceVersion, error)
}
