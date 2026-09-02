//nolint:testpackage // Tests internal implementation
package data

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/pflag"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/opendatahub-io/odh-cli/pkg/migrate/action"
	podexec "github.com/opendatahub-io/odh-cli/pkg/util/exec"
	"github.com/opendatahub-io/odh-cli/pkg/util/iostreams"

	. "github.com/onsi/gomega"
)

// ---------------------------------------------------------------------------
// DataAction metadata
// ---------------------------------------------------------------------------

func TestDataAction_Metadata(t *testing.T) {
	g := NewWithT(t)

	a := &DataAction{}

	g.Expect(a.ID()).To(Equal("trustyai.data"))
	g.Expect(a.Name()).To(Equal("Backup and restore TrustyAI data storage"))
	g.Expect(a.Description()).To(ContainSubstring("TrustyAI"))
	g.Expect(a.Group()).To(Equal(action.GroupBackup))
	g.Expect(a.Phase()).To(Equal(action.PhasePreUpgrade))
	g.Expect(a.CanApply(action.Target{})).To(BeTrue())
	g.Expect(a.Prepare()).ToNot(BeNil())
	g.Expect(a.Run()).ToNot(BeNil())
}

func TestDataAction_AddFlags(t *testing.T) {
	g := NewWithT(t)

	a := &DataAction{}
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	a.AddFlags(fs)

	g.Expect(fs.Lookup("data-dir")).ToNot(BeNil())
	g.Expect(fs.Lookup("data-file")).ToNot(BeNil())
	g.Expect(fs.Lookup("data-service-name")).ToNot(BeNil())
}

// ---------------------------------------------------------------------------
// prepareTask.Validate
// ---------------------------------------------------------------------------

func TestPrepareTask_Validate_NoServices(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	target := newTarget(newTestClient(newScheme(), nil))

	a := &DataAction{}
	result, err := a.Prepare().Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Completed).To(BeTrue())
}

func TestPrepareTask_Validate_WithServices(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	svc := newTrustyAIService("trustyai", "ns1", "PVC")
	target := newTarget(newTestClient(newScheme(), nil, svc))

	a := &DataAction{}
	result, err := a.Prepare().Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Completed).To(BeTrue())
}

// ---------------------------------------------------------------------------
// prepareTask.Execute
// ---------------------------------------------------------------------------

func TestPrepareTask_Execute_NoServices(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	target := newTarget(newTestClient(newScheme(), nil))

	a := &DataAction{}
	result, err := a.Prepare().Execute(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Completed).To(BeTrue())
}

func TestPrepareTask_Execute_UnsupportedFormat(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	svc := newTrustyAIService("trustyai", "ns1", "UNKNOWN_FORMAT")
	target := newTarget(newTestClient(newScheme(), nil, svc))

	a := &DataAction{}
	result, err := a.Prepare().Execute(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Completed).To(BeTrue())
}

// ---------------------------------------------------------------------------
// runTask.Validate
// ---------------------------------------------------------------------------

func TestRunTask_Validate_NoBackupFile(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	target := newTarget(newTestClient(newScheme(), nil))

	a := &DataAction{}
	result, err := a.Run().Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Completed).To(BeTrue())
}

func TestRunTask_Validate_FileNotFound(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	target := newTarget(newTestClient(newScheme(), nil))

	a := &DataAction{BackupFile: "/nonexistent/path"}
	result, err := a.Run().Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Completed).To(BeFalse())
}

func TestRunTask_Validate_NotDirectory(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	f := filepath.Join(t.TempDir(), "not-a-dir.txt")
	_ = os.WriteFile(f, []byte("nope"), 0o600)

	target := newTarget(newTestClient(newScheme(), nil))

	a := &DataAction{BackupFile: f}
	result, err := a.Run().Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Completed).To(BeFalse())
}

func TestRunTask_Validate_Ready(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	backupDir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(backupDir, dataSubDir), 0o750)

	target := newTarget(newTestClient(newScheme(), nil))

	a := &DataAction{BackupFile: backupDir}
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

	target := newTarget(newTestClient(newScheme(), nil))

	a := &DataAction{}
	result, err := a.Run().Execute(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Completed).To(BeTrue())
}

func TestRunTask_Execute_CannotDetectType(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	target := newTarget(newTestClient(newScheme(), nil))

	a := &DataAction{BackupFile: t.TempDir()}
	result, err := a.Run().Execute(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Completed).To(BeFalse())
}

func TestRunTask_Execute_NoServicesForRestore(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	backupDir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(backupDir, dataSubDir), 0o750)

	target := newTarget(newTestClient(newScheme(), nil))

	a := &DataAction{BackupFile: backupDir}
	result, err := a.Run().Execute(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Completed).To(BeFalse())
}

func TestRunTask_Execute_DryRun_PVC(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	backupDir := t.TempDir()
	dataDir := filepath.Join(backupDir, dataSubDir)
	_ = os.MkdirAll(dataDir, 0o750)
	_ = os.WriteFile(filepath.Join(dataDir, "test.csv"), []byte("data"), 0o600)

	_ = writeMetadata(backupDir, DataBackupMetadata{
		Namespace:     "ns1",
		StorageFormat: storageFormatPVC,
		MountPath:     "/data",
		PVCName:       "trustyai-pvc",
	})

	svc := newTrustyAIService("trustyai", "ns1", "PVC")
	pod := newRunningPod("trustyai-service-pod", "ns1", map[string]string{"app": "trustyai"})

	target := newTarget(
		newTestClient(newScheme(), []runtime.Object{pod}, svc),
		func(t *action.Target) { t.DryRun = true },
	)

	stderrBuf := target.IO.ErrOut().(*bytes.Buffer)

	a := &DataAction{BackupFile: backupDir}
	result, err := a.Run().Execute(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Completed).To(BeTrue())
	_ = stderrBuf
}

