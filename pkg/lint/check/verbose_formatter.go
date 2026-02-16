package check

import (
	"fmt"
	"io"
	"sort"

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
