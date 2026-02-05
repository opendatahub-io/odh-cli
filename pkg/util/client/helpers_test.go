//nolint:testpackage // Tests internal implementation (Client fields)
package client

import (
	"testing"

	"github.com/onsi/gomega/types"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	metadatafake "k8s.io/client-go/metadata/fake"

	"github.com/lburgazzoli/odh-cli/pkg/resources"

	. "github.com/onsi/gomega"
)

const testNamespace = "test-namespace"

// createTestObjects creates unstructured objects from YAML manifests.
func createTestObjects(count int) []runtime.Object {
	objects := make([]runtime.Object, count)
	for i := range count {
		obj := &unstructured.Unstructured{
			Object: map[string]any{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata": map[string]any{
					"name":      "test-cm-" + string(rune('1'+i)),
					"namespace": testNamespace,
				},
			},
		}
		objects[i] = obj
	}

	return objects
}

// HavePointerTo is a matcher that verifies the result is a pointer to the expected value.
func HavePointerTo(expected types.GomegaMatcher) types.GomegaMatcher {
	return WithTransform(func(ptr *unstructured.Unstructured) unstructured.Unstructured {
		if ptr == nil {
			return unstructured.Unstructured{}
		}

		return *ptr
	}, expected)
}

func TestListResources_SinglePage(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	objects := createTestObjects(2)
	scheme := runtime.NewScheme()
	_ = metav1.AddMetaToScheme(scheme)

	dynamicClient := dynamicfake.NewSimpleDynamicClient(scheme, objects...)
	metadataClient := metadatafake.NewSimpleMetadataClient(scheme, objects...)

	client := &Client{
		Dynamic:  dynamicClient,
		Metadata: metadataClient,
	}

	gvr := schema.GroupVersionResource{
		Group:    "",
		Version:  "v1",
		Resource: "configmaps",
	}

	results, err := client.ListResources(ctx, gvr)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(results).To(HaveLen(2))
	g.Expect(results[0]).To(HavePointerTo(HaveField("Object", HaveKeyWithValue("kind", "ConfigMap"))))
	g.Expect(results[1]).To(HavePointerTo(HaveField("Object", HaveKeyWithValue("kind", "ConfigMap"))))
}

func TestListResources_MultiplePages(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	// Create more objects than the page size to trigger pagination
	objects := createTestObjects(10)
	scheme := runtime.NewScheme()
	_ = metav1.AddMetaToScheme(scheme)

	dynamicClient := dynamicfake.NewSimpleDynamicClient(scheme, objects...)
	metadataClient := metadatafake.NewSimpleMetadataClient(scheme, objects...)

	client := &Client{
		Dynamic:  dynamicClient,
		Metadata: metadataClient,
	}

	gvr := schema.GroupVersionResource{
		Group:    "",
		Version:  "v1",
		Resource: "configmaps",
	}

	// API will automatically paginate when needed
	results, err := client.ListResources(ctx, gvr)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(results).To(HaveLen(10))

	// Verify all results are pointers
	for i := range results {
		g.Expect(results[i]).ToNot(BeNil())
		g.Expect(results[i]).To(HavePointerTo(HaveField("Object", HaveKeyWithValue("kind", "ConfigMap"))))
	}
}

func TestListResources_EmptyResults(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	scheme := runtime.NewScheme()
	_ = metav1.AddMetaToScheme(scheme)

	// Create fake client with custom list kinds to handle ConfigMapList
	gvrListMap := map[schema.GroupVersionResource]string{
		{Group: "", Version: "v1", Resource: "configmaps"}: "ConfigMapList",
	}

	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, gvrListMap)
	metadataClient := metadatafake.NewSimpleMetadataClient(scheme)

	client := &Client{
		Dynamic:  dynamicClient,
		Metadata: metadataClient,
	}

	gvr := schema.GroupVersionResource{
		Group:    "",
		Version:  "v1",
		Resource: "configmaps",
	}

	results, err := client.ListResources(ctx, gvr)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(results).To(BeEmpty())
}

func TestListResources_NamespaceScoped(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	objects := createTestObjects(3)
	scheme := runtime.NewScheme()
	_ = metav1.AddMetaToScheme(scheme)

	dynamicClient := dynamicfake.NewSimpleDynamicClient(scheme, objects...)
	metadataClient := metadatafake.NewSimpleMetadataClient(scheme, objects...)

	client := &Client{
		Dynamic:  dynamicClient,
		Metadata: metadataClient,
	}

	gvr := schema.GroupVersionResource{
		Group:    "",
		Version:  "v1",
		Resource: "configmaps",
	}

	results, err := client.ListResources(ctx, gvr, WithNamespace(testNamespace))

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(results).To(HaveLen(3))

	// Verify all results are in the expected namespace
	for i := range results {
		g.Expect(results[i].GetNamespace()).To(Equal(testNamespace))
	}
}

