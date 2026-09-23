package deps

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	operatorsv1 "github.com/operator-framework/api/pkg/operators/v1"
	operatorsv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	operatorfake "github.com/operator-framework/operator-lifecycle-manager/pkg/api/client/clientset/versioned/fake"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"
	crfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	discoveryfake "k8s.io/client-go/discovery/fake"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8stesting "k8s.io/client-go/testing"

	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
	"github.com/opendatahub-io/odh-cli/pkg/util/iostreams"
	utilolm "github.com/opendatahub-io/odh-cli/pkg/util/kube/olm"

	. "github.com/onsi/gomega"
)

const (
	testV1DependencyName      = "test-operator"
	testV1DependencyDisplay   = "Test Operator"
	testV1DependencyNamespace = "test-operator-system"
	testV1DependencyPackage   = "test-operator-package"
	testV1DependencyChannel   = "stable-v1"
	testV1CatalogSource       = "test-catalog"
	testV1DependencyVersion   = "1.2.3"
	testV1OtherNamespace      = "other-namespace"
	testV1OtherPackage        = "other-package"
	testV1CatalogSourceType   = "Catalog"
	testV1ForbiddenReason     = "clusterextension list denied"
	testV1RequestedMessage    = "already has an installation request or is installed"
	testV1DryRunUnavailable   = "# OLM v1 path unavailable:"
	testV1SecondDisplay       = "Other Operator"
	testV0SubscriptionAPI     = "apiVersion: operators.coreos.com/v1alpha1"
	testV1ExtensionAPI        = "apiVersion: olm.operatorframework.io/v1"
	testV1DryRunModeHint      = "Use --olm-mode=v0 or --olm-mode=v1 to render active manifests."
)

func TestInstallValidateOLMMode(t *testing.T) {
	tests := []struct {
		name      string
		mode      string
		resources []*metav1.APIResourceList
		want      utilolm.Mode
	}{
		{
			name: "mixed cluster defaults to OLM v0",
			resources: []*metav1.APIResourceList{
				{GroupVersion: "olm.operatorframework.io/v1", APIResources: []metav1.APIResource{{Name: "clusterextensions"}}},
				{GroupVersion: "operators.coreos.com/v1alpha1", APIResources: []metav1.APIResource{{Name: "subscriptions"}}},
			},
			want: utilolm.ModeV0,
		},
		{
			name: "mixed cluster permits explicit v1",
			mode: "v1",
			resources: []*metav1.APIResourceList{
				{GroupVersion: "olm.operatorframework.io/v1", APIResources: []metav1.APIResource{{Name: "clusterextensions"}}},
				{GroupVersion: "operators.coreos.com/v1alpha1", APIResources: []metav1.APIResource{{Name: "subscriptions"}}},
			},
			want: utilolm.ModeV1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			fakeDiscovery := &discoveryfake.FakeDiscovery{Fake: &k8stesting.Fake{}}
			fakeDiscovery.Resources = tt.resources
			cmd := &InstallCommand{
				Timeout: time.Minute,
				OLMMode: tt.mode,
				client: client.NewForTesting(client.TestClientConfig{
					Discovery:         fakeDiscovery,
					ControllerRuntime: crfake.NewClientBuilder().Build(),
				}),
			}

			g.Expect(cmd.Validate()).To(Succeed())
			g.Expect(cmd.selectedOLMMode).To(Equal(tt.want))
		})
	}
}

