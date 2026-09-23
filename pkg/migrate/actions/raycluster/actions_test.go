package raycluster_test

import (
	"bytes"
	"os"
	"testing"

	"github.com/blang/semver/v4"
	"github.com/spf13/pflag"

	fakeapiextensions "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/fake"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	"github.com/opendatahub-io/odh-cli/pkg/migrate/action"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/action/result"
	rayactions "github.com/opendatahub-io/odh-cli/pkg/migrate/actions/raycluster"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
	"github.com/opendatahub-io/odh-cli/pkg/util/iostreams"

	. "github.com/onsi/gomega"
)

//nolint:gochecknoglobals // test-only GVR→ListKind mapping for fake dynamic client
var testListKinds = map[schema.GroupVersionResource]string{
	resources.Namespace.GVR():            resources.Namespace.ListKind(),
	resources.RayCluster.GVR():           resources.RayCluster.ListKind(),
	resources.DataScienceCluster.GVR():   resources.DataScienceCluster.ListKind(),
	resources.DataScienceClusterV1.GVR(): resources.DataScienceClusterV1.ListKind(),
}

func newTestTarget(t *testing.T, objects ...*unstructured.Unstructured) action.Target {
	t.Helper()

	scheme := runtime.NewScheme()
	_ = metav1.AddMetaToScheme(scheme)

	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, testListKinds)

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
			t.Fatalf("failed to create object %s/%s: %v", obj.GetKind(), obj.GetName(), err)
		}
	}

	apiExtClient := fakeapiextensions.NewSimpleClientset()

	testClient := client.NewForTesting(client.TestClientConfig{
		Dynamic:       dynamicClient,
		APIExtensions: apiExtClient,
	})

	currentVersion := semver.MustParse("2.25.0")
	targetVersion := semver.MustParse("3.0.0")

	return action.Target{
		Client:         testClient,
		CurrentVersion: &currentVersion,
		TargetVersion:  &targetVersion,
		DryRun:         false,
		SkipConfirm:    true,
		OutputDir:      t.TempDir(),
		Recorder:       action.NewRootRecorder(),
		IO:             iostreams.NewIOStreams(&bytes.Buffer{}, &bytes.Buffer{}, &bytes.Buffer{}),
	}
}

func gvrForObj(obj *unstructured.Unstructured) schema.GroupVersionResource {
	for gvr, listKind := range testListKinds {
		if listKind == obj.GetKind()+"List" {
			return gvr
		}
	}

	panic("unknown object kind: " + obj.GetKind())
}

func makeObj(rt resources.ResourceType, name string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": rt.APIVersion(),
			"kind":       rt.Kind,
			"metadata": map[string]any{
				"name": name,
			},
		},
	}
}

func makeDSCWithCodeflare(name, state string) *unstructured.Unstructured {
	obj := makeObj(resources.DataScienceCluster, name)
	_ = unstructured.SetNestedField(obj.Object, map[string]any{
		"codeflare": map[string]any{
			"managementState": state,
		},
	}, "spec", "components")

	return obj
}

// ─── NewActions factory ───

func TestNewActions(t *testing.T) {
	g := NewWithT(t)

	backup, migrate := rayactions.NewActions()

	g.Expect(backup).NotTo(BeNil())
	g.Expect(migrate).NotTo(BeNil())
}

// ─── BackupAction metadata ───

func TestBackupAction_Metadata(t *testing.T) {
	g := NewWithT(t)

	backup, _ := rayactions.NewActions()

	g.Expect(backup.ID()).To(Equal("raycluster.backup"))
	g.Expect(backup.Name()).To(Equal("Backup RayClusters for RHOAI 3.x migration"))
	g.Expect(backup.Description()).To(ContainSubstring("Backup RayCluster"))
	g.Expect(backup.Group()).To(Equal(action.GroupBackup))
	g.Expect(backup.Phase()).To(Equal(action.PhasePreUpgrade))
}

// ─── MigrateAction metadata ───