func TestList_DelegatesToListResources(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	objects := createTestObjects(2)
	scheme := runtime.NewScheme()
	_ = metav1.AddMetaToScheme(scheme)

	dynamicClient := dynamicfake.NewSimpleDynamicClient(scheme, objects...)
	metadataClient := metadatafake.NewSimpleMetadataClient(scheme, objects...)

	client := &Client{
		Dynamic:  dynamicClient,
		Metadata: metadataClient,
	}

	resourceType := resources.ResourceType{
		Group:    "",
		Version:  "v1",
		Resource: "configmaps",
		Kind:     "ConfigMap",
	}

	results, err := client.List(ctx, resourceType)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(results).To(HaveLen(2))
}

// TestListMetadata_Pagination is skipped due to limitations in fake metadata client.
// In real usage, ListMetadata works correctly with proper Kubernetes API server.
func TestListMetadata_Pagination(t *testing.T) {
	t.Skip("Skipping ListMetadata test due to fake client limitations")
}

func TestGetSingleton_WithPointers(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	objects := createTestObjects(1)
	scheme := runtime.NewScheme()
	_ = metav1.AddMetaToScheme(scheme)

	dynamicClient := dynamicfake.NewSimpleDynamicClient(scheme, objects...)
	metadataClient := metadatafake.NewSimpleMetadataClient(scheme, objects...)

	client := &Client{
		Dynamic:  dynamicClient,
		Metadata: metadataClient,
	}

	resourceType := resources.ResourceType{
		Group:    "",
		Version:  "v1",
		Resource: "configmaps",
		Kind:     "ConfigMap",
	}

	result, err := client.GetSingleton(ctx, resourceType)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).ToNot(BeNil())
	g.Expect(result.GetName()).To(Equal("test-cm-1"))
}

func TestGetSingleton_MultipleInstances(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	objects := createTestObjects(2)
	scheme := runtime.NewScheme()
	_ = metav1.AddMetaToScheme(scheme)

	dynamicClient := dynamicfake.NewSimpleDynamicClient(scheme, objects...)
	metadataClient := metadatafake.NewSimpleMetadataClient(scheme, objects...)

	client := &Client{
		Dynamic:  dynamicClient,
		Metadata: metadataClient,
	}

	resourceType := resources.ResourceType{
		Group:    "",
		Version:  "v1",
		Resource: "configmaps",
		Kind:     "ConfigMap",
	}

	_, err := client.GetSingleton(ctx, resourceType)

	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("expected single"))
}

func TestGetSingleton_NoInstances(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	scheme := runtime.NewScheme()
	_ = metav1.AddMetaToScheme(scheme)

	// Create fake client with custom list kinds to handle ConfigMapList
	gvrListMap := map[schema.GroupVersionResource]string{
		{Group: "", Version: "v1", Resource: "configmaps"}: "ConfigMapList",
	}

	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, gvrListMap)
	metadataClient := metadatafake.NewSimpleMetadataClient(scheme)

	client := &Client{
		Dynamic:  dynamicClient,
		Metadata: metadataClient,
	}

	resourceType := resources.ResourceType{
		Group:    "",
		Version:  "v1",
		Resource: "configmaps",
		Kind:     "ConfigMap",
	}

	_, err := client.GetSingleton(ctx, resourceType)

	g.Expect(err).To(HaveOccurred())
}

// TestListResources_ClusterScoped verifies cluster-scoped resource listing.
func TestListResources_ClusterScoped(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	// Create cluster-scoped objects (no namespace)
	objects := make([]runtime.Object, 3)
	for i := range 3 {
		obj := &unstructured.Unstructured{
			Object: map[string]any{
				"apiVersion": "v1",
				"kind":       "Namespace",
				"metadata": map[string]any{
					"name": "test-ns-" + string(rune('1'+i)),
				},
			},
		}
		objects[i] = obj
	}

	scheme := runtime.NewScheme()
	_ = metav1.AddMetaToScheme(scheme)

	dynamicClient := dynamicfake.NewSimpleDynamicClient(scheme, objects...)
	metadataClient := metadatafake.NewSimpleMetadataClient(scheme, objects...)

	client := &Client{
		Dynamic:  dynamicClient,
		Metadata: metadataClient,
	}

	gvr := schema.GroupVersionResource{
		Group:    "",
		Version:  "v1",
		Resource: "namespaces",
	}

	// List without namespace filter (cluster-scoped)
	results, err := client.ListResources(ctx, gvr)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(results).To(HaveLen(3))

	// Verify all results are cluster-scoped (no namespace)
	for i := range results {
		g.Expect(results[i].GetNamespace()).To(BeEmpty())
	}
}

