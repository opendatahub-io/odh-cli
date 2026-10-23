package backup_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/blang/semver/v4"
	"github.com/onsi/gomega/gstruct"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	clienttesting "k8s.io/client-go/testing"

	"github.com/opendatahub-io/odh-cli/pkg/migrate/action"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/action/result"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/actions/llamastack/backup"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
	"github.com/opendatahub-io/odh-cli/pkg/util/iostreams"

	. "github.com/onsi/gomega"
)

//nolint:gochecknoglobals
var llsd = &unstructured.Unstructured{
	Object: map[string]any{
		"apiVersion": "llamastack.io/v1alpha1",
		"kind":       "LlamaStackDistribution",
		"metadata": map[string]any{
			"name":      "test-llsd",
			"namespace": "test-ns",
		},
		"spec": map[string]any{
			"server": map[string]any{
				"userConfig": map[string]any{
					"configMapName": "test-configmap",
				},
			},
		},
	},
}

//nolint:gochecknoglobals
var cm = &unstructured.Unstructured{
	Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
		"metadata": map[string]any{
			"name":      "test-configmap",
			"namespace": "test-ns",
		},
		"data": map[string]any{
			"run.yaml":    "run_content",
			"config.yaml": "config_content",
		},
	},
}

//nolint:gochecknoglobals
var pod = &unstructured.Unstructured{
	Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "Pod",
		"metadata": map[string]any{
			"name":      "test-pod",
			"namespace": "test-ns",
			"labels": map[string]any{
				"app.kubernetes.io/instance": "test-llsd",
			},
			"ownerReferences": []any{
				map[string]any{
					"kind": "ReplicaSet",
					"name": "test-deploy-hash123",
				},
			},
		},
		"status": map[string]any{
			"phase": "Running",
			"conditions": []any{
				map[string]any{
					"type":   "Ready",
					"status": "True",
				},
			},
		},
	},
}

//nolint:gochecknoglobals
var podTerminating = &unstructured.Unstructured{
	Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "Pod",
		"metadata": map[string]any{
			"name":      "test-pod-terminating",
			"namespace": "test-ns",
			"labels": map[string]any{
				"app.kubernetes.io/instance": "test-llsd",
			},
			"ownerReferences": []any{
				map[string]any{
					"kind": "ReplicaSet",
					"name": "test-deploy-oldhash",
				},
			},
		},
		"status": map[string]any{
			"phase": "Running",
			"conditions": []any{
				map[string]any{
					"type":   "Ready",
					"status": "False",
				},
			},
		},
	},
}

//nolint:gochecknoglobals
var deploy = &unstructured.Unstructured{
	Object: map[string]any{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"metadata": map[string]any{
			"name":      "test-deploy",
			"namespace": "test-ns",
		},
	},
}

const (
	stepNameBackup     = "llamastack-backup"
	stepNameBackupLLSD = "backup-test-ns-test-llsd"
)

// hasFailedStep checks whether a given child step under the backup parent step has failed.
func hasFailedStep(res *result.ActionResult, parentName, childName string) bool {
	for _, step := range res.Status.Steps {
		if step.Name != parentName {
			continue
		}

		for _, child := range step.Children {
			if child.Name == childName && child.Status == result.StepFailed {
				return true
			}
		}
	}

	return false
}

func TestLlamaStackBackupAction_Validate(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	_, in, out, errOut := genericiooptions.NewTestIOStreams()
	ioStreams := iostreams.NewIOStreams(in, out, errOut)
	recorder := action.NewVerboseRootRecorder(ioStreams)

	target := action.Target{
		Recorder: recorder,
	}

	a := &backup.LlamaStackBackupAction{}
	prepareTask := a.Prepare()

	res, err := prepareTask.Validate(ctx, target)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(res).NotTo(BeNil())
}

