package dependencies

import (
	"errors"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

// Registry holds all registered dependency resolvers.
type Registry struct {
	resolvers []Resolver
}

// NewRegistry creates a new resolver registry, optionally registering the provided resolvers.
// Returns an error if any resolver is nil.
func NewRegistry(resolvers ...Resolver) (*Registry, error) {
	r := &Registry{
		resolvers: make([]Resolver, 0, len(resolvers)),
	}

	if err := r.Register(resolvers...); err != nil {
		return nil, err
	}

	return r, nil
}

// Register adds one or more resolvers to the registry.
// The operation is atomic: either all resolvers are registered or none are.
// Returns an error if any resolver is nil.
func (r *Registry) Register(resolvers ...Resolver) error {
	for _, resolver := range resolvers {
		if resolver == nil {
			return errors.New("cannot register nil resolver")
		}
	}

	r.resolvers = append(r.resolvers, resolvers...)

	return nil
}

// GetResolver finds the appropriate resolver for the given GVR.
func (r *Registry) GetResolver(gvr schema.GroupVersionResource) (Resolver, error) {
	for _, resolver := range r.resolvers {
		if resolver.CanHandle(gvr) {
			return resolver, nil
		}
	}

	return nil, fmt.Errorf("no dependency resolver registered for %s", gvr.String())
}