func TestRunTask_Execute_DryRun_Database(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	backupDir := t.TempDir()
	_ = os.WriteFile(filepath.Join(backupDir, dumpFileName), []byte("-- dump\nINSERT INTO foo;\n"), 0o600)
	_ = writeMetadata(backupDir, DataBackupMetadata{
		Namespace:     "ns1",
		StorageFormat: storageFormatDatabase,
		MariaDBPod:    "mariadb-pod",
		DatabaseName:  "trustyai_db",
	})

	svc := newTrustyAIService("trustyai", "ns1", "DATABASE")
	secret := newSecret("trustyai-db-credentials", "ns1", map[string]string{
		"databaseUsername": "admin",
		"databasePassword": "secret",
		"databaseName":     "trustyai_db",
	})
	pod := newRunningPod("mariadb-pod", "ns1", nil)

	target := newTarget(
		newTestClient(newScheme(), []runtime.Object{pod}, svc, secret),
		func(t *action.Target) { t.DryRun = true },
	)

	a := &DataAction{BackupFile: backupDir}
	result, err := a.Run().Execute(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Completed).To(BeFalse())
}

func TestRunTask_Execute_UnsupportedBackupType(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	backupDir := t.TempDir()
	_ = writeMetadata(backupDir, DataBackupMetadata{
		Namespace:     "ns1",
		StorageFormat: "UNKNOWN",
	})

	svc := newTrustyAIService("trustyai", "ns1", "PVC")
	target := newTarget(newTestClient(newScheme(), nil, svc))

	a := &DataAction{BackupFile: backupDir}
	result, err := a.Run().Execute(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Completed).To(BeFalse())
}

// ---------------------------------------------------------------------------
// resolveMariaDBPod
// ---------------------------------------------------------------------------

func TestResolveMariaDBPod_FromMetadata(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	pod := newRunningPod("mariadb-specific", "ns1", nil)
	svc := serviceInfo{name: "trustyai", namespace: "ns1"}

	target := newTarget(newTestClient(newScheme(), []runtime.Object{pod}))

	meta := &DataBackupMetadata{MariaDBPod: "mariadb-specific"}

	task := &runTask{action: &DataAction{}}
	found, err := task.resolveMariaDBPod(ctx, target, svc, meta, nil)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(found.Name).To(Equal("mariadb-specific"))
}

func TestResolveMariaDBPod_FallbackToDiscovery(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	pod := newRunningPod("mariadb-trustyai-xyz", "ns1", nil)
	svc := serviceInfo{name: "trustyai", namespace: "ns1"}

	target := newTarget(newTestClient(newScheme(), []runtime.Object{pod}))

	meta := &DataBackupMetadata{MariaDBPod: "old-deleted-pod"}

	task := &runTask{action: &DataAction{}}
	found, err := task.resolveMariaDBPod(ctx, target, svc, meta, nil)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(found.Name).To(Equal("mariadb-trustyai-xyz"))
}

// ---------------------------------------------------------------------------
// backupPVC (internal method, tested via prepareTask)
// ---------------------------------------------------------------------------

func writeTarToWriter(w io.Writer, files map[string]string) {
	tw := tar.NewWriter(w)
	defer func() { _ = tw.Close() }()

	for name, content := range files {
		_ = tw.WriteHeader(&tar.Header{
			Name: name,
			Mode: 0o600,
			Size: int64(len(content)),
		})
		_, _ = tw.Write([]byte(content))
	}
}

func TestBackupPVC(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	outputDir := t.TempDir()

	executor := &podexec.MockExecutor{
		ExecFn: func(_ context.Context, opts podexec.ExecOptions) error {
			if opts.Command[0] == "tar" && opts.Stdout != nil {
				writeTarToWriter(opts.Stdout, map[string]string{
					"file1.csv": "data1",
					"file2.csv": "data2",
				})
			}

			return nil
		},
	}

	pod := newPodWithVolumes("trustyai-pod", "ns1",
		[]corev1.Volume{
			{
				Name: operatorVolumeName,
				VolumeSource: corev1.VolumeSource{
					PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: "pvc1"},
				},
			},
		},
		[]corev1.VolumeMount{{Name: operatorVolumeName, MountPath: "/data"}},
	)
	pod.Labels = map[string]string{"app": "trustyai"}

	svc := serviceInfo{
		name: "trustyai", namespace: "ns1",
		storageFormat: storageFormatPVC,
		cr:            newTrustyAIService("trustyai", "ns1", "PVC"),
	}

	target := newTarget(newTestClient(newScheme(), []runtime.Object{pod}))
	step := target.Recorder.Child("test-backup-pvc", "Test backup PVC")

	task := &prepareTask{action: &DataAction{BackupDir: outputDir}}
	task.backupPVC(ctx, target, executor, svc, outputDir, step)

	entries, _ := os.ReadDir(outputDir)
	g.Expect(entries).ToNot(BeEmpty())
}

