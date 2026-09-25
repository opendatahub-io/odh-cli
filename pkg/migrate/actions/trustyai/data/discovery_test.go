//nolint:testpackage // Tests internal implementation
package data

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blang/semver/v4"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8sfake "k8s.io/client-go/kubernetes/fake"

	"github.com/opendatahub-io/odh-cli/pkg/migrate/action"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
	podexec "github.com/opendatahub-io/odh-cli/pkg/util/exec"
	"github.com/opendatahub-io/odh-cli/pkg/util/iostreams"

	. "github.com/onsi/gomega"
)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

func newScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	_ = metav1.AddMetaToScheme(scheme)

	return scheme
}

func newTestClient(scheme *runtime.Scheme, k8sObjects []runtime.Object, dynamicObjects ...runtime.Object) client.Client {
	listKinds := map[schema.GroupVersionResource]string{
		resources.TrustyAIService.GVR(): resources.TrustyAIService.ListKind(),
		resources.Secret.GVR():          resources.Secret.ListKind(),
	}

	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, listKinds, dynamicObjects...)
	k8sClient := k8sfake.NewSimpleClientset(k8sObjects...)

	return client.NewForTesting(client.TestClientConfig{
		Dynamic:    dynamicClient,
		Kubernetes: k8sClient,
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

func newTrustyAIService(name, namespace, storageFormat string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.TrustyAIService.APIVersion(),
			"kind":       resources.TrustyAIService.Kind,
			"metadata": map[string]any{
				"name":      name,
				"namespace": namespace,
			},
			"spec": map[string]any{
				"storage": map[string]any{
					"format": storageFormat,
				},
			},
		},
	}
}

func newSecret(name, namespace string, data map[string]string) *unstructured.Unstructured {
	encodedData := make(map[string]any, len(data))
	for k, v := range data {
		encodedData[k] = base64.StdEncoding.EncodeToString([]byte(v))
	}

	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.Secret.APIVersion(),
			"kind":       resources.Secret.Kind,
			"metadata": map[string]any{
				"name":      name,
				"namespace": namespace,
			},
			"data": encodedData,
		},
	}
}

func newRunningPod(name, namespace string, labels map[string]string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    labels,
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
		},
	}
}

func newPodWithVolumes(name, namespace string, volumes []corev1.Volume, mounts []corev1.VolumeMount) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:         trustyaiContainer,
					VolumeMounts: mounts,
				},
			},
			Volumes: volumes,
		},
	}
}

// ---------------------------------------------------------------------------
// normalizeStorageFormat
// ---------------------------------------------------------------------------

func TestNormalizeStorageFormat(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"PVC", "PVC"},
		{"pvc", "PVC"},
		{"", "PVC"},
		{"DATABASE", "DATABASE"},
		{"database", "DATABASE"},
		{"DB", "DATABASE"},
		{"db", "DATABASE"},
		{"UNKNOWN", "UNKNOWN"},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%q->%s", tt.input, tt.expected), func(t *testing.T) {
			g := NewWithT(t)
			g.Expect(normalizeStorageFormat(tt.input)).To(Equal(tt.expected))
		})
	}
}

// ---------------------------------------------------------------------------
// containsSQLError
// ---------------------------------------------------------------------------

func TestContainsSQLError(t *testing.T) {
	tests := []struct {
		name     string
		output   string
		expected bool
	}{
		{"no error", "some output\nall good", false},
		{"error line", "ERROR 1045 (28000): Access denied", true},
		{"error with whitespace", "  ERROR at line 1", true},
		{"error in middle", "warning\nERROR something\ndone", true},
		{"empty", "", false},
		{"lowercase error", "error is not sql error", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			g.Expect(containsSQLError(tt.output)).To(Equal(tt.expected))
		})
	}
}

// ---------------------------------------------------------------------------
// trySecretKeys
// ---------------------------------------------------------------------------

