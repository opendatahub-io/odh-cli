package pipeline

import (
	"context"
	"fmt"
	"strings"

	"github.com/opendatahub-io/odh-cli/pkg/backup/dependencies"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
	"github.com/opendatahub-io/odh-cli/pkg/util/iostreams"
	"golang.org/x/sync/errgroup"
)

// ResolverStage resolves dependencies for workload instances.
type ResolverStage struct {
	Client      client.Client
	DepRegistry *dependencies.Registry
	Verbose     bool
	IO          iostreams.Interface
}

// Run launches N workers to resolve dependencies.
func (r *ResolverStage) Run(
	ctx context.Context,
	numWorkers int,
	input <-chan WorkloadItem,
	output chan<- WorkloadWithDeps,
) error {
	g, ctx := errgroup.WithContext(ctx)

	for range numWorkers {
		g.Go(func() error {
			return r.worker(ctx, input, output)
		})
	}

	if err := g.Wait(); err != nil {
		return fmt.Errorf("resolver workers failed: %w", err)
	}

	return nil
}

// worker processes workload items from input channel.
func (r *ResolverStage) worker(
	ctx context.Context,
	input <-chan WorkloadItem,
	output chan<- WorkloadWithDeps,
) error {
	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("resolver worker cancelled: %w", ctx.Err())
		case item, ok := <-input:
			if !ok {
				return nil
			}

			result, err := r.resolveWorkload(ctx, item)
			if err != nil {
				r.IO.Errorf("    Warning: Failed to resolve %s/%s: %v",
					item.Instance.GetNamespace(), item.Instance.GetName(), err)

				continue
			}

			select {
			case <-ctx.Done():
				return fmt.Errorf("resolver worker cancelled: %w", ctx.Err())
			case output <- result:
			}
		}
	}
}

// resolveWorkload resolves dependencies for a single workload.
func (r *ResolverStage) resolveWorkload(
	ctx context.Context,
	item WorkloadItem,
) (WorkloadWithDeps, error) {
	resolver, err := r.DepRegistry.GetResolver(item.GVR)
	if err != nil {
		// No resolver - return workload with empty dependencies
		return WorkloadWithDeps{
			GVR:          item.GVR,
			Instance:     item.Instance,
			Dependencies: nil,
		}, nil
	}

	deps, err := resolver.Resolve(ctx, r.Client, item.Instance)
	if err != nil {
		return WorkloadWithDeps{}, fmt.Errorf("resolving dependencies: %w", err)
	}

	if r.Verbose && len(deps) > 0 {
		r.IO.Errorf("Resolving %s/%s...",
			item.Instance.GetNamespace(), item.Instance.GetName())
		r.logDependencies(deps)
	}

	return WorkloadWithDeps{
		GVR:          item.GVR,
		Instance:     item.Instance,
		Dependencies: deps,
	}, nil
}

// logDependencies logs each dependency with type and name.
func (r *ResolverStage) logDependencies(deps []dependencies.Dependency) {
	for i := range deps {
		resourceType := r.formatResourceType(deps[i].GVR.Resource)

		if deps[i].Error != nil {
			// Failed dependency - show with X and reason
			reason := r.formatErrorReason(deps[i].Error)
			r.IO.Errorf("  X %s: %s (%s)", resourceType, deps[i].Name, reason)
		} else {
			// Successful dependency - show with →
			r.IO.Errorf("  → %s: %s", resourceType, deps[i].Name)
		}
	}
}

// formatResourceType converts plural resource name to singular display name.
func (r *ResolverStage) formatResourceType(resource string) string {
	switch resource {
	case "configmaps":
		return "ConfigMap"
	case "secrets":
		return "Secret"
	case "persistentvolumeclaims":
		return "PVC"
	default:
		// Fallback: capitalize first letter and remove trailing 's'
		if len(resource) > 0 {
			singular := strings.TrimSuffix(resource, "s")

			return strings.ToUpper(singular[:1]) + singular[1:]
		}

		return resource
	}
}

// formatErrorReason converts Kubernetes API errors to short user-friendly reasons.
func (r *ResolverStage) formatErrorReason(err error) string {
	errMsg := err.Error()

	// Check for common Kubernetes API error patterns
	switch {
	case strings.Contains(errMsg, "forbidden"):
		return "unauthorized"
	case strings.Contains(errMsg, "not found"):
		return "not found"
	case strings.Contains(errMsg, "timeout"):
		return "timeout"
	case strings.Contains(errMsg, "connection refused"):
		return "connection error"
	default:
		return "error"
	}
}
