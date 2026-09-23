package raycluster_test

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	fakeapiextensions "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/fake"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8stesting "k8s.io/client-go/testing"

	"github.com/opendatahub-io/odh-cli/pkg/migrate/raycluster"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
	"github.com/opendatahub-io/odh-cli/pkg/util/iostreams"

	. "github.com/onsi/gomega"
)

// testListKinds maps GVR→ListKind for the fake dynamic client.
//
//nolint:gochecknoglobals // test-only
var testListKinds = map[schema.GroupVersionResource]string{
	resources.Namespace.GVR():            resources.Namespace.ListKind(),
	resources.RayCluster.GVR():           resources.RayCluster.ListKind(),
	resources.ServiceAccount.GVR():       resources.ServiceAccount.ListKind(),
	resources.HTTPRoute.GVR():            resources.HTTPRoute.ListKind(),
	resources.Gateway.GVR():              resources.Gateway.ListKind(),
	resources.Route.GVR():                resources.Route.ListKind(),
	resources.DataScienceCluster.GVR():   resources.DataScienceCluster.ListKind(),
	resources.DataScienceClusterV1.GVR(): resources.DataScienceClusterV1.ListKind(),
}

// gvrForObj returns the GVR for an unstructured object using our resource type definitions.
func gvrForObj(obj *unstructured.Unstructured) schema.GroupVersionResource {
	gvk := obj.GroupVersionKind()
	for gvr, listKind := range testListKinds {
		if gvr.Group == gvk.Group && gvr.Version == gvk.Version &&
			listKind == gvk.Kind+"List" {
			return gvr
		}
	}

	panic(fmt.Sprintf("unknown GVK %s — add it to testListKinds", gvk))
}

func newFakeClient(t *testing.T, objects ...*unstructured.Unstructured) client.Client {
	t.Helper()

	scheme := runtime.NewScheme()
	_ = metav1.AddMetaToScheme(scheme)

	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		scheme, testListKinds,
	)

	ctx := t.Context()
	for _, obj := range objects {
		gvr := gvrForObj(obj)
		ns := obj.GetNamespace()
		var err error
		if ns != "" {
			_, err = dynamicClient.Resource(gvr).Namespace(ns).Create(ctx, obj, metav1.CreateOptions{})
		} else {
			_, err = dynamicClient.Resource(gvr).Create(ctx, obj, metav1.CreateOptions{})
		}
		if err != nil {
			t.Fatalf("creating fake object %s/%s: %v", ns, obj.GetName(), err)
		}
	}

	return client.NewForTesting(client.TestClientConfig{
		Dynamic: dynamicClient,
	})
}

func newFakeClientWithAPIExtensions(t *testing.T, crds []runtime.Object, objects ...*unstructured.Unstructured) client.Client {
	t.Helper()

	scheme := runtime.NewScheme()
	_ = metav1.AddMetaToScheme(scheme)

	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		scheme, testListKinds,
	)

	ctx := t.Context()
	for _, obj := range objects {
		gvr := gvrForObj(obj)
		ns := obj.GetNamespace()
		var err error
		if ns != "" {
			_, err = dynamicClient.Resource(gvr).Namespace(ns).Create(ctx, obj, metav1.CreateOptions{})
		} else {
			_, err = dynamicClient.Resource(gvr).Create(ctx, obj, metav1.CreateOptions{})
		}
		if err != nil {
			t.Fatalf("creating fake object %s/%s: %v", ns, obj.GetName(), err)
		}
	}

	apiextClient := fakeapiextensions.NewSimpleClientset(crds...)

	return client.NewForTesting(client.TestClientConfig{
		Dynamic:       dynamicClient,
		APIExtensions: apiextClient,
	})
}

func makeRayCluster(name, ns string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.RayCluster.APIVersion(),
			"kind":       resources.RayCluster.Kind,
			"metadata": map[string]any{
				"name":      name,
				"namespace": ns,
			},
			"spec": map[string]any{
				"headGroupSpec": map[string]any{
					"enableIngress": true,
					"template": map[string]any{
						"spec": map[string]any{
							"containers": []any{
								map[string]any{"name": "oauth-proxy"},
								map[string]any{"name": "ray-head"},
							},
						},
					},
				},
			},
		},
	}
}

func makeMigratedRayCluster(name, ns string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.RayCluster.APIVersion(),
			"kind":       resources.RayCluster.Kind,
			"metadata": map[string]any{
				"name":      name,
				"namespace": ns,
				"annotations": map[string]any{
					raycluster.SecureNetworkAnnotation: "true",
				},
			},
			"spec": map[string]any{
				"headGroupSpec": map[string]any{
					"template": map[string]any{
						"spec": map[string]any{
							"containers": []any{
								map[string]any{"name": "ray-head"},
							},
						},
					},
				},
			},
		},
	}
}

func makeNamespace(name string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "Namespace",
			"metadata": map[string]any{
				"name": name,
			},
		},
	}
}

