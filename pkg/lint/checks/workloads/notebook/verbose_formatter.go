package notebook

import (
	"fmt"
	"io"
	"sort"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
)

// NotebookVerboseFormatter provides a namespace-grouped rendering for impacted objects
// in verbose table output. Embed this in any check struct to get a default
// FormatVerboseOutput that lists objects grouped by namespace with requester info
// and CRD-qualified resource names.
//
// Output format:
//
//	namespace: <ns> | requester: <email>
//	  - <resource>.<group>/<name>
//	  - <resource>.<group>/<name>
//
// The CRD FQN (e.g. "notebooks.kubeflow.org") is read from the DiagnosticResult's
// AnnotationResourceCRDName annotation, which is automatically set by SetImpactedObjects
// and AddImpactedObjects. This makes the formatter adoptable by any resource type
// without hardcoding.
//
// Checks that need custom formatting (e.g. ImpactedWorkloadsCheck groups by image)
// should define their own FormatVerboseOutput method instead.
type NotebookVerboseFormatter struct {
	// NamespaceRequesters maps namespace names to their openshift.io/requester annotation value.
	// Populated automatically by the output renderer before FormatVerboseOutput is called.
	NamespaceRequesters map[string]string
}

// SetNamespaceRequesters sets the namespace-to-requester mapping.
// Called by the output renderer before FormatVerboseOutput.
func (f *NotebookVerboseFormatter) SetNamespaceRequesters(m map[string]string) {
	f.NamespaceRequesters = m
}

// FormatVerboseOutput implements check.VerboseOutputFormatter.
// Renders impacted objects grouped by namespace with requester info and CRD FQN.
func (f *NotebookVerboseFormatter) FormatVerboseOutput(out io.Writer, dr *result.DiagnosticResult) {
	crdName := crdFQN(dr)

	nsMap := make(map[string][]string)
	for _, obj := range dr.ImpactedObjects {
		nsMap[obj.Namespace] = append(nsMap[obj.Namespace], obj.Name)
	}

	namespaces := make([]string, 0, len(nsMap))
	for ns := range nsMap {
		namespaces = append(namespaces, ns)
	}
	sort.Strings(namespaces)

	for idx, ns := range namespaces {
		names := make([]string, len(nsMap[ns]))
		copy(names, nsMap[ns])
		sort.Strings(names)

		if ns == "" {
			// Cluster-scoped objects listed without namespace header.
			for _, name := range names {
				_, _ = fmt.Fprintf(out, "      - %s/%s\n", crdName, name)
			}
		} else {
			nsHeader := "namespace: " + ns
			if f.NamespaceRequesters != nil {
				if requester, ok := f.NamespaceRequesters[ns]; ok && requester != "" {
					nsHeader = fmt.Sprintf("namespace: %s | requester: %s", ns, requester)
				}
			}

			_, _ = fmt.Fprintf(out, "      %s\n", nsHeader)
			for _, name := range names {
				_, _ = fmt.Fprintf(out, "        - %s/%s\n", crdName, name)
			}
		}

		// Blank line between namespace groups (except after last).
		if idx < len(namespaces)-1 {
			_, _ = fmt.Fprintln(out)
		}
	}
}

// crdFQN returns the CRD fully-qualified name for the impacted objects in a DiagnosticResult.
// It first checks the AnnotationResourceCRDName annotation (automatically set by SetImpactedObjects
// and AddImpactedObjects from ResourceType.CRDFQN()). Falls back to deriving the FQN from the
// first impacted object's TypeMeta if the annotation is absent.
func crdFQN(dr *result.DiagnosticResult) string {
	// Prefer the annotation set by SetImpactedObjects — uses ResourceType.Resource
	// for the correct plural form (e.g. "inferenceservices", not "inferenceservices" from naive pluralization).
	if name, ok := dr.Annotations[result.AnnotationResourceCRDName]; ok && name != "" {
		return name
	}

	// Fallback: derive from TypeMeta (for callers that set ImpactedObjects directly).
	return deriveCRDFQNFromTypeMeta(dr.ImpactedObjects)
}

// deriveCRDFQNFromTypeMeta derives the CRD FQN from the first impacted object's TypeMeta.
// For example, APIVersion "kubeflow.org/v1" with Kind "Notebook" produces "notebooks.kubeflow.org".
// Falls back to the lowercase Kind if TypeMeta is not populated.
func deriveCRDFQNFromTypeMeta(objects []metav1.PartialObjectMetadata) string {
	if len(objects) == 0 {
		return ""
	}

	obj := objects[0]

	kind := obj.Kind
	if kind == "" {
		return ""
	}

	// Kubernetes CRD plural convention: lowercase kind + "s"
	// (e.g. Notebook → notebooks, InferenceService → inferenceservices, RayCluster → rayclusters).
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