func TestLlamaStackBackupAction_Execute(t *testing.T) {
	scheme := runtime.NewScheme()

	gvrToListKind := map[schema.GroupVersionResource]string{
		{Group: "llamastack.io", Version: "v1alpha1", Resource: "llamastackdistributions"}: "LlamaStackDistributionList",
		{Group: "", Version: "v1", Resource: "pods"}:                                       "PodList",
	}

	t.Run("successfully backs up resources", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, gvrToListKind, llsd, cm, pod, deploy)
		testClient := client.NewForTesting(client.TestClientConfig{
			Dynamic: dynamicClient,
		})

		_, in, out, errOut := genericiooptions.NewTestIOStreams()
		ioStreams := iostreams.NewIOStreams(in, out, errOut)
		recorder := action.NewVerboseRootRecorder(ioStreams)

		outDir := t.TempDir()

		target := action.Target{
			Client:    testClient,
			DryRun:    true, // use dry-run to avoid trying to exec oc/kubectl in tests
			OutputDir: outDir,
			Recorder:  recorder,
			IO:        ioStreams,
		}

		a := &backup.LlamaStackBackupAction{}
		prepareTask := a.Prepare()
		g.Expect(prepareTask).NotTo(BeNil())

		res, err := prepareTask.Execute(ctx, target)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(res).To(gstruct.PointTo(gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
			"Status": gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
				"Completed": BeTrue(),
			}),
		})))
	})

	t.Run("selects Ready pod over non-Ready pod", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		// Use per-test pod fixtures so wrong selection becomes observable.
		// The terminating pod's ownerReference points to a ReplicaSet whose
		// inferred deployment ("wrong-deploy") does not exist. If selectReadyPod
		// picks this pod, backupDeploymentForPod will look up "wrong-deploy"
		// instead of "test-deploy".
		terminatingPod := podTerminating.DeepCopy()
		terminatingPod.Object["metadata"].(map[string]any)["ownerReferences"] = []any{
			map[string]any{"kind": "ReplicaSet", "name": "wrong-deploy-oldhash"},
		}
		readyPod := pod.DeepCopy()

		dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, gvrToListKind, llsd, cm, terminatingPod, readyPod, deploy)

		// Track which deployment names are looked up to verify pod selection.
		var requestedDeployments []string
		dynamicClient.PrependReactor("get", "deployments", func(a clienttesting.Action) (bool, runtime.Object, error) {
			getAction, ok := a.(clienttesting.GetAction)
			if ok {
				requestedDeployments = append(requestedDeployments, getAction.GetName())
			}

			return false, nil, nil // fall through to default behavior
		})

		testClient := client.NewForTesting(client.TestClientConfig{
			Dynamic: dynamicClient,
		})

		_, in, out, errOut := genericiooptions.NewTestIOStreams()
		ioStreams := iostreams.NewIOStreams(in, out, errOut)
		recorder := action.NewVerboseRootRecorder(ioStreams)

		outDir := t.TempDir()

		target := action.Target{
			Client:    testClient,
			DryRun:    true,
			OutputDir: outDir,
			Recorder:  recorder,
			IO:        ioStreams,
		}

		a := &backup.LlamaStackBackupAction{}
		prepareTask := a.Prepare()
		g.Expect(prepareTask).NotTo(BeNil())

		res, err := prepareTask.Execute(ctx, target)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(res).To(gstruct.PointTo(gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
			"Status": gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
				"Completed": BeTrue(),
			}),
		})))
		g.Expect(hasFailedStep(res, stepNameBackup, stepNameBackupLLSD)).To(BeFalse(),
			"ready pod should be selected; selecting terminating pod would look up wrong-deploy")
		g.Expect(requestedDeployments).To(ContainElement("test-deploy"),
			"should look up deployment for the ready pod")
		g.Expect(requestedDeployments).NotTo(ContainElement("wrong-deploy"),
			"should not look up deployment for the terminating pod")
	})

	t.Run("successfully backs up resources to disk (non-dry-run)", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		// Do not include pod and deploy so that we do not test exec tar logic
		dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, gvrToListKind, llsd, cm)
		testClient := client.NewForTesting(client.TestClientConfig{
			Dynamic: dynamicClient,
		})

		_, in, out, errOut := genericiooptions.NewTestIOStreams()
		ioStreams := iostreams.NewIOStreams(in, out, errOut)
		recorder := action.NewVerboseRootRecorder(ioStreams)

		outDir := t.TempDir()

		target := action.Target{
			Client:    testClient,
			DryRun:    false,
			OutputDir: outDir,
			Recorder:  recorder,
			IO:        ioStreams,
		}

		a := &backup.LlamaStackBackupAction{}
		prepareTask := a.Prepare()
		g.Expect(prepareTask).NotTo(BeNil())

		res, err := prepareTask.Execute(ctx, target)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(res).To(gstruct.PointTo(gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
			"Status": gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
				"Completed": BeTrue(),
			}),
		})))

		// Verify files on disk
		nsDir := filepath.Join(outDir, "test-ns", "test-llsd")

		// 1. LLSD YAML
		llsdPath := filepath.Join(nsDir, "llamastackdistributions.llamastack.io-test-llsd.yaml")
		g.Expect(llsdPath).To(BeAnExistingFile())

		// 2. ConfigMap YAML
		cmPath := filepath.Join(nsDir, "configmaps-test-configmap.yaml")
		g.Expect(cmPath).To(BeAnExistingFile())

		// 3. Extracted YAMLs
		runPath := filepath.Join(nsDir, "run.yaml")
		g.Expect(runPath).To(BeAnExistingFile())

		configPath := filepath.Join(nsDir, "config.yaml")
		g.Expect(configPath).To(BeAnExistingFile())
	})

	t.Run("no LLSD CRD present", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, gvrToListKind)
		dynamicClient.PrependReactor("list", "llamastackdistributions", func(clienttesting.Action) (bool, runtime.Object, error) {
			return true, nil, &meta.NoResourceMatchError{PartialResource: schema.GroupVersionResource{Group: "llamastack.io", Resource: "llamastackdistributions"}}
		})

		testClient := client.NewForTesting(client.TestClientConfig{
			Dynamic: dynamicClient,
		})

		_, in, out, errOut := genericiooptions.NewTestIOStreams()
		ioStreams := iostreams.NewIOStreams(in, out, errOut)
		recorder := action.NewVerboseRootRecorder(ioStreams)

		target := action.Target{
			Client:    testClient,
			DryRun:    true,
			OutputDir: t.TempDir(),
			Recorder:  recorder,
			IO:        ioStreams,
		}

		a := &backup.LlamaStackBackupAction{}
		prepareTask := a.Prepare()
		res, err := prepareTask.Execute(ctx, target)

		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(res).To(gstruct.PointTo(gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
			"Status": gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
				"Completed": BeTrue(),
			}),
		})))
	})

	t.Run("no LLSD resources found", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, gvrToListKind)
		testClient := client.NewForTesting(client.TestClientConfig{
			Dynamic: dynamicClient,
		})

		_, in, out, errOut := genericiooptions.NewTestIOStreams()
		ioStreams := iostreams.NewIOStreams(in, out, errOut)
		recorder := action.NewVerboseRootRecorder(ioStreams)

		target := action.Target{
			Client:    testClient,
			DryRun:    true,
			OutputDir: t.TempDir(),
			Recorder:  recorder,
			IO:        ioStreams,
		}

		a := &backup.LlamaStackBackupAction{}
		prepareTask := a.Prepare()
		res, err := prepareTask.Execute(ctx, target)

		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(res).To(gstruct.PointTo(gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
			"Status": gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
				"Completed": BeTrue(),
			}),
		})))
	})

	t.Run("API error listing LLSDs", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, gvrToListKind)
		dynamicClient.PrependReactor("list", "llamastackdistributions", func(clienttesting.Action) (bool, runtime.Object, error) {
			return true, nil, errors.New("fake api error")
		})

		testClient := client.NewForTesting(client.TestClientConfig{
			Dynamic: dynamicClient,
		})

		_, in, out, errOut := genericiooptions.NewTestIOStreams()
		ioStreams := iostreams.NewIOStreams(in, out, errOut)
		recorder := action.NewVerboseRootRecorder(ioStreams)

		target := action.Target{
			Client:    testClient,
			DryRun:    true,
			OutputDir: t.TempDir(),
			Recorder:  recorder,
			IO:        ioStreams,
		}

		a := &backup.LlamaStackBackupAction{}
		prepareTask := a.Prepare()
		res, err := prepareTask.Execute(ctx, target)

		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(res).To(gstruct.PointTo(gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
			"Status": gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
				"Completed": BeFalse(),
			}),
		})))
	})

	t.Run("API error getting ConfigMap fails the backup step", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, gvrToListKind, llsd, cm, pod, deploy)
		dynamicClient.PrependReactor("get", "configmaps", func(clienttesting.Action) (bool, runtime.Object, error) {
			return true, nil, errors.New("fake api error")
		})

		testClient := client.NewForTesting(client.TestClientConfig{
			Dynamic: dynamicClient,
		})

		_, in, out, errOut := genericiooptions.NewTestIOStreams()
		ioStreams := iostreams.NewIOStreams(in, out, errOut)
		recorder := action.NewVerboseRootRecorder(ioStreams)

		target := action.Target{
			Client:    testClient,
			DryRun:    true,
			OutputDir: t.TempDir(),
			Recorder:  recorder,
			IO:        ioStreams,
		}

		a := &backup.LlamaStackBackupAction{}
		prepareTask := a.Prepare()
		res, err := prepareTask.Execute(ctx, target)

		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(res.Status.Completed).To(BeFalse())
		g.Expect(hasFailedStep(res, stepNameBackup, stepNameBackupLLSD)).To(BeTrue(),
			"Expected to find a failed step for "+stepNameBackupLLSD)
	})

	t.Run("API error getting Deployment fails the backup step", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, gvrToListKind, llsd, cm, pod, deploy)
		dynamicClient.PrependReactor("get", "deployments", func(clienttesting.Action) (bool, runtime.Object, error) {
			return true, nil, errors.New("fake api error")
		})

		testClient := client.NewForTesting(client.TestClientConfig{
			Dynamic: dynamicClient,
		})

		_, in, out, errOut := genericiooptions.NewTestIOStreams()
		ioStreams := iostreams.NewIOStreams(in, out, errOut)
		recorder := action.NewVerboseRootRecorder(ioStreams)

		target := action.Target{
			Client:    testClient,
			DryRun:    true,
			OutputDir: t.TempDir(),
			Recorder:  recorder,
			IO:        ioStreams,
		}

		a := &backup.LlamaStackBackupAction{}
		prepareTask := a.Prepare()
		res, err := prepareTask.Execute(ctx, target)

		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(res.Status.Completed).To(BeFalse())
		g.Expect(hasFailedStep(res, stepNameBackup, stepNameBackupLLSD)).To(BeTrue(),
			"Expected to find a failed step for "+stepNameBackupLLSD)
	})

	t.Run("API error listing pods fails the backup step", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, gvrToListKind, llsd, cm, pod, deploy)
		dynamicClient.PrependReactor("list", "pods", func(clienttesting.Action) (bool, runtime.Object, error) {
			return true, nil, errors.New("fake api error")
		})

		testClient := client.NewForTesting(client.TestClientConfig{
			Dynamic: dynamicClient,
		})

		_, in, out, errOut := genericiooptions.NewTestIOStreams()
		ioStreams := iostreams.NewIOStreams(in, out, errOut)
		recorder := action.NewVerboseRootRecorder(ioStreams)

		target := action.Target{
			Client:    testClient,
			DryRun:    true,
			OutputDir: t.TempDir(),
			Recorder:  recorder,
			IO:        ioStreams,
		}

		a := &backup.LlamaStackBackupAction{}
		prepareTask := a.Prepare()
		res, err := prepareTask.Execute(ctx, target)

		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(res.Status.Completed).To(BeFalse())
		g.Expect(hasFailedStep(res, stepNameBackup, stepNameBackupLLSD)).To(BeTrue(),
			"Expected to find a failed step for "+stepNameBackupLLSD)
	})

	t.Run("exec tar fails when oc and kubectl not in PATH", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, gvrToListKind, llsd, cm, pod, deploy)
		testClient := client.NewForTesting(client.TestClientConfig{
			Dynamic: dynamicClient,
		})

		_, in, out, errOut := genericiooptions.NewTestIOStreams()
		ioStreams := iostreams.NewIOStreams(in, out, errOut)
		recorder := action.NewVerboseRootRecorder(ioStreams)

		outDir := t.TempDir()

		target := action.Target{
			Client:    testClient,
			DryRun:    false,
			OutputDir: outDir,
			Recorder:  recorder,
			IO:        ioStreams,
		}

		// Set PATH to an empty directory so exec.LookPath cannot find oc or kubectl
		emptyDir := t.TempDir()
		t.Setenv("PATH", emptyDir)

		a := &backup.LlamaStackBackupAction{}
		prepareTask := a.Prepare()
		res, err := prepareTask.Execute(ctx, target)

		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(res.Status.Completed).To(BeFalse())
		g.Expect(hasFailedStep(res, stepNameBackup, stepNameBackupLLSD)).To(BeTrue(),
			"Expected backup to fail when oc/kubectl are not in PATH")
	})
}