func makeServiceAccount(name, ns string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "ServiceAccount",
			"metadata": map[string]any{
				"name":      name,
				"namespace": ns,
			},
		},
	}
}

func makeDSC(name string, components map[string]any) *unstructured.Unstructured {
	obj := map[string]any{
		"apiVersion": resources.DataScienceCluster.APIVersion(),
		"kind":       resources.DataScienceCluster.Kind,
		"metadata": map[string]any{
			"name": name,
		},
		"spec": map[string]any{},
	}
	if components != nil {
		obj["spec"] = map[string]any{
			"components": components,
		}
	}

	return &unstructured.Unstructured{Object: obj}
}

func newTestIO() (iostreams.Interface, *bytes.Buffer, *bytes.Buffer) {
	var outBuf, errBuf bytes.Buffer
	io := iostreams.NewIOStreams(nil, &outBuf, &errBuf)

	return io, &outBuf, &errBuf
}

// --- GetClusters ---

func TestGetClusters_ClusterNameWithoutNamespace(t *testing.T) {
	g := NewWithT(t)
	c := newFakeClient(t)

	_, err := raycluster.GetClusters(t.Context(), c, "my-cluster", "")
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("namespace must be specified"))
}

func TestGetClusters_SpecificCluster(t *testing.T) {
	g := NewWithT(t)
	rc := makeRayCluster("my-cluster", "my-ns")
	c := newFakeClient(t, rc)

	clusters, err := raycluster.GetClusters(t.Context(), c, "my-cluster", "my-ns")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(clusters).To(HaveLen(1))
	g.Expect(clusters[0].GetName()).To(Equal("my-cluster"))
}

func TestGetClusters_SpecificClusterNotFound(t *testing.T) {
	g := NewWithT(t)
	c := newFakeClient(t)

	clusters, err := raycluster.GetClusters(t.Context(), c, "nonexistent", "my-ns")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(clusters).To(BeEmpty())
}

func TestGetClusters_SpecificNamespace(t *testing.T) {
	g := NewWithT(t)
	rc1 := makeRayCluster("cluster-1", "ns-a")
	rc2 := makeRayCluster("cluster-2", "ns-b")
	c := newFakeClient(t, rc1, rc2)

	clusters, err := raycluster.GetClusters(t.Context(), c, "", "ns-a")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(clusters).To(HaveLen(1))
	g.Expect(clusters[0].GetName()).To(Equal("cluster-1"))
}

func TestGetClusters_AllNamespaces(t *testing.T) {
	g := NewWithT(t)
	ns1 := makeNamespace("ns-a")
	ns2 := makeNamespace("ns-b")
	rc1 := makeRayCluster("cluster-1", "ns-a")
	rc2 := makeRayCluster("cluster-2", "ns-b")
	c := newFakeClient(t, ns1, ns2, rc1, rc2)

	clusters, err := raycluster.GetClusters(t.Context(), c, "", "")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(clusters).To(HaveLen(2))
}

func TestGetClusters_AllNamespacesNoClusters(t *testing.T) {
	g := NewWithT(t)
	ns := makeNamespace("ns-a")
	c := newFakeClient(t, ns)

	clusters, err := raycluster.GetClusters(t.Context(), c, "", "")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(clusters).To(BeEmpty())
}

// --- ListRayClusters ---

func TestListRayClusters_NoClusters(t *testing.T) {
	g := NewWithT(t)
	c := newFakeClient(t)
	io, _, errBuf := newTestIO()

	infos, err := raycluster.ListRayClusters(t.Context(), c, "empty-ns", "table", io)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(infos).To(BeNil())
	g.Expect(errBuf.String()).To(ContainSubstring("No RayClusters found"))
}

func TestListRayClusters_TableFormat(t *testing.T) {
	g := NewWithT(t)
	rc := makeRayCluster("my-cluster", "ns-a")
	c := newFakeClient(t, rc)
	io, outBuf, errBuf := newTestIO()

	infos, err := raycluster.ListRayClusters(t.Context(), c, "ns-a", "table", io)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(infos).To(HaveLen(1))
	g.Expect(infos[0].Name).To(Equal("my-cluster"))
	g.Expect(outBuf.String()).To(ContainSubstring("my-cluster"))
	g.Expect(errBuf.String()).To(ContainSubstring("Found 1 RayCluster(s)"))
}

func TestListRayClusters_JSONFormat(t *testing.T) {
	g := NewWithT(t)
	rc := makeRayCluster("my-cluster", "ns-a")
	c := newFakeClient(t, rc)
	io, outBuf, _ := newTestIO()

	infos, err := raycluster.ListRayClusters(t.Context(), c, "ns-a", "json", io)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(infos).To(HaveLen(1))

	out := outBuf.String()
	g.Expect(out).To(ContainSubstring(`"name": "my-cluster"`))
	g.Expect(out).To(ContainSubstring(`"numWorkers"`))
	g.Expect(out).To(ContainSubstring(`"migrationStatus"`))
	g.Expect(out).NotTo(ContainSubstring(`"num_workers"`))
	g.Expect(out).NotTo(ContainSubstring(`"migration_status"`))
}