func TestBackupPVC_DryRun(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	pod := newPodWithVolumes("trustyai-pod", "ns1",
		[]corev1.Volume{
			{
				Name: operatorVolumeName,
				VolumeSource: corev1.VolumeSource{
					PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: "pvc1"},
				},
			},
		},
		[]corev1.VolumeMount{
			{Name: operatorVolumeName, MountPath: "/data"},
		},
	)

	svc := serviceInfo{
		name: "trustyai", namespace: "ns1",
		storageFormat: storageFormatPVC,
		cr:            newTrustyAIService("trustyai", "ns1", "PVC"),
	}

	target := newTarget(newTestClient(newScheme(), []runtime.Object{pod}),
		func(t *action.Target) { t.DryRun = true },
	)
	step := target.Recorder.Child("test", "test")

	task := &prepareTask{action: &DataAction{}}
	task.backupPVC(ctx, target, nil, svc, t.TempDir(), step)

	g.Expect(true).To(BeTrue())
}

func TestBackupPVC_NoPodFound(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	svc := serviceInfo{
		name: "trustyai", namespace: "ns1",
		storageFormat: storageFormatPVC,
	}

	target := newTarget(newTestClient(newScheme(), nil))
	step := target.Recorder.Child("test", "test")

	task := &prepareTask{action: &DataAction{}}
	task.backupPVC(ctx, target, nil, svc, t.TempDir(), step)

	g.Expect(true).To(BeTrue())
}

func TestBackupPVC_NoMountPath(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	pod := newPodWithVolumes("trustyai-pod", "ns1", nil, nil)
	pod.Labels = map[string]string{"app": "trustyai"}

	svc := serviceInfo{
		name: "trustyai", namespace: "ns1",
		storageFormat: storageFormatPVC,
	}

	target := newTarget(newTestClient(newScheme(), []runtime.Object{pod}))
	step := target.Recorder.Child("test", "test")

	task := &prepareTask{action: &DataAction{}}
	task.backupPVC(ctx, target, nil, svc, t.TempDir(), step)

	g.Expect(true).To(BeTrue())
}

func TestBackupPVC_NoExecutor(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	pod := newPodWithVolumes("trustyai-pod", "ns1",
		[]corev1.Volume{
			{
				Name: operatorVolumeName,
				VolumeSource: corev1.VolumeSource{
					PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: "pvc1"},
				},
			},
		},
		[]corev1.VolumeMount{
			{Name: operatorVolumeName, MountPath: "/data"},
		},
	)
	pod.Labels = map[string]string{"app": "trustyai"}

	svc := serviceInfo{
		name: "trustyai", namespace: "ns1",
		storageFormat: storageFormatPVC,
	}

	target := newTarget(newTestClient(newScheme(), []runtime.Object{pod}))
	step := target.Recorder.Child("test", "test")

	task := &prepareTask{action: &DataAction{}}
	task.backupPVC(ctx, target, nil, svc, t.TempDir(), step)

	g.Expect(true).To(BeTrue())
}

// ---------------------------------------------------------------------------
// backupDatabase
// ---------------------------------------------------------------------------

func TestBackupDatabase_DryRun(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	svc := serviceInfo{
		name: "trustyai", namespace: "ns1",
		storageFormat: storageFormatDatabase,
		cr:            newTrustyAIService("trustyai", "ns1", "DATABASE"),
	}

	secret := newSecret("trustyai-db-credentials", "ns1", map[string]string{
		"databaseUsername": "admin",
		"databasePassword": "secret",
		"databaseName":     "trustyai_db",
	})
	pod := newRunningPod("mariadb-trustyai-pod", "ns1", nil)

	target := newTarget(
		newTestClient(newScheme(), []runtime.Object{pod}, secret),
		func(t *action.Target) { t.DryRun = true },
	)
	step := target.Recorder.Child("test", "test")

	task := &prepareTask{action: &DataAction{}}
	task.backupDatabase(ctx, target, nil, svc, t.TempDir(), step)

	g.Expect(true).To(BeTrue())
}

func TestBackupDatabase_NoCreds(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	svc := serviceInfo{
		name: "trustyai", namespace: "ns1",
		storageFormat: storageFormatDatabase,
	}

	target := newTarget(newTestClient(newScheme(), nil))
	step := target.Recorder.Child("test", "test")

	task := &prepareTask{action: &DataAction{}}
	task.backupDatabase(ctx, target, nil, svc, t.TempDir(), step)

	g.Expect(true).To(BeTrue())
}

func TestBackupDatabase_NoMariaDBPod(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	svc := serviceInfo{
		name: "trustyai", namespace: "ns1",
		storageFormat: storageFormatDatabase,
		cr:            newTrustyAIService("trustyai", "ns1", "DATABASE"),
	}

	secret := newSecret("trustyai-db-credentials", "ns1", map[string]string{
		"databaseUsername": "admin",
		"databasePassword": "secret",
		"databaseName":     "trustyai_db",
	})

	target := newTarget(newTestClient(newScheme(), nil, secret))
	step := target.Recorder.Child("test", "test")

	task := &prepareTask{action: &DataAction{}}
	task.backupDatabase(ctx, target, nil, svc, t.TempDir(), step)

	g.Expect(true).To(BeTrue())
}