func TestShouldInstallDep(t *testing.T) {
	tests := []struct {
		name            string
		targetDeps      []string
		includeOptional bool
		dep             DependencyInfo
		want            bool
	}{
		{
			name:       "target dep matches - always install",
			targetDeps: []string{"cert-manager"},
			dep:        DependencyInfo{Name: "cert-manager", Enabled: "false"},
			want:       true,
		},
		{
			name:       "target dep matches optional - always install",
			targetDeps: []string{"kueue"},
			dep:        DependencyInfo{Name: "kueue", Enabled: "auto"},
			want:       true,
		},
		{
			name:       "required dep - install",
			targetDeps: nil,
			dep:        DependencyInfo{Name: "cert-manager", Enabled: "true"},
			want:       true,
		},
		{
			name:       "optional dep without flag - skip",
			targetDeps: nil,
			dep:        DependencyInfo{Name: "kueue", Enabled: "auto"},
			want:       false,
		},
		{
			name:       "disabled dep without flag - skip",
			targetDeps: nil,
			dep:        DependencyInfo{Name: "servicemesh", Enabled: "false"},
			want:       false,
		},
		{
			name:            "optional dep with flag - install",
			targetDeps:      nil,
			includeOptional: true,
			dep:             DependencyInfo{Name: "kueue", Enabled: "auto"},
			want:            true,
		},
		{
			name:            "disabled dep with flag - still skip",
			targetDeps:      nil,
			includeOptional: true,
			dep:             DependencyInfo{Name: "servicemesh", Enabled: "false"},
			want:            false,
		},
		{
			name:       "different target dep - use normal rules",
			targetDeps: []string{"other-dep"},
			dep:        DependencyInfo{Name: "kueue", Enabled: "auto"},
			want:       false,
		},
		{
			name:       "batch target deps - install matching dep",
			targetDeps: []string{"cert-manager", "kueue"},
			dep:        DependencyInfo{Name: "kueue", Enabled: "auto"},
			want:       true,
		},
		{
			name:       "batch target deps - skip non-matching dep",
			targetDeps: []string{"cert-manager", "kueue"},
			dep:        DependencyInfo{Name: "servicemesh", Enabled: "true"},
			want:       false,
		},
		{
			name:       "explicit empty list - install nothing",
			targetDeps: []string{},
			dep:        DependencyInfo{Name: "cert-manager", Enabled: "true"},
			want:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			streams := genericiooptions.IOStreams{
				Out:    &bytes.Buffer{},
				ErrOut: &bytes.Buffer{},
			}

			cmd := &InstallCommand{
				IO:              iostreams.NewIOStreams(streams.In, streams.Out, streams.ErrOut),
				TargetDeps:      tt.targetDeps,
				IncludeOptional: tt.includeOptional,
			}

			got := cmd.shouldInstallDep(tt.dep)
			g.Expect(got).To(Equal(tt.want))
		})
	}
}

func TestSlicesEqualUnordered(t *testing.T) {
	tests := []struct {
		name string
		a    []string
		b    []string
		want bool
	}{
		{
			name: "equal same order",
			a:    []string{"a", "b", "c"},
			b:    []string{"a", "b", "c"},
			want: true,
		},
		{
			name: "equal different order",
			a:    []string{"a", "b", "c"},
			b:    []string{"c", "a", "b"},
			want: true,
		},
		{
			name: "different lengths",
			a:    []string{"a", "b"},
			b:    []string{"a", "b", "c"},
			want: false,
		},
		{
			name: "different elements",
			a:    []string{"a", "b", "c"},
			b:    []string{"a", "b", "d"},
			want: false,
		},
		{
			name: "duplicates matter - same counts",
			a:    []string{"a", "a", "b"},
			b:    []string{"a", "b", "a"},
			want: true,
		},
		{
			name: "duplicates matter - different counts",
			a:    []string{"a", "a", "b"},
			b:    []string{"a", "b", "b"},
			want: false,
		},
		{
			name: "both empty",
			a:    []string{},
			b:    []string{},
			want: true,
		},
		{
			name: "one empty",
			a:    []string{"a"},
			b:    []string{},
			want: false,
		},
		{
			name: "both nil",
			a:    nil,
			b:    nil,
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			got := slicesEqualUnordered(tt.a, tt.b)
			g.Expect(got).To(Equal(tt.want))
		})
	}
}

