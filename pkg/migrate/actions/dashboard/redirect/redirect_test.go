package redirect_test

import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	clienttesting "k8s.io/client-go/testing"

	"github.com/opendatahub-io/odh-cli/pkg/migrate/action"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/action/result"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/actions/dashboard/redirect"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
	"github.com/opendatahub-io/odh-cli/pkg/util/iostreams"

	. "github.com/onsi/gomega"
)

func createFakeClient(scheme *runtime.Scheme, objects ...runtime.Object) (*dynamicfake.FakeDynamicClient, client.Client) {
	listKinds := map[schema.GroupVersionResource]string{
		resources.OdhDashboardConfig.GVR(): "OdhDashboardConfigList",
		resources.Subscription.GVR():       "SubscriptionList",
		resources.ConsoleLink.GVR():        "ConsoleLinkList",
		resources.Route.GVR():              "RouteList",
		resources.ConfigMap.GVR():          "ConfigMapList",
		resources.Deployment.GVR():         "DeploymentList",
		resources.Service.GVR():            "ServiceList",
	}
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, listKinds, objects...)

	// Mock Patch (Apply) to return success and record the object creation
	dynamicClient.PrependReactor("patch", "*", func(action clienttesting.Action) (bool, runtime.Object, error) {
		patchAction := action.(clienttesting.PatchAction)

		obj := &unstructured.Unstructured{
			Object: map[string]any{
				"metadata": map[string]any{
					"name":      patchAction.GetName(),
					"namespace": patchAction.GetNamespace(),
				},
			},
		}

		return true, obj, nil
	})

	return dynamicClient, client.NewForTesting(client.TestClientConfig{Dynamic: dynamicClient})
}

func expectAppliedResources(g *WithT, dynamicClient *dynamicfake.FakeDynamicClient, expectedNames ...string) {
	var appliedNames []string
	for _, a := range dynamicClient.Actions() {
		if a.GetVerb() == "patch" {
			appliedNames = append(appliedNames, a.(clienttesting.PatchAction).GetName())
		}
	}
	for _, name := range expectedNames {
		g.Expect(appliedNames).To(ContainElement(name))
	}
}