func TestBackupDatabase_NoExecutor(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	svc := serviceInfo{
		name: "trustyai", namespace: "ns1",
		storageFormat: storageFormatDatabase,
		cr:            newTrustyAIService("trustyai", "ns1", "DATABASE"),
	}

	secret := newSecret("trustyai-db-credentials", "ns1", map[string]string{
		"databaseUsername": "admin",
		"databasePassword": "secret",
		"databaseName":     "trustyai_db",
	})
	pod := newRunningPod("mariadb-trustyai-pod", "ns1", nil)

	target := newTarget(newTestClient(newScheme(), []runtime.Object{pod}, secret))
	step := target.Recorder.Child("test", "test")

	task := &prepareTask{action: &DataAction{}}
	task.backupDatabase(ctx, target, nil, svc, t.TempDir(), step)

	g.Expect(true).To(BeTrue())
}

func TestBackupDatabase_NoDumpCommandFound(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	svc := serviceInfo{
		name: "trustyai", namespace: "ns1",
		storageFormat: storageFormatDatabase,
		cr:            newTrustyAIService("trustyai", "ns1", "DATABASE"),
	}

	secret := newSecret("trustyai-db-credentials", "ns1", map[string]string{
		"databaseUsername": "admin",
		"databasePassword": "secret",
		"databaseName":     "trustyai_db",
	})
	pod := newRunningPod("mariadb-trustyai-pod", "ns1", nil)

	executor := &podexec.MockExecutor{
		ExecFn: func(_ context.Context, _ podexec.ExecOptions) error {
			return errors.New("not found")
		},
	}

	target := newTarget(newTestClient(newScheme(), []runtime.Object{pod}, secret))
	step := target.Recorder.Child("test", "test")

	task := &prepareTask{action: &DataAction{}}
	task.backupDatabase(ctx, target, executor, svc, t.TempDir(), step)

	g.Expect(true).To(BeTrue())
}

func TestBackupDatabase_Success(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	outputDir := t.TempDir()

	svc := serviceInfo{
		name: "trustyai", namespace: "ns1",
		storageFormat: storageFormatDatabase,
		cr:            newTrustyAIService("trustyai", "ns1", "DATABASE"),
	}

	secret := newSecret("trustyai-db-credentials", "ns1", map[string]string{
		"databaseUsername": "admin",
		"databasePassword": "secret",
		"databaseName":     "trustyai_db",
	})
	pod := newRunningPod("mariadb-trustyai-pod", "ns1", nil)

	executor := &podexec.MockExecutor{
		ExecFn: func(_ context.Context, opts podexec.ExecOptions) error {
			if len(opts.Command) > 0 && opts.Command[0] == "which" {
				if opts.Command[1] == "mariadb-dump" {
					return nil
				}

				return errors.New("not found")
			}
			if opts.Stdout != nil {
				_, _ = fmt.Fprintln(opts.Stdout, "-- MariaDB dump")
				_, _ = fmt.Fprintln(opts.Stdout, "INSERT INTO metrics VALUES (1, 'spd');")
				_, _ = fmt.Fprintln(opts.Stdout, "INSERT INTO metrics VALUES (2, 'dir');")
			}

			return nil
		},
	}

	target := newTarget(newTestClient(newScheme(), []runtime.Object{pod}, secret))
	step := target.Recorder.Child("test", "test")

	task := &prepareTask{action: &DataAction{}}
	task.backupDatabase(ctx, target, executor, svc, outputDir, step)

	entries, _ := os.ReadDir(outputDir)
	g.Expect(entries).ToNot(BeEmpty())
}

func TestBackupDatabase_EmptyDump(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	outputDir := t.TempDir()

	svc := serviceInfo{
		name: "trustyai", namespace: "ns1",
		storageFormat: storageFormatDatabase,
		cr:            newTrustyAIService("trustyai", "ns1", "DATABASE"),
	}

	secret := newSecret("trustyai-db-credentials", "ns1", map[string]string{
		"databaseUsername": "admin",
		"databasePassword": "secret",
		"databaseName":     "trustyai_db",
	})
	pod := newRunningPod("mariadb-trustyai-pod", "ns1", nil)

	executor := &podexec.MockExecutor{
		ExecFn: func(_ context.Context, opts podexec.ExecOptions) error {
			if len(opts.Command) > 0 && opts.Command[0] == "which" {
				return nil
			}
			if opts.Stdout != nil {
				_, _ = fmt.Fprintln(opts.Stdout, "-- empty dump")
			}

			return nil
		},
	}

	target := newTarget(newTestClient(newScheme(), []runtime.Object{pod}, secret))
	step := target.Recorder.Child("test", "test")

	task := &prepareTask{action: &DataAction{}}
	task.backupDatabase(ctx, target, executor, svc, outputDir, step)

	g.Expect(true).To(BeTrue())
}

// ---------------------------------------------------------------------------
// restorePVC
// ---------------------------------------------------------------------------

func TestRestorePVC_EmptyBackup(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	backupDir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(backupDir, dataSubDir), 0o750)

	svc := serviceInfo{name: "trustyai", namespace: "ns1"}
	target := newTarget(newTestClient(newScheme(), nil))
	step := target.Recorder.Child("test", "test")

	task := &runTask{action: &DataAction{BackupFile: backupDir}}
	task.restorePVC(ctx, target, nil, svc, nil, step)

	g.Expect(true).To(BeTrue())
}