func TestRunDryRun(t *testing.T) {
	t.Run("with deps to install", func(t *testing.T) {
		g := NewWithT(t)

		var buf bytes.Buffer
		streams := genericiooptions.IOStreams{
			Out:    &buf,
			ErrOut: &bytes.Buffer{},
		}

		cmd := &InstallCommand{
			IO: iostreams.NewIOStreams(streams.In, streams.Out, streams.ErrOut),
		}

		deps := []DependencyInfo{
			{
				Name:         "cert-manager",
				DisplayName:  "Cert Manager",
				Namespace:    "cert-manager-operator",
				Subscription: "cert-manager",
				Channel:      "stable",
				Enabled:      "true",
			},
		}

		err := cmd.runDryRun(context.Background(), deps)

		g.Expect(err).ToNot(HaveOccurred())

		output := buf.String()
		g.Expect(output).To(ContainSubstring("[DRY RUN]"))
		g.Expect(output).To(ContainSubstring("Cert Manager"))
		g.Expect(output).To(ContainSubstring("kind: Namespace"))
		g.Expect(output).To(ContainSubstring("kind: OperatorGroup"))
		g.Expect(output).To(ContainSubstring("kind: Subscription"))
		g.Expect(output).To(ContainSubstring("cert-manager-operator"))
		g.Expect(output).To(ContainSubstring("app.kubernetes.io/managed-by: odh-cli"))
	})

	t.Run("bulk mode with no eligible deps shows all-installed", func(t *testing.T) {
		g := NewWithT(t)

		var buf bytes.Buffer
		streams := genericiooptions.IOStreams{
			Out:    &buf,
			ErrOut: &bytes.Buffer{},
		}

		// TargetDeps == nil: bulk mode; optional dep without --include-optional is skipped
		cmd := &InstallCommand{
			IO: iostreams.NewIOStreams(streams.In, streams.Out, streams.ErrOut),
		}

		deps := []DependencyInfo{
			{
				Name:    "kueue",
				Enabled: "auto",
			},
		}

		err := cmd.runDryRun(context.Background(), deps)

		g.Expect(err).ToNot(HaveOccurred())

		output := buf.String()
		g.Expect(output).To(ContainSubstring("[DRY RUN]"))
		g.Expect(output).To(ContainSubstring(msgAllInstalled))
		g.Expect(output).ToNot(ContainSubstring(msgNoDepsToInstall))
	})

	t.Run("explicit empty TargetDeps shows no-deps-to-install", func(t *testing.T) {
		g := NewWithT(t)

		var buf bytes.Buffer
		streams := genericiooptions.IOStreams{
			Out:    &buf,
			ErrOut: &bytes.Buffer{},
		}

		// TargetDeps == []string{}: user explicitly requested nothing
		cmd := &InstallCommand{
			IO:         iostreams.NewIOStreams(streams.In, streams.Out, streams.ErrOut),
			TargetDeps: []string{},
		}

		deps := []DependencyInfo{
			{
				Name:    "cert-manager",
				Enabled: "true",
			},
		}

		err := cmd.runDryRun(context.Background(), deps)

		g.Expect(err).ToNot(HaveOccurred())

		output := buf.String()
		g.Expect(output).To(ContainSubstring("[DRY RUN]"))
		g.Expect(output).To(ContainSubstring(msgNoDepsToInstall))
		g.Expect(output).ToNot(ContainSubstring(msgAllInstalled))
	})

	t.Run("with target dep", func(t *testing.T) {
		g := NewWithT(t)

		var buf bytes.Buffer
		streams := genericiooptions.IOStreams{
			Out:    &buf,
			ErrOut: &bytes.Buffer{},
		}

		cmd := &InstallCommand{
			IO:         iostreams.NewIOStreams(streams.In, streams.Out, streams.ErrOut),
			TargetDeps: []string{"kueue"},
		}

		deps := []DependencyInfo{
			{
				Name:        "cert-manager",
				DisplayName: "Cert Manager",
				Namespace:   "cert-manager-operator",
				Enabled:     "true",
			},
			{
				Name:         "kueue",
				DisplayName:  "Kueue",
				Namespace:    "openshift-kueue-operator",
				Subscription: "kueue-operator",
				Channel:      "stable",
				Enabled:      "auto",
			},
		}

		err := cmd.runDryRun(context.Background(), deps)

		g.Expect(err).ToNot(HaveOccurred())

		output := buf.String()
		// Should only show kueue, not cert-manager
		g.Expect(output).To(ContainSubstring("Kueue"))
		g.Expect(output).ToNot(ContainSubstring("Cert Manager"))
	})
}