func TestTrySecretKeys(t *testing.T) {
	g := NewWithT(t)

	data := map[string][]byte{
		"databaseUser":     []byte("admin"),
		"databasePassword": []byte("secret"),
	}

	g.Expect(trySecretKeys(data, usernameKeys)).To(Equal("admin"))
	g.Expect(trySecretKeys(data, passwordKeys)).To(Equal("secret"))
	g.Expect(trySecretKeys(data, databaseKeys)).To(Equal(""))
	g.Expect(trySecretKeys(nil, usernameKeys)).To(Equal(""))
}

func TestTrySecretKeys_TrimsWhitespace(t *testing.T) {
	g := NewWithT(t)

	data := map[string][]byte{
		"user": []byte("  admin  \n"),
	}

	g.Expect(trySecretKeys(data, usernameKeys)).To(Equal("admin"))
}

// ---------------------------------------------------------------------------
// extractSecretData
// ---------------------------------------------------------------------------

func TestExtractSecretData(t *testing.T) {
	g := NewWithT(t)

	secret := newSecret("test", "ns1", map[string]string{
		"username": "admin",
		"password": "s3cret",
	})

	result := extractSecretData(secret)
	g.Expect(result).To(HaveKeyWithValue("username", []byte("admin")))
	g.Expect(result).To(HaveKeyWithValue("password", []byte("s3cret")))
}

func TestExtractSecretData_NoDataField(t *testing.T) {
	g := NewWithT(t)

	secret := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "Secret",
			"metadata":   map[string]any{"name": "test", "namespace": "ns1"},
		},
	}

	g.Expect(extractSecretData(secret)).To(BeNil())
}

func TestExtractSecretData_InvalidBase64(t *testing.T) {
	g := NewWithT(t)

	secret := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "Secret",
			"metadata":   map[string]any{"name": "test", "namespace": "ns1"},
			"data": map[string]any{
				"valid":   base64.StdEncoding.EncodeToString([]byte("ok")),
				"invalid": "not-valid-base64!!!",
			},
		},
	}

	result := extractSecretData(secret)
	g.Expect(result).To(HaveKeyWithValue("valid", []byte("ok")))
	g.Expect(result).ToNot(HaveKey("invalid"))
}

// ---------------------------------------------------------------------------
// countFiles / countLines
// ---------------------------------------------------------------------------

func TestCountFiles(t *testing.T) {
	g := NewWithT(t)
	dir := t.TempDir()

	_ = os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello"), 0o600)
	_ = os.WriteFile(filepath.Join(dir, "b.txt"), []byte("world"), 0o600)
	_ = os.MkdirAll(filepath.Join(dir, "sub"), 0o750)
	_ = os.WriteFile(filepath.Join(dir, "sub", "c.txt"), []byte("nested"), 0o600)

	count, err := countFiles(dir)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(count).To(Equal(3))
}

func TestCountFiles_EmptyDir(t *testing.T) {
	g := NewWithT(t)

	count, err := countFiles(t.TempDir())
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(count).To(Equal(0))
}

func TestCountLines(t *testing.T) {
	g := NewWithT(t)
	f := filepath.Join(t.TempDir(), "dump.sql")
	_ = os.WriteFile(f, []byte("line1\nline2\nline3\n"), 0o600)

	count, err := countLines(f)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(count).To(Equal(3))
}

func TestCountLines_LongLine(t *testing.T) {
	g := NewWithT(t)
	f := filepath.Join(t.TempDir(), "dump.sql")

	longLine := strings.Repeat("x", 128*1024) // 128KB — exceeds bufio.MaxScanTokenSize (64KB)
	content := "line1\n" + longLine + "\nline3\n"
	_ = os.WriteFile(f, []byte(content), 0o600)

	count, err := countLines(f)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(count).To(Equal(3))
}

func TestCountLines_NoTrailingNewline(t *testing.T) {
	g := NewWithT(t)
	f := filepath.Join(t.TempDir(), "dump.sql")
	_ = os.WriteFile(f, []byte("line1\nline2"), 0o600)

	count, err := countLines(f)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(count).To(Equal(2))
}

func TestCountLines_NotFound(t *testing.T) {
	g := NewWithT(t)

	_, err := countLines("/nonexistent/file")
	g.Expect(err).To(HaveOccurred())
}