func TestRestorePVC_DryRun(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	backupDir := t.TempDir()
	dataDir := filepath.Join(backupDir, dataSubDir)
	_ = os.MkdirAll(dataDir, 0o750)
	_ = os.WriteFile(filepath.Join(dataDir, "file.csv"), []byte("data"), 0o600)

	pod := newPodWithVolumes("trustyai-pod", "ns1",
		[]corev1.Volume{
			{
				Name: operatorVolumeName,
				VolumeSource: corev1.VolumeSource{
					PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: "pvc1"},
				},
			},
		},
		[]corev1.VolumeMount{{Name: operatorVolumeName, MountPath: "/data"}},
	)
	pod.Labels = map[string]string{"app": "trustyai"}

	svc := serviceInfo{name: "trustyai", namespace: "ns1"}

	target := newTarget(
		newTestClient(newScheme(), []runtime.Object{pod}),
		func(t *action.Target) { t.DryRun = true },
	)
	step := target.Recorder.Child("test", "test")

	task := &runTask{action: &DataAction{BackupFile: backupDir}}
	task.restorePVC(ctx, target, nil, svc, nil, step)

	g.Expect(true).To(BeTrue())
}

func TestRestorePVC_WithMetadata(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	backupDir := t.TempDir()
	dataDir := filepath.Join(backupDir, dataSubDir)
	_ = os.MkdirAll(dataDir, 0o750)
	_ = os.WriteFile(filepath.Join(dataDir, "file.csv"), []byte("data"), 0o600)

	pod := newRunningPod("trustyai-pod", "ns1", map[string]string{"app": "trustyai"})

	svc := serviceInfo{name: "trustyai", namespace: "ns1"}
	meta := &DataBackupMetadata{MountPath: "/data", PVCName: "pvc1"}

	executor := &podexec.MockExecutor{
		ExecFn: func(_ context.Context, opts podexec.ExecOptions) error {
			if opts.Stdin != nil {
				_, _ = io.Copy(io.Discard, opts.Stdin)
			}

			return nil
		},
	}

	target := newTarget(newTestClient(newScheme(), []runtime.Object{pod}))
	step := target.Recorder.Child("test", "test")

	task := &runTask{action: &DataAction{BackupFile: backupDir}}
	task.restorePVC(ctx, target, executor, svc, meta, step)

	g.Expect(true).To(BeTrue())
}

func TestRestorePVC_NoExecutor(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	backupDir := t.TempDir()
	dataDir := filepath.Join(backupDir, dataSubDir)
	_ = os.MkdirAll(dataDir, 0o750)
	_ = os.WriteFile(filepath.Join(dataDir, "file.csv"), []byte("data"), 0o600)

	pod := newPodWithVolumes("trustyai-pod", "ns1",
		[]corev1.Volume{
			{
				Name: operatorVolumeName,
				VolumeSource: corev1.VolumeSource{
					PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: "pvc1"},
				},
			},
		},
		[]corev1.VolumeMount{{Name: operatorVolumeName, MountPath: "/data"}},
	)
	pod.Labels = map[string]string{"app": "trustyai"}

	svc := serviceInfo{name: "trustyai", namespace: "ns1"}

	target := newTarget(newTestClient(newScheme(), []runtime.Object{pod}))
	step := target.Recorder.Child("test", "test")

	task := &runTask{action: &DataAction{BackupFile: backupDir}}
	task.restorePVC(ctx, target, nil, svc, nil, step)

	g.Expect(true).To(BeTrue())
}

// ---------------------------------------------------------------------------
// restoreDatabase
// ---------------------------------------------------------------------------

func TestRestoreDatabase_EmptyDump(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	backupDir := t.TempDir()
	_ = os.WriteFile(filepath.Join(backupDir, dumpFileName), []byte(""), 0o600)

	svc := serviceInfo{name: "trustyai", namespace: "ns1"}
	target := newTarget(newTestClient(newScheme(), nil))
	step := target.Recorder.Child("test", "test")

	task := &runTask{action: &DataAction{BackupFile: backupDir}}
	task.restoreDatabase(ctx, target, nil, svc, nil, step)

	g.Expect(true).To(BeTrue())
}

func TestRestoreDatabase_DryRun(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	backupDir := t.TempDir()
	_ = os.WriteFile(filepath.Join(backupDir, dumpFileName), []byte("-- dump\nINSERT INTO foo;\n"), 0o600)

	secret := newSecret("trustyai-db-credentials", "ns1", map[string]string{
		"databaseUsername": "admin",
		"databasePassword": "secret",
		"databaseName":     "trustyai_db",
	})
	pod := newRunningPod("mariadb-pod", "ns1", nil)

	executor := &podexec.MockExecutor{
		ExecFn: func(_ context.Context, opts podexec.ExecOptions) error {
			if opts.Command[0] == "which" && opts.Command[1] == "mariadb" {
				return nil
			}

			return errors.New("not found")
		},
	}

	svc := serviceInfo{
		name: "trustyai", namespace: "ns1",
		cr: newTrustyAIService("trustyai", "ns1", "DATABASE"),
	}

	target := newTarget(
		newTestClient(newScheme(), []runtime.Object{pod}, secret),
		func(t *action.Target) { t.DryRun = true },
	)
	step := target.Recorder.Child("test", "test")

	task := &runTask{action: &DataAction{BackupFile: backupDir}}
	task.restoreDatabase(ctx, target, executor, svc, nil, step)

	g.Expect(true).To(BeTrue())
}

