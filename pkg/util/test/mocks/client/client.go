package client

import (
	"context"

	olmclientset "github.com/operator-framework/operator-lifecycle-manager/pkg/api/client/clientset/versioned"
	"github.com/stretchr/testify/mock"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"

	apiextensionsclientset "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	authorizationv1client "k8s.io/client-go/kubernetes/typed/authorization/v1"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/metadata"

	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
)

// MockClient is a mock implementation of client.Client using testify/mock.
type MockClient struct {
	mock.Mock
}

var _ client.Client = (*MockClient)(nil)

// Reader methods

func (m *MockClient) List(
	ctx context.Context,
	resourceType resources.ResourceType,
	opts ...client.ListResourcesOption,
) ([]*unstructured.Unstructured, error) {
	args := m.Called(ctx, resourceType, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}

	result, _ := args.Get(0).([]*unstructured.Unstructured)

	return result, args.Error(1)
}

func (m *MockClient) ListMetadata(
	ctx context.Context,
	resourceType resources.ResourceType,
	opts ...client.ListResourcesOption,
) ([]*metav1.PartialObjectMetadata, error) {
	args := m.Called(ctx, resourceType, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}

	result, _ := args.Get(0).([]*metav1.PartialObjectMetadata)

	return result, args.Error(1)
}

func (m *MockClient) ListResources(
	ctx context.Context,
	gvr schema.GroupVersionResource,
	opts ...client.ListResourcesOption,
) ([]*unstructured.Unstructured, error) {
	args := m.Called(ctx, gvr, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}

	result, _ := args.Get(0).([]*unstructured.Unstructured)

	return result, args.Error(1)
}

func (m *MockClient) Get(
	ctx context.Context,
	gvr schema.GroupVersionResource,
	name string,
	opts ...client.GetOption,
) (*unstructured.Unstructured, error) {
	args := m.Called(ctx, gvr, name, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}

	result, _ := args.Get(0).(*unstructured.Unstructured)

	return result, args.Error(1)
}

func (m *MockClient) GetResource(
	ctx context.Context,
	resourceType resources.ResourceType,
	name string,
	opts ...client.GetOption,
) (*unstructured.Unstructured, error) {
	args := m.Called(ctx, resourceType, name, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}

	result, _ := args.Get(0).(*unstructured.Unstructured)

	return result, args.Error(1)
}

func (m *MockClient) GetResourceMetadata(
	ctx context.Context,
	resourceType resources.ResourceType,
	name string,
	opts ...client.GetOption,
) (*metav1.PartialObjectMetadata, error) {
	args := m.Called(ctx, resourceType, name, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}

	result, _ := args.Get(0).(*metav1.PartialObjectMetadata)

	return result, args.Error(1)
}

func (m *MockClient) OLM() client.OLMReader {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}

	result, _ := args.Get(0).(client.OLMReader)

	return result
}

// Writer methods

func (m *MockClient) Patch(
	ctx context.Context,
	resourceType resources.ResourceType,
	name string,
	pt types.PatchType,
	data []byte,
	opts ...client.PatchOption,
) (*unstructured.Unstructured, error) {
	args := m.Called(ctx, resourceType, name, pt, data, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}

	result, _ := args.Get(0).(*unstructured.Unstructured)

	return result, args.Error(1)
}

// Client-specific methods (return nil for mocks)

func (m *MockClient) Dynamic() dynamic.Interface {
	return nil
}

func (m *MockClient) Discovery() discovery.DiscoveryInterface {
	return nil
}

func (m *MockClient) APIExtensions() apiextensionsclientset.Interface {
	return nil
}

func (m *MockClient) Metadata() metadata.Interface {
	return nil
}

func (m *MockClient) RESTMapper() meta.RESTMapper {
	return nil
}

func (m *MockClient) ControllerRuntime() crclient.Client {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}

	result, _ := args.Get(0).(crclient.Client)

	return result
}

func (m *MockClient) OLMClient() olmclientset.Interface {
	return nil
}

func (m *MockClient) CoreV1() corev1client.CoreV1Interface {
	return nil
}

func (m *MockClient) AuthorizationV1() authorizationv1client.AuthorizationV1Interface {
	return nil
}