func TestRunDryRunContinuesAfterUnavailableV1Alternative(t *testing.T) {
	g := NewWithT(t)
	var stdout bytes.Buffer
	command := &InstallCommand{IO: iostreams.NewIOStreams(nil, &stdout, &bytes.Buffer{})}
	deps := []DependencyInfo{
		{
			Name:             testV1DependencyName,
			DisplayName:      testV1DependencyDisplay,
			Namespace:        testV1DependencyNamespace,
			Subscription:     testV1DependencyPackage,
			Enabled:          enabledTrue,
			TargetNamespaces: []string{testV1DependencyNamespace, testV1OtherNamespace},
		},
		{
			Name:         testV1OtherPackage,
			DisplayName:  testV1SecondDisplay,
			Namespace:    testV1OtherNamespace,
			Subscription: testV1OtherPackage,
			Enabled:      enabledTrue,
		},
	}

	g.Expect(command.runDryRun(t.Context(), deps)).To(Succeed())
	g.Expect(stdout.String()).To(ContainSubstring(testV1DryRunUnavailable))
	g.Expect(stdout.String()).To(ContainSubstring(testV1DryRunModeHint))
	g.Expect(stdout.String()).To(ContainSubstring("# " + testV1SecondDisplay))
	g.Expect(stdout.String()).To(ContainSubstring("# " + testV0SubscriptionAPI))
	g.Expect(stdout.String()).To(ContainSubstring("# " + testV1ExtensionAPI))
	g.Expect(stdout.String()).ToNot(ContainSubstring("\n" + testV0SubscriptionAPI))
	g.Expect(stdout.String()).ToNot(ContainSubstring("\n" + testV1ExtensionAPI))
}

func TestRunDryRunExplicitModesKeepActiveManifests(t *testing.T) {
	for _, tt := range []struct {
		mode       utilolm.Mode
		apiVersion string
	}{
		{mode: utilolm.ModeV0, apiVersion: testV0SubscriptionAPI},
		{mode: utilolm.ModeV1, apiVersion: testV1ExtensionAPI},
	} {
		t.Run(string(tt.mode), func(t *testing.T) {
			g := NewWithT(t)
			var stdout bytes.Buffer
			command := &InstallCommand{
				IO:              iostreams.NewIOStreams(nil, &stdout, &bytes.Buffer{}),
				selectedOLMMode: tt.mode,
			}
			dep := DependencyInfo{
				Name:         testV1DependencyName,
				DisplayName:  testV1DependencyDisplay,
				Namespace:    testV1DependencyNamespace,
				Subscription: testV1DependencyPackage,
				Enabled:      enabledTrue,
			}

			g.Expect(command.runDryRun(t.Context(), []DependencyInfo{dep})).To(Succeed())
			g.Expect(stdout.String()).To(ContainSubstring("\n" + tt.apiVersion))
		})
	}
}

func TestRunInstall_EmptyToInstall(t *testing.T) {
	// filterDepsToInstall short-circuits before any API call when shouldInstallDep
	// returns false for all deps, so these tests need no cluster client.

	t.Run("bulk mode with no eligible deps shows all-installed", func(t *testing.T) {
		g := NewWithT(t)

		var buf bytes.Buffer
		streams := genericiooptions.IOStreams{
			Out:    &buf,
			ErrOut: &bytes.Buffer{},
		}

		// TargetDeps == nil: bulk mode; all deps disabled, none reach isAlreadyInstalled
		cmd := &InstallCommand{
			IO: iostreams.NewIOStreams(streams.In, streams.Out, streams.ErrOut),
		}

		deps := []DependencyInfo{
			{Name: "servicemesh", Enabled: "false"},
		}

		err := cmd.runInstall(context.Background(), deps)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(buf.String()).To(ContainSubstring(msgAllRequested))
		g.Expect(buf.String()).ToNot(ContainSubstring(msgNoDepsToInstall))
	})

	t.Run("explicit empty TargetDeps shows no-deps-to-install", func(t *testing.T) {
		g := NewWithT(t)

		var buf bytes.Buffer
		streams := genericiooptions.IOStreams{
			Out:    &buf,
			ErrOut: &bytes.Buffer{},
		}

		// TargetDeps == []string{}: user explicitly requested nothing
		cmd := &InstallCommand{
			IO:         iostreams.NewIOStreams(streams.In, streams.Out, streams.ErrOut),
			TargetDeps: []string{},
		}

		deps := []DependencyInfo{
			{Name: "cert-manager", Enabled: "true"},
		}

		err := cmd.runInstall(context.Background(), deps)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(buf.String()).To(ContainSubstring(msgNoDepsToInstall))
		g.Expect(buf.String()).ToNot(ContainSubstring(msgAllInstalled))
	})
}

