package check

import (
	"fmt"
	"io"
	"sort"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
)

// VerboseOutputFormatter is optionally implemented by checks that provide
// custom formatting for impacted objects in verbose table output.
// When a check implements this interface, its FormatVerboseOutput method
// is used instead of the default namespace-grouped rendering.
type VerboseOutputFormatter interface {
	FormatVerboseOutput(out io.Writer, dr *result.DiagnosticResult)
}

// DefaultVerboseFormatter provides the standard namespace-grouped rendering
// used when a check does not implement VerboseOutputFormatter.
type DefaultVerboseFormatter struct {
	// NamespaceRequesters maps namespace names to their openshift.io/requester annotation value.
	NamespaceRequesters map[string]string
}

// FormatVerboseOutput renders impacted objects grouped by namespace.
// Cluster-scoped objects (empty namespace) are listed first without a header.
// Namespaced objects are grouped under their namespace with an optional requester annotation.
func (f *DefaultVerboseFormatter) FormatVerboseOutput(out io.Writer, dr *result.DiagnosticResult) {
	nsGroups := groupByNamespace(dr.ImpactedObjects)

	for _, nsg := range nsGroups {
		if nsg.namespace == "" {
			for _, obj := range nsg.objects {
				_, _ = fmt.Fprintf(out, "    - %s\n", formatImpactedObject(obj))
			}
		} else {
			nsHeader := nsg.namespace
			if f.NamespaceRequesters != nil {
				if requester, ok := f.NamespaceRequesters[nsg.namespace]; ok && requester != "" {
					nsHeader = fmt.Sprintf("%s (requester: %s)", nsg.namespace, requester)
				}
			}

			_, _ = fmt.Fprintf(out, "    %s:\n", nsHeader)

			for _, obj := range nsg.objects {
				_, _ = fmt.Fprintf(out, "      - %s\n", formatImpactedObject(obj))
			}
		}
	}
}

// namespaceGroup holds objects within a single namespace for display.
type namespaceGroup struct {
	namespace string
	objects   []metav1.PartialObjectMetadata
}

// qualifiedObject holds a CRD-qualified object reference with optional context.
type qualifiedObject struct {
	name    string
	crdFQN  string
	context string
}

// formatImpactedObject returns the display string for an impacted object.
// Includes the Kind from TypeMeta when available to help identify the resource type.
func formatImpactedObject(obj metav1.PartialObjectMetadata) string {
	if obj.Kind != "" {
		return fmt.Sprintf("%s (%s)", obj.Name, obj.Kind)
	}

	return obj.Name
}

// groupByNamespace sub-groups objects by namespace, sorted alphabetically.
// Cluster-scoped objects (empty namespace) are placed first.
func groupByNamespace(objects []metav1.PartialObjectMetadata) []namespaceGroup {
	nsMap := make(map[string][]metav1.PartialObjectMetadata)

	for _, obj := range objects {
		nsMap[obj.Namespace] = append(nsMap[obj.Namespace], obj)
	}

	namespaces := make([]string, 0, len(nsMap))
	for ns := range nsMap {
		namespaces = append(namespaces, ns)
	}

	sort.Strings(namespaces)

	groups := make([]namespaceGroup, 0, len(namespaces))
	for _, ns := range namespaces {
		groups = append(groups, namespaceGroup{
			namespace: ns,
			objects:   nsMap[ns],
		})
	}

	return groups
}

// EnhancedVerboseFormatter provides namespace-grouped rendering with CRD-qualified
// resource names and requester info. Embed this in any check struct to get a
// FormatVerboseOutput that lists objects grouped by namespace.
//
// Output format:
//
//	namespace: <ns> | requester: <email>
//	  - <resource>.<group>/<name>
//	  - <resource>.<group>/<name>
//
// The CRD FQN is derived per object from its TypeMeta, so results with mixed resource
// types (e.g. Notebook and RayCluster) render each object with its correct CRD prefix.
// When all objects share the same type, the AnnotationResourceCRDName annotation is
// preferred for an accurate plural form.
//
// Checks that need custom formatting (e.g. grouping by image or status)
// should define their own FormatVerboseOutput method instead.
type EnhancedVerboseFormatter struct {
	// NamespaceRequesters maps namespace names to their openshift.io/requester annotation value.
	// Populated automatically by the output renderer before FormatVerboseOutput is called.
	NamespaceRequesters map[string]string
}

// SetNamespaceRequesters sets the namespace-to-requester mapping.
// Called by the output renderer before FormatVerboseOutput.
func (f *EnhancedVerboseFormatter) SetNamespaceRequesters(m map[string]string) {
	f.NamespaceRequesters = m
}

