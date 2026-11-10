//nolint:testpackage // Tests internal implementation
package metrics

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blang/semver/v4"
	"github.com/spf13/pflag"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/rest"

	"github.com/opendatahub-io/odh-cli/pkg/migrate/action"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/action/result"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
	"github.com/opendatahub-io/odh-cli/pkg/util/iostreams"

	. "github.com/onsi/gomega"
)

// ---------------------------------------------------------------------------
// Shared test helpers
// ---------------------------------------------------------------------------

func newScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	_ = metav1.AddMetaToScheme(scheme)

	return scheme
}

func newFakeClient(scheme *runtime.Scheme, objects ...runtime.Object) client.Client {
	listKinds := map[schema.GroupVersionResource]string{
		resources.Route.GVR():           resources.Route.ListKind(),
		resources.TrustyAIService.GVR(): resources.TrustyAIService.ListKind(),
	}

	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, listKinds, objects...)

	return client.NewForTesting(client.TestClientConfig{
		Dynamic: dynamicClient,
	})
}

func newTarget(k8sClient client.Client, opts ...func(*action.Target)) action.Target {
	currentVersion := semver.MustParse("2.25.0")
	targetVersion := semver.MustParse("3.0.0")

	io := iostreams.NewIOStreams(
		&bytes.Buffer{},
		&bytes.Buffer{},
		&bytes.Buffer{},
	)

	target := action.Target{
		Client:         k8sClient,
		CurrentVersion: &currentVersion,
		TargetVersion:  &targetVersion,
		DryRun:         false,
		SkipConfirm:    true,
		Recorder:       action.NewVerboseRootRecorder(io),
		IO:             io,
	}

	for _, opt := range opts {
		opt(&target)
	}

	return target
}

func withDryRun(t *action.Target) {
	t.DryRun = true
}

func newRoute(name, namespace, host string, labels map[string]string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.Route.APIVersion(),
			"kind":       resources.Route.Kind,
			"metadata": map[string]any{
				"name":      name,
				"namespace": namespace,
			},
			"spec": map[string]any{
				"host": host,
			},
		},
	}

	if labels != nil {
		obj.SetLabels(labels)
	}

	return obj
}

func newTrustyAIService(name, namespace string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.TrustyAIService.APIVersion(),
			"kind":       resources.TrustyAIService.Kind,
			"metadata": map[string]any{
				"name":      name,
				"namespace": namespace,
			},
		},
	}
}

type actionRecorder struct {
	action.StepRecorder

	action action.Action
}

func (r *actionRecorder) Action() action.Action { return r.action }

func (r *actionRecorder) Build() *result.ActionResult {
	if root, ok := r.StepRecorder.(action.RootRecorder); ok {
		return root.Build()
	}

	return nil
}

func metricsJSON(t *testing.T, requests ...map[string]any) []byte {
	t.Helper()

	data, err := json.Marshal(map[string]any{"requests": requests})
	if err != nil {
		t.Fatal(err)
	}

	return data
}

func newMetricEntry(metricName, modelID string) map[string]any {
	return map[string]any{
		"id": metricName + "-" + modelID,
		"request": map[string]any{
			"metricName": metricName,
			"modelId":    modelID,
		},
	}
}

func serverHost(server *httptest.Server) string {
	return strings.TrimPrefix(server.URL, "https://")
}

// ---------------------------------------------------------------------------
// MetricsAction metadata
// ---------------------------------------------------------------------------

func TestMetricsAction_Metadata(t *testing.T) {
	g := NewWithT(t)

	a := &MetricsAction{}

	g.Expect(a.ID()).To(Equal("trustyai.metrics"))
	g.Expect(a.Name()).To(Equal("Backup and restore TrustyAI scheduled metrics"))
	g.Expect(a.Description()).To(ContainSubstring("metrics"))
	g.Expect(a.Group()).To(Equal(action.GroupBackup))
	g.Expect(a.Phase()).To(Equal(action.PhasePreUpgrade))
	g.Expect(a.CanApply(action.Target{})).To(BeTrue())
	g.Expect(a.Prepare()).ToNot(BeNil())
	g.Expect(a.Run()).ToNot(BeNil())
}