// ---------------------------------------------------------------------------
// writeMetadata / loadMetadata / detectBackupType
// ---------------------------------------------------------------------------

func TestWriteAndLoadMetadata(t *testing.T) {
	g := NewWithT(t)
	dir := t.TempDir()

	meta := DataBackupMetadata{
		Timestamp:       "20250605-120000",
		Namespace:       "ns1",
		TrustyAIService: "trustyai-service",
		StorageFormat:   "PVC",
		PVCName:         "my-pvc",
		MountPath:       "/data",
		SourcePod:       "pod-abc",
		FileCount:       42,
	}

	err := writeMetadata(dir, meta)
	g.Expect(err).ToNot(HaveOccurred())

	loaded := loadMetadata(dir)
	g.Expect(loaded).ToNot(BeNil())
	g.Expect(loaded.Namespace).To(Equal("ns1"))
	g.Expect(loaded.StorageFormat).To(Equal("PVC"))
	g.Expect(loaded.FileCount).To(Equal(42))
}

func TestLoadMetadata_NotFound(t *testing.T) {
	g := NewWithT(t)

	g.Expect(loadMetadata(t.TempDir())).To(BeNil())
}

func TestDetectBackupType_FromMetadata(t *testing.T) {
	g := NewWithT(t)
	dir := t.TempDir()

	_ = writeMetadata(dir, DataBackupMetadata{StorageFormat: "DATABASE"})

	storageFormat, err := detectBackupType(dir)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(storageFormat).To(Equal("DATABASE"))
}

func TestDetectBackupType_FromDataDir(t *testing.T) {
	g := NewWithT(t)
	dir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(dir, dataSubDir), 0o750)

	storageFormat, err := detectBackupType(dir)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(storageFormat).To(Equal("PVC"))
}

func TestDetectBackupType_FromDumpSQL(t *testing.T) {
	g := NewWithT(t)
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, dumpFileName), []byte("SQL"), 0o600)

	storageFormat, err := detectBackupType(dir)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(storageFormat).To(Equal("DATABASE"))
}

func TestDetectBackupType_Unknown(t *testing.T) {
	g := NewWithT(t)

	_, err := detectBackupType(t.TempDir())
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("cannot determine backup type"))
}

// ---------------------------------------------------------------------------
// findMountByVolumeName / findMountByPVCName / findMountPath
// ---------------------------------------------------------------------------

func TestFindMountByVolumeName(t *testing.T) {
	g := NewWithT(t)

	pod := newPodWithVolumes("pod1", "ns1",
		[]corev1.Volume{
			{
				Name: operatorVolumeName,
				VolumeSource: corev1.VolumeSource{
					PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
						ClaimName: "trustyai-pvc",
					},
				},
			},
		},
		[]corev1.VolumeMount{
			{Name: operatorVolumeName, MountPath: "/data"},
		},
	)

	info := findMountByVolumeName(pod, operatorVolumeName)
	g.Expect(info).ToNot(BeNil())
	g.Expect(info.mountPath).To(Equal("/data"))
	g.Expect(info.pvcName).To(Equal("trustyai-pvc"))
}

func TestFindMountByVolumeName_NotFound(t *testing.T) {
	g := NewWithT(t)

	pod := newPodWithVolumes("pod1", "ns1", nil, nil)
	g.Expect(findMountByVolumeName(pod, operatorVolumeName)).To(BeNil())
}

func TestFindMountByVolumeName_NoContainers(t *testing.T) {
	g := NewWithT(t)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "pod1"},
		Spec: corev1.PodSpec{
			Volumes: []corev1.Volume{
				{
					Name: operatorVolumeName,
					VolumeSource: corev1.VolumeSource{
						PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
							ClaimName: "pvc1",
						},
					},
				},
			},
		},
	}

	g.Expect(findMountByVolumeName(pod, operatorVolumeName)).To(BeNil())
}