// FormatVerboseOutput implements VerboseOutputFormatter.
// Renders impacted objects grouped by namespace with requester info and CRD FQN.
// Each object's CRD FQN is derived from its own TypeMeta, so mixed-kind results
// render correctly (e.g. notebooks.kubeflow.org/nb-1 alongside rayclusters.ray.io/rc-1).
func (f *EnhancedVerboseFormatter) FormatVerboseOutput(out io.Writer, dr *result.DiagnosticResult) {
	// Build a per-object qualified name. For single-kind results, the annotation
	// provides the most accurate CRD FQN. For mixed-kind results, each object's
	// TypeMeta is used to derive its own prefix.
	crdFQNByKind := buildCRDFQNByKind(dr)

	// Group objects by namespace, preserving per-object kind and optional context.
	nsMap := make(map[string][]qualifiedObject)
	for _, obj := range dr.ImpactedObjects {
		fqn := crdFQNByKind[obj.Kind]

		var ctx string
		if obj.Annotations != nil {
			ctx = obj.Annotations[result.AnnotationObjectContext]
		}

		nsMap[obj.Namespace] = append(nsMap[obj.Namespace], qualifiedObject{
			name:    obj.Name,
			crdFQN:  fqn,
			context: ctx,
		})
	}

	namespaces := make([]string, 0, len(nsMap))
	for ns := range nsMap {
		namespaces = append(namespaces, ns)
	}
	sort.Strings(namespaces)

	for idx, ns := range namespaces {
		objects := nsMap[ns]
		sort.Slice(objects, func(i, j int) bool {
			if objects[i].crdFQN != objects[j].crdFQN {
				return objects[i].crdFQN < objects[j].crdFQN
			}

			return objects[i].name < objects[j].name
		})

		if ns == "" {
			// Cluster-scoped objects listed without namespace header.
			writeQualifiedObjects(out, objects, "      ")
		} else {
			nsHeader := f.namespaceHeader(ns)

			_, _ = fmt.Fprintf(out, "      %s\n", nsHeader)
			writeQualifiedObjects(out, objects, "        ")
		}

		// Blank line between namespace groups (except after last).
		if idx < len(namespaces)-1 {
			_, _ = fmt.Fprintln(out)
		}
	}
}

// namespaceHeader returns the formatted namespace header, including requester info if available.
func (f *EnhancedVerboseFormatter) namespaceHeader(ns string) string {
	header := "namespace: " + ns
	if f.NamespaceRequesters != nil {
		if requester, ok := f.NamespaceRequesters[ns]; ok && requester != "" {
			header = fmt.Sprintf("namespace: %s | requester: %s", ns, requester)
		}
	}

	return header
}

// writeQualifiedObjects writes a list of qualified objects with the given indent prefix.
// Objects with a non-empty context annotation get a sub-bullet on the following line.
func writeQualifiedObjects(out io.Writer, objects []qualifiedObject, indent string) {
	for _, obj := range objects {
		_, _ = fmt.Fprintf(out, "%s- %s/%s\n", indent, obj.crdFQN, obj.name)
		if obj.context != "" {
			_, _ = fmt.Fprintf(out, "%s  — (%s)\n", indent, obj.context)
		}
	}
}

// buildCRDFQNByKind returns a map from Kind to CRD FQN for the impacted objects.
// Resolution priority per kind:
//  1. Per-object AnnotationObjectCRDName (authoritative, set from ResourceType.CRDFQN())
//  2. Result-level AnnotationResourceCRDName (single-kind results only)
//  3. Naive derivation from TypeMeta (fallback)
func buildCRDFQNByKind(dr *result.DiagnosticResult) map[string]string {
	fqnByKind := make(map[string]string)

	for _, obj := range dr.ImpactedObjects {
		if _, ok := fqnByKind[obj.Kind]; ok {
			continue
		}

		// Prefer per-object annotation set from ResourceType.CRDFQN().
		if name, ok := obj.Annotations[result.AnnotationObjectCRDName]; ok && name != "" {
			fqnByKind[obj.Kind] = name

			continue
		}

		fqnByKind[obj.Kind] = DeriveCRDFQNFromTypeMeta([]metav1.PartialObjectMetadata{obj})
	}

	// When there is exactly one kind and the result-level annotation is set,
	// prefer it over naive derivation (backward compatibility for callers
	// that use SetImpactedObjects/AddImpactedObjects).
	if len(fqnByKind) == 1 {
		if name, ok := dr.Annotations[result.AnnotationResourceCRDName]; ok && name != "" {
			for kind := range fqnByKind {
				fqnByKind[kind] = name
			}
		}
	}

	return fqnByKind
}

// CRDFullyQualifiedName returns the CRD fully-qualified name for the impacted objects
// in a DiagnosticResult. It first checks the AnnotationResourceCRDName annotation
// (automatically set by SetImpactedObjects and AddImpactedObjects from ResourceType.CRDFQN()).
// Falls back to deriving the FQN from the first impacted object's TypeMeta if the
// annotation is absent.
func CRDFullyQualifiedName(dr *result.DiagnosticResult) string {
	// Prefer the annotation set by SetImpactedObjects — uses ResourceType.Resource
	// for the correct plural form (e.g. "inferenceservices", not a naive pluralization).
	if name, ok := dr.Annotations[result.AnnotationResourceCRDName]; ok && name != "" {
		return name
	}

	// Fallback: derive from TypeMeta (for callers that set ImpactedObjects directly).
	return DeriveCRDFQNFromTypeMeta(dr.ImpactedObjects)
}

// DeriveCRDFQNFromTypeMeta derives the CRD FQN from the first impacted object's TypeMeta.
// For example, APIVersion "kubeflow.org/v1" with Kind "Notebook" produces "notebooks.kubeflow.org".
// Falls back to the lowercase Kind if TypeMeta is not populated.
func DeriveCRDFQNFromTypeMeta(objects []metav1.PartialObjectMetadata) string {
	if len(objects) == 0 {
		return ""
	}

	obj := objects[0]

	kind := obj.Kind
	if kind == "" {
		return ""
	}

	// Naive plural as fallback — callers should set AnnotationResourceCRDName
	// via ResourceType.CRDFQN() for authoritative plural forms.
	plural := strings.ToLower(kind) + "s"

	// Extract API group from APIVersion (e.g. "kubeflow.org/v1" → "kubeflow.org").
	// Core resources have no group (APIVersion is just "v1"), so return plural only.
	apiVersion := obj.APIVersion
	if apiVersion == "" {
		return plural
	}

	group, _, found := strings.Cut(apiVersion, "/")
	if !found {
		// Core API (e.g. "v1") — no group prefix.
		return plural
	}

	return plural + "." + group
}