func TestListRayClusters_YAMLFormat(t *testing.T) {
	g := NewWithT(t)
	rc := makeRayCluster("my-cluster", "ns-a")
	c := newFakeClient(t, rc)
	io, outBuf, _ := newTestIO()

	infos, err := raycluster.ListRayClusters(t.Context(), c, "ns-a", "yaml", io)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(infos).To(HaveLen(1))
	g.Expect(outBuf.String()).To(ContainSubstring("name: my-cluster"))
}

func TestListRayClusters_InvalidFormat(t *testing.T) {
	g := NewWithT(t)
	rc := makeRayCluster("my-cluster", "ns-a")
	c := newFakeClient(t, rc)
	io, _, _ := newTestIO()

	infos, err := raycluster.ListRayClusters(t.Context(), c, "ns-a", "xml", io)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("unsupported output format"))
	g.Expect(infos).To(BeNil())
}

func TestListRayClusters_AllNamespaces(t *testing.T) {
	g := NewWithT(t)
	ns := makeNamespace("ns-a")
	rc1 := makeRayCluster("cluster-1", "ns-a")
	rc2 := makeRayCluster("cluster-2", "ns-a")
	c := newFakeClient(t, ns, rc1, rc2)
	io, _, errBuf := newTestIO()

	infos, err := raycluster.ListRayClusters(t.Context(), c, "ns-a", "table", io)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(infos).To(HaveLen(2))
	g.Expect(errBuf.String()).To(ContainSubstring("namespace 'ns-a'"))
}

// --- GetClusterRoute ---

func TestGetClusterRoute_NoHTTPRoutes(t *testing.T) {
	g := NewWithT(t)
	c := newFakeClient(t)

	url := raycluster.GetClusterRoute(t.Context(), c, "my-cluster", "my-ns")
	g.Expect(url).To(BeEmpty())
}

func makeHTTPRoute(name, ns, clusterName, clusterNS, gwName, gwNS string) *unstructured.Unstructured {
	route := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.HTTPRoute.APIVersion(),
			"kind":       resources.HTTPRoute.Kind,
			"metadata": map[string]any{
				"name":      name,
				"namespace": ns,
			},
			"spec": map[string]any{
				"parentRefs": []any{
					map[string]any{
						"name":      gwName,
						"namespace": gwNS,
					},
				},
			},
		},
	}
	route.SetLabels(map[string]string{
		"ray.io/cluster-name":      clusterName,
		"ray.io/cluster-namespace": clusterNS,
	})

	return route
}

func newFakeClientWithRouteReactor(t *testing.T, routeObjs []*unstructured.Unstructured, otherObjs ...*unstructured.Unstructured) client.Client {
	t.Helper()

	scheme := runtime.NewScheme()
	_ = metav1.AddMetaToScheme(scheme)

	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		scheme, testListKinds,
	)

	ctx := t.Context()
	for _, obj := range routeObjs {
		gvr := gvrForObj(obj)
		ns := obj.GetNamespace()
		if ns != "" {
			_, err := dynamicClient.Resource(gvr).Namespace(ns).Create(ctx, obj, metav1.CreateOptions{})
			if err != nil {
				t.Fatalf("creating fake route object %s/%s: %v", ns, obj.GetName(), err)
			}
		}
	}
	for _, obj := range otherObjs {
		gvr := gvrForObj(obj)
		ns := obj.GetNamespace()
		var err error
		if ns != "" {
			_, err = dynamicClient.Resource(gvr).Namespace(ns).Create(ctx, obj, metav1.CreateOptions{})
		} else {
			_, err = dynamicClient.Resource(gvr).Create(ctx, obj, metav1.CreateOptions{})
		}
		if err != nil {
			t.Fatalf("creating fake object %s/%s: %v", ns, obj.GetName(), err)
		}
	}

	dynamicClient.PrependReactor("list", "httproutes", func(action k8stesting.Action) (bool, runtime.Object, error) {
		listAction := action.(k8stesting.ListAction)
		result := &unstructured.UnstructuredList{}
		result.SetGroupVersionKind(schema.GroupVersionKind{
			Group:   resources.HTTPRoute.Group,
			Version: resources.HTTPRoute.Version,
			Kind:    resources.HTTPRoute.ListKind(),
		})
		for _, obj := range routeObjs {
			if listAction.GetNamespace() != "" && obj.GetNamespace() != listAction.GetNamespace() {
				continue
			}
			result.Items = append(result.Items, *obj)
		}

		return true, result, nil
	})

	return client.NewForTesting(client.TestClientConfig{Dynamic: dynamicClient})
}

