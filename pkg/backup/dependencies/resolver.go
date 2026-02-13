package dependencies

import (
	"context"

	"github.com/opendatahub-io/odh-cli/pkg/util/client"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// Dependency represents a discovered dependency resource.
type Dependency struct {
	GVR      schema.GroupVersionResource
	Resource *unstructured.Unstructured
	Name     string // Resource name (populated even if Resource is nil)
	Error    error  // Non-nil if resource couldn't be fetched
}

// Resolver finds dependencies for a specific workload type.
type Resolver interface {
	// Resolve finds all dependencies for the given workload
	Resolve(
		ctx context.Context,
		c client.Reader,
		obj *unstructured.Unstructured,
	) ([]Dependency, error)

	// CanHandle returns true if this resolver can handle the given GVR
	CanHandle(gvr schema.GroupVersionResource) bool
}