func TestFindMountByVolumeName_SidecarBeforeApp(t *testing.T) {
	g := NewWithT(t)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "pod1", Namespace: "ns1"},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "istio-proxy"},
				{
					Name:         trustyaiContainer,
					VolumeMounts: []corev1.VolumeMount{{Name: operatorVolumeName, MountPath: "/data"}},
				},
			},
			Volumes: []corev1.Volume{
				{
					Name: operatorVolumeName,
					VolumeSource: corev1.VolumeSource{
						PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: "trustyai-pvc"},
					},
				},
			},
		},
	}

	info := findMountByVolumeName(pod, operatorVolumeName)
	g.Expect(info).ToNot(BeNil())
	g.Expect(info.mountPath).To(Equal("/data"))
	g.Expect(info.pvcName).To(Equal("trustyai-pvc"))
}

func TestFindMountByPVCName_SidecarBeforeApp(t *testing.T) {
	g := NewWithT(t)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "pod1", Namespace: "ns1"},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "istio-proxy"},
				{
					Name:         trustyaiContainer,
					VolumeMounts: []corev1.VolumeMount{{Name: "data-vol", MountPath: "/opt/data"}},
				},
			},
			Volumes: []corev1.Volume{
				{
					Name: "data-vol",
					VolumeSource: corev1.VolumeSource{
						PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: "trustyai-service-pvc"},
					},
				},
			},
		},
	}

	info := findMountByPVCName(pod, "trustyai-service")
	g.Expect(info).ToNot(BeNil())
	g.Expect(info.mountPath).To(Equal("/opt/data"))
	g.Expect(info.pvcName).To(Equal("trustyai-service-pvc"))
}

func TestFindMountByPVCName(t *testing.T) {
	g := NewWithT(t)

	pod := newPodWithVolumes("pod1", "ns1",
		[]corev1.Volume{
			{
				Name: "data-vol",
				VolumeSource: corev1.VolumeSource{
					PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
						ClaimName: "trustyai-service-pvc",
					},
				},
			},
		},
		[]corev1.VolumeMount{
			{Name: "data-vol", MountPath: "/opt/data"},
		},
	)

	info := findMountByPVCName(pod, "trustyai-service")
	g.Expect(info).ToNot(BeNil())
	g.Expect(info.mountPath).To(Equal("/opt/data"))
	g.Expect(info.pvcName).To(Equal("trustyai-service-pvc"))
}

func TestFindMountByPVCName_NonPVCVolume(t *testing.T) {
	g := NewWithT(t)

	pod := newPodWithVolumes("pod1", "ns1",
		[]corev1.Volume{
			{
				Name:         "config-vol",
				VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{LocalObjectReference: corev1.LocalObjectReference{Name: "my-config"}}},
			},
			{
				Name: "data-vol",
				VolumeSource: corev1.VolumeSource{
					PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
						ClaimName: "trustyai-pvc",
					},
				},
			},
		},
		[]corev1.VolumeMount{
			{Name: "config-vol", MountPath: "/config"},
			{Name: "data-vol", MountPath: "/data"},
		},
	)

	info := findMountByPVCName(pod, "trustyai")
	g.Expect(info).ToNot(BeNil())
	g.Expect(info.mountPath).To(Equal("/data"))
}

func TestFindMountByPVCName_NoMatch(t *testing.T) {
	g := NewWithT(t)

	pod := newPodWithVolumes("pod1", "ns1",
		[]corev1.Volume{
			{
				Name: "other-vol",
				VolumeSource: corev1.VolumeSource{
					PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
						ClaimName: "other-pvc",
					},
				},
			},
		},
		[]corev1.VolumeMount{
			{Name: "other-vol", MountPath: "/other"},
		},
	)

	g.Expect(findMountByPVCName(pod, "trustyai")).To(BeNil())
}

func TestFindMountPath_ByVolumeName(t *testing.T) {
	g := NewWithT(t)

	pod := newPodWithVolumes("pod1", "ns1",
		[]corev1.Volume{
			{
				Name: operatorVolumeName,
				VolumeSource: corev1.VolumeSource{
					PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
						ClaimName: "trustyai-pvc",
					},
				},
			},
		},
		[]corev1.VolumeMount{
			{Name: operatorVolumeName, MountPath: "/data"},
		},
	)

	info, err := findMountPath(pod, nil, "trustyai-service")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(info.mountPath).To(Equal("/data"))
}