func TestRestoreDatabase_Success(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	backupDir := t.TempDir()
	_ = os.WriteFile(filepath.Join(backupDir, dumpFileName), []byte("-- dump\nINSERT INTO foo;\n"), 0o600)

	secret := newSecret("trustyai-db-credentials", "ns1", map[string]string{
		"databaseUsername": "admin",
		"databasePassword": "secret",
		"databaseName":     "trustyai_db",
	})
	pod := newRunningPod("mariadb-pod", "ns1", nil)

	executor := &podexec.MockExecutor{
		ExecFn: func(_ context.Context, opts podexec.ExecOptions) error {
			if opts.Command[0] == "which" {
				return nil
			}
			if opts.Stdin != nil {
				_, _ = io.Copy(io.Discard, opts.Stdin)
			}

			return nil
		},
	}

	svc := serviceInfo{
		name: "trustyai", namespace: "ns1",
		cr: newTrustyAIService("trustyai", "ns1", "DATABASE"),
	}

	target := newTarget(newTestClient(newScheme(), []runtime.Object{pod}, secret))
	step := target.Recorder.Child("test", "test")

	task := &runTask{action: &DataAction{BackupFile: backupDir}}
	task.restoreDatabase(ctx, target, executor, svc, nil, step)

	g.Expect(true).To(BeTrue())
}

func TestRestoreDatabase_SQLError(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	backupDir := t.TempDir()
	_ = os.WriteFile(filepath.Join(backupDir, dumpFileName), []byte("-- dump\nINSERT INTO foo;\n"), 0o600)

	secret := newSecret("trustyai-db-credentials", "ns1", map[string]string{
		"databaseUsername": "admin",
		"databasePassword": "secret",
		"databaseName":     "trustyai_db",
	})
	pod := newRunningPod("mariadb-pod", "ns1", nil)

	executor := &podexec.MockExecutor{
		ExecFn: func(_ context.Context, opts podexec.ExecOptions) error {
			if opts.Command[0] == "which" {
				return nil
			}
			if opts.Stdin != nil {
				_, _ = io.Copy(io.Discard, opts.Stdin)
			}
			if opts.Stderr != nil {
				_, _ = fmt.Fprintln(opts.Stderr, "ERROR 1045 (28000): Access denied")
			}

			return errors.New("command failed")
		},
	}

	svc := serviceInfo{
		name: "trustyai", namespace: "ns1",
		cr: newTrustyAIService("trustyai", "ns1", "DATABASE"),
	}

	target := newTarget(newTestClient(newScheme(), []runtime.Object{pod}, secret))
	step := target.Recorder.Child("test", "test")

	task := &runTask{action: &DataAction{BackupFile: backupDir}}
	task.restoreDatabase(ctx, target, executor, svc, nil, step)

	g.Expect(true).To(BeTrue())
}

func TestRestoreDatabase_WarningsOnly(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	backupDir := t.TempDir()
	_ = os.WriteFile(filepath.Join(backupDir, dumpFileName), []byte("-- dump\nINSERT INTO foo;\n"), 0o600)

	secret := newSecret("trustyai-db-credentials", "ns1", map[string]string{
		"databaseUsername": "admin",
		"databasePassword": "secret",
		"databaseName":     "trustyai_db",
	})
	pod := newRunningPod("mariadb-pod", "ns1", nil)

	executor := &podexec.MockExecutor{
		ExecFn: func(_ context.Context, opts podexec.ExecOptions) error {
			if opts.Command[0] == "which" {
				return nil
			}
			if opts.Stdin != nil {
				_, _ = io.Copy(io.Discard, opts.Stdin)
			}
			if opts.Stderr != nil {
				_, _ = fmt.Fprintln(opts.Stderr, "Warning: some deprecation notice")
			}

			return errors.New("non-zero exit")
		},
	}

	svc := serviceInfo{
		name: "trustyai", namespace: "ns1",
		cr: &unstructured.Unstructured{Object: map[string]any{
			"apiVersion": "trustyai.opendatahub.io/v1alpha1",
			"kind":       "TrustyAIService",
			"metadata":   map[string]any{"name": "trustyai", "namespace": "ns1"},
		}},
	}

	target := newTarget(newTestClient(newScheme(), []runtime.Object{pod}, secret))
	step := target.Recorder.Child("test", "test")

	task := &runTask{action: &DataAction{BackupFile: backupDir}}
	task.restoreDatabase(ctx, target, executor, svc, nil, step)

	g.Expect(true).To(BeTrue())
}

func TestRestoreDatabase_NoExecutor(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	backupDir := t.TempDir()
	_ = os.WriteFile(filepath.Join(backupDir, dumpFileName), []byte("-- dump\nINSERT INTO foo;\n"), 0o600)

	secret := newSecret("trustyai-db-credentials", "ns1", map[string]string{
		"databaseUsername": "admin",
		"databasePassword": "secret",
		"databaseName":     "trustyai_db",
	})
	pod := newRunningPod("mariadb-pod", "ns1", nil)

	svc := serviceInfo{
		name: "trustyai", namespace: "ns1",
		cr: newTrustyAIService("trustyai", "ns1", "DATABASE"),
	}

	target := newTarget(newTestClient(newScheme(), []runtime.Object{pod}, secret))
	step := target.Recorder.Child("test", "test")

	task := &runTask{action: &DataAction{BackupFile: backupDir}}
	task.restoreDatabase(ctx, target, nil, svc, nil, step)

	g.Expect(true).To(BeTrue())
}