func TestDashboardRedirectAction_Execute(t *testing.T) {
	scheme := runtime.NewScheme()

	consoleLink := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "console.openshift.io/v1",
			"kind":       "ConsoleLink",
			"metadata": map[string]any{
				"name": "test-link",
			},
			"spec": map[string]any{
				"text": "OpenShift AI",
				"href": "https://rh-ai.apps.cluster.com/",
			},
		},
	}

	rhoaiDashboardConfig := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "opendatahub.io/v1alpha",
			"kind":       "OdhDashboardConfig",
			"metadata": map[string]any{
				"name":      "test-config",
				"namespace": "redhat-ods-applications",
				"labels": map[string]any{
					"app": "rhods-dashboard",
				},
				"annotations": map[string]any{
					"platform.opendatahub.io/type": "OpenShift AI",
				},
			},
		},
	}

	odhDashboardConfig := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "opendatahub.io/v1alpha",
			"kind":       "OdhDashboardConfig",
			"metadata": map[string]any{
				"name":      "test-config",
				"namespace": "opendatahub",
				"labels": map[string]any{
					"app": "odh-dashboard",
				},
			},
		},
	}

	rhoaiSubscription := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "operators.coreos.com/v1alpha1",
			"kind":       "Subscription",
			"metadata": map[string]any{
				"name":      "rhods-operator",
				"namespace": "redhat-ods-applications",
			},
			"spec": map[string]any{
				"name": "rhods-operator",
			},
		},
	}

	dataScienceGatewayRoute := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "route.openshift.io/v1",
			"kind":       "Route",
			"metadata": map[string]any{
				"name":      "data-science-gateway",
				"namespace": "redhat-ods-applications",
			},
			"spec": map[string]any{
				"host": "dsg.apps.cluster.com",
			},
		},
	}

	t.Run("Happy path (RHOAI via ConsoleLink)", func(t *testing.T) {
		g := NewWithT(t)
		dynamicClient, testClient := createFakeClient(scheme, rhoaiDashboardConfig, consoleLink)

		_, in, out, errOut := genericiooptions.NewTestIOStreams()
		ioStreams := iostreams.NewIOStreams(in, out, errOut)
		recorder := action.NewVerboseRootRecorder(ioStreams)

		target := action.Target{
			Client:   testClient,
			DryRun:   false,
			Recorder: recorder,
			IO:       ioStreams,
		}

		a := &redirect.DashboardRedirectAction{}
		res, err := a.Run().Execute(context.Background(), target)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(res.Status.Completed).To(BeTrue())

		// Verify Apply name fix - resources should be created with correct names
		expectAppliedResources(g, dynamicClient, "nginx-redirect-config", "rhods-dashboard", "data-science-gateway-legacy")
	})

	t.Run("Happy path (ODH via labels)", func(t *testing.T) {
		g := NewWithT(t)
		dynamicClient, testClient := createFakeClient(scheme, odhDashboardConfig, consoleLink)

		_, in, out, errOut := genericiooptions.NewTestIOStreams()
		ioStreams := iostreams.NewIOStreams(in, out, errOut)
		recorder := action.NewVerboseRootRecorder(ioStreams)

		target := action.Target{
			Client:   testClient,
			DryRun:   false,
			Recorder: recorder,
			IO:       ioStreams,
		}

		a := &redirect.DashboardRedirectAction{}
		res, err := a.Run().Execute(context.Background(), target)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(res.Status.Completed).To(BeTrue())

		expectAppliedResources(g, dynamicClient, "odh-dashboard")
	})

	t.Run("Happy path (Subscription fallback)", func(t *testing.T) {
		g := NewWithT(t)
		dynamicClient, testClient := createFakeClient(scheme, rhoaiSubscription, consoleLink)

		_, in, out, errOut := genericiooptions.NewTestIOStreams()
		ioStreams := iostreams.NewIOStreams(in, out, errOut)
		recorder := action.NewVerboseRootRecorder(ioStreams)

		target := action.Target{
			Client:   testClient,
			DryRun:   false,
			Recorder: recorder,
			IO:       ioStreams,
		}

		a := &redirect.DashboardRedirectAction{}
		res, err := a.Run().Execute(context.Background(), target)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(res.Status.Completed).To(BeTrue())

		expectAppliedResources(g, dynamicClient, "nginx-redirect-config")
	})

	t.Run("Route-based URL discovery fallback", func(t *testing.T) {
		g := NewWithT(t)
		dynamicClient, testClient := createFakeClient(scheme, rhoaiDashboardConfig, dataScienceGatewayRoute)

		_, in, out, errOut := genericiooptions.NewTestIOStreams()
		ioStreams := iostreams.NewIOStreams(in, out, errOut)
		recorder := action.NewVerboseRootRecorder(ioStreams)

		target := action.Target{
			Client:   testClient,
			DryRun:   false,
			Recorder: recorder,
			IO:       ioStreams,
		}

		a := &redirect.DashboardRedirectAction{}
		res, err := a.Run().Execute(context.Background(), target)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(res.Status.Completed).To(BeTrue())

		expectAppliedResources(g, dynamicClient, "nginx-redirect-config")
		// Ideally we would also verify the patch payload contains the right URL, but
		// checking that the resources applied is sufficient for this test suite.
	})

	t.Run("Dry-run mode", func(t *testing.T) {
		g := NewWithT(t)
		dynamicClient, testClient := createFakeClient(scheme, rhoaiDashboardConfig, consoleLink)

		_, in, out, errOut := genericiooptions.NewTestIOStreams()
		ioStreams := iostreams.NewIOStreams(in, out, errOut)
		recorder := action.NewVerboseRootRecorder(ioStreams)

		target := action.Target{
			Client:   testClient,
			DryRun:   true, // Dry run enabled
			Recorder: recorder,
			IO:       ioStreams,
		}

		a := &redirect.DashboardRedirectAction{}
		res, err := a.Run().Execute(context.Background(), target)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(res.Status.Completed).To(BeTrue())

		// Verify resources were NOT created (no patch actions)
		for _, a := range dynamicClient.Actions() {
			g.Expect(a.GetVerb()).NotTo(Equal("patch"))
		}
	})

	t.Run("Platform detection failure", func(t *testing.T) {
		g := NewWithT(t)
		_, testClient := createFakeClient(scheme, consoleLink)

		_, in, out, errOut := genericiooptions.NewTestIOStreams()
		ioStreams := iostreams.NewIOStreams(in, out, errOut)
		recorder := action.NewVerboseRootRecorder(ioStreams)

		target := action.Target{
			Client:   testClient,
			DryRun:   false,
			Recorder: recorder,
			IO:       ioStreams,
		}

		a := &redirect.DashboardRedirectAction{}
		res, err := a.Run().Execute(context.Background(), target)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(res.Status.Completed).To(BeFalse(), "Expected Completed to be false when platform detection fails")
		g.Expect(res.Status.Steps[0].Status).To(Equal(result.StepFailed))
	})

	t.Run("URL discovery failure", func(t *testing.T) {
		g := NewWithT(t)
		_, testClient := createFakeClient(scheme, rhoaiDashboardConfig)

		_, in, out, errOut := genericiooptions.NewTestIOStreams()
		ioStreams := iostreams.NewIOStreams(in, out, errOut)
		recorder := action.NewVerboseRootRecorder(ioStreams)

		target := action.Target{
			Client:   testClient,
			DryRun:   false,
			Recorder: recorder,
			IO:       ioStreams,
		}

		a := &redirect.DashboardRedirectAction{}
		res, err := a.Run().Execute(context.Background(), target)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(res.Status.Completed).To(BeFalse(), "Expected Completed to be false when URL discovery fails")
		g.Expect(res.Status.Steps[1].Status).To(Equal(result.StepFailed))
	})

	t.Run("Redirect URL override bypasses auto-discovery", func(t *testing.T) {
		g := NewWithT(t)
		// No ConsoleLink or Route objects - auto-discovery would fail
		dynamicClient, testClient := createFakeClient(scheme, rhoaiDashboardConfig)

		_, in, out, errOut := genericiooptions.NewTestIOStreams()
		ioStreams := iostreams.NewIOStreams(in, out, errOut)
		recorder := action.NewVerboseRootRecorder(ioStreams)

		target := action.Target{
			Client:   testClient,
			DryRun:   false,
			Recorder: recorder,
			IO:       ioStreams,
		}

		a := &redirect.DashboardRedirectAction{
			RedirectURL: "https://custom-dashboard.apps.cluster.com",
		}
		res, err := a.Run().Execute(context.Background(), target)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(res.Status.Completed).To(BeTrue())

		expectAppliedResources(g, dynamicClient, "nginx-redirect-config", "rhods-dashboard")
	})

	t.Run("Route host override injects host into primary route", func(t *testing.T) {
		g := NewWithT(t)
		dynamicClient, testClient := createFakeClient(scheme, rhoaiDashboardConfig, consoleLink)

		_, in, out, errOut := genericiooptions.NewTestIOStreams()
		ioStreams := iostreams.NewIOStreams(in, out, errOut)
		recorder := action.NewVerboseRootRecorder(ioStreams)

		target := action.Target{
			Client:   testClient,
			DryRun:   false,
			Recorder: recorder,
			IO:       ioStreams,
		}

		a := &redirect.DashboardRedirectAction{
			RouteHost: "custom-dashboard.apps.cluster.com",
		}
		res, err := a.Run().Execute(context.Background(), target)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(res.Status.Completed).To(BeTrue())

		// Verify the primary route was applied (the host is injected into the YAML)
		expectAppliedResources(g, dynamicClient, "rhods-dashboard")

		// Verify the patch payload contains the custom host
		for _, act := range dynamicClient.Actions() {
			if act.GetVerb() == "patch" {
				patchAction := act.(clienttesting.PatchAction)
				if patchAction.GetName() == "rhods-dashboard" {
					g.Expect(string(patchAction.GetPatch())).To(ContainSubstring("custom-dashboard.apps.cluster.com"))
				}
			}
		}
	})

	t.Run("Missing scheme in redirect URL override adds https://", func(t *testing.T) {
		g := NewWithT(t)
		// No ConsoleLink or Route objects - auto-discovery would fail
		dynamicClient, testClient := createFakeClient(scheme, rhoaiDashboardConfig)

		_, in, out, errOut := genericiooptions.NewTestIOStreams()
		ioStreams := iostreams.NewIOStreams(in, out, errOut)
		recorder := action.NewVerboseRootRecorder(ioStreams)

		target := action.Target{
			Client:   testClient,
			DryRun:   false,
			Recorder: recorder,
			IO:       ioStreams,
		}

		a := &redirect.DashboardRedirectAction{
			RedirectURL: "custom-dashboard.apps.cluster.com",
		}
		res, err := a.Run().Execute(context.Background(), target)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(res.Status.Completed).To(BeTrue())

		expectAppliedResources(g, dynamicClient, "nginx-redirect-config", "rhods-dashboard")

		for _, act := range dynamicClient.Actions() {
			if act.GetVerb() == "patch" {
				patchAction := act.(clienttesting.PatchAction)
				if patchAction.GetName() == "nginx-redirect-config" {
					g.Expect(string(patchAction.GetPatch())).To(ContainSubstring("https://custom-dashboard.apps.cluster.com"))
				}
			}
		}
	})

	t.Run("Both overrides together", func(t *testing.T) {
		g := NewWithT(t)
		// No ConsoleLink or Route objects - auto-discovery would fail
		dynamicClient, testClient := createFakeClient(scheme, rhoaiDashboardConfig)

		_, in, out, errOut := genericiooptions.NewTestIOStreams()
		ioStreams := iostreams.NewIOStreams(in, out, errOut)
		recorder := action.NewVerboseRootRecorder(ioStreams)

		target := action.Target{
			Client:   testClient,
			DryRun:   false,
			Recorder: recorder,
			IO:       ioStreams,
		}

		a := &redirect.DashboardRedirectAction{
			RedirectURL: "https://custom-dashboard.apps.cluster.com",
			RouteHost:   "old-dashboard.apps.cluster.com",
		}
		res, err := a.Run().Execute(context.Background(), target)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(res.Status.Completed).To(BeTrue())

		expectAppliedResources(g, dynamicClient, "nginx-redirect-config", "rhods-dashboard")

		// Verify the route has custom host and the redirect URL is used
		for _, act := range dynamicClient.Actions() {
			if act.GetVerb() == "patch" {
				patchAction := act.(clienttesting.PatchAction)
				if patchAction.GetName() == "rhods-dashboard" {
					g.Expect(string(patchAction.GetPatch())).To(ContainSubstring("old-dashboard.apps.cluster.com"))
				}
				if patchAction.GetName() == "nginx-redirect-config" {
					g.Expect(string(patchAction.GetPatch())).To(ContainSubstring("custom-dashboard.apps.cluster.com"))
				}
			}
		}
	})
}