func TestFindMountPath_FallbackToCR(t *testing.T) {
	g := NewWithT(t)

	pod := newPodWithVolumes("pod1", "ns1", nil, nil)
	cr := newTrustyAIService("trustyai", "ns1", "PVC")

	_ = unstructured.SetNestedField(cr.Object, "/custom/path", "spec", "storage", "folder")

	info, err := findMountPath(pod, cr, "trustyai")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(info.mountPath).To(Equal("/custom/path"))
}

func TestFindMountPath_ByPVCName(t *testing.T) {
	g := NewWithT(t)

	pod := newPodWithVolumes("pod1", "ns1",
		[]corev1.Volume{
			{
				Name: "custom-vol",
				VolumeSource: corev1.VolumeSource{
					PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
						ClaimName: "trustyai-service-pvc",
					},
				},
			},
		},
		[]corev1.VolumeMount{
			{Name: "custom-vol", MountPath: "/storage"},
		},
	)

	info, err := findMountPath(pod, nil, "trustyai-service")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(info.mountPath).To(Equal("/storage"))
	g.Expect(info.pvcName).To(Equal("trustyai-service-pvc"))
}

func TestFindMountPath_NoMatch(t *testing.T) {
	g := NewWithT(t)

	pod := newPodWithVolumes("pod1", "ns1", nil, nil)

	_, err := findMountPath(pod, nil, "trustyai")
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("could not determine PVC mount path"))
}

// ---------------------------------------------------------------------------
// findRunningPodByLabel / findRunningPodByName
// ---------------------------------------------------------------------------

func TestFindRunningPodByLabel(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	pod := newRunningPod("trustyai-pod", "ns1", map[string]string{
		"app": "trustyai-service",
	})

	target := newTarget(newTestClient(newScheme(), []runtime.Object{pod}))

	found, err := findRunningPodByLabel(ctx, target, "ns1", "app=trustyai-service")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(found).ToNot(BeNil())
	g.Expect(found.Name).To(Equal("trustyai-pod"))
}

func TestFindRunningPodByLabel_NoPods(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	target := newTarget(newTestClient(newScheme(), nil))

	found, err := findRunningPodByLabel(ctx, target, "ns1", "app=trustyai-service")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(found).To(BeNil())
}

func TestFindRunningPodByName(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	pod := newRunningPod("mariadb-trustyai-abc", "ns1", nil)
	target := newTarget(newTestClient(newScheme(), []runtime.Object{pod}))

	found, err := findRunningPodByName(ctx, target, "ns1", "mariadb")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(found).ToNot(BeNil())
	g.Expect(found.Name).To(Equal("mariadb-trustyai-abc"))
}

func TestFindRunningPodByName_NoMatch(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	pod := newRunningPod("other-pod", "ns1", nil)
	target := newTarget(newTestClient(newScheme(), []runtime.Object{pod}))

	found, err := findRunningPodByName(ctx, target, "ns1", "mariadb")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(found).To(BeNil())
}

func TestFindRunningPodByName_CaseInsensitive(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	pod := newRunningPod("MariaDB-Pod-123", "ns1", nil)
	target := newTarget(newTestClient(newScheme(), []runtime.Object{pod}))

	found, err := findRunningPodByName(ctx, target, "ns1", "mariadb")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(found).ToNot(BeNil())
}

// ---------------------------------------------------------------------------
// findServicePod
// ---------------------------------------------------------------------------

func TestFindServicePod_ByLabel(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	pod := newRunningPod("trustyai-abc", "ns1", map[string]string{
		"app": "trustyai-service",
	})

	target := newTarget(newTestClient(newScheme(), []runtime.Object{pod}))

	found, err := findServicePod(ctx, target, "ns1", "trustyai-service")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(found.Name).To(Equal("trustyai-abc"))
}

func TestFindServicePod_ByName(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	pod := newRunningPod("trustyai-service-xyz", "ns1", nil)
	target := newTarget(newTestClient(newScheme(), []runtime.Object{pod}))

	found, err := findServicePod(ctx, target, "ns1", "trustyai-service")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(found.Name).To(Equal("trustyai-service-xyz"))
}