func TestGetClusterRoute_FoundViaClusterWide(t *testing.T) {
	g := NewWithT(t)

	httpRoute := makeHTTPRoute("ray-route", "redhat-ods-applications", "my-cluster", "my-ns", "my-gateway", "my-gw-ns")

	gateway := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.Gateway.APIVersion(),
			"kind":       resources.Gateway.Kind,
			"metadata": map[string]any{
				"name":      "my-gateway",
				"namespace": "my-gw-ns",
			},
			"spec": map[string]any{
				"listeners": []any{
					map[string]any{
						"hostname": "ray.example.com",
					},
				},
			},
		},
	}

	c := newFakeClientWithRouteReactor(t, []*unstructured.Unstructured{httpRoute}, gateway)

	url := raycluster.GetClusterRoute(t.Context(), c, "my-cluster", "my-ns")
	g.Expect(url).To(Equal("https://ray.example.com/ray/my-ns/my-cluster"))
}

func TestGetClusterRoute_FallbackToOCPRoute(t *testing.T) {
	g := NewWithT(t)

	httpRoute := makeHTTPRoute("ray-route", "redhat-ods-applications", "my-cluster", "my-ns", "my-gateway", "my-gw-ns")

	gateway := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.Gateway.APIVersion(),
			"kind":       resources.Gateway.Kind,
			"metadata": map[string]any{
				"name":      "my-gateway",
				"namespace": "my-gw-ns",
			},
			"spec": map[string]any{
				"listeners": []any{
					map[string]any{},
				},
			},
		},
	}

	ocpRoute := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.Route.APIVersion(),
			"kind":       resources.Route.Kind,
			"metadata": map[string]any{
				"name":      "my-gateway",
				"namespace": "my-gw-ns",
			},
			"spec": map[string]any{
				"host": "ray-ocp.example.com",
			},
		},
	}

	c := newFakeClientWithRouteReactor(t, []*unstructured.Unstructured{httpRoute}, gateway, ocpRoute)

	url := raycluster.GetClusterRoute(t.Context(), c, "my-cluster", "my-ns")
	g.Expect(url).To(Equal("https://ray-ocp.example.com/ray/my-ns/my-cluster"))
}

func TestGetClusterRoute_NoGatewayHostname(t *testing.T) {
	g := NewWithT(t)

	httpRoute := makeHTTPRoute("ray-route", "redhat-ods-applications", "my-cluster", "my-ns", "my-gateway", "my-gw-ns")

	gateway := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.Gateway.APIVersion(),
			"kind":       resources.Gateway.Kind,
			"metadata": map[string]any{
				"name":      "my-gateway",
				"namespace": "my-gw-ns",
			},
			"spec": map[string]any{
				"listeners": []any{
					map[string]any{},
				},
			},
		},
	}

	c := newFakeClientWithRouteReactor(t, []*unstructured.Unstructured{httpRoute}, gateway)
	url := raycluster.GetClusterRoute(t.Context(), c, "my-cluster", "my-ns")
	g.Expect(url).To(BeEmpty())
}

// --- RunPreUpgradeChecks ---

func TestRunPreUpgradeChecks_AllPass(t *testing.T) {
	g := NewWithT(t)

	ns := makeNamespace("ns-a")
	rc := makeRayCluster("cluster-1", "ns-a")

	certManagerCRD := &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: "certificates.cert-manager.io"},
	}

	dsc := makeDSC("default-dsc", map[string]any{
		"codeflare": map[string]any{"managementState": "Removed"},
	})

	c := newFakeClientWithAPIExtensions(t, []runtime.Object{certManagerCRD}, ns, rc, dsc)

	checks := raycluster.RunPreUpgradeChecks(t.Context(), c, false)
	g.Expect(checks).To(HaveLen(3))
	for _, chk := range checks {
		g.Expect(chk.Passed).To(BeTrue(), "check %s should pass", chk.Name)
	}
}

func TestRunPreUpgradeChecks_PermissionsDenied(t *testing.T) {
	g := NewWithT(t)

	scheme := runtime.NewScheme()
	_ = metav1.AddMetaToScheme(scheme)

	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		scheme, testListKinds,
	)
	dynamicClient.PrependReactor("list", "namespaces", func(_ k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("forbidden: namespaces is forbidden")
	})

	apiextClient := fakeapiextensions.NewSimpleClientset()

	c := client.NewForTesting(client.TestClientConfig{
		Dynamic:       dynamicClient,
		APIExtensions: apiextClient,
	})

	checks := raycluster.RunPreUpgradeChecks(t.Context(), c, false)
	g.Expect(checks).To(HaveLen(3))
	g.Expect(checks[0].Passed).To(BeFalse())
	g.Expect(checks[0].Name).To(Equal("Permissions"))
}

func TestRunPreUpgradeChecks_CertManagerNotFound(t *testing.T) {
	g := NewWithT(t)

	ns := makeNamespace("ns-a")
	rc := makeRayCluster("cluster-1", "ns-a")

	apiextClient := fakeapiextensions.NewSimpleClientset()

	dsc := makeDSC("default-dsc", map[string]any{
		"codeflare": map[string]any{"managementState": "Removed"},
	})

	scheme := runtime.NewScheme()
	_ = metav1.AddMetaToScheme(scheme)
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		scheme, testListKinds, ns, rc, dsc,
	)

	c := client.NewForTesting(client.TestClientConfig{
		Dynamic:       dynamicClient,
		APIExtensions: apiextClient,
	})

	checks := raycluster.RunPreUpgradeChecks(t.Context(), c, false)
	g.Expect(checks).To(HaveLen(3))
	g.Expect(checks[1].Name).To(Equal("cert-manager"))
	g.Expect(checks[1].Passed).To(BeFalse())
	g.Expect(checks[1].Message).To(ContainSubstring("not detected"))
}