func TestRunInstall_PendingV0SubscriptionOnOLMV1(t *testing.T) {
	g := NewWithT(t)

	scheme := runtime.NewScheme()
	gvk := resources.ClusterExtension.GVK()
	scheme.AddKnownTypeWithName(gvk, &unstructured.Unstructured{})
	scheme.AddKnownTypeWithName(gvk.GroupVersion().WithKind(resources.ClusterExtension.ListKind()), &unstructured.UnstructuredList{})

	sub := &operatorsv1alpha1.Subscription{
		ObjectMeta: metav1.ObjectMeta{Name: testV1DependencyName, Namespace: testV1DependencyNamespace},
		Spec:       &operatorsv1alpha1.SubscriptionSpec{Package: testV1DependencyPackage},
	}
	kubeClient := client.NewForTesting(client.TestClientConfig{
		OLM:               operatorfake.NewSimpleClientset(sub), //nolint:staticcheck // Fake clientset is used only in tests.
		ControllerRuntime: crfake.NewClientBuilder().WithScheme(scheme).Build(),
	})

	var buf bytes.Buffer
	streams := genericiooptions.IOStreams{Out: &buf, ErrOut: &bytes.Buffer{}}
	cmd := &InstallCommand{
		IO:              iostreams.NewIOStreams(streams.In, streams.Out, streams.ErrOut),
		TargetDeps:      []string{testV1DependencyName},
		client:          kubeClient,
		selectedOLMMode: utilolm.ModeV1,
	}
	dep := DependencyInfo{
		Name:         testV1DependencyName,
		Enabled:      enabledTrue,
		Subscription: testV1DependencyPackage,
		Namespace:    testV1DependencyNamespace,
	}

	err := cmd.runInstall(t.Context(), []DependencyInfo{dep})

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(buf.String()).To(ContainSubstring(testV1RequestedMessage))
	g.Expect(buf.String()).ToNot(ContainSubstring("is already installed"))
}

func TestV0InstallChecksExistingClusterExtension(t *testing.T) {
	for _, tt := range []struct {
		name               string
		extensionNamespace string
		extensionPackage   string
		wantSkipped        bool
	}{
		{name: "same package and namespace is skipped", extensionNamespace: testV1DependencyNamespace, extensionPackage: testV1DependencyPackage, wantSkipped: true},
		{name: "different namespace is installed", extensionNamespace: testV1OtherNamespace, extensionPackage: testV1DependencyPackage},
		{name: "different package is installed", extensionNamespace: testV1DependencyNamespace, extensionPackage: testV1OtherPackage},
	} {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			scheme := runtime.NewScheme()
			gvk := resources.ClusterExtension.GVK()
			scheme.AddKnownTypeWithName(gvk, &unstructured.Unstructured{})
			scheme.AddKnownTypeWithName(gvk.GroupVersion().WithKind(resources.ClusterExtension.ListKind()), &unstructured.UnstructuredList{})

			extension := &unstructured.Unstructured{Object: map[string]any{
				"apiVersion": resources.ClusterExtension.APIVersion(),
				"kind":       resources.ClusterExtension.Kind,
				"metadata":   map[string]any{"name": testV1DependencyName},
				"spec": map[string]any{
					"namespace": tt.extensionNamespace,
					"source": map[string]any{
						"sourceType": testV1CatalogSourceType,
						"catalog":    map[string]any{"packageName": tt.extensionPackage},
					},
				},
			}}
			kubeClient := client.NewForTesting(client.TestClientConfig{
				OLM:               operatorfake.NewSimpleClientset(), //nolint:staticcheck // Fake clientset is used only in tests.
				ControllerRuntime: crfake.NewClientBuilder().WithScheme(scheme).WithObjects(extension).Build(),
			})
			var stdout bytes.Buffer
			command := &InstallCommand{
				IO:              iostreams.NewIOStreams(nil, &stdout, &bytes.Buffer{}),
				TargetDeps:      []string{testV1DependencyName},
				client:          kubeClient,
				selectedOLMMode: utilolm.ModeV0,
			}
			dependency := DependencyInfo{
				Name:         testV1DependencyName,
				Enabled:      enabledTrue,
				Subscription: testV1DependencyPackage,
				Namespace:    testV1DependencyNamespace,
			}

			toInstall, err := command.filterDepsToInstall(t.Context(), []DependencyInfo{dependency})
			g.Expect(err).ToNot(HaveOccurred())
			if tt.wantSkipped {
				g.Expect(toInstall).To(BeEmpty())
				g.Expect(command.runInstall(t.Context(), []DependencyInfo{dependency})).To(Succeed())
				g.Expect(stdout.String()).To(ContainSubstring(testV1RequestedMessage))
			} else {
				g.Expect(toInstall).To(Equal([]DependencyInfo{dependency}))
			}
		})
	}
}

