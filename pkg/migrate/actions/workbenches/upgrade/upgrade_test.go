//nolint:testpackage // Tests internal implementation (unexported helpers)
package upgrade

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/blang/semver/v4"
	"github.com/spf13/pflag"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8stesting "k8s.io/client-go/testing"

	"github.com/opendatahub-io/odh-cli/pkg/migrate/action"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
	"github.com/opendatahub-io/odh-cli/pkg/util/iostreams"

	. "github.com/onsi/gomega"
)

// --- Test Fixtures ---

type notebookOpts struct {
	Labels      map[string]any
	Annotations map[string]any
	Containers  []any
	Status      map[string]any
}

func newNotebook(name, namespace string, opts notebookOpts) *unstructured.Unstructured {
	metadata := map[string]any{
		"name":      name,
		"namespace": namespace,
	}

	if len(opts.Labels) > 0 {
		metadata["labels"] = opts.Labels
	}

	if len(opts.Annotations) > 0 {
		metadata["annotations"] = opts.Annotations
	}

	obj := map[string]any{
		"apiVersion": resources.Notebook.APIVersion(),
		"kind":       resources.Notebook.Kind,
		"metadata":   metadata,
	}

	if opts.Containers != nil {
		obj["spec"] = map[string]any{
			"template": map[string]any{
				"spec": map[string]any{
					"containers": opts.Containers,
				},
			},
		}
	}

	if opts.Status != nil {
		obj["status"] = opts.Status
	}

	return &unstructured.Unstructured{Object: obj}
}

func container(name, image string) map[string]any {
	return map[string]any{
		"name":  name,
		"image": image,
	}
}

func containerWithEnv(name, image string, envVars []map[string]any) map[string]any {
	env := make([]any, len(envVars))
	for i, e := range envVars {
		env[i] = e
	}

	return map[string]any{
		"name":  name,
		"image": image,
		"env":   env,
	}
}

func containerWithVolumeMounts(name, image string, mounts []map[string]any) map[string]any {
	vms := make([]any, len(mounts))
	for i, m := range mounts {
		vms[i] = m
	}

	return map[string]any{
		"name":         name,
		"image":        image,
		"volumeMounts": vms,
	}
}

func stoppedAnnotations() map[string]any {
	return map[string]any{
		"kubeflow-resource-stopped": "2026-01-15T00:00:00Z",
	}
}

func dashboardAnnotations() map[string]any {
	return map[string]any{
		"notebooks.opendatahub.io/last-size-selection": `{"name":"Small"}`,
	}
}

func stoppedDashboardAnnotations() map[string]any {
	return map[string]any{
		"kubeflow-resource-stopped":                    "2026-01-15T00:00:00Z",
		"notebooks.opendatahub.io/last-size-selection": `{"name":"Small"}`,
		"opendatahub.io/accelerator-name":              "migrated-gpu",
	}
}

func newScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	_ = metav1.AddMetaToScheme(scheme)

	return scheme
}

func newFakeClient(scheme *runtime.Scheme, objects ...runtime.Object) client.Client {
	listKinds := map[schema.GroupVersionResource]string{
		resources.Notebook.GVR(): resources.Notebook.ListKind(),
	}

	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, listKinds, objects...)

	return client.NewForTesting(client.TestClientConfig{
		Dynamic: dynamicClient,
	})
}

func newFailingListClient() client.Client {
	scheme := newScheme()
	listKinds := map[schema.GroupVersionResource]string{
		resources.Notebook.GVR(): resources.Notebook.ListKind(),
	}

	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, listKinds)
	dynamicClient.PrependReactor("list", "notebooks", func(_ k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("simulated API server error")
	})

	return client.NewForTesting(client.TestClientConfig{
		Dynamic: dynamicClient,
	})
}

func newFailingUpdateClient(scheme *runtime.Scheme, objects ...runtime.Object) client.Client {
	listKinds := map[schema.GroupVersionResource]string{
		resources.Notebook.GVR(): resources.Notebook.ListKind(),
	}

	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, listKinds, objects...)
	dynamicClient.PrependReactor("update", "notebooks", func(_ k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("simulated update error")
	})

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

func withOutputDir(dir string) func(*action.Target) {
	return func(t *action.Target) {
		t.OutputDir = dir
	}
}

// --- Action Metadata Tests ---

func TestWorkbenchUpgradeAction_Metadata(t *testing.T) {
	g := NewWithT(t)

	a := &WorkbenchUpgradeAction{}

	g.Expect(a.ID()).To(Equal("workbenches.upgrade-2x-to-3x"))
	g.Expect(a.Name()).To(Equal("Upgrade workbenches from 2.x to 3.x"))
	g.Expect(a.Description()).To(ContainSubstring("container names"))
	g.Expect(a.Group()).To(Equal(action.GroupMigration))
}

func TestWorkbenchUpgradeAction_CanApply(t *testing.T) {
	v3 := semver.MustParse("3.0.0")
	v33 := semver.MustParse("3.3.0")
	v35 := semver.MustParse("3.5.0")

	tests := []struct {
		name          string
		current       string
		targetVersion *semver.Version
		expected      bool
	}{
		{"applies for 2.25 -> 3.0", "2.25.0", &v3, true},
		{"applies for 2.16 -> 3.0", "2.16.0", &v3, true},
		{"applies for 2.25 -> 3.3", "2.25.0", &v33, true},
		{"applies for 2.25 -> 3.5", "2.25.0", &v35, true},
		{"does not apply for 2.15 (too old)", "2.15.0", &v3, false},
		{"does not apply for 2.0 (too old)", "2.0.0", &v3, false},
		{"does not apply for 3.0 current", "3.0.0", &v3, false},
		{"does not apply for 1.0 current", "1.0.0", &v3, false},
		{"does not apply for nil target version", "2.25.0", nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			a := &WorkbenchUpgradeAction{}
			ver := semver.MustParse(tt.current)
			target := action.Target{
				CurrentVersion: &ver,
				TargetVersion:  tt.targetVersion,
			}

			g.Expect(a.CanApply(target)).To(Equal(tt.expected))
		})
	}
}

func TestWorkbenchUpgradeAction_CanApply_NilCurrentVersion(t *testing.T) {
	g := NewWithT(t)

	a := &WorkbenchUpgradeAction{}
	target := action.Target{CurrentVersion: nil, TargetVersion: nil}
	g.Expect(a.CanApply(target)).To(BeFalse())
}

func TestWorkbenchUpgradeAction_PrepareNotNil(t *testing.T) {
	g := NewWithT(t)

	a := &WorkbenchUpgradeAction{}
	g.Expect(a.Prepare()).ToNot(BeNil())
}

func TestWorkbenchUpgradeAction_RunNotNil(t *testing.T) {
	g := NewWithT(t)

	a := &WorkbenchUpgradeAction{}
	g.Expect(a.Run()).ToNot(BeNil())
}

// --- Helper Function Tests ---

