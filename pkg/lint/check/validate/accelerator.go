package validate

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/types"

	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
	"github.com/opendatahub-io/odh-cli/pkg/util/kube"
)

const (
	// AnnotationAcceleratorName is set on workloads to reference an AcceleratorProfile by name.
	AnnotationAcceleratorName = "opendatahub.io/accelerator-name"

	// AnnotationAcceleratorNamespace is set on workloads to specify the namespace
	// of the referenced AcceleratorProfile. When absent, the applications namespace
	// (from DSCInitialization) is used as default.
	AnnotationAcceleratorNamespace = "opendatahub.io/accelerator-profile-namespace"
)

// FindWorkloadsWithAcceleratorRefs lists workloads of the given resource type,
// checks which ones reference AcceleratorProfiles via annotations, and returns
// the impacted workload names along with a count of missing profiles.
func FindWorkloadsWithAcceleratorRefs(
	ctx context.Context,
	target check.Target,
	workloadType resources.ResourceType,
) ([]types.NamespacedName, int, error) {
	workloads, err := target.Client.ListMetadata(ctx, workloadType)
	if err != nil {
		if client.IsResourceTypeNotFound(err) {
			return nil, 0, nil
		}

		return nil, 0, fmt.Errorf("listing %s: %w", workloadType.Kind, err)
	}

	// Resolve the applications namespace for AcceleratorProfile lookups.
	// AcceleratorProfiles live in the applications namespace, but workloads may not
	// have the namespace annotation set, so we need a proper default.
	appNS, err := client.GetApplicationsNamespace(ctx, target.Client)
	if err != nil {
		return nil, 0, fmt.Errorf("getting applications namespace: %w", err)
	}

	profileCache, err := kube.BuildResourceNameSet(ctx, target.Client, resources.AcceleratorProfile)
	if err != nil {
		return nil, 0, fmt.Errorf("building AcceleratorProfile cache: %w", err)
	}

	var impacted []types.NamespacedName

	missingCount := 0

	for _, w := range workloads {
		profileRef := types.NamespacedName{
			Namespace: kube.GetAnnotation(w, AnnotationAcceleratorNamespace),
			Name:      kube.GetAnnotation(w, AnnotationAcceleratorName),
		}

		if profileRef.Name == "" {
			continue
		}

		if profileRef.Namespace == "" {
			profileRef.Namespace = appNS
		}

		impacted = append(impacted, types.NamespacedName{
			Namespace: w.GetNamespace(),
			Name:      w.GetName(),
		})

		if !profileCache.Has(profileRef) {
			missingCount++
		}
	}

	return impacted, missingCount, nil
}