func TestMetricsAction_AddFlags(t *testing.T) {
	g := NewWithT(t)

	a := &MetricsAction{}
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	a.AddFlags(fs)

	g.Expect(fs.Lookup("metrics-dir")).ToNot(BeNil())
	g.Expect(fs.Lookup("metrics-file")).ToNot(BeNil())
	g.Expect(fs.Lookup("metrics-type")).ToNot(BeNil())
	g.Expect(fs.Lookup("metrics-skip-existing")).ToNot(BeNil())
	g.Expect(fs.Lookup("metrics-route-label")).ToNot(BeNil())
}

// ---------------------------------------------------------------------------
// extractRouteHost
// ---------------------------------------------------------------------------

func TestExtractRouteHost(t *testing.T) {
	tests := []struct {
		name     string
		route    *unstructured.Unstructured
		expected string
	}{
		{
			"valid host",
			newRoute("r1", "ns1", "trustyai.example.com", nil),
			"trustyai.example.com",
		},
		{
			"empty host",
			newRoute("r1", "ns1", "", nil),
			"",
		},
		{
			"no spec",
			&unstructured.Unstructured{Object: map[string]any{
				"apiVersion": "route.openshift.io/v1",
				"kind":       "Route",
				"metadata":   map[string]any{"name": "r1", "namespace": "ns1"},
			}},
			"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			host, err := extractRouteHost(tt.route)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(host).To(Equal(tt.expected))
		})
	}
}

// ---------------------------------------------------------------------------
// findRouteByLabel / findRouteByName
// ---------------------------------------------------------------------------

func TestFindRouteByLabel_Found(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	route := newRoute("tai-route", "ns1", "trustyai.example.com", map[string]string{
		"trustyai-service-name": "trustyai-service",
	})

	k8sClient := newFakeClient(newScheme(), route)

	host, err := findRouteByLabel(ctx, k8sClient, "ns1", "trustyai-service-name=trustyai-service")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(host).To(Equal("trustyai.example.com"))
}

func TestFindRouteByLabel_NotFound(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	k8sClient := newFakeClient(newScheme())

	host, err := findRouteByLabel(ctx, k8sClient, "ns1", "nonexistent=label")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(host).To(Equal(""))
}

func TestFindRouteByName_Found(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	route := newRoute("trustyai-service", "ns1", "trustyai.example.com", nil)
	k8sClient := newFakeClient(newScheme(), route)

	host, err := findRouteByName(ctx, k8sClient, "ns1", "trustyai-service")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(host).To(Equal("trustyai.example.com"))
}

func TestFindRouteByName_NotFound(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	k8sClient := newFakeClient(newScheme())

	host, err := findRouteByName(ctx, k8sClient, "ns1", "nonexistent")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(host).To(Equal(""))
}

// ---------------------------------------------------------------------------
// discoverRouteInNamespace
// ---------------------------------------------------------------------------

func TestDiscoverRouteInNamespace_ByPrimaryLabel(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	route := newRoute("tai-route", "ns1", "primary.example.com", map[string]string{
		"trustyai-service-name": "trustyai-service",
	})

	k8sClient := newFakeClient(newScheme(), route)

	host, err := discoverRouteInNamespace(ctx, k8sClient, "ns1", defaultRouteLabel)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(host).To(Equal("primary.example.com"))
}

func TestDiscoverRouteInNamespace_ByName(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	route := newRoute("trustyai-service", "ns1", "byname.example.com", nil)
	k8sClient := newFakeClient(newScheme(), route)

	host, err := discoverRouteInNamespace(ctx, k8sClient, "ns1", "nonexistent=label")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(host).To(Equal("byname.example.com"))
}

func TestDiscoverRouteInNamespace_FallbackLabel(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	route := newRoute("tai-route", "ns1", "fallback.example.com", map[string]string{
		"app": "trustyai-service",
	})

	k8sClient := newFakeClient(newScheme(), route)

	host, err := discoverRouteInNamespace(ctx, k8sClient, "ns1", "nonexistent=label")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(host).To(Equal("fallback.example.com"))
}