func TestIsStopped(t *testing.T) {
	tests := []struct {
		name     string
		nb       *unstructured.Unstructured
		expected bool
	}{
		{
			"stopped notebook",
			newNotebook("nb1", "ns1", notebookOpts{
				Annotations: map[string]any{
					"kubeflow-resource-stopped": "2026-01-15T00:00:00Z",
				},
			}),
			true,
		},
		{
			"running notebook (no annotation)",
			newNotebook("nb1", "ns1", notebookOpts{}),
			false,
		},
		{
			"running notebook (nil annotations)",
			newNotebook("nb1", "ns1", notebookOpts{
				Annotations: nil,
			}),
			false,
		},
		{
			"notebook with controller lock (still counts as stopped)",
			newNotebook("nb1", "ns1", notebookOpts{
				Annotations: map[string]any{
					"kubeflow-resource-stopped": "odh-notebook-controller-lock",
				},
			}),
			true,
		},
		{
			"empty annotation value treated as not stopped",
			newNotebook("nb1", "ns1", notebookOpts{
				Annotations: map[string]any{
					"kubeflow-resource-stopped": "",
				},
			}),
			false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			g.Expect(isStopped(tt.nb)).To(Equal(tt.expected))
		})
	}
}

func TestHasDashboardAnnotation(t *testing.T) {
	tests := []struct {
		name     string
		nb       *unstructured.Unstructured
		expected bool
	}{
		{
			"has accelerator annotation",
			newNotebook("nb1", "ns1", notebookOpts{
				Annotations: map[string]any{
					"opendatahub.io/accelerator-name": "gpu",
				},
			}),
			true,
		},
		{
			"has size selection annotation",
			newNotebook("nb1", "ns1", notebookOpts{
				Annotations: map[string]any{
					"notebooks.opendatahub.io/last-size-selection": `{"name":"Small"}`,
				},
			}),
			true,
		},
		{
			"no dashboard annotations",
			newNotebook("nb1", "ns1", notebookOpts{}),
			false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			g.Expect(hasDashboardAnnotation(tt.nb)).To(Equal(tt.expected))
		})
	}
}

func TestIsInfrastructureContainer(t *testing.T) {
	tests := []struct {
		name     string
		ctrName  string
		image    string
		expected bool
	}{
		{"oauth-proxy sidecar", "oauth-proxy", "registry/ose-oauth-proxy-rhel9:latest", true},
		{"oauth-proxy but custom image", "oauth-proxy", "custom-image:latest", false},
		{"workload named oauth-proxy", "oauth-proxy", "jupyter:latest", false},
		{"normal container", "my-notebook", "jupyter:latest", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			g.Expect(isInfrastructureContainer(tt.ctrName, tt.image)).To(Equal(tt.expected))
		})
	}
}

func TestExtractWorkloadContainers(t *testing.T) {
	tests := []struct {
		name          string
		nb            *unstructured.Unstructured
		expectedCount int
		expectedNames []string
	}{
		{
			"single workload container",
			newNotebook("nb1", "ns1", notebookOpts{
				Containers: []any{
					container("nb1", "jupyter:latest"),
				},
			}),
			1, []string{"nb1"},
		},
		{
			"workload with oauth-proxy sidecar",
			newNotebook("nb1", "ns1", notebookOpts{
				Containers: []any{
					container("nb1", "jupyter:latest"),
					container("oauth-proxy", "registry/ose-oauth-proxy-rhel9:latest"),
				},
			}),
			1, []string{"nb1"},
		},
		{
			"multiple workload containers",
			newNotebook("nb1", "ns1", notebookOpts{
				Containers: []any{
					container("main", "jupyter:latest"),
					container("sidecar", "helper:latest"),
				},
			}),
			2, []string{"main", "sidecar"},
		},
		{
			"no containers",
			newNotebook("nb1", "ns1", notebookOpts{
				Containers: []any{},
			}),
			0, nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			containers, err := extractWorkloadContainers(tt.nb)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(containers).To(HaveLen(tt.expectedCount))

			for i, name := range tt.expectedNames {
				g.Expect(containers[i].Name).To(Equal(name))
			}
		})
	}
}

func TestHasContainerNameMismatch(t *testing.T) {
	tests := []struct {
		name     string
		nb       *unstructured.Unstructured
		expected bool
	}{
		{
			"mismatch with dashboard annotation",
			newNotebook("my-notebook", "ns1", notebookOpts{
				Annotations: dashboardAnnotations(),
				Containers:  []any{container("old-name", "jupyter:latest")},
			}),
			true,
		},
		{
			"no mismatch",
			newNotebook("my-notebook", "ns1", notebookOpts{
				Annotations: dashboardAnnotations(),
				Containers:  []any{container("my-notebook", "jupyter:latest")},
			}),
			false,
		},
		{
			"mismatch but no dashboard annotation (not checked)",
			newNotebook("my-notebook", "ns1", notebookOpts{
				Containers: []any{container("old-name", "jupyter:latest")},
			}),
			false,
		},
		{
			"no containers",
			newNotebook("my-notebook", "ns1", notebookOpts{
				Annotations: dashboardAnnotations(),
				Containers:  []any{},
			}),
			false,
		},
		{
			"multiple workload containers skipped",
			newNotebook("my-notebook", "ns1", notebookOpts{
				Annotations: dashboardAnnotations(),
				Containers: []any{
					container("wrong-name", "jupyter:latest"),
					container("sidecar", "helper:latest"),
				},
			}),
			false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			hasMismatch, err := hasContainerNameMismatch(tt.nb)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(hasMismatch).To(Equal(tt.expected))
		})
	}
}