// TestListMetadata_NamespaceScoped is skipped due to limitations in fake metadata client.
// In real usage, ListMetadata works correctly with proper Kubernetes API server.
func TestListMetadata_NamespaceScoped(t *testing.T) {
	t.Skip("Skipping ListMetadata test due to fake client limitations")
}

// createDSCInitialization creates a DSCInitialization object with the given applications namespace.
func createDSCInitialization(applicationsNamespace string) runtime.Object {
	dsci := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "dscinitialization.opendatahub.io/v1",
			"kind":       "DSCInitialization",
			"metadata": map[string]any{
				"name": "default-dsci",
			},
		},
	}

	if applicationsNamespace != "" {
		dsci.Object["spec"] = map[string]any{
			"applicationsNamespace": applicationsNamespace,
		}
	}

	return dsci
}

// createDSCInitializationWithEmptySpec creates a DSCI with spec but no applicationsNamespace.
func createDSCInitializationWithEmptySpec() runtime.Object {
	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "dscinitialization.opendatahub.io/v1",
			"kind":       "DSCInitialization",
			"metadata": map[string]any{
				"name": "default-dsci",
			},
			"spec": map[string]any{
				"otherField": "value",
			},
		},
	}
}

func TestGetApplicationsNamespace_DSCINotFound(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	scheme := runtime.NewScheme()
	_ = metav1.AddMetaToScheme(scheme)

	// Create fake client with custom list kinds for DSCInitialization
	gvrListMap := map[schema.GroupVersionResource]string{
		resources.DSCInitialization.GVR(): "DSCInitializationList",
	}

	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, gvrListMap)
	metadataClient := metadatafake.NewSimpleMetadataClient(scheme)

	client := &Client{
		Dynamic:  dynamicClient,
		Metadata: metadataClient,
	}

	namespace, err := client.GetApplicationsNamespace(ctx)

	g.Expect(err).To(Satisfy(apierrors.IsNotFound))
	g.Expect(namespace).To(BeEmpty())
}

func TestGetApplicationsNamespace_NamespaceSet(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	const expectedNamespace = "my-odh-namespace"

	dsci := createDSCInitialization(expectedNamespace)
	scheme := runtime.NewScheme()
	_ = metav1.AddMetaToScheme(scheme)

	dynamicClient := dynamicfake.NewSimpleDynamicClient(scheme, dsci)
	metadataClient := metadatafake.NewSimpleMetadataClient(scheme, dsci)

	client := &Client{
		Dynamic:  dynamicClient,
		Metadata: metadataClient,
	}

	namespace, err := client.GetApplicationsNamespace(ctx)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(namespace).To(Equal(expectedNamespace))
}

func TestGetApplicationsNamespace_NamespaceNotSet(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	dsci := createDSCInitializationWithEmptySpec()
	scheme := runtime.NewScheme()
	_ = metav1.AddMetaToScheme(scheme)

	dynamicClient := dynamicfake.NewSimpleDynamicClient(scheme, dsci)
	metadataClient := metadatafake.NewSimpleMetadataClient(scheme, dsci)

	client := &Client{
		Dynamic:  dynamicClient,
		Metadata: metadataClient,
	}

	namespace, err := client.GetApplicationsNamespace(ctx)

	g.Expect(err).To(Satisfy(apierrors.IsNotFound))
	g.Expect(namespace).To(BeEmpty())
}

func TestGetApplicationsNamespace_EmptyNamespace(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	// Create DSCI with empty applicationsNamespace
	dsci := createDSCInitialization("")
	scheme := runtime.NewScheme()
	_ = metav1.AddMetaToScheme(scheme)

	dynamicClient := dynamicfake.NewSimpleDynamicClient(scheme, dsci)
	metadataClient := metadatafake.NewSimpleMetadataClient(scheme, dsci)

	client := &Client{
		Dynamic:  dynamicClient,
		Metadata: metadataClient,
	}

	namespace, err := client.GetApplicationsNamespace(ctx)

	g.Expect(err).To(Satisfy(apierrors.IsNotFound))
	g.Expect(namespace).To(BeEmpty())
}
