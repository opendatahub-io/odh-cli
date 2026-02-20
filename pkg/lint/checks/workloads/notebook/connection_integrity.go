package notebook

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/validate"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
	"github.com/opendatahub-io/odh-cli/pkg/util/kube"
)

// ConnectionIntegrityCheck verifies that Notebooks referencing connections via the
// opendatahub.io/connections annotation have backing Secrets that exist in the
// notebook's namespace.
type ConnectionIntegrityCheck struct {
	check.BaseCheck
}

func NewConnectionIntegrityCheck() *ConnectionIntegrityCheck {
	return &ConnectionIntegrityCheck{
		BaseCheck: check.BaseCheck{
			CheckGroup:       check.GroupWorkload,
			Kind:             kind,
			Type:             check.CheckTypeDataIntegrity,
			CheckID:          "workloads.notebook.connection-integrity",
			CheckName:        "Workloads :: Notebook :: Connection Integrity",
			CheckDescription: "Verifies that Notebooks referencing connections have backing Secrets that exist on the cluster",
			CheckRemediation: "Create the missing connection Secret or update the Notebook annotations to reference an existing connection",
		},
	}
}

// CanApply returns whether this check should run for the given target.
// Applies whenever Workbenches is Managed, regardless of version.
func (c *ConnectionIntegrityCheck) CanApply(ctx context.Context, target check.Target) (bool, error) {
	return isWorkbenchesManaged(ctx, target)
}

// Validate lists Notebooks with the connections annotation and verifies that each
// referenced Secret exists in the notebook's namespace.
func (c *ConnectionIntegrityCheck) Validate(
	ctx context.Context,
	target check.Target,
) (*result.DiagnosticResult, error) {
	return validate.WorkloadsMetadata(c, target, resources.Notebook).
		Run(ctx, c.checkConnections)
}

// checkConnections cross-references notebook connection annotations against existing Secrets.
func (c *ConnectionIntegrityCheck) checkConnections(
	ctx context.Context,
	req *validate.WorkloadRequest[*metav1.PartialObjectMetadata],
) error {
	dr := req.Result

	// First pass: parse connection references and collect unique namespaces.
	type notebookConnections struct {
		namespace string
		name      string
		refs      []types.NamespacedName
	}

	var connected []notebookConnections
	targetNamespaces := sets.New[string]()

	for _, nb := range req.Items {
		connValue := kube.GetAnnotation(nb, AnnotationConnections)
		if connValue == "" {
			continue
		}

		refs := parseConnections(connValue, nb.GetNamespace())
		if len(refs) == 0 {
			continue
		}

		connected = append(connected, notebookConnections{
			namespace: nb.GetNamespace(),
			name:      nb.GetName(),
			refs:      refs,
		})

		for _, ref := range refs {
			targetNamespaces.Insert(ref.Namespace)
		}
	}

	// Build Secret cache scoped to only the namespaces referenced by connections.
	secretCache, err := buildSecretCache(ctx, req.Client, targetNamespaces)
	if err != nil {
		return err
	}

	// Second pass: check connections against the scoped cache.
	impacted := make([]types.NamespacedName, 0)

	for _, nc := range connected {
		for _, ref := range nc.refs {
			if !secretCache.Has(ref) {
				impacted = append(impacted, types.NamespacedName{
					Namespace: nc.namespace,
					Name:      nc.name,
				})

				break // one missing Secret is enough to flag the notebook
			}
		}
	}

	totalImpacted := len(impacted)
	dr.Annotations[check.AnnotationImpactedWorkloadCount] = strconv.Itoa(totalImpacted)

	dr.Status.Conditions = append(dr.Status.Conditions, c.newCondition(totalImpacted))
	dr.SetImpactedObjects(resources.Notebook, impacted)

	return nil
}

// parseConnections parses the comma-separated connection annotation value into
// Secret references. Each entry is expected in namespace/name format. If no
// namespace is specified, the notebook's namespace is used as default.
func parseConnections(value string, notebookNamespace string) []types.NamespacedName {
	var refs []types.NamespacedName

	for part := range strings.SplitSeq(value, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		ref := types.NamespacedName{Namespace: notebookNamespace}

		if ns, name, hasSep := strings.Cut(part, "/"); hasSep {
			ref.Name = name
			if ns != "" {
				ref.Namespace = ns
			}
		} else {
			ref.Name = ns
		}

		if ref.Name != "" {
			refs = append(refs, ref)
		}
	}

	return refs
}

// buildSecretCache builds a cache of existing Secrets scoped to the given namespaces.
func buildSecretCache(
	ctx context.Context,
	c client.Reader,
	namespaces sets.Set[string],
) (sets.Set[types.NamespacedName], error) {
	cache := sets.New[types.NamespacedName]()

	for ns := range namespaces {
		secrets, err := c.ListMetadata(ctx, resources.Secret, client.WithNamespace(ns))
		if err != nil {
			if client.IsResourceTypeNotFound(err) {
				continue
			}

			return nil, fmt.Errorf("listing Secrets in namespace %s: %w", ns, err)
		}

		for _, s := range secrets {
			cache.Insert(types.NamespacedName{
				Namespace: s.GetNamespace(),
				Name:      s.GetName(),
			})
		}
	}

	return cache, nil
}

func (c *ConnectionIntegrityCheck) newCondition(totalImpacted int) result.Condition {
	if totalImpacted == 0 {
		return check.NewCondition(
			ConditionTypeConnectionIntegrity,
			metav1.ConditionTrue,
			check.WithReason(check.ReasonRequirementsMet),
			check.WithMessage(MsgAllConnectionsValid),
		)
	}

	return check.NewCondition(
		ConditionTypeConnectionIntegrity,
		metav1.ConditionFalse,
		check.WithReason(check.ReasonResourceNotFound),
		check.WithMessage(MsgConnectionsMissing, totalImpacted),
		check.WithImpact(result.ImpactBlocking),
		check.WithRemediation(c.CheckRemediation),
	)
}
