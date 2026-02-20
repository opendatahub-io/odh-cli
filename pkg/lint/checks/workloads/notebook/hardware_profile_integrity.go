package notebook

import (
	"context"
	"fmt"
	"strconv"

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

// HardwareProfileIntegrityCheck verifies that Notebooks referencing infrastructure HardwareProfiles
// via annotations point to HardwareProfiles that actually exist on the cluster.
type HardwareProfileIntegrityCheck struct {
	check.BaseCheck
}

func NewHardwareProfileIntegrityCheck() *HardwareProfileIntegrityCheck {
	return &HardwareProfileIntegrityCheck{
		BaseCheck: check.BaseCheck{
			CheckGroup:       check.GroupWorkload,
			Kind:             kind,
			Type:             check.CheckTypeDataIntegrity,
			CheckID:          "workloads.notebook.hardware-profile-integrity",
			CheckName:        "Workloads :: Notebook :: HardwareProfile Integrity",
			CheckDescription: "Verifies that Notebooks referencing infrastructure HardwareProfiles point to profiles that exist on the cluster",
			CheckRemediation: "Create the missing HardwareProfile or update the Notebook annotations to reference an existing profile",
		},
	}
}

// CanApply returns whether this check should run for the given target.
// Applies whenever Workbenches is Managed, regardless of version.
func (c *HardwareProfileIntegrityCheck) CanApply(ctx context.Context, target check.Target) (bool, error) {
	return isWorkbenchesManaged(ctx, target)
}

// Validate lists Notebooks with hardware profile annotations and checks that each
// referenced HardwareProfile (infrastructure.opendatahub.io) exists on the cluster.
func (c *HardwareProfileIntegrityCheck) Validate(
	ctx context.Context,
	target check.Target,
) (*result.DiagnosticResult, error) {
	return validate.WorkloadsMetadata(c, target, resources.Notebook).
		Run(ctx, c.checkHardwareProfiles)
}

// checkHardwareProfiles cross-references notebook hardware profile annotations against existing profiles.
func (c *HardwareProfileIntegrityCheck) checkHardwareProfiles(
	ctx context.Context,
	req *validate.WorkloadRequest[*metav1.PartialObjectMetadata],
) error {
	dr := req.Result

	// First pass: parse HardwareProfile references and collect unique namespaces.
	type notebookProfile struct {
		namespace string
		name      string
		ref       types.NamespacedName
	}

	var referenced []notebookProfile
	targetNamespaces := sets.New[string]()

	for _, nb := range req.Items {
		profileName := kube.GetAnnotation(nb, AnnotationHardwareProfileName)
		if profileName == "" {
			continue
		}

		profileNS := kube.GetAnnotation(nb, AnnotationHardwareProfileNamespace)
		if profileNS == "" {
			profileNS = nb.GetNamespace()
		}

		referenced = append(referenced, notebookProfile{
			namespace: nb.GetNamespace(),
			name:      nb.GetName(),
			ref: types.NamespacedName{
				Namespace: profileNS,
				Name:      profileName,
			},
		})

		targetNamespaces.Insert(profileNS)
	}

	// Build HardwareProfile cache scoped to only the namespaces referenced by notebooks.
	profileCache := sets.New[types.NamespacedName]()

	for ns := range targetNamespaces {
		profiles, err := req.Client.ListMetadata(ctx, resources.InfrastructureHardwareProfile, client.WithNamespace(ns))
		if err != nil {
			if client.IsResourceTypeNotFound(err) {
				continue
			}

			return fmt.Errorf("listing HardwareProfiles in namespace %s: %w", ns, err)
		}

		for _, p := range profiles {
			profileCache.Insert(types.NamespacedName{
				Namespace: p.GetNamespace(),
				Name:      p.GetName(),
			})
		}
	}

	// Second pass: check profile references against the scoped cache.
	impacted := make([]types.NamespacedName, 0)

	for _, np := range referenced {
		if !profileCache.Has(np.ref) {
			impacted = append(impacted, types.NamespacedName{
				Namespace: np.namespace,
				Name:      np.name,
			})
		}
	}

	totalImpacted := len(impacted)
	dr.Annotations[check.AnnotationImpactedWorkloadCount] = strconv.Itoa(totalImpacted)

	dr.Status.Conditions = append(dr.Status.Conditions, c.newCondition(totalImpacted))
	dr.SetImpactedObjects(resources.Notebook, impacted)

	return nil
}

func (c *HardwareProfileIntegrityCheck) newCondition(totalImpacted int) result.Condition {
	if totalImpacted == 0 {
		return check.NewCondition(
			ConditionTypeHardwareProfileIntegrity,
			metav1.ConditionTrue,
			check.WithReason(check.ReasonRequirementsMet),
			check.WithMessage(MsgAllHardwareProfilesValid),
		)
	}

	return check.NewCondition(
		ConditionTypeHardwareProfileIntegrity,
		metav1.ConditionFalse,
		check.WithReason(check.ReasonResourceNotFound),
		check.WithMessage(MsgHardwareProfilesMissing, totalImpacted),
		check.WithImpact(result.ImpactBlocking),
		check.WithRemediation(c.CheckRemediation),
	)
}