func TestMigrateAction_Metadata(t *testing.T) {
	g := NewWithT(t)

	_, migrate := rayactions.NewActions()

	g.Expect(migrate.ID()).To(Equal("raycluster.migrate"))
	g.Expect(migrate.Name()).To(Equal("Migrate RayClusters to RHOAI 3.x"))
	g.Expect(migrate.Description()).To(ContainSubstring("Migrate RayClusters"))
	g.Expect(migrate.Group()).To(Equal(action.GroupMigration))
	g.Expect(migrate.Phase()).To(Equal(action.PhasePostUpgrade))
}

// ─── CanApply ───

func TestBackupAction_CanApply(t *testing.T) {
	tests := []struct {
		name           string
		currentVersion *semver.Version
		expected       bool
	}{
		{
			name:           "nil current version",
			currentVersion: nil,
			expected:       false,
		},
		{
			name:           "current version 2.x",
			currentVersion: new(semver.MustParse("2.25.0")),
			expected:       true,
		},
		{
			name:           "current version 3.x",
			currentVersion: new(semver.MustParse("3.0.0")),
			expected:       false,
		},
		{
			name:           "current version 1.x",
			currentVersion: new(semver.MustParse("1.5.0")),
			expected:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			backup, _ := rayactions.NewActions()

			target := action.Target{CurrentVersion: tt.currentVersion}
			g.Expect(backup.CanApply(target)).To(Equal(tt.expected))
		})
	}
}

func TestMigrateAction_CanApply(t *testing.T) {
	tests := []struct {
		name          string
		targetVersion *semver.Version
		expected      bool
	}{
		{
			name:          "nil target version",
			targetVersion: nil,
			expected:      false,
		},
		{
			name:          "target version 3.x",
			targetVersion: new(semver.MustParse("3.0.0")),
			expected:      true,
		},
		{
			name:          "target version 4.x",
			targetVersion: new(semver.MustParse("4.0.0")),
			expected:      true,
		},
		{
			name:          "target version 2.x",
			targetVersion: new(semver.MustParse("2.25.0")),
			expected:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			_, migrate := rayactions.NewActions()

			target := action.Target{TargetVersion: tt.targetVersion}
			g.Expect(migrate.CanApply(target)).To(Equal(tt.expected))
		})
	}
}

// ─── AddFlags ───

func TestBackupAction_AddFlags(t *testing.T) {
	g := NewWithT(t)
	backup, _ := rayactions.NewActions()

	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	backup.AddFlags(fs)

	g.Expect(fs.Lookup("raycluster-cluster")).NotTo(BeNil())
	g.Expect(fs.Lookup("raycluster-namespace")).NotTo(BeNil())
	g.Expect(fs.Lookup("raycluster-output-dir")).NotTo(BeNil())
}

func TestMigrateAction_AddFlags(t *testing.T) {
	g := NewWithT(t)
	_, migrate := rayactions.NewActions()

	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	migrate.AddFlags(fs)

	g.Expect(fs.Lookup("raycluster-cluster")).NotTo(BeNil())
	g.Expect(fs.Lookup("raycluster-namespace")).NotTo(BeNil())
	g.Expect(fs.Lookup("raycluster-from-backup")).NotTo(BeNil())
	g.Expect(fs.Lookup("raycluster-timeout")).NotTo(BeNil())
}

// ─── Prepare / Run nil checks ───

func TestBackupAction_Prepare_NotNil(t *testing.T) {
	g := NewWithT(t)
	backup, _ := rayactions.NewActions()

	g.Expect(backup.Prepare()).NotTo(BeNil())
}

func TestBackupAction_Run_NotNil(t *testing.T) {
	g := NewWithT(t)
	backup, _ := rayactions.NewActions()

	g.Expect(backup.Run()).NotTo(BeNil())
}

func TestMigrateAction_Prepare_Nil(t *testing.T) {
	g := NewWithT(t)
	_, migrate := rayactions.NewActions()

	g.Expect(migrate.Prepare()).To(BeNil())
}

func TestMigrateAction_Run_NotNil(t *testing.T) {
	g := NewWithT(t)
	_, migrate := rayactions.NewActions()

	g.Expect(migrate.Run()).NotTo(BeNil())
}

// ─── BackupAction Prepare Task ───

