package ray

import (
	"github.com/opendatahub-io/odh-cli/pkg/constants"
	"github.com/opendatahub-io/odh-cli/pkg/lint/checks/workloads/kueue"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
)

const (
	ConditionTypeRayClusterKueueLabels        = "RayClusterKueueLabels"
	ConditionTypeRayClusterKueueMissingLabels = "RayClusterKueueMissingLabels"
)

func NewKueueLabelsRayClusterCheck() *kueue.KueueLabelCheck {
	return kueue.NewCheck(kueue.CheckConfig{
		Kind:                      kind,
		Component:                 constants.ComponentRay,
		Resource:                  resources.RayCluster,
		ConditionType:             ConditionTypeRayClusterKueueLabels,
		MissingLabelConditionType: ConditionTypeRayClusterKueueMissingLabels,
		KindLabel:                 "RayCluster",
		CheckID:                   "workloads.ray.kueue-labels-raycluster",
		CheckName:                 "Workloads :: Ray :: RayCluster Kueue Labels",
	})
}
