package check

import (
	"io"
	"sync"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ImpactedObjectRenderer is a function that renders an impacted object for verbose table output.
// The function receives the object metadata and returns the formatted display string.
type ImpactedObjectRenderer func(obj metav1.PartialObjectMetadata) string

// ImpactedGroupRenderer is a function that renders all impacted objects for a group/kind.
// This provides full control over formatting, allowing custom grouping, sorting, and structure.
// When registered, it replaces the default per-object rendering entirely for that group/kind.
// The maxDisplay parameter indicates the suggested maximum number of items to display.
type ImpactedGroupRenderer func(out io.Writer, objects []metav1.PartialObjectMetadata, maxDisplay int)

//nolint:gochecknoglobals
var (
	rendererMu              sync.RWMutex
	impactedObjectRenderers = make(map[rendererKey]ImpactedObjectRenderer)
	impactedGroupRenderers  = make(map[rendererKey]ImpactedGroupRenderer)
)

// rendererKey identifies a renderer by check group, kind, and check type.
type rendererKey struct {
	group     CheckGroup
	kind      string
	checkType CheckType
}

// RegisterImpactedObjectRenderer registers a custom per-object renderer for a specific check.
// When --verbose is used, objects from checks matching this group/kind/checkType will use the custom renderer.
// Note: If a group renderer is also registered, the group renderer takes precedence.
func RegisterImpactedObjectRenderer(group CheckGroup, kind string, checkType CheckType, renderer ImpactedObjectRenderer) {
	rendererMu.Lock()
	defer rendererMu.Unlock()

	impactedObjectRenderers[rendererKey{group: group, kind: kind, checkType: checkType}] = renderer
}

// GetImpactedObjectRenderer returns the per-object renderer for the given check.
// If no custom renderer is registered, returns the default renderer that formats as "namespace/name (Kind)".
func GetImpactedObjectRenderer(group CheckGroup, kind string, checkType CheckType) ImpactedObjectRenderer {
	rendererMu.RLock()
	defer rendererMu.RUnlock()

	key := rendererKey{group: group, kind: kind, checkType: checkType}
	if renderer, ok := impactedObjectRenderers[key]; ok {
		return renderer
	}

	return defaultObjectRenderer
}

// defaultObjectRenderer formats an object as "namespace/name (Kind)".
func defaultObjectRenderer(obj metav1.PartialObjectMetadata) string {
	name := obj.Name
	if obj.Namespace != "" {
		name = obj.Namespace + "/" + name
	}

	return name + " (" + obj.Kind + ")"
}

// RegisterImpactedGroupRenderer registers a custom group renderer for a specific check.
// Group renderers have full control over how objects are displayed, including grouping and structure.
// When registered, the group renderer takes precedence over any per-object renderer.
func RegisterImpactedGroupRenderer(group CheckGroup, kind string, checkType CheckType, renderer ImpactedGroupRenderer) {
	rendererMu.Lock()
	defer rendererMu.Unlock()

	impactedGroupRenderers[rendererKey{group: group, kind: kind, checkType: checkType}] = renderer
}

// GetImpactedGroupRenderer returns the custom group renderer for the given check, or nil if none.
func GetImpactedGroupRenderer(group CheckGroup, kind string, checkType CheckType) ImpactedGroupRenderer {
	rendererMu.RLock()
	defer rendererMu.RUnlock()

	return impactedGroupRenderers[rendererKey{group: group, kind: kind, checkType: checkType}]
}