func TestBackupPrepareTask_Validate(t *testing.T) {
	g := NewWithT(t)
	backup, _ := rayactions.NewActions()

	dsc := makeDSCWithCodeflare("default-dsc", "Removed")
	target := newTestTarget(t, dsc)

	task := backup.Prepare()
	res, err := task.Validate(t.Context(), target)

	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(res).NotTo(BeNil())
	g.Expect(res.Status.Steps).NotTo(BeEmpty())
	g.Expect(res.Status.Steps[0].Name).To(Equal("preflight-checks"))
}

func TestBackupPrepareTask_Execute(t *testing.T) {
	g := NewWithT(t)
	backup, _ := rayactions.NewActions()

	dsc := makeDSCWithCodeflare("default-dsc", "Removed")
	target := newTestTarget(t, dsc)

	task := backup.Prepare()
	res, err := task.Execute(t.Context(), target)

	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(res).NotTo(BeNil())
	g.Expect(res.Status.Steps).NotTo(BeEmpty())
	g.Expect(res.Status.Steps[0].Name).To(Equal("preflight-checks"))
}

func TestBackupPrepareTask_Execute_WithFailedChecks(t *testing.T) {
	g := NewWithT(t)
	backup, _ := rayactions.NewActions()

	dsc := makeDSCWithCodeflare("default-dsc", "Managed")
	target := newTestTarget(t, dsc)

	task := backup.Prepare()
	res, err := task.Execute(t.Context(), target)

	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(res).NotTo(BeNil())

	preflightStep := res.Status.Steps[0]
	g.Expect(preflightStep.Status).To(Equal(result.StepFailed))
	g.Expect(preflightStep.Message).To(ContainSubstring("failed"))
}

func TestBackupPrepareTask_Validate_WithFailedChecks(t *testing.T) {
	g := NewWithT(t)
	backup, _ := rayactions.NewActions()

	dsc := makeDSCWithCodeflare("default-dsc", "Managed")
	target := newTestTarget(t, dsc)

	task := backup.Prepare()
	res, err := task.Validate(t.Context(), target)

	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(res).NotTo(BeNil())

	preflightStep := res.Status.Steps[0]
	g.Expect(preflightStep.Status).To(Equal(result.StepFailed))
}

// ─── BackupAction Run Task ───

func TestBackupRunTask_Validate(t *testing.T) {
	g := NewWithT(t)
	backup, _ := rayactions.NewActions()

	dsc := makeDSCWithCodeflare("default-dsc", "Removed")
	target := newTestTarget(t, dsc)

	task := backup.Run()
	res, err := task.Validate(t.Context(), target)

	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(res).NotTo(BeNil())
	g.Expect(res.Status.Steps).NotTo(BeEmpty())
	g.Expect(res.Status.Steps[0].Name).To(Equal("preflight-checks"))
}

func TestBackupRunTask_Execute_NoClusters(t *testing.T) {
	g := NewWithT(t)
	backup, _ := rayactions.NewActions()

	certManagerNS := makeObj(resources.Namespace, "cert-manager")
	dsc := makeDSCWithCodeflare("default-dsc", "Removed")
	target := newTestTarget(t, certManagerNS, dsc)

	task := backup.Run()
	res, err := task.Execute(t.Context(), target)

	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(res).NotTo(BeNil())

	backupStep := findStep(res.Status.Steps, "backup-rayclusters")
	g.Expect(backupStep).NotTo(BeNil())
	g.Expect(backupStep.Status).To(Equal(result.StepSkipped))
	g.Expect(backupStep.Message).To(ContainSubstring("No RayClusters"))
}

func TestBackupRunTask_Execute_PreflightFails(t *testing.T) {
	g := NewWithT(t)
	backup, _ := rayactions.NewActions()

	dsc := makeDSCWithCodeflare("default-dsc", "Managed")
	target := newTestTarget(t, dsc)

	task := backup.Run()
	res, err := task.Execute(t.Context(), target)

	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(res).NotTo(BeNil())

	backupStep := findStep(res.Status.Steps, "backup-rayclusters")
	g.Expect(backupStep).NotTo(BeNil())
	g.Expect(backupStep.Status).To(Equal(result.StepFailed))
	g.Expect(backupStep.Message).To(ContainSubstring("failed"))
}

