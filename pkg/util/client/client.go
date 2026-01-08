package client

import (
	"fmt"

	olmclientset "github.com/operator-framework/operator-lifecycle-manager/pkg/api/client/clientset/versioned"

	apiextensionsclientset "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/metadata"
	"k8s.io/client-go/restmapper"
)

// Client provides access to Kubernetes dynamic and discovery clients.
type Client struct {
	Dynamic       dynamic.Interface
	Discovery     discovery.DiscoveryInterface
	APIExtensions apiextensionsclientset.Interface
	OLM           olmclientset.Interface
	Metadata      metadata.Interface
	RESTMapper    meta.RESTMapper
}

// NewClient creates a unified client with both dynamic and discovery capabilities.
func NewClient(configFlags *genericclioptions.ConfigFlags) (*Client, error) {
	restConfig, err := configFlags.ToRESTConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to create REST config: %w", err)
	}

	dynamicClient, err := dynamic.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create dynamic client: %w", err)
	}

	discoveryClient, err := discovery.NewDiscoveryClientForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create discovery client: %w", err)
	}

	apiExtensionsClient, err := apiextensionsclientset.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create apiextensions client: %w", err)
	}

	olmClient, err := olmclientset.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create OLM client: %w", err)
	}

	metadataClient, err := metadata.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create metadata client: %w", err)
	}

	// Create RESTMapper with caching for efficient GVK→GVR mapping
	restMapper := restmapper.NewDeferredDiscoveryRESTMapper(
		memory.NewMemCacheClient(discoveryClient),
	)

	return &Client{
		Dynamic:       dynamicClient,
		Discovery:     discoveryClient,
		APIExtensions: apiExtensionsClient,
		OLM:           olmClient,
		Metadata:      metadataClient,
		RESTMapper:    restMapper,
	}, nil
}

// NewDynamicClient creates a new dynamic client from ConfigFlags.
func NewDynamicClient(configFlags *genericclioptions.ConfigFlags) (dynamic.Interface, error) {
	restConfig, err := configFlags.ToRESTConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to create REST config: %w", err)
	}

	dynamicClient, err := dynamic.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create dynamic client: %w", err)
	}

	return dynamicClient, nil
}

// NewDiscoveryClient creates a new discovery client from ConfigFlags.
func NewDiscoveryClient(configFlags *genericclioptions.ConfigFlags) (discovery.DiscoveryInterface, error) {
	restConfig, err := configFlags.ToRESTConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to create REST config: %w", err)
	}

	discoveryClient, err := discovery.NewDiscoveryClientForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create discovery client: %w", err)
	}

	return discoveryClient, nil
}
