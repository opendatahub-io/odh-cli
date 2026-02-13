package pipeline_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/opendatahub-io/odh-cli/pkg/backup/dependencies"
	"github.com/opendatahub-io/odh-cli/pkg/backup/pipeline"
	"github.com/opendatahub-io/odh-cli/pkg/util/iostreams"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	. "github.com/onsi/gomega"
)

const (
	testNamespace = "test-namespace"
	notebookName  = "test-notebook"
)

func createTestWorkload() *unstructured.Unstructured {
	workload := &unstructured.Unstructured{}
	workload.SetNamespace(testNamespace)
	workload.SetName(notebookName)

	return workload
}

func TestWriterStage(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	t.Run("should write workload and dependencies", func(t *testing.T) {
		io := iostreams.NewIOStreams(nil, nil, nil)
		var writtenResources []string

		writeFunc := func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured) error {
			writtenResources = append(writtenResources, fmt.Sprintf("%s/%s", obj.GetNamespace(), obj.GetName()))

			return nil
		}

		writer := &pipeline.WriterStage{
			WriteResource: writeFunc,
			IO:            io,
		}

		input := make(chan pipeline.WorkloadWithDeps, 1)

		// Create test workload with dependencies
		workload := createTestWorkload()
		dep1 := createTestWorkload()
		dep1.SetName("dep-1")
		dep2 := createTestWorkload()
		dep2.SetName("dep-2")

		input <- pipeline.WorkloadWithDeps{
			GVR:      schema.GroupVersionResource{Group: "test", Version: "v1", Resource: "tests"},
			Instance: workload,
			Dependencies: []dependencies.Dependency{
				{
					GVR:      schema.GroupVersionResource{Group: "v1", Version: "", Resource: "configmaps"},
					Resource: dep1,
				},
				{
					GVR:      schema.GroupVersionResource{Group: "v1", Version: "", Resource: "persistentvolumeclaims"},
					Resource: dep2,
				},
			},
		}
		close(input)

		// Run writer
		err := writer.Run(ctx, input)
		g.Expect(err).ToNot(HaveOccurred())

		// Verify workload and dependencies were written
		g.Expect(writtenResources).To(HaveLen(3))
		g.Expect(writtenResources[0]).To(Equal(fmt.Sprintf("%s/%s", testNamespace, notebookName)))
		g.Expect(writtenResources[1]).To(Equal(testNamespace + "/dep-1"))
		g.Expect(writtenResources[2]).To(Equal(testNamespace + "/dep-2"))
	})

	t.Run("should handle write errors gracefully", func(t *testing.T) {
		io := iostreams.NewIOStreams(nil, nil, nil)
		var writeCalls int

		writeFunc := func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured) error {
			writeCalls++

			return errors.New("write error")
		}

		writer := &pipeline.WriterStage{
			WriteResource: writeFunc,
			IO:            io,
		}

		input := make(chan pipeline.WorkloadWithDeps, 1)

		workload := createTestWorkload()
		input <- pipeline.WorkloadWithDeps{
			GVR:          schema.GroupVersionResource{Group: "test", Version: "v1", Resource: "tests"},
			Instance:     workload,
			Dependencies: nil,
		}
		close(input)

		// Run writer
		err := writer.Run(ctx, input)
		g.Expect(err).ToNot(HaveOccurred())

		// Verify write was attempted
		g.Expect(writeCalls).To(Equal(1))
	})

	t.Run("should handle context cancellation", func(t *testing.T) {
		io := iostreams.NewIOStreams(nil, nil, nil)

		writeFunc := func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured) error {
			return nil
		}

		writer := &pipeline.WriterStage{
			WriteResource: writeFunc,
			IO:            io,
		}

		input := make(chan pipeline.WorkloadWithDeps)

		cancelCtx, cancel := context.WithCancel(ctx)
		cancel() // Cancel immediately

		err := writer.Run(cancelCtx, input)
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("writer cancelled"))
	})

	t.Run("should handle empty input", func(t *testing.T) {
		io := iostreams.NewIOStreams(nil, nil, nil)
		var writeCalls int

		writeFunc := func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured) error {
			writeCalls++

			return nil
		}

		writer := &pipeline.WriterStage{
			WriteResource: writeFunc,
			IO:            io,
		}

		input := make(chan pipeline.WorkloadWithDeps)
		close(input)

		// Run writer
		err := writer.Run(ctx, input)
		g.Expect(err).ToNot(HaveOccurred())

		// Verify no writes occurred
		g.Expect(writeCalls).To(Equal(0))
	})

	t.Run("should handle timeout", func(t *testing.T) {
		io := iostreams.NewIOStreams(nil, nil, nil)

		writeFunc := func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured) error {
			return nil
		}

		writer := &pipeline.WriterStage{
			WriteResource: writeFunc,
			IO:            io,
		}

		input := make(chan pipeline.WorkloadWithDeps)

		// Create context with very short timeout
		timeoutCtx, cancel := context.WithTimeout(ctx, 1*time.Nanosecond)
		defer cancel()

		// Wait for timeout
		time.Sleep(10 * time.Millisecond)

		err := writer.Run(timeoutCtx, input)
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("writer cancelled"))
	})

	t.Run("dry-run should group output per workload", func(t *testing.T) {
		bufOut := &bytes.Buffer{}
		bufErr := &bytes.Buffer{}
		io := iostreams.NewIOStreams(nil, bufOut, bufErr)

		// No actual writes in dry-run mode
		writeFunc := func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured) error {
			return nil
		}

		writer := &pipeline.WriterStage{
			WriteResource: writeFunc,
			IO:            io,
			DryRun:        true,
			OutputDir:     "/tmp/backup",
		}

		input := make(chan pipeline.WorkloadWithDeps, 1)

		// Create workload with dependencies
		workload := createTestWorkload()
		dep1 := &unstructured.Unstructured{}
		dep1.SetNamespace(testNamespace)
		dep1.SetName("config-1")
		dep2 := &unstructured.Unstructured{}
		dep2.SetNamespace(testNamespace)
		dep2.SetName("pvc-1")

		input <- pipeline.WorkloadWithDeps{
			GVR:      schema.GroupVersionResource{Group: "kubeflow.org", Version: "v1", Resource: "notebooks"},
			Instance: workload,
			Dependencies: []dependencies.Dependency{
				{
					GVR:      schema.GroupVersionResource{Version: "v1", Resource: "configmaps"},
					Resource: dep1,
				},
				{
					GVR:      schema.GroupVersionResource{Version: "v1", Resource: "persistentvolumeclaims"},
					Resource: dep2,
				},
			},
		}
		close(input)

		err := writer.Run(ctx, input)
		g.Expect(err).ToNot(HaveOccurred())

		output := bufErr.String()

		// Verify grouped output format
		g.Expect(output).To(ContainSubstring("Would create:"))
		g.Expect(output).To(ContainSubstring("- /tmp/backup/test-namespace/notebooks.kubeflow.org-test-notebook.yaml"))
		g.Expect(output).To(ContainSubstring("- /tmp/backup/test-namespace/configmaps-config-1.yaml"))
		g.Expect(output).To(ContainSubstring("- /tmp/backup/test-namespace/persistentvolumeclaims-pvc-1.yaml"))

		// Verify paths appear after "Would create:" (grouped output)
		wouldCreateIdx := containsIndex(output, "Would create:")
		path1Idx := containsIndex(output, "notebooks.kubeflow.org-test-notebook.yaml")
		path2Idx := containsIndex(output, "configmaps-config-1.yaml")
		g.Expect(path1Idx).To(BeNumerically(">", wouldCreateIdx))
		g.Expect(path2Idx).To(BeNumerically(">", wouldCreateIdx))
	})

	t.Run("dry-run should exclude failed dependencies", func(t *testing.T) {
		bufOut := &bytes.Buffer{}
		bufErr := &bytes.Buffer{}
		io := iostreams.NewIOStreams(nil, bufOut, bufErr)

		writeFunc := func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured) error {
			return nil
		}

		writer := &pipeline.WriterStage{
			WriteResource: writeFunc,
			IO:            io,
			DryRun:        true,
			OutputDir:     "/tmp/backup",
		}

		input := make(chan pipeline.WorkloadWithDeps, 1)

		workload := createTestWorkload()
		dep1 := &unstructured.Unstructured{}
		dep1.SetNamespace(testNamespace)
		dep1.SetName("config-1")

		input <- pipeline.WorkloadWithDeps{
			GVR:      schema.GroupVersionResource{Group: "kubeflow.org", Version: "v1", Resource: "notebooks"},
			Instance: workload,
			Dependencies: []dependencies.Dependency{
				{
					GVR:      schema.GroupVersionResource{Version: "v1", Resource: "configmaps"},
					Resource: dep1,
				},
				{
					GVR:      schema.GroupVersionResource{Version: "v1", Resource: "secrets"},
					Resource: nil,
					Error:    errors.New("unauthorized"),
				},
			},
		}
		close(input)

		err := writer.Run(ctx, input)
		g.Expect(err).ToNot(HaveOccurred())

		output := bufErr.String()

		// Verify workload and successful dependency appear
		g.Expect(output).To(ContainSubstring("notebooks.kubeflow.org-test-notebook.yaml"))
		g.Expect(output).To(ContainSubstring("configmaps-config-1.yaml"))

		// Verify failed dependency does NOT appear in "Would create:" list
		g.Expect(output).ToNot(ContainSubstring("secrets-"))
	})

	t.Run("dry-run stdout mode should use descriptive format", func(t *testing.T) {
		bufOut := &bytes.Buffer{}
		bufErr := &bytes.Buffer{}
		io := iostreams.NewIOStreams(nil, bufOut, bufErr)

		writeFunc := func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured) error {
			return nil
		}

		writer := &pipeline.WriterStage{
			WriteResource: writeFunc,
			IO:            io,
			DryRun:        true,
			OutputDir:     "", // Empty = stdout mode
		}

		input := make(chan pipeline.WorkloadWithDeps, 1)

		workload := createTestWorkload()
		dep1 := &unstructured.Unstructured{}
		dep1.SetNamespace(testNamespace)
		dep1.SetName("config-1")

		input <- pipeline.WorkloadWithDeps{
			GVR:      schema.GroupVersionResource{Group: "kubeflow.org", Version: "v1", Resource: "notebooks"},
			Instance: workload,
			Dependencies: []dependencies.Dependency{
				{
					GVR:      schema.GroupVersionResource{Version: "v1", Resource: "configmaps"},
					Resource: dep1,
				},
			},
		}
		close(input)

		err := writer.Run(ctx, input)
		g.Expect(err).ToNot(HaveOccurred())

		output := bufErr.String()

		// Verify descriptive format (namespace/name (resource)) instead of file paths
		g.Expect(output).To(ContainSubstring("test-namespace/test-notebook (notebooks)"))
		g.Expect(output).To(ContainSubstring("test-namespace/config-1 (configmaps)"))

		// Verify no file paths
		g.Expect(output).ToNot(ContainSubstring(".yaml"))
	})

	t.Run("dry-run should handle cluster-scoped resources", func(t *testing.T) {
		bufOut := &bytes.Buffer{}
		bufErr := &bytes.Buffer{}
		io := iostreams.NewIOStreams(nil, bufOut, bufErr)

		writeFunc := func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured) error {
			return nil
		}

		writer := &pipeline.WriterStage{
			WriteResource: writeFunc,
			IO:            io,
			DryRun:        true,
			OutputDir:     "/tmp/backup",
		}

		input := make(chan pipeline.WorkloadWithDeps, 1)

		// Cluster-scoped resource (no namespace)
		workload := &unstructured.Unstructured{}
		workload.SetName("my-node")

		input <- pipeline.WorkloadWithDeps{
			GVR:          schema.GroupVersionResource{Version: "v1", Resource: "nodes"},
			Instance:     workload,
			Dependencies: nil,
		}
		close(input)

		err := writer.Run(ctx, input)
		g.Expect(err).ToNot(HaveOccurred())

		output := bufErr.String()

		// Verify cluster-scoped directory used
		g.Expect(output).To(ContainSubstring("/tmp/backup/cluster-scoped/nodes-my-node.yaml"))
	})

	t.Run("normal mode should remain unchanged", func(t *testing.T) {
		bufOut := &bytes.Buffer{}
		bufErr := &bytes.Buffer{}
		io := iostreams.NewIOStreams(nil, bufOut, bufErr)

		var writeCalls int
		writeFunc := func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured) error {
			writeCalls++

			return nil
		}

		writer := &pipeline.WriterStage{
			WriteResource: writeFunc,
			IO:            io,
			DryRun:        false, // Normal mode
		}

		input := make(chan pipeline.WorkloadWithDeps, 1)

		workload := createTestWorkload()
		dep1 := &unstructured.Unstructured{}
		dep1.SetNamespace(testNamespace)
		dep1.SetName("config-1")

		input <- pipeline.WorkloadWithDeps{
			GVR:      schema.GroupVersionResource{Group: "kubeflow.org", Version: "v1", Resource: "notebooks"},
			Instance: workload,
			Dependencies: []dependencies.Dependency{
				{
					GVR:      schema.GroupVersionResource{Version: "v1", Resource: "configmaps"},
					Resource: dep1,
				},
			},
		}
		close(input)

		err := writer.Run(ctx, input)
		g.Expect(err).ToNot(HaveOccurred())

		// Verify WriteResource was called for workload and dependency
		g.Expect(writeCalls).To(Equal(2))

		output := bufErr.String()

		// Verify no "Would create:" output in normal mode
		g.Expect(output).ToNot(ContainSubstring("Would create:"))
	})
}

// Helper function to find the index of a substring.
func containsIndex(s string, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}

	return -1
}