func TestBackupRunTask_Execute_UsesTargetOutputDir(t *testing.T) {
	g := NewWithT(t)
	backup, _ := rayactions.NewActions()

	certManagerNS := makeObj(resources.Namespace, "cert-manager")
	dsc := makeDSCWithCodeflare("default-dsc", "Removed")
	target := newTestTarget(t, certManagerNS, dsc)
	target.OutputDir = t.TempDir()

	task := backup.Run()
	res, err := task.Execute(t.Context(), target)

	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(res).NotTo(BeNil())
}

func TestBackupRunTask_Execute_UsesOptsOutputDir(t *testing.T) {
	g := NewWithT(t)
	backup, _ := rayactions.NewActions()

	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	backup.AddFlags(fs)

	certManagerNS := makeObj(resources.Namespace, "cert-manager")
	dsc := makeDSCWithCodeflare("default-dsc", "Removed")
	target := newTestTarget(t, certManagerNS, dsc)
	target.OutputDir = ""

	task := backup.Run()
	res, err := task.Execute(t.Context(), target)

	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(res).NotTo(BeNil())
}

func TestBackupRunTask_Execute_DryRun(t *testing.T) {
	g := NewWithT(t)
	backup, _ := rayactions.NewActions()

	testNS := makeObj(resources.Namespace, "test-ns")
	certManagerNS := makeObj(resources.Namespace, "cert-manager")
	dsc := makeDSCWithCodeflare("default-dsc", "Removed")

	rc := makeObj(resources.RayCluster, "my-cluster")
	rc.SetNamespace("test-ns")

	target := newTestTarget(t, testNS, certManagerNS, dsc, rc)
	target.DryRun = true

	task := backup.Run()
	res, err := task.Execute(t.Context(), target)

	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(res).NotTo(BeNil())

	backupStep := findStep(res.Status.Steps, "backup-rayclusters")
	g.Expect(backupStep).NotTo(BeNil())
	g.Expect(backupStep.Status).To(Equal(result.StepSkipped))
	g.Expect(backupStep.Message).To(ContainSubstring("Dry-run"))
	g.Expect(backupStep.Message).To(ContainSubstring("1 RayCluster(s)"))

	entries, err := os.ReadDir(target.OutputDir)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(entries).To(BeEmpty(), "dry-run should not create any files")
}

func TestBackupRunTask_Execute_DryRun_NoClusters(t *testing.T) {
	g := NewWithT(t)
	backup, _ := rayactions.NewActions()

	certManagerNS := makeObj(resources.Namespace, "cert-manager")
	dsc := makeDSCWithCodeflare("default-dsc", "Removed")
	target := newTestTarget(t, certManagerNS, dsc)
	target.DryRun = true

	task := backup.Run()
	res, err := task.Execute(t.Context(), target)

	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(res).NotTo(BeNil())

	backupStep := findStep(res.Status.Steps, "backup-rayclusters")
	g.Expect(backupStep).NotTo(BeNil())
	g.Expect(backupStep.Status).To(Equal(result.StepSkipped))
	g.Expect(backupStep.Message).To(ContainSubstring("Dry-run"))
	g.Expect(backupStep.Message).To(ContainSubstring("no RayClusters"))
}

// ─── MigrateAction Run Task ───

func TestMigrateRunTask_Validate(t *testing.T) {
	g := NewWithT(t)
	_, migrate := rayactions.NewActions()

	target := newTestTarget(t)

	task := migrate.Run()
	res, err := task.Validate(t.Context(), target)

	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(res).To(BeNil())
}

func TestMigrateRunTask_Execute_NoClusters(t *testing.T) {
	g := NewWithT(t)
	_, migrate := rayactions.NewActions()

	target := newTestTarget(t)

	task := migrate.Run()
	res, err := task.Execute(t.Context(), target)

	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(res).NotTo(BeNil())

	migStep := findStep(res.Status.Steps, "post-upgrade-migration")
	g.Expect(migStep).NotTo(BeNil())
	g.Expect(migStep.Status).To(Equal(result.StepCompleted))
	g.Expect(migStep.Message).To(ContainSubstring("Migrated: 0"))
}