func TestDiscoverRouteInNamespace_NotFound(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	k8sClient := newFakeClient(newScheme())

	host, err := discoverRouteInNamespace(ctx, k8sClient, "ns1", defaultRouteLabel)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(host).To(Equal(""))
}

// ---------------------------------------------------------------------------
// discoverTrustyAINamespaces
// ---------------------------------------------------------------------------

func TestDiscoverTrustyAINamespaces_NoRoutes(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	target := newTarget(newFakeClient(newScheme()))

	ns, err := discoverTrustyAINamespaces(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(ns).To(BeEmpty())
}

func TestDiscoverTrustyAINamespaces_WithRoutes(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	route := newRoute("tai-route", "ns1", "tai.example.com", map[string]string{
		"trustyai-service-name": "trustyai-service",
	})

	target := newTarget(newFakeClient(newScheme(), route))

	ns, err := discoverTrustyAINamespaces(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(ns).To(HaveLen(1))
	g.Expect(ns["ns1"]).To(Equal("tai.example.com"))
}

func TestDiscoverTrustyAINamespaces_FallbackToTrustyAIService(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	svc := newTrustyAIService("trustyai", "ns1")
	route := newRoute("trustyai-service", "ns1", "tai.example.com", nil)

	target := newTarget(newFakeClient(newScheme(), svc, route))

	ns, err := discoverTrustyAINamespaces(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(ns).To(HaveLen(1))
}

func TestDiscoverTrustyAINamespaces_TrustyAIServiceNoRoutes(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	svc := newTrustyAIService("trustyai", "ns1")
	target := newTarget(newFakeClient(newScheme(), svc))

	ns, err := discoverTrustyAINamespaces(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(ns).To(BeEmpty())
}

func TestDiscoverTrustyAINamespaces_MultipleRoutesInNamespace(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	route1 := newRoute("route1", "ns1", "host1.example.com", map[string]string{
		"trustyai-service-name": "trustyai-service",
	})
	route2 := newRoute("route2", "ns1", "host2.example.com", nil)

	target := newTarget(newFakeClient(newScheme(), route1, route2))

	ns, err := discoverTrustyAINamespaces(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(ns).To(HaveLen(1))
	g.Expect(ns["ns1"]).To(Equal("host1.example.com"))
}

func TestDiscoverTrustyAINamespaces_CustomRouteLabel(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	route := newRoute("tai-route", "ns1", "custom.example.com", map[string]string{
		"custom-label": "custom-value",
	})

	metricsAction := &MetricsAction{RouteLabel: "custom-label=custom-value"}

	target := newTarget(newFakeClient(newScheme(), route))
	target.Recorder = &actionRecorder{
		StepRecorder: target.Recorder,
		action:       metricsAction,
	}

	ns, err := discoverTrustyAINamespaces(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(ns).To(HaveLen(1))
	g.Expect(ns["ns1"]).To(Equal("custom.example.com"))
}

func TestDiscoverTrustyAINamespaces_TrustyAIFallbackWithRoute(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	unrelatedRoute := newRoute("other-route", "other-ns", "other.example.com", nil)
	svc := newTrustyAIService("trustyai", "ns1")
	serviceRoute := newRoute("trustyai-service", "ns1", "tai.example.com", nil)

	target := newTarget(newFakeClient(newScheme(), unrelatedRoute, svc, serviceRoute))

	ns, err := discoverTrustyAINamespaces(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(ns).To(HaveKeyWithValue("ns1", "tai.example.com"))
}

func TestDiscoverTrustyAINamespaces_RouteNoHost(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	route := newRoute("tai-route", "ns1", "", nil)
	target := newTarget(newFakeClient(newScheme(), route))

	ns, err := discoverTrustyAINamespaces(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(ns).To(BeEmpty())
}

// ---------------------------------------------------------------------------
// newHTTPHelper
// ---------------------------------------------------------------------------

func TestNewHTTPHelper_NilConfig(t *testing.T) {
	g := NewWithT(t)

	h, err := newHTTPHelper(nil)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(h.bearerToken).To(Equal(""))
}

func TestNewHTTPHelper_WithToken(t *testing.T) {
	g := NewWithT(t)

	cfg := &rest.Config{BearerToken: "my-token"}

	h, err := newHTTPHelper(cfg)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(h.bearerToken).To(Equal("my-token"))
}

func TestNewHTTPHelper_WithTokenFile(t *testing.T) {
	g := NewWithT(t)

	tokenFile := filepath.Join(t.TempDir(), "token")
	_ = os.WriteFile(tokenFile, []byte("file-token\n"), 0o600)

	cfg := &rest.Config{BearerTokenFile: tokenFile}

	h, err := newHTTPHelper(cfg)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(h.bearerToken).To(Equal("file-token"))
}

func TestNewHTTPHelper_TokenFileMissing(t *testing.T) {
	g := NewWithT(t)

	cfg := &rest.Config{BearerTokenFile: "/nonexistent/token"}

	_, err := newHTTPHelper(cfg)
	g.Expect(err).To(HaveOccurred())
}

// ---------------------------------------------------------------------------
// httpHelper.get / httpHelper.post
// ---------------------------------------------------------------------------

func TestHTTPHelper_Get(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		g.Expect(r.Method).To(Equal(http.MethodGet))
		g.Expect(r.Header.Get("Authorization")).To(Equal("Bearer test-token"))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"requests":[]}`))
	}))
	defer server.Close()

	h := &httpHelper{client: server.Client(), bearerToken: "test-token"}
	body, err := h.get(ctx, server.URL+"/metrics/all/requests")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(string(body)).To(ContainSubstring("requests"))
}

func TestHTTPHelper_Get_Error(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("internal error"))
	}))
	defer server.Close()

	h := &httpHelper{client: server.Client(), bearerToken: ""}
	_, err := h.get(ctx, server.URL+"/fail")
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("HTTP 500"))
}

func TestHTTPHelper_Get_NoToken(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		g.Expect(r.Header.Get("Authorization")).To(Equal(""))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("{}"))
	}))
	defer server.Close()

	h := &httpHelper{client: server.Client(), bearerToken: ""}
	_, err := h.get(ctx, server.URL+"/test")
	g.Expect(err).ToNot(HaveOccurred())
}

func TestHTTPHelper_Post(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		g.Expect(r.Method).To(Equal(http.MethodPost))
		g.Expect(r.Header.Get("Content-Type")).To(Equal("application/json"))
		g.Expect(r.Header.Get("Authorization")).To(Equal("Bearer test-token"))
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	h := &httpHelper{client: server.Client(), bearerToken: "test-token"}
	status, err := h.post(ctx, server.URL+"/metrics/group/fairness/spd/request", []byte(`{"modelId":"m1"}`))
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(status).To(Equal(http.StatusOK))
}

func TestHTTPHelper_Post_NoToken(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		g.Expect(r.Header.Get("Authorization")).To(Equal(""))
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	h := &httpHelper{client: server.Client(), bearerToken: ""}
	status, err := h.post(ctx, server.URL+"/test", []byte("{}"))
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(status).To(Equal(http.StatusCreated))
}

// ---------------------------------------------------------------------------
// fetchExistingMetrics
// ---------------------------------------------------------------------------

func TestFetchExistingMetrics(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(metricsJSON(t,
			newMetricEntry("SPD", "model-a"),
			newMetricEntry("DIR", "model-b"),
		))
	}))
	defer server.Close()

	h := &httpHelper{client: server.Client()}
	existing, err := fetchExistingMetrics(ctx, h, serverHost(server))
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(existing).To(HaveKey("model-a:SPD"))
	g.Expect(existing).To(HaveKey("model-b:DIR"))
}

func TestFetchExistingMetrics_Empty(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"requests":[]}`))
	}))
	defer server.Close()

	h := &httpHelper{client: server.Client()}
	existing, err := fetchExistingMetrics(ctx, h, serverHost(server))
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(existing).To(BeEmpty())
}

// ---------------------------------------------------------------------------
// prepareTask.Validate
// ---------------------------------------------------------------------------

func TestPrepareTask_Validate_NoRoutes(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	target := newTarget(newFakeClient(newScheme()))

	a := &MetricsAction{}
	result, err := a.Prepare().Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Completed).To(BeTrue())
}

func TestPrepareTask_Validate_WithRoutes(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	route := newRoute("tai-route", "ns1", "tai.example.com", map[string]string{
		"trustyai-service-name": "trustyai-service",
	})

	target := newTarget(newFakeClient(newScheme(), route))

	a := &MetricsAction{}
	result, err := a.Prepare().Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Completed).To(BeTrue())
}