func TestFindServicePod_BySecondLabel(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	pod := newRunningPod("trustyai-pod", "ns1", map[string]string{
		"app.kubernetes.io/name": "trustyai-service",
	})

	target := newTarget(newTestClient(newScheme(), []runtime.Object{pod}))

	found, err := findServicePod(ctx, target, "ns1", "trustyai-service")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(found.Name).To(Equal("trustyai-pod"))
}

func TestFindServicePod_ByThirdLabel(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	pod := newRunningPod("trustyai-pod", "ns1", map[string]string{
		"app.kubernetes.io/part-of": "trustyai",
	})

	target := newTarget(newTestClient(newScheme(), []runtime.Object{pod}))

	found, err := findServicePod(ctx, target, "ns1", "trustyai-service")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(found.Name).To(Equal("trustyai-pod"))
}

func TestFindServicePod_NotFound(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	target := newTarget(newTestClient(newScheme(), nil))

	_, err := findServicePod(ctx, target, "ns1", "trustyai-service")
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("no running TrustyAI pod found"))
}

// ---------------------------------------------------------------------------
// findMariaDBPod
// ---------------------------------------------------------------------------

func TestFindMariaDBPod_ByPattern(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	pod := newRunningPod("mariadb-trustyai-service-xyz", "ns1", nil)
	target := newTarget(newTestClient(newScheme(), []runtime.Object{pod}))

	found, err := findMariaDBPod(ctx, target, "ns1", "trustyai-service", nil)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(found.Name).To(Equal("mariadb-trustyai-service-xyz"))
}

func TestFindMariaDBPod_BySecretHost(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	pod := newRunningPod("custom-db-host-pod", "ns1", nil)
	target := newTarget(newTestClient(newScheme(), []runtime.Object{pod}))

	secretData := map[string][]byte{
		"databaseService": []byte("custom-db-host"),
	}

	found, err := findMariaDBPod(ctx, target, "ns1", "trustyai", secretData)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(found.Name).To(Equal("custom-db-host-pod"))
}

func TestFindMariaDBPod_NotFound(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	target := newTarget(newTestClient(newScheme(), nil))

	_, err := findMariaDBPod(ctx, target, "ns1", "trustyai", nil)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("no running MariaDB pod found"))
}

// ---------------------------------------------------------------------------
// secretExists / findSecretByPattern
// ---------------------------------------------------------------------------

func TestSecretExists(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	secret := newSecret("my-secret", "ns1", map[string]string{"key": "val"})
	target := newTarget(newTestClient(newScheme(), nil, secret))

	g.Expect(secretExists(ctx, target, "ns1", "my-secret")).To(BeTrue())
	g.Expect(secretExists(ctx, target, "ns1", "nonexistent")).To(BeFalse())
}

func TestFindSecretByPattern(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	secret := newSecret("trustyai-db-credentials", "ns1", map[string]string{"key": "val"})
	target := newTarget(newTestClient(newScheme(), nil, secret))

	name, err := findSecretByPattern(ctx, target, "ns1", "db-credentials")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(name).To(Equal("trustyai-db-credentials"))
}

func TestFindSecretByPattern_NoMatch(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	target := newTarget(newTestClient(newScheme(), nil))

	name, err := findSecretByPattern(ctx, target, "ns1", "db-credentials")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(name).To(Equal(""))
}

// ---------------------------------------------------------------------------
// findCredentialsSecret
// ---------------------------------------------------------------------------

func TestFindCredentialsSecret_FromCR(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	cr := newTrustyAIService("trustyai", "ns1", "DATABASE")
	_ = unstructured.SetNestedField(cr.Object, "my-db-secret", "spec", "storage", "databaseConfigurations")

	secret := newSecret("my-db-secret", "ns1", map[string]string{"key": "val"})
	target := newTarget(newTestClient(newScheme(), nil, secret))

	name, err := findCredentialsSecret(ctx, target, "ns1", cr, "trustyai")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(name).To(Equal("my-db-secret"))
}

