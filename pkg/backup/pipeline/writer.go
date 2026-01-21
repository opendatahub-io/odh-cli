package pipeline

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/lburgazzoli/odh-cli/pkg/util/iostreams"
)

// WriteResourceFunc is a function that writes a resource.
type WriteResourceFunc func(gvr schema.GroupVersionResource, obj *unstructured.Unstructured) error

// WriterStage writes workloads and dependencies to disk/stdout.
type WriterStage struct {
	WriteResource WriteResourceFunc
	IO            iostreams.Interface
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
	// Write workload first
	if err := w.WriteResource(item.GVR, item.Instance); err != nil {
		return fmt.Errorf("writing workload: %w", err)
	}

	// Write dependencies
	for _, dep := range item.Dependencies {
		if err := w.WriteResource(dep.GVR, dep.Resource); err != nil {
			w.IO.Errorf("  Warning: Failed to write dependency %s/%s: %v",
				dep.Resource.GetNamespace(), dep.Resource.GetName(), err)
		}
	}

	return nil
}