// ---------------------------------------------------------------------------
// prepareTask.Execute
// ---------------------------------------------------------------------------

func TestPrepareTask_Execute_NoRoutes(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	target := newTarget(newFakeClient(newScheme()))

	a := &MetricsAction{}
	result, err := a.Prepare().Execute(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Completed).To(BeTrue())
}

func TestPrepareTask_Execute_DryRun(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(metricsJSON(t,
			newMetricEntry("SPD", "model-a"),
			newMetricEntry("DIR", "model-b"),
		))
	}))
	defer server.Close()

	route := newRoute("tai-route", "ns1", serverHost(server), map[string]string{
		"trustyai-service-name": "trustyai-service",
	})

	target := newTarget(newFakeClient(newScheme(), route), withDryRun, func(t *action.Target) {
		t.RESTConfig = &rest.Config{BearerToken: "test"}
	})

	a := &MetricsAction{}
	result, err := a.Prepare().Execute(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Completed).To(BeTrue())
}

func TestPrepareTask_Execute_BackupToFile(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(metricsJSON(t,
			newMetricEntry("SPD", "model-a"),
		))
	}))
	defer server.Close()

	route := newRoute("tai-route", "ns1", serverHost(server), map[string]string{
		"trustyai-service-name": "trustyai-service",
	})

	outputDir := t.TempDir()
	target := newTarget(newFakeClient(newScheme(), route), func(t *action.Target) {
		t.RESTConfig = &rest.Config{BearerToken: "test"}
		t.OutputDir = outputDir
	})

	a := &MetricsAction{}
	result, err := a.Prepare().Execute(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Completed).To(BeTrue())

	metricsDir := filepath.Join(outputDir, "trustyai-metrics")
	entries, _ := os.ReadDir(metricsDir)
	g.Expect(entries).To(HaveLen(1))
}