func TestFindCredentialsSecret_ByConvention(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	secret := newSecret("trustyai-db-credentials", "ns1", map[string]string{"key": "val"})
	target := newTarget(newTestClient(newScheme(), nil, secret))

	name, err := findCredentialsSecret(ctx, target, "ns1", nil, "trustyai")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(name).To(Equal("trustyai-db-credentials"))
}

func TestFindCredentialsSecret_ByPattern(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	secret := newSecret("some-db-credentials-thing", "ns1", map[string]string{"key": "val"})
	target := newTarget(newTestClient(newScheme(), nil, secret))

	name, err := findCredentialsSecret(ctx, target, "ns1", nil, "nonexistent-svc")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(name).To(Equal("some-db-credentials-thing"))
}

func TestFindCredentialsSecret_ByMariaDBPattern(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	secret := newSecret("some-mariadb-secret", "ns1", map[string]string{"key": "val"})
	target := newTarget(newTestClient(newScheme(), nil, secret))

	name, err := findCredentialsSecret(ctx, target, "ns1", nil, "nonexistent-svc")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(name).To(Equal("some-mariadb-secret"))
}

func TestFindCredentialsSecret_NotFound(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	target := newTarget(newTestClient(newScheme(), nil))

	_, err := findCredentialsSecret(ctx, target, "ns1", nil, "trustyai")
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("no database credentials secret found"))
}

// ---------------------------------------------------------------------------
// findDBCredentials
// ---------------------------------------------------------------------------

func TestFindDBCredentials(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	secret := newSecret("trustyai-db-credentials", "ns1", map[string]string{
		"databaseUsername": "admin",
		"databasePassword": "secret123",
		"databaseName":     "trustyai_db",
	})

	target := newTarget(newTestClient(newScheme(), nil, secret))

	creds, err := findDBCredentials(ctx, target, "ns1", nil, "trustyai")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(creds.username).To(Equal("admin"))
	g.Expect(creds.password).To(Equal("secret123"))
	g.Expect(creds.database).To(Equal("trustyai_db"))
	g.Expect(creds.secretName).To(Equal("trustyai-db-credentials"))
}

func TestFindDBCredentials_MissingPassword(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	secret := newSecret("trustyai-db-credentials", "ns1", map[string]string{
		"databaseUsername": "admin",
		"databaseName":     "trustyai_db",
	})

	target := newTarget(newTestClient(newScheme(), nil, secret))

	_, err := findDBCredentials(ctx, target, "ns1", nil, "trustyai")
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("could not find password"))
}

func TestFindDBCredentials_MissingUsername(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	secret := newSecret("trustyai-db-credentials", "ns1", map[string]string{
		"databasePassword": "secret",
		"databaseName":     "trustyai_db",
	})

	target := newTarget(newTestClient(newScheme(), nil, secret))

	_, err := findDBCredentials(ctx, target, "ns1", nil, "trustyai")
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("could not find username"))
}

func TestFindDBCredentials_MissingDatabase(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	secret := newSecret("trustyai-db-credentials", "ns1", map[string]string{
		"databaseUsername": "admin",
		"databasePassword": "secret",
	})

	target := newTarget(newTestClient(newScheme(), nil, secret))

	_, err := findDBCredentials(ctx, target, "ns1", nil, "trustyai")
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("could not find database name"))
}

// ---------------------------------------------------------------------------
// discoverTrustyAIServices
// ---------------------------------------------------------------------------

func TestDiscoverTrustyAIServices(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	svc1 := newTrustyAIService("svc1", "ns1", "PVC")
	svc2 := newTrustyAIService("svc2", "ns2", "DATABASE")

	target := newTarget(newTestClient(newScheme(), nil, svc1, svc2))

	services, err := discoverTrustyAIServices(ctx, target, "")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(services).To(HaveLen(2))
}

func TestDiscoverTrustyAIServices_WithFilter(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	svc1 := newTrustyAIService("svc1", "ns1", "PVC")
	svc2 := newTrustyAIService("svc2", "ns2", "DATABASE")

	target := newTarget(newTestClient(newScheme(), nil, svc1, svc2))

	services, err := discoverTrustyAIServices(ctx, target, "svc2")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(services).To(HaveLen(1))
	g.Expect(services[0].name).To(Equal("svc2"))
	g.Expect(services[0].storageFormat).To(Equal("DATABASE"))
}