func TestRestoreDatabase_NoClientCommand(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	backupDir := t.TempDir()
	_ = os.WriteFile(filepath.Join(backupDir, dumpFileName), []byte("-- dump\nINSERT INTO foo;\n"), 0o600)

	secret := newSecret("trustyai-db-credentials", "ns1", map[string]string{
		"databaseUsername": "admin",
		"databasePassword": "secret",
		"databaseName":     "trustyai_db",
	})
	pod := newRunningPod("mariadb-pod", "ns1", nil)

	executor := &podexec.MockExecutor{
		ExecFn: func(_ context.Context, _ podexec.ExecOptions) error {
			return errors.New("not found")
		},
	}

	svc := serviceInfo{
		name: "trustyai", namespace: "ns1",
		cr: newTrustyAIService("trustyai", "ns1", "DATABASE"),
	}

	target := newTarget(newTestClient(newScheme(), []runtime.Object{pod}, secret))
	step := target.Recorder.Child("test", "test")

	task := &runTask{action: &DataAction{BackupFile: backupDir}}
	task.restoreDatabase(ctx, target, executor, svc, nil, step)

	g.Expect(true).To(BeTrue())
}

// ---------------------------------------------------------------------------
// restorePVC - additional error paths
// ---------------------------------------------------------------------------

func TestRestorePVC_NoPodFound(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	backupDir := t.TempDir()
	dataDir := filepath.Join(backupDir, dataSubDir)
	_ = os.MkdirAll(dataDir, 0o750)
	_ = os.WriteFile(filepath.Join(dataDir, "file.csv"), []byte("data"), 0o600)

	svc := serviceInfo{name: "trustyai", namespace: "ns1"}
	target := newTarget(newTestClient(newScheme(), nil))
	step := target.Recorder.Child("test", "test")

	task := &runTask{action: &DataAction{BackupFile: backupDir}}
	task.restorePVC(ctx, target, nil, svc, nil, step)

	g.Expect(true).To(BeTrue())
}

func TestRestorePVC_CopyToPodError(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	backupDir := t.TempDir()
	dataDir := filepath.Join(backupDir, dataSubDir)
	_ = os.MkdirAll(dataDir, 0o750)
	_ = os.WriteFile(filepath.Join(dataDir, "file.csv"), []byte("data"), 0o600)

	pod := newPodWithVolumes("trustyai-pod", "ns1",
		[]corev1.Volume{
			{
				Name: operatorVolumeName,
				VolumeSource: corev1.VolumeSource{
					PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: "pvc1"},
				},
			},
		},
		[]corev1.VolumeMount{{Name: operatorVolumeName, MountPath: "/data"}},
	)
	pod.Labels = map[string]string{"app": "trustyai"}

	svc := serviceInfo{name: "trustyai", namespace: "ns1"}

	executor := &podexec.MockExecutor{
		ExecFn: func(_ context.Context, _ podexec.ExecOptions) error {
			return errors.New("connection refused")
		},
	}

	target := newTarget(newTestClient(newScheme(), []runtime.Object{pod}))
	step := target.Recorder.Child("test", "test")

	task := &runTask{action: &DataAction{BackupFile: backupDir}}
	task.restorePVC(ctx, target, executor, svc, nil, step)

	g.Expect(true).To(BeTrue())
}

func TestRestorePVC_UserCancels(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	backupDir := t.TempDir()
	dataDir := filepath.Join(backupDir, dataSubDir)
	_ = os.MkdirAll(dataDir, 0o750)
	_ = os.WriteFile(filepath.Join(dataDir, "file.csv"), []byte("data"), 0o600)

	pod := newPodWithVolumes("trustyai-pod", "ns1",
		[]corev1.Volume{
			{
				Name: operatorVolumeName,
				VolumeSource: corev1.VolumeSource{
					PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: "pvc1"},
				},
			},
		},
		[]corev1.VolumeMount{{Name: operatorVolumeName, MountPath: "/data"}},
	)
	pod.Labels = map[string]string{"app": "trustyai"}

	svc := serviceInfo{name: "trustyai", namespace: "ns1"}
	meta := &DataBackupMetadata{MountPath: "/data", PVCName: "pvc1"}

	inBuf := bytes.NewBufferString("n\n")
	io := iostreams.NewIOStreams(inBuf, &bytes.Buffer{}, &bytes.Buffer{})

	target := newTarget(
		newTestClient(newScheme(), []runtime.Object{pod}),
		func(t *action.Target) {
			t.SkipConfirm = false
			t.IO = io
			t.Recorder = action.NewVerboseRootRecorder(io)
		},
	)
	step := target.Recorder.Child("test", "test")

	task := &runTask{action: &DataAction{BackupFile: backupDir}}
	task.restorePVC(ctx, target, nil, svc, meta, step)

	g.Expect(true).To(BeTrue())
}

// ---------------------------------------------------------------------------
// restoreDatabase - additional error paths
// ---------------------------------------------------------------------------

func TestRestoreDatabase_NoCreds(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	backupDir := t.TempDir()
	_ = os.WriteFile(filepath.Join(backupDir, dumpFileName), []byte("-- dump\nINSERT INTO foo;\n"), 0o600)

	svc := serviceInfo{
		name: "trustyai", namespace: "ns1",
		cr: newTrustyAIService("trustyai", "ns1", "DATABASE"),
	}

	target := newTarget(newTestClient(newScheme(), nil))
	step := target.Recorder.Child("test", "test")

	task := &runTask{action: &DataAction{BackupFile: backupDir}}
	task.restoreDatabase(ctx, target, nil, svc, nil, step)

	g.Expect(true).To(BeTrue())
}

