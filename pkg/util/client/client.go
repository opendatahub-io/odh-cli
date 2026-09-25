package client

import (
	"context"
	"errors"
	"fmt"

	olmclientset "github.com/operator-framework/operator-lifecycle-manager/pkg/api/client/clientset/versioned"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"

	apiextensionsclientset "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	authorizationv1client "k8s.io/client-go/kubernetes/typed/authorization/v1"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/metadata"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"

	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util"
)

// Compile-time verification that defaultClient implements Client (and therefore Reader + Writer).
var _ Client = (*defaultClient)(nil)

// defaultClient is the concrete implementation of Client, Reader, and Writer.
type defaultClient struct {
	dynamic           dynamic.Interface
	discovery         discovery.DiscoveryInterface
	apiExtensions     apiextensionsclientset.Interface
	olm               olmclientset.Interface
	metadata          metadata.Interface
	kubernetes        kubernetes.Interface
	restMapper        meta.RESTMapper
	controllerRuntime crclient.Client

	olmReader OLMReader
}

func (c *defaultClient) Dynamic() dynamic.Interface                      { return c.dynamic }
func (c *defaultClient) Discovery() discovery.DiscoveryInterface         { return c.discovery }
func (c *defaultClient) APIExtensions() apiextensionsclientset.Interface { return c.apiExtensions }
func (c *defaultClient) Metadata() metadata.Interface                    { return c.metadata }
func (c *defaultClient) RESTMapper() meta.RESTMapper                     { return c.restMapper }
func (c *defaultClient) ControllerRuntime() crclient.Client              { return c.controllerRuntime }
func (c *defaultClient) OLM() OLMReader                                  { return c.olmReader }
func (c *defaultClient) OLMClient() olmclientset.Interface               { return c.olm }
func (c *defaultClient) CoreV1() corev1client.CoreV1Interface {
	if c.kubernetes == nil {
		panic("CoreV1 called on a client with no kubernetes client configured")
	}

	return c.kubernetes.CoreV1()
}

func (c *defaultClient) AuthorizationV1() authorizationv1client.AuthorizationV1Interface {
	if c.kubernetes == nil {
		panic("AuthorizationV1 called on a client with no kubernetes client configured")
	}

	return c.kubernetes.AuthorizationV1()
}

// NewClientWithConfig creates a client from a pre-configured REST config.
// This allows callers to customize throttling settings before client creation.
func NewClientWithConfig(restConfig *rest.Config) (Client, error) {
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

	kubeClient, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	controllerRuntimeClient, err := NewControllerRuntimeClient(restConfig)
	if err != nil {
		return nil, err
	}

	// Create RESTMapper with caching for efficient GVK->GVR mapping.
	restMapper := restmapper.NewDeferredDiscoveryRESTMapper(
		memory.NewMemCacheClient(discoveryClient),
	)

	return &defaultClient{
		dynamic:           dynamicClient,
		discovery:         discoveryClient,
		apiExtensions:     apiExtensionsClient,
		olm:               olmClient,
		metadata:          metadataClient,
		kubernetes:        kubeClient,
		restMapper:        restMapper,
		controllerRuntime: controllerRuntimeClient,
		olmReader:         newOLMReader(olmClient),
	}, nil
}

// NewClient creates a unified client with default throttling settings.
// The client is configured with appropriate throttling for parallel CLI operations.
func NewClient(configFlags *genericclioptions.ConfigFlags) (Client, error) {
	restConfig, err := NewRESTConfig(configFlags, DefaultQPS, DefaultBurst)
	if err != nil {
		return nil, fmt.Errorf("failed to create REST config: %w", err)
	}

	return NewClientWithConfig(restConfig)
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

// Patch applies a patch to an existing cluster-scoped resource.
func (c *defaultClient) Patch(
	ctx context.Context,
	resourceType resources.ResourceType,
	name string,
	patchType types.PatchType,
	data []byte,
	opts ...PatchOption,
) (*unstructured.Unstructured, error) {
	if name == "" {
		return nil, errors.New("resource name cannot be empty")
	}

	if len(data) == 0 {
		return nil, errors.New("patch data cannot be empty")
	}

	cfg := &PatchConfig{}
	util.ApplyOptions(cfg, opts...)

	patchOpts := metav1.PatchOptions{}
	if cfg.DryRun {
		patchOpts.DryRun = []string{metav1.DryRunAll}
	}

	if cfg.FieldOwner != "" {
		patchOpts.FieldManager = cfg.FieldOwner
	}

	gvr := resourceType.GVR()

	result, err := c.dynamic.Resource(gvr).Patch(ctx, name, patchType, data, patchOpts)
	if err != nil {
		return nil, fmt.Errorf("patching resource: %w", err)
	}

	return result, nil
}