func TestRunPreUpgradeChecks_CertManagerNamespaceFound(t *testing.T) {
	g := NewWithT(t)

	certManagerNS := makeNamespace("cert-manager")
	dsc := makeDSC("default-dsc", map[string]any{
		"codeflare": map[string]any{"managementState": "Removed"},
	})

	apiextClient := fakeapiextensions.NewSimpleClientset()

	scheme := runtime.NewScheme()
	_ = metav1.AddMetaToScheme(scheme)
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		scheme, testListKinds, certManagerNS, dsc,
	)

	c := client.NewForTesting(client.TestClientConfig{
		Dynamic:       dynamicClient,
		APIExtensions: apiextClient,
	})

	checks := raycluster.RunPreUpgradeChecks(t.Context(), c, false)
	g.Expect(checks[1].Name).To(Equal("cert-manager"))
	g.Expect(checks[1].Passed).To(BeTrue())
	g.Expect(checks[1].Message).To(ContainSubstring("cert-manager namespace found"))
}

func TestRunPreUpgradeChecks_CodeflareManaged(t *testing.T) {
	g := NewWithT(t)

	ns := makeNamespace("ns-a")
	dsc := makeDSC("default-dsc", map[string]any{
		"codeflare": map[string]any{"managementState": "Managed"},
	})

	certManagerCRD := &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: "certificates.cert-manager.io"},
	}

	c := newFakeClientWithAPIExtensions(t, []runtime.Object{certManagerCRD}, ns, dsc)

	checks := raycluster.RunPreUpgradeChecks(t.Context(), c, false)
	g.Expect(checks[2].Name).To(Equal("codeflare-operator"))
	g.Expect(checks[2].Passed).To(BeFalse())
	g.Expect(checks[2].Message).To(ContainSubstring("Managed"))
}

func TestRunPreUpgradeChecks_CodeflareAutoRemove(t *testing.T) {
	g := NewWithT(t)

	ns := makeNamespace("ns-a")
	dsc := makeDSC("default-dsc", map[string]any{
		"codeflare": map[string]any{"managementState": "Managed"},
	})

	certManagerCRD := &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: "certificates.cert-manager.io"},
	}

	c := newFakeClientWithAPIExtensions(t, []runtime.Object{certManagerCRD}, ns, dsc)

	checks := raycluster.RunPreUpgradeChecks(t.Context(), c, true)
	g.Expect(checks[2].Name).To(Equal("codeflare-operator"))
	g.Expect(checks[2].Passed).To(BeTrue())
	g.Expect(checks[2].Message).To(ContainSubstring("Set codeflare to Removed"))

	// Verify DSC was actually patched
	updated, err := c.Dynamic().Resource(resources.DataScienceCluster.GVR()).
		Get(t.Context(), "default-dsc", metav1.GetOptions{})
	g.Expect(err).ToNot(HaveOccurred())

	state, _, _ := unstructured.NestedString(updated.Object, "spec", "components", "codeflare", "managementState")
	g.Expect(state).To(Equal("Removed"))
}

func TestRunPreUpgradeChecks_CodeflareUnmanaged(t *testing.T) {
	g := NewWithT(t)

	dsc := makeDSC("default-dsc", map[string]any{
		"codeflare": map[string]any{"managementState": "Unmanaged"},
	})

	certManagerCRD := &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: "certificates.cert-manager.io"},
	}

	c := newFakeClientWithAPIExtensions(t, []runtime.Object{certManagerCRD}, dsc)

	checks := raycluster.RunPreUpgradeChecks(t.Context(), c, false)
	g.Expect(checks[2].Passed).To(BeTrue())
	g.Expect(checks[2].Message).To(ContainSubstring("Unmanaged"))
}

func TestRunPreUpgradeChecks_NoDSC(t *testing.T) {
	g := NewWithT(t)

	certManagerCRD := &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: "certificates.cert-manager.io"},
	}

	c := newFakeClientWithAPIExtensions(t, []runtime.Object{certManagerCRD})

	checks := raycluster.RunPreUpgradeChecks(t.Context(), c, false)
	g.Expect(checks[2].Name).To(Equal("codeflare-operator"))
	g.Expect(checks[2].Passed).To(BeTrue())
}

func TestRunPreUpgradeChecks_CodeflareNoManagementState(t *testing.T) {
	g := NewWithT(t)

	dsc := makeDSC("default-dsc", map[string]any{
		"codeflare": map[string]any{},
	})

	certManagerCRD := &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: "certificates.cert-manager.io"},
	}

	c := newFakeClientWithAPIExtensions(t, []runtime.Object{certManagerCRD}, dsc)

	checks := raycluster.RunPreUpgradeChecks(t.Context(), c, false)
	g.Expect(checks[2].Passed).To(BeTrue())
	g.Expect(checks[2].Message).To(ContainSubstring("without managementState"))
}