func TestV0InstallContinuesWhenClusterExtensionReadForbidden(t *testing.T) {
	g := NewWithT(t)
	controllerRuntime := crfake.NewClientBuilder().WithInterceptorFuncs(interceptor.Funcs{
		List: func(_ context.Context, _ crclient.WithWatch, _ crclient.ObjectList, _ ...crclient.ListOption) error {
			return apierrors.NewForbidden(
				schema.GroupResource{Group: resources.ClusterExtension.Group, Resource: resources.ClusterExtension.Resource},
				"", errors.New(testV1ForbiddenReason),
			)
		},
	}).Build()
	kubeClient := client.NewForTesting(client.TestClientConfig{
		OLM:               operatorfake.NewSimpleClientset(), //nolint:staticcheck // Fake clientset is used only in tests.
		ControllerRuntime: controllerRuntime,
	})
	var stderr bytes.Buffer
	command := &InstallCommand{
		IO:              iostreams.NewIOStreams(nil, &bytes.Buffer{}, &stderr),
		client:          kubeClient,
		selectedOLMMode: utilolm.ModeV0,
	}
	dependency := DependencyInfo{
		Name:         testV1DependencyName,
		Enabled:      enabledTrue,
		Subscription: testV1DependencyPackage,
		Namespace:    testV1DependencyNamespace,
	}

	toInstall, err := command.filterDepsToInstall(t.Context(), []DependencyInfo{dependency})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(toInstall).To(Equal([]DependencyInfo{dependency}))
	g.Expect(stderr.String()).To(ContainSubstring("continuing with OLM v0"))
}

func TestSyncWriter(t *testing.T) {
	t.Run("concurrent writes are safe", func(t *testing.T) {
		g := NewWithT(t)

		var buf bytes.Buffer
		sw := &syncWriter{w: &buf}

		var wg sync.WaitGroup
		numWriters := 10
		writesPerWriter := 100

		for i := range numWriters {
			wg.Add(1)

			go func(writerID int) {
				defer wg.Done()

				for range writesPerWriter {
					_, err := sw.Write([]byte("x"))
					g.Expect(err).ToNot(HaveOccurred())
				}
			}(i)
		}

		wg.Wait()

		// All writes should complete
		g.Expect(buf.Len()).To(Equal(numWriters * writesPerWriter))
	})

	t.Run("content is preserved", func(t *testing.T) {
		g := NewWithT(t)

		var buf bytes.Buffer
		sw := &syncWriter{w: &buf}

		_, err := sw.Write([]byte("hello "))
		g.Expect(err).ToNot(HaveOccurred())

		_, err = sw.Write([]byte("world"))
		g.Expect(err).ToNot(HaveOccurred())

		g.Expect(buf.String()).To(Equal("hello world"))
	})
}

