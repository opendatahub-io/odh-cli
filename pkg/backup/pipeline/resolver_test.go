package pipeline_test

import (
	"context"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/opendatahub-io/odh-cli/pkg/backup/dependencies"
	"github.com/opendatahub-io/odh-cli/pkg/backup/pipeline"
	"github.com/opendatahub-io/odh-cli/pkg/util/iostreams"

	. "github.com/onsi/gomega"
)

func TestResolverStage(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	t.Run("should process workloads without resolver", func(t *testing.T) {
		io := iostreams.NewIOStreams(nil, nil, nil)
		registry := dependencies.NewRegistry()

		resolver := &pipeline.ResolverStage{
			Client:      nil,
			DepRegistry: registry,
			Verbose:     false,
			IO:          io,
		}

		input := make(chan pipeline.WorkloadItem, 1)
		output := make(chan pipeline.WorkloadWithDeps, 1)

		// Create test workload
		workload := createTestWorkload()
		input <- pipeline.WorkloadItem{
			GVR:      schema.GroupVersionResource{Group: "test", Version: "v1", Resource: "tests"},
			Instance: workload,
		}
		close(input)

		// Run resolver
		go func() {
			err := resolver.Run(ctx, 1, input, output)
			g.Expect(err).ToNot(HaveOccurred())
			close(output)
		}()

		// Collect results
		var results []pipeline.WorkloadWithDeps
		for result := range output {
			results = append(results, result)
		}

		// Verify
		g.Expect(results).To(HaveLen(1))
		g.Expect(results[0].Instance).To(Equal(workload))
		g.Expect(results[0].Dependencies).To(BeNil())
	})

	t.Run("should handle context cancellation", func(t *testing.T) {
		io := iostreams.NewIOStreams(nil, nil, nil)
		registry := dependencies.NewRegistry()

		resolver := &pipeline.ResolverStage{
			Client:      nil,
			DepRegistry: registry,
			Verbose:     false,
			IO:          io,
		}

		input := make(chan pipeline.WorkloadItem)
		output := make(chan pipeline.WorkloadWithDeps)

		cancelCtx, cancel := context.WithCancel(ctx)
		cancel() // Cancel immediately

		err := resolver.Run(cancelCtx, 1, input, output)
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("resolver"))
	})

	t.Run("should process multiple workloads with multiple workers", func(t *testing.T) {
		io := iostreams.NewIOStreams(nil, nil, nil)
		registry := dependencies.NewRegistry()

		resolver := &pipeline.ResolverStage{
			Client:      nil,
			DepRegistry: registry,
			Verbose:     false,
			IO:          io,
		}

		input := make(chan pipeline.WorkloadItem, 3)
		output := make(chan pipeline.WorkloadWithDeps, 3)

		// Create test workloads
		gvr := schema.GroupVersionResource{Group: "test", Version: "v1", Resource: "tests"}
		for range 3 {
			workload := createTestWorkload()
			input <- pipeline.WorkloadItem{GVR: gvr, Instance: workload}
		}
		close(input)

		// Run resolver with 2 workers
		go func() {
			err := resolver.Run(ctx, 2, input, output)
			g.Expect(err).ToNot(HaveOccurred())
			close(output)
		}()

		// Collect results
		var results []pipeline.WorkloadWithDeps
		for result := range output {
			results = append(results, result)
		}

		// Verify
		g.Expect(results).To(HaveLen(3))
	})

	t.Run("should handle worker timeout", func(t *testing.T) {
		io := iostreams.NewIOStreams(nil, nil, nil)
		registry := dependencies.NewRegistry()

		resolver := &pipeline.ResolverStage{
			Client:      nil,
			DepRegistry: registry,
			Verbose:     false,
			IO:          io,
		}

		input := make(chan pipeline.WorkloadItem)
		output := make(chan pipeline.WorkloadWithDeps)

		// Create context with very short timeout
		timeoutCtx, cancel := context.WithTimeout(ctx, 1*time.Nanosecond)
		defer cancel()

		// Wait for timeout
		time.Sleep(10 * time.Millisecond)

		err := resolver.Run(timeoutCtx, 1, input, output)
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("resolver"))
	})
}