// --- PostUpgrade (live) ---

func TestPostUpgrade_LiveNoClusters(t *testing.T) {
	g := NewWithT(t)
	ns := makeNamespace("test-ns")
	c := newFakeClient(t, ns)
	io, _, errBuf := newTestIO()

	result, err := raycluster.PostUpgrade(t.Context(), c, raycluster.PostUpgradeOptions{
		Namespace:   "test-ns",
		DryRun:      true,
		SkipConfirm: true,
	}, io)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Migrated).To(Equal(0))
	g.Expect(errBuf.String()).To(ContainSubstring("No RayClusters found"))
}

func TestPostUpgrade_LiveAllMigrated(t *testing.T) {
	g := NewWithT(t)
	rc := makeMigratedRayCluster("my-cluster", "my-ns")
	c := newFakeClient(t, rc)
	io, _, errBuf := newTestIO()

	result, err := raycluster.PostUpgrade(t.Context(), c, raycluster.PostUpgradeOptions{
		Namespace:   "my-ns",
		DryRun:      true,
		SkipConfirm: true,
	}, io)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Skipped).To(Equal(1))
	g.Expect(result.Migrated).To(Equal(0))
	g.Expect(errBuf.String()).To(ContainSubstring("already migrated"))
}

func TestPostUpgrade_LiveDryRun(t *testing.T) {
	g := NewWithT(t)
	rc := makeRayCluster("my-cluster", "my-ns")
	c := newFakeClient(t, rc)
	io, _, errBuf := newTestIO()

	result, err := raycluster.PostUpgrade(t.Context(), c, raycluster.PostUpgradeOptions{
		Namespace:   "my-ns",
		DryRun:      true,
		SkipConfirm: true,
	}, io)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Migrated).To(Equal(1))
	g.Expect(errBuf.String()).To(ContainSubstring("DRY RUN"))
}

func TestPostUpgrade_LiveActual(t *testing.T) {
	g := NewWithT(t)
	rc := makeRayCluster("my-cluster", "my-ns")
	c := newFakeClient(t, rc)
	io, _, errBuf := newTestIO()

	result, err := raycluster.PostUpgrade(t.Context(), c, raycluster.PostUpgradeOptions{
		Namespace:    "my-ns",
		SkipConfirm:  true,
		RouteTimeout: 1,
	}, io)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Migrated).To(Equal(1))
	g.Expect(errBuf.String()).To(ContainSubstring("Migrated: 1"))
}

func TestPostUpgrade_LiveClusterNameWithoutNamespace(t *testing.T) {
	g := NewWithT(t)
	c := newFakeClient(t)
	io, _, _ := newTestIO()

	_, err := raycluster.PostUpgrade(t.Context(), c, raycluster.PostUpgradeOptions{
		ClusterName: "my-cluster",
	}, io)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("namespace must be specified"))
}

func TestPostUpgrade_LiveMultipleClusters(t *testing.T) {
	g := NewWithT(t)
	rc1 := makeRayCluster("cluster-1", "ns-a")
	rc2 := makeMigratedRayCluster("cluster-2", "ns-a")
	rc3 := makeRayCluster("cluster-3", "ns-a")
	c := newFakeClient(t, rc1, rc2, rc3)
	io, _, errBuf := newTestIO()

	result, err := raycluster.PostUpgrade(t.Context(), c, raycluster.PostUpgradeOptions{
		Namespace:    "ns-a",
		DryRun:       true,
		SkipConfirm:  true,
		RouteTimeout: 1,
	}, io)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Migrated).To(Equal(2))
	g.Expect(result.Skipped).To(Equal(1))
	g.Expect(errBuf.String()).To(ContainSubstring("2 to migrate, 1 already migrated"))
}

func TestPostUpgrade_LiveCleanupServiceAccounts(t *testing.T) {
	g := NewWithT(t)
	rc := makeRayCluster("my-cluster", "my-ns")
	oldSA := makeServiceAccount("my-cluster-oauth-proxy-old", "my-ns")
	kuberaySA := makeServiceAccount("my-cluster-oauth-proxy-sa", "my-ns")
	c := newFakeClient(t, rc, oldSA, kuberaySA)
	io, _, errBuf := newTestIO()

	result, err := raycluster.PostUpgrade(t.Context(), c, raycluster.PostUpgradeOptions{
		Namespace:    "my-ns",
		SkipConfirm:  true,
		RouteTimeout: 1,
	}, io)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Migrated).To(Equal(1))
	g.Expect(errBuf.String()).To(ContainSubstring("Deleting old ServiceAccount"))
}

// --- PostUpgrade (from backup) ---