func TestEnforcePermissions_SkipsSymlinks(t *testing.T) {
	g := NewWithT(t)

	// Create a target file outside the walked directory to detect symlink following.
	targetFile := filepath.Join(t.TempDir(), "outside-target.txt")
	g.Expect(os.WriteFile(targetFile, []byte("secret"), 0o600)).To(Succeed())

	// Widen permissions so we can detect if enforcePermissions changes them.
	g.Expect(os.Chmod(targetFile, 0o755)).To(Succeed()) //nolint:gosec // intentionally wide perms for test detection

	originalInfo, err := os.Stat(targetFile)
	g.Expect(err).NotTo(HaveOccurred())
	originalMode := originalInfo.Mode().Perm()

	// Create the directory tree to walk, containing a symlink to the target.
	walkDir := filepath.Join(t.TempDir(), "pod-data")
	g.Expect(os.MkdirAll(walkDir, 0o700)).To(Succeed())
	g.Expect(os.WriteFile(filepath.Join(walkDir, "regular.txt"), []byte("data"), 0o600)).To(Succeed())
	g.Expect(os.Symlink(targetFile, filepath.Join(walkDir, "evil-link"))).To(Succeed())

	g.Expect(backup.EnforcePermissions(walkDir)).To(Succeed())

	// The symlink target's permissions must be unchanged.
	afterInfo, err := os.Stat(targetFile)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(afterInfo.Mode().Perm()).To(Equal(originalMode),
		"enforcePermissions must not chmod through symlinks")
}

func TestLlamaStackBackupAction_CanApply(t *testing.T) {
	v30 := semver.MustParse("3.0.0")
	v34 := semver.MustParse("3.4.0")
	v35 := semver.MustParse("3.5.0")
	v36 := semver.MustParse("3.6.0")
	v40 := semver.MustParse("4.0.0")
	v225 := semver.MustParse("2.25.0")

	tests := []struct {
		name          string
		targetVersion *semver.Version
		expected      bool
	}{
		{"applies for target 3.0", &v30, true},
		{"applies for target 3.4", &v34, true},
		{"applies for target 3.5", &v35, true},
		{"does not apply for target 3.6", &v36, false},
		{"does not apply for target 4.0", &v40, false},
		{"does not apply for target 2.25", &v225, false},
		{"does not apply for nil target version", nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			a := &backup.LlamaStackBackupAction{}
			target := action.Target{
				TargetVersion: tt.targetVersion,
			}

			g.Expect(a.CanApply(target)).To(Equal(tt.expected))
		})
	}
}