func TestPrepareTask_Execute_FairnessFilter(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		g.Expect(r.URL.RawQuery).To(Equal("type=fairness"))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(metricsJSON(t, newMetricEntry("SPD", "model-a")))
	}))
	defer server.Close()

	route := newRoute("tai-route", "ns1", serverHost(server), map[string]string{
		"trustyai-service-name": "trustyai-service",
	})

	outputDir := t.TempDir()
	target := newTarget(newFakeClient(newScheme(), route), func(tgt *action.Target) {
		tgt.RESTConfig = &rest.Config{BearerToken: "test"}
		tgt.OutputDir = outputDir
	})

	a := &MetricsAction{MetricType: "fairness"}
	result, err := a.Prepare().Execute(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Completed).To(BeTrue())
}

func TestPrepareTask_Execute_HTTPError(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("server error"))
	}))
	defer server.Close()

	route := newRoute("tai-route", "ns1", serverHost(server), map[string]string{
		"trustyai-service-name": "trustyai-service",
	})

	target := newTarget(newFakeClient(newScheme(), route), func(t *action.Target) {
		t.RESTConfig = &rest.Config{BearerToken: "test"}
	})

	a := &MetricsAction{}
	result, err := a.Prepare().Execute(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Completed).To(BeFalse())
}

func TestPrepareTask_Execute_NoRESTConfig(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	route := newRoute("tai-route", "ns1", "tai.example.com", map[string]string{
		"trustyai-service-name": "trustyai-service",
	})

	target := newTarget(newFakeClient(newScheme(), route))

	a := &MetricsAction{}
	result, err := a.Prepare().Execute(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Completed).To(BeFalse())
}