func TestEnsureOperatorGroup(t *testing.T) {
	t.Run("returns false when AlreadyExists", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		scheme := runtime.NewScheme()
		err := operatorsv1.AddToScheme(scheme)
		g.Expect(err).ToNot(HaveOccurred())

		listKinds := map[schema.GroupVersionResource]string{
			operatorsv1.SchemeGroupVersion.WithResource("operatorgroups"): "OperatorGroupList",
		}

		dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, listKinds)

		// Configure fake to return AlreadyExists on Create
		dynamicClient.PrependReactor("create", "operatorgroups", func(_ k8stesting.Action) (bool, runtime.Object, error) {
			return true, nil, apierrors.NewAlreadyExists(
				schema.GroupResource{Group: "operators.coreos.com", Resource: "operatorgroups"},
				"test-og",
			)
		})

		testClient := client.NewForTesting(client.TestClientConfig{
			Dynamic: dynamicClient,
		})

		streams := genericiooptions.IOStreams{
			Out:    &bytes.Buffer{},
			ErrOut: &bytes.Buffer{},
		}

		cmd := &InstallCommand{
			IO:     iostreams.NewIOStreams(streams.In, streams.Out, streams.ErrOut),
			client: testClient,
		}

		created, err := cmd.ensureOperatorGroup(ctx, "test-namespace", nil)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(created).To(BeFalse(), "should return false when OG already exists")
	})

	t.Run("returns true when created successfully", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		scheme := runtime.NewScheme()
		err := operatorsv1.AddToScheme(scheme)
		g.Expect(err).ToNot(HaveOccurred())

		listKinds := map[schema.GroupVersionResource]string{
			operatorsv1.SchemeGroupVersion.WithResource("operatorgroups"): "OperatorGroupList",
		}

		dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, listKinds)

		testClient := client.NewForTesting(client.TestClientConfig{
			Dynamic: dynamicClient,
		})

		streams := genericiooptions.IOStreams{
			Out:    &bytes.Buffer{},
			ErrOut: &bytes.Buffer{},
		}

		cmd := &InstallCommand{
			IO:     iostreams.NewIOStreams(streams.In, streams.Out, streams.ErrOut),
			client: testClient,
		}

		created, err := cmd.ensureOperatorGroup(ctx, "test-namespace", nil)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(created).To(BeTrue(), "should return true when OG is created")
	})
}

func TestCreateDepResourcesWithOLMV1(t *testing.T) {
	g := NewWithT(t)

	scheme := runtime.NewScheme()
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, map[schema.GroupVersionResource]string{
		corev1.SchemeGroupVersion.WithResource("namespaces"): "NamespaceList",
		resources.ClusterExtension.GVR():                     resources.ClusterExtension.ListKind(),
	})

	streams := genericiooptions.IOStreams{
		Out:    &bytes.Buffer{},
		ErrOut: &bytes.Buffer{},
	}
	command := &InstallCommand{
		IO:              iostreams.NewIOStreams(streams.In, streams.Out, streams.ErrOut),
		client:          client.NewForTesting(client.TestClientConfig{Dynamic: dynamicClient}),
		selectedOLMMode: utilolm.ModeV1,
	}
	dependency := DependencyInfo{
		Name:             testV1DependencyName,
		DisplayName:      testV1DependencyDisplay,
		Namespace:        testV1DependencyNamespace,
		Subscription:     testV1DependencyPackage,
		Channel:          testV1DependencyChannel,
		Source:           testV1CatalogSource,
		TargetNamespaces: []string{testV1OtherNamespace},
	}

	err := command.createDepResources(t.Context(), dependency)
	g.Expect(err).ToNot(HaveOccurred())

	namespace, err := dynamicClient.Resource(corev1.SchemeGroupVersion.WithResource("namespaces")).
		Get(t.Context(), testV1DependencyNamespace, metav1.GetOptions{})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(namespace.GetLabels()).To(HaveKeyWithValue(labelManagedBy, labelManagedByValue))

	extension, err := dynamicClient.Resource(resources.ClusterExtension.GVR()).
		Get(t.Context(), testV1DependencyPackage, metav1.GetOptions{})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(extension.GetNamespace()).To(BeEmpty())
	g.Expect(extension.GetLabels()).To(HaveKeyWithValue(labelManagedBy, labelManagedByValue))

	extensionNamespace, found, err := unstructured.NestedString(extension.Object, "spec", "namespace")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(found).To(BeTrue())
	g.Expect(extensionNamespace).To(Equal(testV1DependencyNamespace))

	configType, found, err := unstructured.NestedString(extension.Object, "spec", "config", "configType")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(found).To(BeTrue())
	g.Expect(configType).To(Equal("Inline"))

	watchNamespace, found, err := unstructured.NestedString(extension.Object, "spec", "config", "inline", "watchNamespace")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(found).To(BeTrue())
	g.Expect(watchNamespace).To(Equal(testV1OtherNamespace))

	packageName, found, err := unstructured.NestedString(
		extension.Object,
		"spec", "source", "catalog", "packageName",
	)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(found).To(BeTrue())
	g.Expect(packageName).To(Equal(testV1DependencyPackage))

	channels, found, err := unstructured.NestedStringSlice(
		extension.Object,
		"spec", "source", "catalog", "channels",
	)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(found).To(BeTrue())
	g.Expect(channels).To(Equal([]string{testV1DependencyChannel}))

	createdResources := make([]string, 0, 2)
	for _, action := range dynamicClient.Actions() {
		if action.GetVerb() == "create" {
			createdResources = append(createdResources, action.GetResource().Resource)
		}
	}
	g.Expect(createdResources).To(ConsistOf("namespaces", "clusterextensions"))
}