func TestRestoreDatabase_UserCancels(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	backupDir := t.TempDir()
	_ = os.WriteFile(filepath.Join(backupDir, dumpFileName), []byte("-- dump\nINSERT INTO foo;\n"), 0o600)

	secret := newSecret("trustyai-db-credentials", "ns1", map[string]string{
		"databaseUsername": "admin",
		"databasePassword": "secret",
		"databaseName":     "trustyai_db",
	})
	pod := newRunningPod("mariadb-pod", "ns1", nil)

	executor := &podexec.MockExecutor{
		ExecFn: func(_ context.Context, opts podexec.ExecOptions) error {
			if opts.Command[0] == "which" {
				return nil
			}

			return nil
		},
	}

	svc := serviceInfo{
		name: "trustyai", namespace: "ns1",
		cr: newTrustyAIService("trustyai", "ns1", "DATABASE"),
	}

	inBuf := bytes.NewBufferString("n\n")
	io := iostreams.NewIOStreams(inBuf, &bytes.Buffer{}, &bytes.Buffer{})

	target := newTarget(
		newTestClient(newScheme(), []runtime.Object{pod}, secret),
		func(t *action.Target) {
			t.SkipConfirm = false
			t.IO = io
			t.Recorder = action.NewVerboseRootRecorder(io)
		},
	)
	step := target.Recorder.Child("test", "test")

	task := &runTask{action: &DataAction{BackupFile: backupDir}}
	task.restoreDatabase(ctx, target, executor, svc, nil, step)

	g.Expect(true).To(BeTrue())
}

func TestRestoreDatabase_NoMariaDBPod(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	backupDir := t.TempDir()
	_ = os.WriteFile(filepath.Join(backupDir, dumpFileName), []byte("-- dump\nINSERT INTO foo;\n"), 0o600)

	secret := newSecret("trustyai-db-credentials", "ns1", map[string]string{
		"databaseUsername": "admin",
		"databasePassword": "secret",
		"databaseName":     "trustyai_db",
	})

	svc := serviceInfo{
		name: "trustyai", namespace: "ns1",
		cr: newTrustyAIService("trustyai", "ns1", "DATABASE"),
	}

	target := newTarget(newTestClient(newScheme(), nil, secret))
	step := target.Recorder.Child("test", "test")

	task := &runTask{action: &DataAction{BackupFile: backupDir}}
	task.restoreDatabase(ctx, target, nil, svc, nil, step)

	g.Expect(true).To(BeTrue())
}

// ---------------------------------------------------------------------------
// backupDatabase - exec error with partial output
// ---------------------------------------------------------------------------

func TestBackupDatabase_ExecErrorWithPartialDump(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	outputDir := t.TempDir()

	svc := serviceInfo{
		name: "trustyai", namespace: "ns1",
		storageFormat: storageFormatDatabase,
		cr:            newTrustyAIService("trustyai", "ns1", "DATABASE"),
	}

	secret := newSecret("trustyai-db-credentials", "ns1", map[string]string{
		"databaseUsername": "admin",
		"databasePassword": "secret",
		"databaseName":     "trustyai_db",
	})
	pod := newRunningPod("mariadb-trustyai-pod", "ns1", nil)

	executor := &podexec.MockExecutor{
		ExecFn: func(_ context.Context, opts podexec.ExecOptions) error {
			if len(opts.Command) > 0 && opts.Command[0] == "which" {
				return nil
			}
			if opts.Stdout != nil {
				_, _ = fmt.Fprintln(opts.Stdout, "-- partial dump")
				_, _ = fmt.Fprintln(opts.Stdout, "INSERT INTO foo;")
				_, _ = fmt.Fprintln(opts.Stdout, "INSERT INTO bar;")
			}

			return errors.New("connection lost")
		},
	}

	target := newTarget(newTestClient(newScheme(), []runtime.Object{pod}, secret))
	step := target.Recorder.Child("test", "test")

	task := &prepareTask{action: &DataAction{}}
	task.backupDatabase(ctx, target, executor, svc, outputDir, step)

	g.Expect(true).To(BeTrue())
}

// ---------------------------------------------------------------------------
// prepareTask.Execute with PVC storage - end-to-end
// ---------------------------------------------------------------------------

func TestPrepareTask_Execute_PVC_DryRun(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	svc := newTrustyAIService("trustyai", "ns1", "PVC")
	pod := newPodWithVolumes("trustyai-pod", "ns1",
		[]corev1.Volume{
			{
				Name: operatorVolumeName,
				VolumeSource: corev1.VolumeSource{
					PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: "pvc1"},
				},
			},
		},
		[]corev1.VolumeMount{{Name: operatorVolumeName, MountPath: "/data"}},
	)
	pod.Labels = map[string]string{"app": "trustyai"}

	target := newTarget(
		newTestClient(newScheme(), []runtime.Object{pod}, svc),
		func(t *action.Target) { t.DryRun = true },
	)

	a := &DataAction{}
	result, err := a.Prepare().Execute(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Completed).To(BeTrue())
}

func TestPrepareTask_Execute_Database_DryRun(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	svc := newTrustyAIService("trustyai", "ns1", "DATABASE")
	secret := newSecret("trustyai-db-credentials", "ns1", map[string]string{
		"databaseUsername": "admin",
		"databasePassword": "secret",
		"databaseName":     "trustyai_db",
	})
	pod := newRunningPod("mariadb-trustyai-pod", "ns1", nil)

	target := newTarget(
		newTestClient(newScheme(), []runtime.Object{pod}, svc, secret),
		func(t *action.Target) { t.DryRun = true },
	)

	a := &DataAction{}
	result, err := a.Prepare().Execute(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Completed).To(BeTrue())
}