func TestDiscoverTrustyAIServices_NoneFound(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	target := newTarget(newTestClient(newScheme(), nil))

	services, err := discoverTrustyAIServices(ctx, target, "")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(services).To(BeEmpty())
}

func TestDiscoverTrustyAIServices_NormalizesFormat(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	svc := newTrustyAIService("svc1", "ns1", "db")
	target := newTarget(newTestClient(newScheme(), nil, svc))

	services, err := discoverTrustyAIServices(ctx, target, "")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(services).To(HaveLen(1))
	g.Expect(services[0].storageFormat).To(Equal("DATABASE"))
}

// ---------------------------------------------------------------------------
// commandExistsInPod / detectDumpCommand / detectClientCommand
// ---------------------------------------------------------------------------

func TestCommandExistsInPod(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "db-pod", Namespace: "ns1"}}

	executor := &podexec.MockExecutor{
		ExecFn: func(_ context.Context, opts podexec.ExecOptions) error {
			if opts.Command[1] == "mariadb-dump" {
				return nil
			}

			return errors.New("not found")
		},
	}

	g.Expect(commandExistsInPod(ctx, executor, "ns1", pod, "mariadb-dump")).To(BeTrue())
	g.Expect(commandExistsInPod(ctx, executor, "ns1", pod, "mysqldump")).To(BeFalse())
}

func TestDetectDumpCommand_MariaDB(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "db-pod", Namespace: "ns1"}}

	executor := &podexec.MockExecutor{
		ExecFn: func(_ context.Context, opts podexec.ExecOptions) error {
			if opts.Command[1] == "mariadb-dump" {
				return nil
			}

			return errors.New("not found")
		},
	}

	cmd, err := detectDumpCommand(ctx, executor, "ns1", pod)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(cmd).To(Equal("mariadb-dump"))
}

func TestDetectDumpCommand_MySQL(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "db-pod", Namespace: "ns1"}}

	executor := &podexec.MockExecutor{
		ExecFn: func(_ context.Context, opts podexec.ExecOptions) error {
			if opts.Command[1] == "mysqldump" {
				return nil
			}

			return errors.New("not found")
		},
	}

	cmd, err := detectDumpCommand(ctx, executor, "ns1", pod)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(cmd).To(Equal("mysqldump"))
}

func TestDetectDumpCommand_NoneFound(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "db-pod", Namespace: "ns1"}}

	executor := &podexec.MockExecutor{
		ExecFn: func(_ context.Context, _ podexec.ExecOptions) error {
			return errors.New("not found")
		},
	}

	_, err := detectDumpCommand(ctx, executor, "ns1", pod)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("neither mariadb-dump nor mysqldump"))
}

func TestDetectClientCommand_MariaDB(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "db-pod", Namespace: "ns1"}}

	executor := &podexec.MockExecutor{
		ExecFn: func(_ context.Context, opts podexec.ExecOptions) error {
			if opts.Command[1] == "mariadb" {
				return nil
			}

			return errors.New("not found")
		},
	}

	cmd, err := detectClientCommand(ctx, executor, "ns1", pod)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(cmd).To(Equal("mariadb"))
}

func TestDetectClientCommand_MySQL(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "db-pod", Namespace: "ns1"}}

	executor := &podexec.MockExecutor{
		ExecFn: func(_ context.Context, opts podexec.ExecOptions) error {
			if opts.Command[1] == "mysql" {
				return nil
			}

			return errors.New("not found")
		},
	}

	cmd, err := detectClientCommand(ctx, executor, "ns1", pod)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(cmd).To(Equal("mysql"))
}

func TestDetectClientCommand_NoneFound(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "db-pod", Namespace: "ns1"}}

	executor := &podexec.MockExecutor{
		ExecFn: func(_ context.Context, _ podexec.ExecOptions) error {
			return errors.New("not found")
		},
	}

	_, err := detectClientCommand(ctx, executor, "ns1", pod)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("neither mariadb nor mysql"))
}