func TestMigrateRunTask_Execute_DryRun(t *testing.T) {
	g := NewWithT(t)
	_, migrate := rayactions.NewActions()

	target := newTestTarget(t)
	target.DryRun = true

	task := migrate.Run()
	res, err := task.Execute(t.Context(), target)

	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(res).NotTo(BeNil())

	migStep := findStep(res.Status.Steps, "post-upgrade-migration")
	g.Expect(migStep).NotTo(BeNil())
	g.Expect(migStep.Status).To(Equal(result.StepSkipped))
	g.Expect(migStep.Message).To(ContainSubstring("Dry-run"))
}

func TestMigrateRunTask_Execute_FromBackupError(t *testing.T) {
	g := NewWithT(t)
	_, migrate := rayactions.NewActions()

	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	migrate.AddFlags(fs)
	_ = fs.Set("raycluster-from-backup", "/nonexistent/backup/path")

	target := newTestTarget(t)

	task := migrate.Run()
	res, err := task.Execute(t.Context(), target)

	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(res).NotTo(BeNil())

	migStep := findStep(res.Status.Steps, "post-upgrade-migration")
	g.Expect(migStep).NotTo(BeNil())
	g.Expect(migStep.Status).To(Equal(result.StepFailed))
	g.Expect(migStep.Message).To(ContainSubstring("failed"))
}

func TestMigrateRunTask_Execute_SkipConfirm(t *testing.T) {
	g := NewWithT(t)
	_, migrate := rayactions.NewActions()

	target := newTestTarget(t)
	target.SkipConfirm = true

	task := migrate.Run()
	res, err := task.Execute(t.Context(), target)

	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(res).NotTo(BeNil())
	g.Expect(res.Status.Completed).To(BeTrue())
}

// ─── Shared options flow between actions ───

func TestSharedOptions_FlagFlow(t *testing.T) {
	g := NewWithT(t)

	backup, migrate := rayactions.NewActions()

	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	backup.AddFlags(fs)
	migrate.AddFlags(fs)

	err := fs.Parse([]string{
		"--raycluster-cluster", "my-cluster",
		"--raycluster-namespace", "my-ns",
		"--raycluster-output-dir", "/tmp/backups",
		"--raycluster-from-backup", "/tmp/restore",
		"--raycluster-timeout", "5m",
	})
	g.Expect(err).NotTo(HaveOccurred())

	clusterVal, _ := fs.GetString("raycluster-cluster")
	g.Expect(clusterVal).To(Equal("my-cluster"))

	namespaceVal, _ := fs.GetString("raycluster-namespace")
	g.Expect(namespaceVal).To(Equal("my-ns"))

	outputVal, _ := fs.GetString("raycluster-output-dir")
	g.Expect(outputVal).To(Equal("/tmp/backups"))

	backupVal, _ := fs.GetString("raycluster-from-backup")
	g.Expect(backupVal).To(Equal("/tmp/restore"))
}

// ─── ActionConfigurer interface ───

func TestBackupAction_ImplementsActionConfigurer(t *testing.T) {
	g := NewWithT(t)
	backup, _ := rayactions.NewActions()

	var configurer action.ActionConfigurer
	g.Expect(backup).To(BeAssignableToTypeOf(&rayactions.BackupAction{}))
	configurer, ok := any(backup).(action.ActionConfigurer)
	g.Expect(ok).To(BeTrue())
	g.Expect(configurer).NotTo(BeNil())
}

func TestMigrateAction_ImplementsActionConfigurer(t *testing.T) {
	g := NewWithT(t)
	_, migrate := rayactions.NewActions()

	g.Expect(migrate).To(BeAssignableToTypeOf(&rayactions.MigrateAction{}))
	configurer, ok := any(migrate).(action.ActionConfigurer)
	g.Expect(ok).To(BeTrue())
	g.Expect(configurer).NotTo(BeNil())
}

// ─── helpers ───

func findStep(steps []result.ActionStep, name string) *result.ActionStep {
	for i := range steps {
		if steps[i].Name == name {
			return &steps[i]
		}

		if found := findStep(steps[i].Children, name); found != nil {
			return found
		}
	}

	return nil
}