// ---------------------------------------------------------------------------
// runTask.Validate
// ---------------------------------------------------------------------------

func TestRunTask_Validate_NoBackupFile(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	target := newTarget(newFakeClient(newScheme()))

	a := &MetricsAction{}
	result, err := a.Run().Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Completed).To(BeTrue())
}

func TestRunTask_Validate_FileNotFound(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	target := newTarget(newFakeClient(newScheme()))

	a := &MetricsAction{BackupFile: "/nonexistent/file.json"}
	result, err := a.Run().Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Completed).To(BeFalse())
}

func TestRunTask_Validate_NoRoutes(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	backupFile := filepath.Join(t.TempDir(), "backup.json")
	_ = os.WriteFile(backupFile, metricsJSON(t, newMetricEntry("SPD", "m1")), 0o600)

	target := newTarget(newFakeClient(newScheme()))

	a := &MetricsAction{BackupFile: backupFile}
	result, err := a.Run().Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Completed).To(BeFalse())
}

func TestRunTask_Validate_Ready(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	backupFile := filepath.Join(t.TempDir(), "backup.json")
	_ = os.WriteFile(backupFile, metricsJSON(t, newMetricEntry("SPD", "m1")), 0o600)

	route := newRoute("tai-route", "ns1", "tai.example.com", map[string]string{
		"trustyai-service-name": "trustyai-service",
	})

	target := newTarget(newFakeClient(newScheme(), route))

	a := &MetricsAction{BackupFile: backupFile}
	result, err := a.Run().Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Completed).To(BeTrue())
}

// ---------------------------------------------------------------------------
// runTask.Execute
// ---------------------------------------------------------------------------

func TestRunTask_Execute_NoBackupFile(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	target := newTarget(newFakeClient(newScheme()))

	a := &MetricsAction{}
	result, err := a.Run().Execute(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Completed).To(BeTrue())
}

func TestRunTask_Execute_EmptyBackup(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	backupFile := filepath.Join(t.TempDir(), "backup.json")
	_ = os.WriteFile(backupFile, []byte(`{"requests":[]}`), 0o600)

	target := newTarget(newFakeClient(newScheme()))

	a := &MetricsAction{BackupFile: backupFile}
	result, err := a.Run().Execute(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Completed).To(BeTrue())
}

func TestRunTask_Execute_InvalidBackup(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	backupFile := filepath.Join(t.TempDir(), "backup.json")
	_ = os.WriteFile(backupFile, []byte(`{invalid json`), 0o600)

	target := newTarget(newFakeClient(newScheme()))

	a := &MetricsAction{BackupFile: backupFile}
	result, err := a.Run().Execute(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Completed).To(BeFalse())
}

func TestRunTask_Execute_NoRoutes(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	backupFile := filepath.Join(t.TempDir(), "backup.json")
	_ = os.WriteFile(backupFile, metricsJSON(t, newMetricEntry("SPD", "m1")), 0o600)

	target := newTarget(newFakeClient(newScheme()))

	a := &MetricsAction{BackupFile: backupFile}
	result, err := a.Run().Execute(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Completed).To(BeFalse())
}

func TestRunTask_Execute_DryRun(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	backupFile := filepath.Join(t.TempDir(), "backup.json")
	_ = os.WriteFile(backupFile, metricsJSON(t, newMetricEntry("SPD", "m1")), 0o600)

	route := newRoute("tai-route", "ns1", serverHost(server), map[string]string{
		"trustyai-service-name": "trustyai-service",
	})

	target := newTarget(newFakeClient(newScheme(), route), withDryRun, func(t *action.Target) {
		t.RESTConfig = &rest.Config{BearerToken: "test"}
	})

	a := &MetricsAction{BackupFile: backupFile}
	result, err := a.Run().Execute(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Completed).To(BeTrue())
}

