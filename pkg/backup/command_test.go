//nolint:testpackage // Tests internal implementation (depRegistry field)
package backup

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/cli-runtime/pkg/genericiooptions"

	. "github.com/onsi/gomega"
)

func TestCommandDefaults(t *testing.T) {
	g := NewWithT(t)

	cmd := NewCommand(genericiooptions.IOStreams{})

	g.Expect(cmd.Dependencies).To(BeTrue(), "Dependencies should default to true")
}

func TestCompleteWithDependenciesEnabled(t *testing.T) {
	g := NewWithT(t)

	cmd := NewCommand(genericiooptions.IOStreams{})
	cmd.Dependencies = true

	err := cmd.Complete()

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(cmd.depRegistry).ToNot(BeNil())

	// Verify notebook resolver is registered
	notebookGVR := schema.GroupVersionResource{
		Group:    "kubeflow.org",
		Version:  "v1",
		Resource: "notebooks",
	}
	resolver, err := cmd.depRegistry.GetResolver(notebookGVR)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(resolver).ToNot(BeNil())
}

func TestCompleteWithDependenciesDisabled(t *testing.T) {
	g := NewWithT(t)

	cmd := NewCommand(genericiooptions.IOStreams{})
	cmd.Dependencies = false

	err := cmd.Complete()

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(cmd.depRegistry).ToNot(BeNil())

	// Verify no resolvers registered
	notebookGVR := schema.GroupVersionResource{
		Group:    "kubeflow.org",
		Version:  "v1",
		Resource: "notebooks",
	}
	_, err = cmd.depRegistry.GetResolver(notebookGVR)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("no dependency resolver registered"))
}

func TestDryRunEnablesVerbose(t *testing.T) {
	g := NewWithT(t)

	cmd := NewCommand(genericiooptions.IOStreams{})
	cmd.DryRun = true
	cmd.Verbose = false

	err := cmd.Complete()

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(cmd.Verbose).To(BeTrue(), "Verbose should be auto-enabled when DryRun is true")
}

func TestDryRunLogsFilePaths(t *testing.T) {
	g := NewWithT(t)

	var errBuf bytes.Buffer
	streams := genericiooptions.IOStreams{
		Out:    os.Stdout,
		ErrOut: &errBuf,
	}

	cmd := NewCommand(streams)
	cmd.DryRun = true
	cmd.OutputDir = "/tmp/test-backup"

	err := cmd.Complete()
	g.Expect(err).ToNot(HaveOccurred())

	gvr := schema.GroupVersionResource{
		Group:    "kubeflow.org",
		Version:  "v1",
		Resource: "notebooks",
	}

	obj := &unstructured.Unstructured{}
	obj.SetNamespace("test-namespace")
	obj.SetName("test-notebook")

	err = cmd.writeResource(gvr, obj)
	g.Expect(err).ToNot(HaveOccurred())

	output := errBuf.String()
	g.Expect(output).To(ContainSubstring("Would create:"))
	g.Expect(output).To(ContainSubstring("/tmp/test-backup/test-namespace/notebooks.kubeflow.org-test-notebook.yaml"))
}

func TestDryRunStdoutMode(t *testing.T) {
	g := NewWithT(t)

	var errBuf bytes.Buffer
	streams := genericiooptions.IOStreams{
		Out:    os.Stdout,
		ErrOut: &errBuf,
	}

	cmd := NewCommand(streams)
	cmd.DryRun = true
	cmd.OutputDir = ""

	err := cmd.Complete()
	g.Expect(err).ToNot(HaveOccurred())

	gvr := schema.GroupVersionResource{
		Group:    "kubeflow.org",
		Version:  "v1",
		Resource: "notebooks",
	}

	obj := &unstructured.Unstructured{}
	obj.SetNamespace("test-namespace")
	obj.SetName("test-notebook")

	err = cmd.writeResource(gvr, obj)
	g.Expect(err).ToNot(HaveOccurred())

	output := errBuf.String()
	g.Expect(output).To(ContainSubstring("Would write to stdout:"))
	g.Expect(output).To(ContainSubstring("test-namespace/test-notebook"))
	g.Expect(output).To(ContainSubstring("notebooks"))
}

func TestDryRunNoFilesCreated(t *testing.T) {
	g := NewWithT(t)

	tmpDir := t.TempDir()

	var errBuf bytes.Buffer
	streams := genericiooptions.IOStreams{
		Out:    os.Stdout,
		ErrOut: &errBuf,
	}

	cmd := NewCommand(streams)
	cmd.DryRun = true
	cmd.OutputDir = tmpDir

	err := cmd.Complete()
	g.Expect(err).ToNot(HaveOccurred())

	gvr := schema.GroupVersionResource{
		Group:    "kubeflow.org",
		Version:  "v1",
		Resource: "notebooks",
	}

	obj := &unstructured.Unstructured{}
	obj.SetNamespace("test-namespace")
	obj.SetName("test-notebook")

	err = cmd.writeResource(gvr, obj)
	g.Expect(err).ToNot(HaveOccurred())

	entries, err := os.ReadDir(tmpDir)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(entries).To(BeEmpty(), "Dry-run should not create any files")
}

func TestDryRunClusterScopedResource(t *testing.T) {
	g := NewWithT(t)

	var errBuf bytes.Buffer
	streams := genericiooptions.IOStreams{
		Out:    os.Stdout,
		ErrOut: &errBuf,
	}

	cmd := NewCommand(streams)
	cmd.DryRun = true
	cmd.OutputDir = "/tmp/test-backup"

	err := cmd.Complete()
	g.Expect(err).ToNot(HaveOccurred())

	gvr := schema.GroupVersionResource{
		Group:    "",
		Version:  "v1",
		Resource: "nodes",
	}

	obj := &unstructured.Unstructured{}
	obj.SetName("test-node")

	err = cmd.writeResource(gvr, obj)
	g.Expect(err).ToNot(HaveOccurred())

	output := errBuf.String()
	g.Expect(output).To(ContainSubstring("Would create:"))
	g.Expect(output).To(ContainSubstring("/tmp/test-backup/cluster-scoped/nodes-test-node.yaml"))
}

func TestNormalModeStillWorks(t *testing.T) {
	g := NewWithT(t)

	tmpDir := t.TempDir()

	streams := genericiooptions.IOStreams{
		Out:    os.Stdout,
		ErrOut: os.Stderr,
	}

	cmd := NewCommand(streams)
	cmd.DryRun = false
	cmd.OutputDir = tmpDir

	err := cmd.Complete()
	g.Expect(err).ToNot(HaveOccurred())

	gvr := schema.GroupVersionResource{
		Group:    "kubeflow.org",
		Version:  "v1",
		Resource: "notebooks",
	}

	obj := &unstructured.Unstructured{}
	obj.SetNamespace("test-namespace")
	obj.SetName("test-notebook")
	obj.SetAPIVersion("kubeflow.org/v1")
	obj.SetKind("Notebook")

	err = cmd.writeResource(gvr, obj)
	g.Expect(err).ToNot(HaveOccurred())

	expectedFile := filepath.Join(tmpDir, "test-namespace", "notebooks.kubeflow.org-test-notebook.yaml")
	_, err = os.Stat(expectedFile)
	g.Expect(err).ToNot(HaveOccurred(), "Normal mode should create files")
}
