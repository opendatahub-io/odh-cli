package pipeline

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/opendatahub-io/odh-cli/pkg/util/iostreams"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// WriteResourceFunc is a function that writes a resource.
type WriteResourceFunc func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured) error

// WriterStage writes workloads and dependencies to disk/stdout.
type WriterStage struct {
	WriteResource WriteResourceFunc
	IO            iostreams.Interface
	DryRun        bool   // Enable dry-run mode with grouped output
	OutputDir     string // Output directory for path generation (empty = stdout)
}

// Run reads from input channel and writes sequentially.
func (w *WriterStage) Run(
	ctx context.Context,
	input <-chan WorkloadWithDeps,
) error {
	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("writer cancelled: %w", ctx.Err())
		case item, ok := <-input:
			if !ok {
				return nil
			}

			if err := w.writeWorkloadWithDeps(item); err != nil {
				w.IO.Errorf("    Warning: Failed to write %s/%s: %v",
					item.Instance.GetNamespace(), item.Instance.GetName(), err)
			}
		}
	}
}

// writeWorkloadWithDeps writes workload and dependencies.
func (w *WriterStage) writeWorkloadWithDeps(item WorkloadWithDeps) error {
	// In dry-run mode, collect all paths before logging
	if w.DryRun {
		return w.writeWorkloadWithDepsDryRun(item)
	}

	// Normal mode: Write immediately (unchanged behavior)
	// Write workload first
	if err := w.WriteResource(item.GVR, item.Instance); err != nil {
		return fmt.Errorf("writing workload: %w", err)
	}

	// Write dependencies (skip ones with errors - they weren't fetched)
	for _, dep := range item.Dependencies {
		if dep.Error != nil {
			// Skip - this dependency couldn't be fetched
			continue
		}

		if err := w.WriteResource(dep.GVR, dep.Resource); err != nil {
			w.IO.Errorf("  Warning: Failed to write dependency %s/%s: %v",
				dep.Resource.GetNamespace(), dep.Resource.GetName(), err)
		}
	}

	return nil
}

// writeWorkloadWithDepsDryRun handles dry-run mode with grouped output.
func (w *WriterStage) writeWorkloadWithDepsDryRun(item WorkloadWithDeps) error {
	// Pre-allocate paths slice (workload + dependencies)
	paths := make([]string, 0, 1+len(item.Dependencies))

	// Collect workload path
	workloadPath := w.getResourcePath(item.GVR, item.Instance)
	paths = append(paths, workloadPath)

	// Collect dependency paths (skip ones with errors)
	for _, dep := range item.Dependencies {
		if dep.Error != nil {
			continue // Skip - already logged by resolver with X marker
		}

		depPath := w.getResourcePath(dep.GVR, dep.Resource)
		paths = append(paths, depPath)
	}

	// Log grouped output with proper indentation
	w.IO.Errorf("")                  // Empty line before "Would create:"
	w.IO.Errorf("    Would create:") // 4 spaces indentation
	for _, path := range paths {
		w.IO.Errorf("    - %s", path) // 4 spaces + dash + space
	}

	return nil
}

// getResourcePath generates the file path or description for a resource.
func (w *WriterStage) getResourcePath(
	gvr schema.GroupVersionResource,
	obj *unstructured.Unstructured,
) string {
	namespace := obj.GetNamespace()
	if namespace == "" {
		namespace = "cluster-scoped"
	}
	name := obj.GetName()

	if w.OutputDir == "" {
		// Stdout mode: Return descriptive string
		return fmt.Sprintf("%s/%s (%s)", namespace, name, gvr.Resource)
	}

	// File mode: Generate full path
	gvrStr := gvr.Resource
	if gvr.Group != "" {
		gvrStr = gvr.Resource + "." + gvr.Group
	}

	filename := fmt.Sprintf("%s-%s.yaml", gvrStr, name)

	return filepath.Join(w.OutputDir, namespace, filename)
}