func TestRunTask_Execute_RestoresMetrics(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	var postedEndpoints []string

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			postedEndpoints = append(postedEndpoints, r.URL.Path)
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()

	backupFile := filepath.Join(t.TempDir(), "backup.json")
	_ = os.WriteFile(backupFile, metricsJSON(t,
		newMetricEntry("SPD", "model-a"),
		newMetricEntry("DIR", "model-b"),
	), 0o600)

	route := newRoute("tai-route", "ns1", serverHost(server), map[string]string{
		"trustyai-service-name": "trustyai-service",
	})

	target := newTarget(newFakeClient(newScheme(), route), func(t *action.Target) {
		t.RESTConfig = &rest.Config{BearerToken: "test"}
	})

	a := &MetricsAction{BackupFile: backupFile}
	result, err := a.Run().Execute(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Completed).To(BeTrue())
	g.Expect(postedEndpoints).To(ContainElement("/metrics/group/fairness/spd/request"))
	g.Expect(postedEndpoints).To(ContainElement("/metrics/group/fairness/dir/request"))
}

func TestRunTask_Execute_SkipExisting(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	var postCount int

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(metricsJSON(t, newMetricEntry("SPD", "model-a")))

			return
		}
		if r.Method == http.MethodPost {
			postCount++
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()

	backupFile := filepath.Join(t.TempDir(), "backup.json")
	_ = os.WriteFile(backupFile, metricsJSON(t,
		newMetricEntry("SPD", "model-a"),
		newMetricEntry("DIR", "model-b"),
	), 0o600)

	route := newRoute("tai-route", "ns1", serverHost(server), map[string]string{
		"trustyai-service-name": "trustyai-service",
	})

	target := newTarget(newFakeClient(newScheme(), route), func(t *action.Target) {
		t.RESTConfig = &rest.Config{BearerToken: "test"}
	})

	a := &MetricsAction{BackupFile: backupFile, SkipExisting: true}
	result, err := a.Run().Execute(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Completed).To(BeTrue())
	g.Expect(postCount).To(Equal(1))
}

func TestRunTask_Execute_UnknownMetricType(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()

	entry := map[string]any{
		"id":      "unknown-1",
		"request": map[string]any{"metricName": "UNKNOWN_METRIC", "modelId": "m1"},
	}

	backupFile := filepath.Join(t.TempDir(), "backup.json")
	_ = os.WriteFile(backupFile, metricsJSON(t, entry), 0o600)

	route := newRoute("tai-route", "ns1", serverHost(server), map[string]string{
		"trustyai-service-name": "trustyai-service",
	})

	target := newTarget(newFakeClient(newScheme(), route), func(t *action.Target) {
		t.RESTConfig = &rest.Config{BearerToken: "test"}
	})

	a := &MetricsAction{BackupFile: backupFile}
	result, err := a.Run().Execute(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Completed).To(BeFalse())
}

func TestRunTask_Execute_HTTPError(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			w.WriteHeader(http.StatusInternalServerError)

			return
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()

	backupFile := filepath.Join(t.TempDir(), "backup.json")
	_ = os.WriteFile(backupFile, metricsJSON(t, newMetricEntry("SPD", "m1")), 0o600)

	route := newRoute("tai-route", "ns1", serverHost(server), map[string]string{
		"trustyai-service-name": "trustyai-service",
	})

	target := newTarget(newFakeClient(newScheme(), route), func(t *action.Target) {
		t.RESTConfig = &rest.Config{BearerToken: "test"}
	})

	a := &MetricsAction{BackupFile: backupFile}
	result, err := a.Run().Execute(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Completed).To(BeFalse())
}

func TestRunTask_Execute_NoRESTConfig(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	backupFile := filepath.Join(t.TempDir(), "backup.json")
	_ = os.WriteFile(backupFile, metricsJSON(t, newMetricEntry("SPD", "m1")), 0o600)

	route := newRoute("tai-route", "ns1", "tai.example.com", map[string]string{
		"trustyai-service-name": "trustyai-service",
	})

	target := newTarget(newFakeClient(newScheme(), route))

	a := &MetricsAction{BackupFile: backupFile}
	result, err := a.Run().Execute(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Completed).To(BeFalse())
}