func TestCreateDepResourcesWithOLMV1RejectsMultipleTargetNamespaces(t *testing.T) {
	g := NewWithT(t)
	scheme := runtime.NewScheme()
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, map[schema.GroupVersionResource]string{
		corev1.SchemeGroupVersion.WithResource("namespaces"): "NamespaceList",
		resources.ClusterExtension.GVR():                     resources.ClusterExtension.ListKind(),
	})
	command := &InstallCommand{
		IO:              iostreams.NewIOStreams(nil, &bytes.Buffer{}, &bytes.Buffer{}),
		client:          client.NewForTesting(client.TestClientConfig{Dynamic: dynamicClient}),
		selectedOLMMode: utilolm.ModeV1,
	}
	dependency := DependencyInfo{
		Name:             testV1DependencyName,
		Namespace:        testV1DependencyNamespace,
		Subscription:     testV1DependencyPackage,
		TargetNamespaces: []string{testV1DependencyNamespace, testV1OtherNamespace},
	}

	err := command.createDepResources(t.Context(), dependency)
	g.Expect(err).To(MatchError(ContainSubstring("OLM v1 supports only one watch namespace")))
	g.Expect(dynamicClient.Actions()).To(BeEmpty())
}

func TestDryRunClusterExtensionOmitsWatchNamespaceWithoutTargets(t *testing.T) {
	g := NewWithT(t)
	command := &InstallCommand{}
	dependency := DependencyInfo{
		Name:         testV1DependencyName,
		Namespace:    testV1DependencyNamespace,
		Subscription: testV1DependencyPackage,
	}
	var output bytes.Buffer

	g.Expect(command.printDryRunClusterExtension(&output, dependency)).To(Succeed())
	g.Expect(output.String()).ToNot(ContainSubstring("watchNamespace"))
	g.Expect(output.String()).ToNot(ContainSubstring("configType"))
}

func TestWaitForOperatorWithOLMV1(t *testing.T) {
	g := NewWithT(t)

	operatorConditionGVK := schema.GroupVersionKind{
		Group: "operators.coreos.com", Version: "v2", Kind: "OperatorCondition",
	}
	clusterExtensionGVK := resources.ClusterExtension.GVK()
	scheme := runtime.NewScheme()
	for _, gvk := range []schema.GroupVersionKind{operatorConditionGVK, clusterExtensionGVK} {
		scheme.AddKnownTypeWithName(gvk, &unstructured.Unstructured{})
		scheme.AddKnownTypeWithName(gvk.GroupVersion().WithKind(gvk.Kind+"List"), &unstructured.UnstructuredList{})
	}

	extension := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": resources.ClusterExtension.APIVersion(),
		"kind":       resources.ClusterExtension.Kind,
		"metadata": map[string]any{
			"name": testV1DependencyName,
		},
		"spec": map[string]any{
			"source": map[string]any{
				"sourceType": "Catalog",
				"catalog": map[string]any{
					"packageName": testV1DependencyPackage,
				},
			},
		},
		"status": map[string]any{
			"conditions": []any{map[string]any{
				"type":   "Installed",
				"status": "True",
				"reason": "Succeeded",
			}},
			"install": map[string]any{
				"bundle": map[string]any{"version": testV1DependencyVersion},
			},
		},
	}}
	controllerRuntimeClient := crfake.NewClientBuilder().
		WithScheme(scheme).
		WithRuntimeObjects(extension).
		Build()
	command := &InstallCommand{
		client:  client.NewForTesting(client.TestClientConfig{ControllerRuntime: controllerRuntimeClient}),
		Timeout: time.Second,
	}

	version, err := command.waitForOperator(t.Context(), &bytes.Buffer{}, testV1DependencyPackage)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(version).To(Equal(testV1DependencyVersion))
}