func TestFixContainerName(t *testing.T) {
	t.Run("renames primary container to match CR name", func(t *testing.T) {
		g := NewWithT(t)

		nb := newNotebook("my-notebook", "ns1", notebookOpts{
			Annotations: dashboardAnnotations(),
			Containers: []any{
				container("old-name", "jupyter:latest"),
			},
		})

		oldName, err := fixContainerName(nb)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(oldName).To(Equal("old-name"))

		containers, err := extractWorkloadContainers(nb)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(containers).To(HaveLen(1))
		g.Expect(containers[0].Name).To(Equal("my-notebook"))
		g.Expect(containers[0].Image).To(Equal("jupyter:latest"))
	})

	t.Run("skips infrastructure containers", func(t *testing.T) {
		g := NewWithT(t)

		nb := newNotebook("my-notebook", "ns1", notebookOpts{
			Annotations: dashboardAnnotations(),
			Containers: []any{
				container("oauth-proxy", "registry/ose-oauth-proxy-rhel9:latest"),
				container("old-name", "jupyter:latest"),
			},
		})

		oldName, err := fixContainerName(nb)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(oldName).To(Equal("old-name"))

		containers, err := extractWorkloadContainers(nb)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(containers).To(HaveLen(1))
		g.Expect(containers[0].Name).To(Equal("my-notebook"))
	})

	t.Run("preserves environment variables", func(t *testing.T) {
		g := NewWithT(t)

		nb := newNotebook("my-notebook", "ns1", notebookOpts{
			Annotations: dashboardAnnotations(),
			Containers: []any{
				containerWithEnv("old-name", "jupyter:latest", []map[string]any{
					{"name": "MY_VAR", "value": "my-value"},
					{"name": "SECRET_REF", "valueFrom": map[string]any{
						"secretKeyRef": map[string]any{"name": "secret", "key": "key"},
					}},
				}),
			},
		})

		_, err := fixContainerName(nb)
		g.Expect(err).ToNot(HaveOccurred())

		containers, err := extractWorkloadContainers(nb)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(containers[0].Name).To(Equal("my-notebook"))
	})

	t.Run("preserves volume mounts", func(t *testing.T) {
		g := NewWithT(t)

		nb := newNotebook("my-notebook", "ns1", notebookOpts{
			Annotations: dashboardAnnotations(),
			Containers: []any{
				containerWithVolumeMounts("old-name", "jupyter:latest", []map[string]any{
					{"name": "data-vol", "mountPath": "/data"},
					{"name": "config-vol", "mountPath": "/config"},
				}),
			},
		})

		_, err := fixContainerName(nb)
		g.Expect(err).ToNot(HaveOccurred())

		containers, err := extractWorkloadContainers(nb)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(containers[0].Name).To(Equal("my-notebook"))
	})

	t.Run("rejects multiple workload containers", func(t *testing.T) {
		g := NewWithT(t)

		nb := newNotebook("my-notebook", "ns1", notebookOpts{
			Annotations: dashboardAnnotations(),
			Containers: []any{
				container("wrong-name", "jupyter:latest"),
				container("sidecar", "helper:latest"),
			},
		})

		_, err := fixContainerName(nb)
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("expected exactly 1 workload container, found 2"))
	})
}

// --- Prepare Task Tests ---

func TestPrepareTask_Validate_NoNotebooks(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	k8sClient := newFakeClient(newScheme())
	target := newTarget(k8sClient)

	a := &WorkbenchUpgradeAction{}
	prepTask := a.Prepare()

	result, err := prepTask.Validate(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).ToNot(BeNil())
	g.Expect(result.Status.Completed).To(BeTrue())
}

func TestPrepareTask_Validate_WithNotebooks(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	nb := newNotebook("nb1", "ns1", notebookOpts{
		Annotations: stoppedAnnotations(),
		Containers:  []any{container("nb1", "jupyter:latest")},
	})

	k8sClient := newFakeClient(newScheme(), nb)
	target := newTarget(k8sClient)

	a := &WorkbenchUpgradeAction{}
	prepTask := a.Prepare()

	result, err := prepTask.Validate(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).ToNot(BeNil())
	g.Expect(result.Status.Completed).To(BeTrue())
}

func TestPrepareTask_Execute_NoNotebooks(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	k8sClient := newFakeClient(newScheme())
	target := newTarget(k8sClient)

	a := &WorkbenchUpgradeAction{}
	prepTask := a.Prepare()

	result, err := prepTask.Execute(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).ToNot(BeNil())
	g.Expect(result.Status.Completed).To(BeTrue())
}

func TestPrepareTask_Execute_DryRun(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	nb := newNotebook("nb1", "ns1", notebookOpts{
		Annotations: stoppedAnnotations(),
		Containers:  []any{container("nb1", "jupyter:latest")},
	})

	k8sClient := newFakeClient(newScheme(), nb)
	target := newTarget(k8sClient, withDryRun)

	a := &WorkbenchUpgradeAction{}
	prepTask := a.Prepare()

	result, err := prepTask.Execute(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).ToNot(BeNil())
	g.Expect(result.Status.Completed).To(BeTrue())
}

func TestPrepareTask_Execute_WithBackup(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	nb := newNotebook("nb1", "ns1", notebookOpts{
		Annotations: stoppedAnnotations(),
		Containers:  []any{container("nb1", "jupyter:latest")},
	})

	outputDir := t.TempDir()
	k8sClient := newFakeClient(newScheme(), nb)
	target := newTarget(k8sClient, withOutputDir(outputDir))

	a := &WorkbenchUpgradeAction{}
	prepTask := a.Prepare()

	result, err := prepTask.Execute(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).ToNot(BeNil())
	g.Expect(result.Status.Completed).To(BeTrue())
}

func TestPrepareTask_Execute_BackupFailure_ReturnsError(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	nb := newNotebook("nb1", "ns1", notebookOpts{
		Annotations: stoppedAnnotations(),
		Containers:  []any{container("nb1", "jupyter:latest")},
	})

	blocker := filepath.Join(t.TempDir(), "not-a-dir")
	g.Expect(os.WriteFile(blocker, []byte("x"), 0o600)).To(Succeed())

	k8sClient := newFakeClient(newScheme(), nb)
	target := newTarget(k8sClient, withOutputDir(filepath.Join(blocker, "sub")))

	a := &WorkbenchUpgradeAction{}
	prepTask := a.Prepare()

	result, err := prepTask.Execute(ctx, target)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("prepare backup failed"))
	g.Expect(result).ToNot(BeNil())
}

// --- Run Task Tests ---

func TestRunTask_Validate_NoNotebooks(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	k8sClient := newFakeClient(newScheme())
	target := newTarget(k8sClient)

	a := &WorkbenchUpgradeAction{}
	runTask := a.Run()

	result, err := runTask.Validate(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).ToNot(BeNil())
	g.Expect(result.Status.Completed).To(BeTrue())
}

func TestRunTask_Validate_WithMismatch(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	nb := newNotebook("my-notebook", "ns1", notebookOpts{
		Annotations: stoppedDashboardAnnotations(),
		Containers:  []any{container("old-name", "jupyter:latest")},
	})

	k8sClient := newFakeClient(newScheme(), nb)
	target := newTarget(k8sClient)

	a := &WorkbenchUpgradeAction{}
	runTask := a.Run()

	result, err := runTask.Validate(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).ToNot(BeNil())
	g.Expect(result.Status.Completed).To(BeTrue())
}

func TestRunTask_Execute_NoNotebooks(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	k8sClient := newFakeClient(newScheme())
	target := newTarget(k8sClient)

	a := &WorkbenchUpgradeAction{}
	runTask := a.Run()

	result, err := runTask.Execute(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).ToNot(BeNil())
	g.Expect(result.Status.Completed).To(BeTrue())
}

func TestRunTask_Execute_AllCorrectNames(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	nb := newNotebook("my-notebook", "ns1", notebookOpts{
		Annotations: stoppedDashboardAnnotations(),
		Containers:  []any{container("my-notebook", "jupyter:latest")},
	})

	k8sClient := newFakeClient(newScheme(), nb)
	target := newTarget(k8sClient)

	a := &WorkbenchUpgradeAction{}
	runTask := a.Run()

	result, err := runTask.Execute(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).ToNot(BeNil())
	g.Expect(result.Status.Completed).To(BeTrue())
}