func TestRunTask_Execute_UserCancels(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()

	backupFile := filepath.Join(t.TempDir(), "backup.json")
	_ = os.WriteFile(backupFile, metricsJSON(t, newMetricEntry("SPD", "m1")), 0o600)

	route := newRoute("tai-route", "ns1", serverHost(server), map[string]string{
		"trustyai-service-name": "trustyai-service",
	})

	inBuf := bytes.NewBufferString("n\n")
	io := iostreams.NewIOStreams(inBuf, &bytes.Buffer{}, &bytes.Buffer{})

	target := newTarget(newFakeClient(newScheme(), route), func(t *action.Target) {
		t.RESTConfig = &rest.Config{BearerToken: "test"}
		t.SkipConfirm = false
		t.IO = io
		t.Recorder = action.NewVerboseRootRecorder(io)
	})

	a := &MetricsAction{BackupFile: backupFile}
	result, err := a.Run().Execute(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Completed).To(BeTrue())
}

// ---------------------------------------------------------------------------
// parseBackupEntries
// ---------------------------------------------------------------------------

func TestRunTask_Execute_MissingRequestField(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()

	entry := map[string]any{
		"id":      "no-request-1",
		"request": map[string]any{"metricName": "SPD", "modelId": "m1"},
	}

	entryJSON, err := json.Marshal(entry)
	g.Expect(err).ToNot(HaveOccurred())

	var rawEntry map[string]json.RawMessage
	_ = json.Unmarshal(entryJSON, &rawEntry)
	delete(rawEntry, "request")

	modifiedEntry, err := json.Marshal(rawEntry)
	g.Expect(err).ToNot(HaveOccurred())

	backupFile := filepath.Join(t.TempDir(), "backup.json")
	_ = os.WriteFile(backupFile, []byte(`{"requests":[`+string(modifiedEntry)+`]}`), 0o600)

	route := newRoute("tai-route", "ns1", serverHost(server), map[string]string{
		"trustyai-service-name": "trustyai-service",
	})

	target := newTarget(newFakeClient(newScheme(), route), func(t *action.Target) {
		t.RESTConfig = &rest.Config{BearerToken: "test"}
	})

	a := &MetricsAction{BackupFile: backupFile}
	result, err := a.Run().Execute(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Completed).To(BeFalse())
}

func TestPrepareTask_Execute_InvalidResponse(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`not json`))
	}))
	defer server.Close()

	route := newRoute("tai-route", "ns1", serverHost(server), map[string]string{
		"trustyai-service-name": "trustyai-service",
	})

	outputDir := t.TempDir()
	target := newTarget(newFakeClient(newScheme(), route), func(tgt *action.Target) {
		tgt.RESTConfig = &rest.Config{BearerToken: "test"}
		tgt.OutputDir = outputDir
	})

	a := &MetricsAction{}
	result, err := a.Prepare().Execute(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Completed).To(BeFalse())
}

func TestParseBackupEntries(t *testing.T) {
	g := NewWithT(t)

	backupFile := filepath.Join(t.TempDir(), "backup.json")
	_ = os.WriteFile(backupFile, metricsJSON(t,
		newMetricEntry("SPD", "model-a"),
		newMetricEntry("DIR", "model-b"),
	), 0o600)

	task := &runTask{action: &MetricsAction{BackupFile: backupFile}}
	entries, err := task.parseBackupEntries()

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(entries).To(HaveLen(2))
	g.Expect(entries[0].Request.MetricName).To(Equal("SPD"))
	g.Expect(entries[1].Request.ModelID).To(Equal("model-b"))
}

func TestParseBackupEntries_InvalidJSON(t *testing.T) {
	g := NewWithT(t)

	backupFile := filepath.Join(t.TempDir(), "bad.json")
	_ = os.WriteFile(backupFile, []byte("not json"), 0o600)

	task := &runTask{action: &MetricsAction{BackupFile: backupFile}}
	_, err := task.parseBackupEntries()

	g.Expect(err).To(HaveOccurred())
}

func TestParseBackupEntries_FileNotFound(t *testing.T) {
	g := NewWithT(t)

	task := &runTask{action: &MetricsAction{BackupFile: "/nonexistent"}}
	_, err := task.parseBackupEntries()

	g.Expect(err).To(HaveOccurred())
}