func TestPostUpgrade_BackupPathNotExist(t *testing.T) {
	g := NewWithT(t)
	c := newFakeClient(t)
	io, _, _ := newTestIO()

	_, err := raycluster.PostUpgrade(t.Context(), c, raycluster.PostUpgradeOptions{
		FromBackup: "/nonexistent/path",
	}, io)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("does not exist"))
}

func TestPostUpgrade_BackupNoYAMLFiles(t *testing.T) {
	g := NewWithT(t)
	dir := t.TempDir()
	c := newFakeClient(t)
	io, _, errBuf := newTestIO()

	result, err := raycluster.PostUpgrade(t.Context(), c, raycluster.PostUpgradeOptions{
		FromBackup: dir,
	}, io)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Migrated).To(Equal(0))
	g.Expect(errBuf.String()).To(ContainSubstring("No YAML files found"))
}

func TestPostUpgrade_BackupDryRun(t *testing.T) {
	g := NewWithT(t)
	dir := t.TempDir()

	content := `apiVersion: ray.io/v1
kind: RayCluster
metadata:
  name: backup-cluster
  namespace: my-ns
spec:
  headGroupSpec:
    template:
      spec:
        containers:
        - name: ray-head
`
	g.Expect(os.WriteFile(filepath.Join(dir, "cluster.yaml"), []byte(content), 0o600)).To(Succeed())

	c := newFakeClient(t)
	io, _, errBuf := newTestIO()

	result, err := raycluster.PostUpgrade(t.Context(), c, raycluster.PostUpgradeOptions{
		FromBackup:  dir,
		DryRun:      true,
		SkipConfirm: true,
	}, io)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Migrated).To(Equal(1))
	g.Expect(errBuf.String()).To(ContainSubstring("DRY RUN"))
	g.Expect(errBuf.String()).To(ContainSubstring("Would restore"))
}

func TestPostUpgrade_BackupActualRestore(t *testing.T) {
	g := NewWithT(t)
	dir := t.TempDir()

	content := `apiVersion: ray.io/v1
kind: RayCluster
metadata:
  name: backup-cluster
  namespace: my-ns
spec:
  headGroupSpec:
    template:
      spec:
        containers:
        - name: ray-head
`
	g.Expect(os.WriteFile(filepath.Join(dir, "cluster.yaml"), []byte(content), 0o600)).To(Succeed())

	c := newFakeClient(t)
	io, _, errBuf := newTestIO()

	result, err := raycluster.PostUpgrade(t.Context(), c, raycluster.PostUpgradeOptions{
		FromBackup:   dir,
		SkipConfirm:  true,
		RouteTimeout: 1,
	}, io)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Migrated).To(Equal(1))
	g.Expect(errBuf.String()).To(ContainSubstring("Restored from backup"))
}

func TestPostUpgrade_BackupRestoreExistingCluster(t *testing.T) {
	g := NewWithT(t)
	dir := t.TempDir()

	content := `apiVersion: ray.io/v1
kind: RayCluster
metadata:
  name: existing-cluster
  namespace: my-ns
spec:
  headGroupSpec:
    template:
      spec:
        containers:
        - name: ray-head
`
	g.Expect(os.WriteFile(filepath.Join(dir, "cluster.yaml"), []byte(content), 0o600)).To(Succeed())

	existing := makeRayCluster("existing-cluster", "my-ns")
	c := newFakeClient(t, existing)
	io, _, errBuf := newTestIO()

	result, err := raycluster.PostUpgrade(t.Context(), c, raycluster.PostUpgradeOptions{
		FromBackup:   dir,
		SkipConfirm:  true,
		RouteTimeout: 1,
	}, io)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Migrated).To(Equal(1))
	g.Expect(errBuf.String()).To(ContainSubstring("Deleting existing cluster"))
}

func TestPostUpgrade_BackupNoMatchingClusters(t *testing.T) {
	g := NewWithT(t)
	dir := t.TempDir()

	content := `apiVersion: ray.io/v1
kind: RayCluster
metadata:
  name: backup-cluster
  namespace: my-ns
spec: {}
`
	g.Expect(os.WriteFile(filepath.Join(dir, "cluster.yaml"), []byte(content), 0o600)).To(Succeed())

	c := newFakeClient(t)
	io, _, errBuf := newTestIO()

	result, err := raycluster.PostUpgrade(t.Context(), c, raycluster.PostUpgradeOptions{
		FromBackup:  dir,
		ClusterName: "other-cluster",
		Namespace:   "my-ns",
		SkipConfirm: true,
	}, io)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Migrated).To(Equal(0))
	g.Expect(errBuf.String()).To(ContainSubstring("No matching RayClusters"))
}