func TestRunTask_Execute_FixContainerNameMismatch(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	nb := newNotebook("my-notebook", "ns1", notebookOpts{
		Annotations: stoppedDashboardAnnotations(),
		Containers:  []any{container("old-name", "jupyter:latest")},
	})

	k8sClient := newFakeClient(newScheme(), nb)
	target := newTarget(k8sClient)

	a := &WorkbenchUpgradeAction{}
	runTask := a.Run()

	actionResult, err := runTask.Execute(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(actionResult).ToNot(BeNil())
	g.Expect(actionResult.Status.Completed).To(BeTrue())

	updated, err := k8sClient.Dynamic().Resource(resources.Notebook.GVR()).
		Namespace("ns1").Get(context.Background(), "my-notebook", metav1.GetOptions{})
	g.Expect(err).ToNot(HaveOccurred())

	containers, _ := extractWorkloadContainersFromUnstructured(updated)
	g.Expect(containers).To(HaveLen(1))
	g.Expect(containers[0]["name"]).To(Equal("my-notebook"))
	g.Expect(containers[0]["image"]).To(Equal("jupyter:latest"))
}

func TestRunTask_Execute_DryRun_NoChanges(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	nb := newNotebook("my-notebook", "ns1", notebookOpts{
		Annotations: stoppedDashboardAnnotations(),
		Containers:  []any{container("old-name", "jupyter:latest")},
	})

	k8sClient := newFakeClient(newScheme(), nb)
	target := newTarget(k8sClient, withDryRun)

	a := &WorkbenchUpgradeAction{}
	runTask := a.Run()

	actionResult, err := runTask.Execute(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(actionResult).ToNot(BeNil())
	g.Expect(actionResult.Status.Completed).To(BeTrue())

	original, err := k8sClient.Dynamic().Resource(resources.Notebook.GVR()).
		Namespace("ns1").Get(context.Background(), "my-notebook", metav1.GetOptions{})
	g.Expect(err).ToNot(HaveOccurred())

	containers, _ := extractWorkloadContainersFromUnstructured(original)
	g.Expect(containers).To(HaveLen(1))
	g.Expect(containers[0]["name"]).To(Equal("old-name"))
}

func TestRunTask_Execute_SkipsNonStoppedNotebooks(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	stoppedNb := newNotebook("stopped-nb", "ns1", notebookOpts{
		Annotations: stoppedDashboardAnnotations(),
		Containers:  []any{container("old-name", "jupyter:latest")},
	})

	runningNb := newNotebook("running-nb", "ns1", notebookOpts{
		Annotations: dashboardAnnotations(),
		Containers:  []any{container("wrong-name", "jupyter:latest")},
	})

	k8sClient := newFakeClient(newScheme(), stoppedNb, runningNb)
	target := newTarget(k8sClient)

	a := &WorkbenchUpgradeAction{}
	runTask := a.Run()

	actionResult, err := runTask.Execute(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(actionResult).ToNot(BeNil())
	g.Expect(actionResult.Status.Completed).To(BeTrue())

	stoppedUpdated, err := k8sClient.Dynamic().Resource(resources.Notebook.GVR()).
		Namespace("ns1").Get(context.Background(), "stopped-nb", metav1.GetOptions{})
	g.Expect(err).ToNot(HaveOccurred())
	stoppedContainers, _ := extractWorkloadContainersFromUnstructured(stoppedUpdated)
	g.Expect(stoppedContainers[0]["name"]).To(Equal("stopped-nb"))

	runningOriginal, err := k8sClient.Dynamic().Resource(resources.Notebook.GVR()).
		Namespace("ns1").Get(context.Background(), "running-nb", metav1.GetOptions{})
	g.Expect(err).ToNot(HaveOccurred())
	runningContainers, _ := extractWorkloadContainersFromUnstructured(runningOriginal)
	g.Expect(runningContainers[0]["name"]).To(Equal("wrong-name"))
}

func TestRunTask_Execute_MultipleNamespaces(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	nb1 := newNotebook("nb1", "ns-alpha", notebookOpts{
		Annotations: stoppedDashboardAnnotations(),
		Containers:  []any{container("wrong-alpha", "jupyter:latest")},
	})

	nb2 := newNotebook("nb2", "ns-beta", notebookOpts{
		Annotations: stoppedDashboardAnnotations(),
		Containers:  []any{container("wrong-beta", "rstudio:latest")},
	})

	k8sClient := newFakeClient(newScheme(), nb1, nb2)
	target := newTarget(k8sClient)

	a := &WorkbenchUpgradeAction{}
	runTask := a.Run()

	actionResult, err := runTask.Execute(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(actionResult).ToNot(BeNil())
	g.Expect(actionResult.Status.Completed).To(BeTrue())

	updated1, _ := k8sClient.Dynamic().Resource(resources.Notebook.GVR()).
		Namespace("ns-alpha").Get(context.Background(), "nb1", metav1.GetOptions{})
	containers1, _ := extractWorkloadContainersFromUnstructured(updated1)
	g.Expect(containers1[0]["name"]).To(Equal("nb1"))

	updated2, _ := k8sClient.Dynamic().Resource(resources.Notebook.GVR()).
		Namespace("ns-beta").Get(context.Background(), "nb2", metav1.GetOptions{})
	containers2, _ := extractWorkloadContainersFromUnstructured(updated2)
	g.Expect(containers2[0]["name"]).To(Equal("nb2"))
}

func TestRunTask_Execute_MultipleWorkloadContainers_Skipped(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	nb := newNotebook("my-notebook", "ns1", notebookOpts{
		Annotations: stoppedDashboardAnnotations(),
		Containers: []any{
			container("oauth-proxy", "registry/ose-oauth-proxy-rhel9:latest"),
			container("wrong-name", "jupyter:latest"),
			container("helper", "sidecar:latest"),
		},
	})

	k8sClient := newFakeClient(newScheme(), nb)
	target := newTarget(k8sClient)

	a := &WorkbenchUpgradeAction{}
	runTask := a.Run()

	actionResult, err := runTask.Execute(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(actionResult).ToNot(BeNil())
	g.Expect(actionResult.Status.Completed).To(BeTrue())

	updated, _ := k8sClient.Dynamic().Resource(resources.Notebook.GVR()).
		Namespace("ns1").Get(context.Background(), "my-notebook", metav1.GetOptions{})
	allContainers, _ := extractWorkloadContainersFromUnstructured(updated)

	g.Expect(allContainers).To(HaveLen(3))
	g.Expect(allContainers[0]["name"]).To(Equal("oauth-proxy"))
	g.Expect(allContainers[1]["name"]).To(Equal("wrong-name"))
	g.Expect(allContainers[2]["name"]).To(Equal("helper"))
}

func TestRunTask_Execute_PreservesEnvVars(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	nb := newNotebook("my-notebook", "ns1", notebookOpts{
		Annotations: stoppedDashboardAnnotations(),
		Containers: []any{
			containerWithEnv("wrong-name", "jupyter:latest", []map[string]any{
				{"name": "VAR1", "value": "val1"},
				{"name": "VAR2", "value": "val2"},
			}),
		},
	})

	k8sClient := newFakeClient(newScheme(), nb)
	target := newTarget(k8sClient)

	a := &WorkbenchUpgradeAction{}
	runTask := a.Run()

	_, err := runTask.Execute(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())

	updated, _ := k8sClient.Dynamic().Resource(resources.Notebook.GVR()).
		Namespace("ns1").Get(context.Background(), "my-notebook", metav1.GetOptions{})
	allContainers, _ := extractWorkloadContainersFromUnstructured(updated)

	g.Expect(allContainers[0]["name"]).To(Equal("my-notebook"))
	envVars, ok := allContainers[0]["env"].([]any)
	g.Expect(ok).To(BeTrue())
	g.Expect(envVars).To(HaveLen(2))
}

func TestRunTask_Execute_PreservesVolumeMounts(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	nb := newNotebook("my-notebook", "ns1", notebookOpts{
		Annotations: stoppedDashboardAnnotations(),
		Containers: []any{
			containerWithVolumeMounts("wrong-name", "jupyter:latest", []map[string]any{
				{"name": "pvc-data", "mountPath": "/opt/data"},
				{"name": "config", "mountPath": "/etc/config"},
			}),
		},
	})

	k8sClient := newFakeClient(newScheme(), nb)
	target := newTarget(k8sClient)

	a := &WorkbenchUpgradeAction{}
	runTask := a.Run()

	_, err := runTask.Execute(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())

	updated, _ := k8sClient.Dynamic().Resource(resources.Notebook.GVR()).
		Namespace("ns1").Get(context.Background(), "my-notebook", metav1.GetOptions{})
	allContainers, _ := extractWorkloadContainersFromUnstructured(updated)

	g.Expect(allContainers[0]["name"]).To(Equal("my-notebook"))
	mounts, ok := allContainers[0]["volumeMounts"].([]any)
	g.Expect(ok).To(BeTrue())
	g.Expect(mounts).To(HaveLen(2))
}

func TestRunTask_Execute_NoDashboardAnnotation_NoChange(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	nb := newNotebook("my-notebook", "ns1", notebookOpts{
		Annotations: map[string]any{
			"kubeflow-resource-stopped": "2026-01-15T00:00:00Z",
		},
		Containers: []any{container("different-name", "jupyter:latest")},
	})

	k8sClient := newFakeClient(newScheme(), nb)
	target := newTarget(k8sClient)

	a := &WorkbenchUpgradeAction{}
	runTask := a.Run()

	actionResult, err := runTask.Execute(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(actionResult).ToNot(BeNil())
	g.Expect(actionResult.Status.Completed).To(BeTrue())

	original, _ := k8sClient.Dynamic().Resource(resources.Notebook.GVR()).
		Namespace("ns1").Get(context.Background(), "my-notebook", metav1.GetOptions{})
	containers, _ := extractWorkloadContainersFromUnstructured(original)
	g.Expect(containers[0]["name"]).To(Equal("different-name"))
}

func TestRunTask_Execute_AllNonStopped(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	nb := newNotebook("running-nb", "ns1", notebookOpts{
		Annotations: dashboardAnnotations(),
		Containers:  []any{container("wrong-name", "jupyter:latest")},
	})

	k8sClient := newFakeClient(newScheme(), nb)
	target := newTarget(k8sClient)

	a := &WorkbenchUpgradeAction{}
	runTask := a.Run()

	actionResult, err := runTask.Execute(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(actionResult).ToNot(BeNil())
	g.Expect(actionResult.Status.Completed).To(BeTrue())

	original, _ := k8sClient.Dynamic().Resource(resources.Notebook.GVR()).
		Namespace("ns1").Get(context.Background(), "running-nb", metav1.GetOptions{})
	containers, _ := extractWorkloadContainersFromUnstructured(original)
	g.Expect(containers[0]["name"]).To(Equal("wrong-name"))
}

func TestRunTask_Execute_CustomImages(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	nb := newNotebook("custom-nb", "ns1", notebookOpts{
		Annotations: stoppedDashboardAnnotations(),
		Containers:  []any{container("wrong-name", "my-registry.example.com/custom-jupyter:v1")},
	})

	k8sClient := newFakeClient(newScheme(), nb)
	target := newTarget(k8sClient)

	a := &WorkbenchUpgradeAction{}
	runTask := a.Run()

	actionResult, err := runTask.Execute(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(actionResult).ToNot(BeNil())
	g.Expect(actionResult.Status.Completed).To(BeTrue())

	updated, _ := k8sClient.Dynamic().Resource(resources.Notebook.GVR()).
		Namespace("ns1").Get(context.Background(), "custom-nb", metav1.GetOptions{})
	containers, _ := extractWorkloadContainersFromUnstructured(updated)
	g.Expect(containers[0]["name"]).To(Equal("custom-nb"))
	g.Expect(containers[0]["image"]).To(Equal("my-registry.example.com/custom-jupyter:v1"))
}

func TestRunTask_Execute_MountedPVCs(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	nb := newNotebook("pvc-nb", "ns1", notebookOpts{
		Annotations: stoppedDashboardAnnotations(),
		Containers: []any{
			containerWithVolumeMounts("wrong-name", "jupyter:latest", []map[string]any{
				{"name": "data-pvc", "mountPath": "/opt/app-root/src"},
			}),
		},
	})

	k8sClient := newFakeClient(newScheme(), nb)
	target := newTarget(k8sClient)

	a := &WorkbenchUpgradeAction{}
	runTask := a.Run()

	_, err := runTask.Execute(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())

	updated, _ := k8sClient.Dynamic().Resource(resources.Notebook.GVR()).
		Namespace("ns1").Get(context.Background(), "pvc-nb", metav1.GetOptions{})
	containers, _ := extractWorkloadContainersFromUnstructured(updated)
	g.Expect(containers[0]["name"]).To(Equal("pvc-nb"))

	mounts, ok := containers[0]["volumeMounts"].([]any)
	g.Expect(ok).To(BeTrue())
	g.Expect(mounts).To(HaveLen(1))

	mount, ok := mounts[0].(map[string]any)
	g.Expect(ok).To(BeTrue())
	g.Expect(mount["mountPath"]).To(Equal("/opt/app-root/src"))
}

// --- HandleNonStoppedNotebooks Tests ---

func TestHandleNonStoppedNotebooks_AllStopped(t *testing.T) {
	g := NewWithT(t)

	nbs := []*unstructured.Unstructured{
		newNotebook("nb1", "ns1", notebookOpts{
			Annotations: map[string]any{"kubeflow-resource-stopped": "ts1"},
		}),
		newNotebook("nb2", "ns1", notebookOpts{
			Annotations: map[string]any{"kubeflow-resource-stopped": "ts2"},
		}),
	}

	io := iostreams.NewIOStreams(&bytes.Buffer{}, &bytes.Buffer{}, &bytes.Buffer{})
	recorder := action.NewVerboseRootRecorder(io)

	a := &WorkbenchUpgradeAction{}
	target := action.Target{
		DryRun:      false,
		SkipConfirm: true,
		IO:          io,
		Recorder:    recorder,
	}

	stopped := a.handleNonStoppedNotebooks(target, nbs, recorder)
	g.Expect(stopped).To(HaveLen(2))
}

func TestHandleNonStoppedNotebooks_MixedState(t *testing.T) {
	g := NewWithT(t)

	nbs := []*unstructured.Unstructured{
		newNotebook("stopped-nb", "ns1", notebookOpts{
			Annotations: map[string]any{"kubeflow-resource-stopped": "ts1"},
		}),
		newNotebook("running-nb", "ns1", notebookOpts{}),
		newNotebook("also-stopped", "ns2", notebookOpts{
			Annotations: map[string]any{"kubeflow-resource-stopped": "ts2"},
		}),
	}

	io := iostreams.NewIOStreams(&bytes.Buffer{}, &bytes.Buffer{}, &bytes.Buffer{})
	recorder := action.NewVerboseRootRecorder(io)

	a := &WorkbenchUpgradeAction{}
	target := action.Target{
		DryRun:      false,
		SkipConfirm: true,
		IO:          io,
		Recorder:    recorder,
	}

	stopped := a.handleNonStoppedNotebooks(target, nbs, recorder)
	g.Expect(stopped).To(HaveLen(2))
}

func TestHandleNonStoppedNotebooks_DryRun(t *testing.T) {
	g := NewWithT(t)

	nbs := []*unstructured.Unstructured{
		newNotebook("running-nb", "ns1", notebookOpts{}),
	}

	io := iostreams.NewIOStreams(&bytes.Buffer{}, &bytes.Buffer{}, &bytes.Buffer{})
	recorder := action.NewVerboseRootRecorder(io)

	a := &WorkbenchUpgradeAction{}
	target := action.Target{
		DryRun:      true,
		SkipConfirm: true,
		IO:          io,
		Recorder:    recorder,
	}

	stopped := a.handleNonStoppedNotebooks(target, nbs, recorder)
	g.Expect(stopped).To(BeEmpty())
}

// --- Step Recording Tests ---

func TestRunTask_StepRecording(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	nb := newNotebook("my-nb", "ns1", notebookOpts{
		Annotations: stoppedDashboardAnnotations(),
		Containers:  []any{container("old-name", "jupyter:latest")},
	})

	k8sClient := newFakeClient(newScheme(), nb)
	target := newTarget(k8sClient)

	a := &WorkbenchUpgradeAction{}
	runTask := a.Run()

	actionResult, err := runTask.Execute(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(actionResult.Status.Steps).ToNot(BeEmpty())

	hasFixStep := false

	for _, step := range actionResult.Status.Steps {
		if step.Name == "fix-container-names" {
			hasFixStep = true
		}
	}

	g.Expect(hasFixStep).To(BeTrue())
}

// --- Non-SkipConfirm Path Tests ---

func TestHandleNonStoppedNotebooks_NonSkipConfirm_ShowsWarning(t *testing.T) {
	g := NewWithT(t)

	nbs := []*unstructured.Unstructured{
		newNotebook("stopped-nb", "ns1", notebookOpts{
			Annotations: map[string]any{"kubeflow-resource-stopped": "ts1"},
		}),
		newNotebook("running-nb", "ns2", notebookOpts{}),
	}

	errBuf := &bytes.Buffer{}
	io := iostreams.NewIOStreams(&bytes.Buffer{}, &bytes.Buffer{}, errBuf)
	recorder := action.NewVerboseRootRecorder(io)

	a := &WorkbenchUpgradeAction{}
	target := action.Target{
		DryRun:      false,
		SkipConfirm: false,
		IO:          io,
		Recorder:    recorder,
	}

	stopped := a.handleNonStoppedNotebooks(target, nbs, recorder)
	g.Expect(stopped).To(HaveLen(1))
	g.Expect(errBuf.String()).To(ContainSubstring("non-stopped"))
}

func TestRunTask_Execute_DryRun_WithNonStopped(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	nb := newNotebook("running-nb", "ns1", notebookOpts{
		Annotations: dashboardAnnotations(),
		Containers:  []any{container("wrong-name", "jupyter:latest")},
	})

	k8sClient := newFakeClient(newScheme(), nb)
	target := newTarget(k8sClient, withDryRun)

	a := &WorkbenchUpgradeAction{}
	runTask := a.Run()

	actionResult, err := runTask.Execute(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(actionResult).ToNot(BeNil())
	g.Expect(actionResult.Status.Completed).To(BeTrue())
}

func TestRunTask_Execute_DryRun_WithMismatch(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	nb := newNotebook("my-nb", "ns1", notebookOpts{
		Annotations: stoppedDashboardAnnotations(),
		Containers:  []any{container("old-name", "jupyter:latest")},
	})

	k8sClient := newFakeClient(newScheme(), nb)
	target := newTarget(k8sClient, withDryRun)

	a := &WorkbenchUpgradeAction{}
	runTask := a.Run()

	actionResult, err := runTask.Execute(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(actionResult).ToNot(BeNil())
	g.Expect(actionResult.Status.Completed).To(BeTrue())

	original, _ := k8sClient.Dynamic().Resource(resources.Notebook.GVR()).
		Namespace("ns1").Get(context.Background(), "my-nb", metav1.GetOptions{})
	containers, _ := extractWorkloadContainersFromUnstructured(original)
	g.Expect(containers[0]["name"]).To(Equal("old-name"))
}

func TestRunTask_Validate_AllCorrectNames(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	nb := newNotebook("my-nb", "ns1", notebookOpts{
		Annotations: stoppedDashboardAnnotations(),
		Containers:  []any{container("my-nb", "jupyter:latest")},
	})

	k8sClient := newFakeClient(newScheme(), nb)
	target := newTarget(k8sClient)

	a := &WorkbenchUpgradeAction{}
	runTask := a.Run()

	result, err := runTask.Validate(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).ToNot(BeNil())
	g.Expect(result.Status.Completed).To(BeTrue())
}

func TestRunTask_Validate_WithNonStopped(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	nb := newNotebook("running-nb", "ns1", notebookOpts{
		Annotations: dashboardAnnotations(),
		Containers:  []any{container("wrong-name", "jupyter:latest")},
	})

	k8sClient := newFakeClient(newScheme(), nb)
	target := newTarget(k8sClient)

	a := &WorkbenchUpgradeAction{}
	runTask := a.Run()

	result, err := runTask.Validate(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).ToNot(BeNil())
	g.Expect(result.Status.Completed).To(BeTrue())
}

func TestRunTask_Execute_NonSkipConfirm_UserConfirms(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	nb := newNotebook("my-nb", "ns1", notebookOpts{
		Annotations: stoppedDashboardAnnotations(),
		Containers:  []any{container("old-name", "jupyter:latest")},
	})

	k8sClient := newFakeClient(newScheme(), nb)

	inBuf := bytes.NewBufferString("y\n")
	target := newTarget(k8sClient)
	target.SkipConfirm = false
	target.IO = iostreams.NewIOStreams(inBuf, &bytes.Buffer{}, &bytes.Buffer{})
	target.Recorder = action.NewVerboseRootRecorder(target.IO)

	a := &WorkbenchUpgradeAction{}
	runTask := a.Run()

	actionResult, err := runTask.Execute(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(actionResult).ToNot(BeNil())
	g.Expect(actionResult.Status.Completed).To(BeTrue())

	updated, _ := k8sClient.Dynamic().Resource(resources.Notebook.GVR()).
		Namespace("ns1").Get(context.Background(), "my-nb", metav1.GetOptions{})
	containers, _ := extractWorkloadContainersFromUnstructured(updated)
	g.Expect(containers[0]["name"]).To(Equal("my-nb"))
}

func TestRunTask_Execute_NonSkipConfirm_UserCancels(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	nb := newNotebook("my-nb", "ns1", notebookOpts{
		Annotations: stoppedDashboardAnnotations(),
		Containers:  []any{container("old-name", "jupyter:latest")},
	})

	k8sClient := newFakeClient(newScheme(), nb)

	inBuf := bytes.NewBufferString("n\n")
	target := newTarget(k8sClient)
	target.SkipConfirm = false
	target.IO = iostreams.NewIOStreams(inBuf, &bytes.Buffer{}, &bytes.Buffer{})
	target.Recorder = action.NewVerboseRootRecorder(target.IO)

	a := &WorkbenchUpgradeAction{}
	runTask := a.Run()

	actionResult, err := runTask.Execute(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(actionResult).ToNot(BeNil())
	g.Expect(actionResult.Status.Completed).To(BeTrue())

	original, _ := k8sClient.Dynamic().Resource(resources.Notebook.GVR()).
		Namespace("ns1").Get(context.Background(), "my-nb", metav1.GetOptions{})
	containers, _ := extractWorkloadContainersFromUnstructured(original)
	g.Expect(containers[0]["name"]).To(Equal("old-name"))
}

func TestPrepareTask_Execute_MultipleNamespaces(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	nb1 := newNotebook("nb1", "ns-a", notebookOpts{
		Annotations: stoppedAnnotations(),
		Containers:  []any{container("nb1", "jupyter:latest")},
	})
	nb2 := newNotebook("nb2", "ns-b", notebookOpts{
		Annotations: stoppedAnnotations(),
		Containers:  []any{container("nb2", "rstudio:latest")},
	})

	outputDir := t.TempDir()
	k8sClient := newFakeClient(newScheme(), nb1, nb2)
	target := newTarget(k8sClient, withOutputDir(outputDir))

	a := &WorkbenchUpgradeAction{}
	prepTask := a.Prepare()

	result, err := prepTask.Execute(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).ToNot(BeNil())
	g.Expect(result.Status.Completed).To(BeTrue())
}

func TestPromptBeforeModification_SkipConfirm(t *testing.T) {
	g := NewWithT(t)

	a := &WorkbenchUpgradeAction{}
	io := iostreams.NewIOStreams(&bytes.Buffer{}, &bytes.Buffer{}, &bytes.Buffer{})
	target := action.Target{
		SkipConfirm: true,
		IO:          io,
	}

	g.Expect(a.promptBeforeModification(target, 5)).To(BeTrue())
}

func TestPromptBeforeModification_UserConfirms(t *testing.T) {
	g := NewWithT(t)

	a := &WorkbenchUpgradeAction{}
	inBuf := bytes.NewBufferString("y\n")
	io := iostreams.NewIOStreams(inBuf, &bytes.Buffer{}, &bytes.Buffer{})
	target := action.Target{
		SkipConfirm: false,
		IO:          io,
	}

	g.Expect(a.promptBeforeModification(target, 3)).To(BeTrue())
}

func TestPromptBeforeModification_UserDeclines(t *testing.T) {
	g := NewWithT(t)

	a := &WorkbenchUpgradeAction{}
	inBuf := bytes.NewBufferString("n\n")
	io := iostreams.NewIOStreams(inBuf, &bytes.Buffer{}, &bytes.Buffer{})
	target := action.Target{
		SkipConfirm: false,
		IO:          io,
	}

	g.Expect(a.promptBeforeModification(target, 3)).To(BeFalse())
}

func TestExtractWorkloadContainers_NoSpec(t *testing.T) {
	g := NewWithT(t)

	nb := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.Notebook.APIVersion(),
			"kind":       resources.Notebook.Kind,
			"metadata": map[string]any{
				"name":      "bare-nb",
				"namespace": "ns1",
			},
		},
	}

	_, err := extractWorkloadContainers(nb)
	g.Expect(err).To(HaveOccurred())
}

func TestHasContainerNameMismatch_NoSpec(t *testing.T) {
	g := NewWithT(t)

	nb := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.Notebook.APIVersion(),
			"kind":       resources.Notebook.Kind,
			"metadata": map[string]any{
				"name":      "bare-nb",
				"namespace": "ns1",
				"annotations": map[string]any{
					"notebooks.opendatahub.io/last-size-selection": `{"name":"Small"}`,
				},
			},
		},
	}

	_, err := hasContainerNameMismatch(nb)
	g.Expect(err).To(HaveOccurred())
}

// --- Error Path Tests ---

func TestPrepareTask_Validate_ListError(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	k8sClient := newFailingListClient()
	target := newTarget(k8sClient)

	a := &WorkbenchUpgradeAction{}
	prepTask := a.Prepare()

	result, err := prepTask.Validate(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).ToNot(BeNil())
	g.Expect(result.Status.Completed).To(BeTrue())
}

func TestPrepareTask_Execute_ListError(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	k8sClient := newFailingListClient()
	target := newTarget(k8sClient)

	a := &WorkbenchUpgradeAction{}
	prepTask := a.Prepare()

	result, err := prepTask.Execute(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).ToNot(BeNil())
	g.Expect(result.Status.Completed).To(BeTrue())
}

func TestRunTask_Validate_ListError(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	k8sClient := newFailingListClient()
	target := newTarget(k8sClient)

	a := &WorkbenchUpgradeAction{}
	runTask := a.Run()

	result, err := runTask.Validate(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).ToNot(BeNil())
	g.Expect(result.Status.Completed).To(BeTrue())
}

func TestRunTask_Execute_ListError(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	k8sClient := newFailingListClient()
	target := newTarget(k8sClient)

	a := &WorkbenchUpgradeAction{}
	runTask := a.Run()

	result, err := runTask.Execute(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).ToNot(BeNil())
	g.Expect(result.Status.Completed).To(BeTrue())
}

func TestRunTask_Execute_UpdateError(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	nb := newNotebook("my-nb", "ns1", notebookOpts{
		Annotations: stoppedDashboardAnnotations(),
		Containers:  []any{container("old-name", "jupyter:latest")},
	})

	k8sClient := newFailingUpdateClient(newScheme(), nb)
	target := newTarget(k8sClient)

	a := &WorkbenchUpgradeAction{}
	runTask := a.Run()

	actionResult, err := runTask.Execute(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(actionResult).ToNot(BeNil())
	g.Expect(actionResult.Status.Completed).To(BeTrue())

	updated, _ := k8sClient.Dynamic().Resource(resources.Notebook.GVR()).
		Namespace("ns1").Get(context.Background(), "my-nb", metav1.GetOptions{})
	containers, _ := extractWorkloadContainersFromUnstructured(updated)
	g.Expect(containers[0]["name"]).To(Equal("old-name"))
}

func TestFixContainerNames_WithNoSpecNotebook(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	nbNoSpec := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.Notebook.APIVersion(),
			"kind":       resources.Notebook.Kind,
			"metadata": map[string]any{
				"name":      "broken-nb",
				"namespace": "ns1",
				"annotations": map[string]any{
					"notebooks.opendatahub.io/last-size-selection": `{"name":"Small"}`,
				},
			},
		},
	}

	io := iostreams.NewIOStreams(&bytes.Buffer{}, &bytes.Buffer{}, &bytes.Buffer{})
	recorder := action.NewVerboseRootRecorder(io)

	target := action.Target{
		DryRun:      false,
		SkipConfirm: true,
		IO:          io,
		Recorder:    recorder,
	}

	task := &runTask{action: &WorkbenchUpgradeAction{}}
	task.fixContainerNames(ctx, target, []*unstructured.Unstructured{nbNoSpec})

	result := recorder.Build()
	g.Expect(result).ToNot(BeNil())
}

func TestFixContainerName_NoSpec(t *testing.T) {
	g := NewWithT(t)

	nb := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.Notebook.APIVersion(),
			"kind":       resources.Notebook.Kind,
			"metadata": map[string]any{
				"name":      "bare-nb",
				"namespace": "ns1",
			},
		},
	}

	_, err := fixContainerName(nb)
	g.Expect(err).To(HaveOccurred())
}

func TestBackupNotebooks_InvalidOutputDir(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	nb := newNotebook("nb1", "ns1", notebookOpts{
		Annotations: stoppedAnnotations(),
		Containers:  []any{container("nb1", "jupyter:latest")},
	})

	io := iostreams.NewIOStreams(&bytes.Buffer{}, &bytes.Buffer{}, &bytes.Buffer{})
	recorder := action.NewVerboseRootRecorder(io)

	blocker := filepath.Join(t.TempDir(), "not-a-dir")
	_ = os.WriteFile(blocker, []byte("x"), 0o600)
	invalidDir := filepath.Join(blocker, "sub")

	target := action.Target{
		DryRun:      false,
		SkipConfirm: true,
		OutputDir:   invalidDir,
		IO:          io,
		Recorder:    recorder,
	}

	a := &WorkbenchUpgradeAction{}
	err := a.backupNotebooks(ctx, target, []*unstructured.Unstructured{nb}, recorder)
	g.Expect(err).To(HaveOccurred())

	actionResult := recorder.Build()
	g.Expect(actionResult).ToNot(BeNil())
}

func TestBackupNotebooks_DryRun(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	nb := newNotebook("nb1", "ns1", notebookOpts{
		Annotations: stoppedAnnotations(),
		Containers:  []any{container("nb1", "jupyter:latest")},
	})

	io := iostreams.NewIOStreams(&bytes.Buffer{}, &bytes.Buffer{}, &bytes.Buffer{})
	recorder := action.NewVerboseRootRecorder(io)

	target := action.Target{
		DryRun:      true,
		SkipConfirm: true,
		OutputDir:   "/tmp/backup",
		IO:          io,
		Recorder:    recorder,
	}

	a := &WorkbenchUpgradeAction{}
	err := a.backupNotebooks(ctx, target, []*unstructured.Unstructured{nb}, recorder)
	g.Expect(err).ToNot(HaveOccurred())

	actionResult := recorder.Build()
	g.Expect(actionResult).ToNot(BeNil())
}

func TestRunTask_Validate_MixedNonStoppedWithMismatch(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	stoppedMismatch := newNotebook("stopped-nb", "ns1", notebookOpts{
		Annotations: stoppedDashboardAnnotations(),
		Containers:  []any{container("old-name", "jupyter:latest")},
	})

	runningNb := newNotebook("running-nb", "ns1", notebookOpts{
		Annotations: dashboardAnnotations(),
		Containers:  []any{container("wrong-name", "jupyter:latest")},
	})

	k8sClient := newFakeClient(newScheme(), stoppedMismatch, runningNb)
	target := newTarget(k8sClient)

	a := &WorkbenchUpgradeAction{}
	runTask := a.Run()

	result, err := runTask.Validate(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).ToNot(BeNil())
	g.Expect(result.Status.Completed).To(BeTrue())
}

func TestExtractWorkloadContainers_NonMapEntry(t *testing.T) {
	g := NewWithT(t)

	nb := newNotebook("nb1", "ns1", notebookOpts{
		Containers: []any{
			"not-a-map",
			container("real", "jupyter:latest"),
		},
	})

	containers, err := extractWorkloadContainers(nb)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(containers).To(HaveLen(1))
	g.Expect(containers[0].Name).To(Equal("real"))
}

func TestHandleNonStoppedNotebooks_ForceNonStopped(t *testing.T) {
	g := NewWithT(t)

	nbs := []*unstructured.Unstructured{
		newNotebook("stopped-nb", "ns1", notebookOpts{
			Annotations: map[string]any{"kubeflow-resource-stopped": "ts1"},
		}),
		newNotebook("running-nb", "ns1", notebookOpts{}),
	}

	io := iostreams.NewIOStreams(&bytes.Buffer{}, &bytes.Buffer{}, &bytes.Buffer{})
	recorder := action.NewVerboseRootRecorder(io)

	a := &WorkbenchUpgradeAction{ForceNonStopped: true}
	target := action.Target{
		DryRun:      false,
		SkipConfirm: true,
		IO:          io,
		Recorder:    recorder,
	}

	result := a.handleNonStoppedNotebooks(target, nbs, recorder)
	g.Expect(result).To(HaveLen(2))
}

func TestWorkbenchUpgradeAction_AddFlags(t *testing.T) {
	g := NewWithT(t)

	a := &WorkbenchUpgradeAction{}
	g.Expect(a.Phase()).To(Equal(action.PhasePreUpgrade))

	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	a.AddFlags(fs)

	flag := fs.Lookup("force-non-stopped")
	g.Expect(flag).ToNot(BeNil())
	g.Expect(flag.DefValue).To(Equal("false"))
}

// --- Test Helpers ---

func extractWorkloadContainersFromUnstructured(nb *unstructured.Unstructured) ([]map[string]any, error) {
	spec, ok := nb.Object["spec"].(map[string]any)
	if !ok {
		return nil, nil
	}

	template, ok := spec["template"].(map[string]any)
	if !ok {
		return nil, nil
	}

	podSpec, ok := template["spec"].(map[string]any)
	if !ok {
		return nil, nil
	}

	rawContainers, ok := podSpec["containers"].([]any)
	if !ok {
		return nil, nil
	}

	var result []map[string]any

	for _, raw := range rawContainers {
		containerMap, ok := raw.(map[string]any)
		if !ok {
			continue
		}

		result = append(result, containerMap)
	}

	return result, nil
}
