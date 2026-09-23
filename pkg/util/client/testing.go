package client

import (
	olmclientset "github.com/operator-framework/operator-lifecycle-manager/pkg/api/client/clientset/versioned"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"

	apiextensionsclientset "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/metadata"
)

// TestClientConfig holds all sub-clients for constructing a test client.
type TestClientConfig struct {
	Dynamic           dynamic.Interface
	Discovery         discovery.DiscoveryInterface
	APIExtensions     apiextensionsclientset.Interface
	OLM               olmclientset.Interface
	Metadata          metadata.Interface
	Kubernetes        kubernetes.Interface
	RESTMapper        meta.RESTMapper
	ControllerRuntime crclient.Client
}

// NewForTesting creates a Client for use in tests.
// Only the sub-clients that are needed for the test need to be populated.
func NewForTesting(cfg TestClientConfig) Client {
	return &defaultClient{
		dynamic:           cfg.Dynamic,
		discovery:         cfg.Discovery,
		apiExtensions:     cfg.APIExtensions,
		olm:               cfg.OLM,
		metadata:          cfg.Metadata,
		kubernetes:        cfg.Kubernetes,
		restMapper:        cfg.RESTMapper,
		controllerRuntime: cfg.ControllerRuntime,
		olmReader:         newOLMReader(cfg.OLM),
	}
}