func TestPostUpgrade_BackupSingleFile(t *testing.T) {
	g := NewWithT(t)
	dir := t.TempDir()
	f := filepath.Join(dir, "single.yaml")

	content := `apiVersion: ray.io/v1
kind: RayCluster
metadata:
  name: single-cluster
  namespace: test-ns
spec:
  headGroupSpec:
    template:
      spec:
        containers:
        - name: ray-head
`
	g.Expect(os.WriteFile(f, []byte(content), 0o600)).To(Succeed())

	c := newFakeClient(t)
	io, _, errBuf := newTestIO()

	result, err := raycluster.PostUpgrade(t.Context(), c, raycluster.PostUpgradeOptions{
		FromBackup:   f,
		DryRun:       true,
		SkipConfirm:  true,
		RouteTimeout: 1,
	}, io)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Migrated).To(Equal(1))
	g.Expect(errBuf.String()).To(ContainSubstring("single-cluster"))
}

// --- PreUpgrade (with fake client) ---

func TestPreUpgrade_BackupSuccess(t *testing.T) {
	g := NewWithT(t)
	rc := makeRayCluster("my-cluster", "my-ns")
	c := newFakeClient(t, rc)
	io, _, errBuf := newTestIO()

	outputDir := filepath.Join(t.TempDir(), "backups")

	checks := []raycluster.PreflightCheck{
		{Name: "test", Passed: true, Message: "ok", Required: true},
	}

	files, err := raycluster.PreUpgrade(t.Context(), c, outputDir, "", "my-ns", checks, io)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(files).To(HaveLen(1))
	g.Expect(errBuf.String()).To(ContainSubstring("Backed up: my-cluster"))

	_, err = os.Stat(files[0])
	g.Expect(err).ToNot(HaveOccurred())
}

func TestPreUpgrade_NoClusters(t *testing.T) {
	g := NewWithT(t)
	c := newFakeClient(t)
	io, _, errBuf := newTestIO()

	outputDir := filepath.Join(t.TempDir(), "backups")

	checks := []raycluster.PreflightCheck{
		{Name: "test", Passed: true, Message: "ok", Required: true},
	}

	files, err := raycluster.PreUpgrade(t.Context(), c, outputDir, "", "empty-ns", checks, io)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(files).To(BeNil())
	g.Expect(errBuf.String()).To(ContainSubstring("No RayClusters found"))
}

// --- reportNoYAMLFiles ---

func TestReportNoYAMLFiles_WithSubdirectories(t *testing.T) {
	g := NewWithT(t)
	dir := t.TempDir()

	g.Expect(os.Mkdir(filepath.Join(dir, "rhoai-3.x"), 0o750)).To(Succeed())
	g.Expect(os.Mkdir(filepath.Join(dir, "rhoai-2.x"), 0o750)).To(Succeed())

	io, _, errBuf := newTestIO()
	raycluster.ReportNoYAMLFilesForTesting(dir, io)

	g.Expect(errBuf.String()).To(ContainSubstring("Found subdirectories"))
	g.Expect(errBuf.String()).To(ContainSubstring("Hint"))
}

func TestReportNoYAMLFiles_NonexistentPath(t *testing.T) {
	g := NewWithT(t)
	io, _, errBuf := newTestIO()

	raycluster.ReportNoYAMLFilesForTesting("/nonexistent/path", io)
	g.Expect(errBuf.String()).To(ContainSubstring("No YAML files found"))
}

func TestReportNoYAMLFiles_FileNotDir(t *testing.T) {
	g := NewWithT(t)
	dir := t.TempDir()
	f := filepath.Join(dir, "file.txt")
	g.Expect(os.WriteFile(f, []byte("test"), 0o600)).To(Succeed())

	io, _, errBuf := newTestIO()
	raycluster.ReportNoYAMLFilesForTesting(f, io)
	g.Expect(errBuf.String()).To(ContainSubstring("No YAML files found"))
	g.Expect(errBuf.String()).ToNot(ContainSubstring("subdirectories"))
}

func TestReportNoYAMLFiles_EmptyDir(t *testing.T) {
	g := NewWithT(t)
	dir := t.TempDir()

	io, _, errBuf := newTestIO()
	raycluster.ReportNoYAMLFilesForTesting(dir, io)
	g.Expect(errBuf.String()).To(ContainSubstring("No YAML files found"))
	g.Expect(errBuf.String()).ToNot(ContainSubstring("subdirectories"))
}

// --- parseBackupItems ---

func TestParseBackupItems_MultipleMixed(t *testing.T) {
	g := NewWithT(t)
	dir := t.TempDir()

	rayclusterYAML := `apiVersion: ray.io/v1
kind: RayCluster
metadata:
  name: test-cluster
  namespace: test-ns
spec: {}
`
	configmapYAML := `apiVersion: v1
kind: ConfigMap
metadata:
  name: test-cm
`

	g.Expect(os.WriteFile(filepath.Join(dir, "rc.yaml"), []byte(rayclusterYAML), 0o600)).To(Succeed())
	g.Expect(os.WriteFile(filepath.Join(dir, "cm.yaml"), []byte(configmapYAML), 0o600)).To(Succeed())

	io, _, _ := newTestIO()
	items := raycluster.ParseBackupItemsForTesting(
		[]string{filepath.Join(dir, "rc.yaml"), filepath.Join(dir, "cm.yaml")},
		"", "", io,
	)
	g.Expect(items).To(HaveLen(1))
}
