package kueue

import (
	"github.com/opendatahub-io/odh-cli/pkg/constants"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
)

const (
	ConditionTypeRayClusterKueueLabels        = "RayClusterKueueLabels"
	ConditionTypeRayClusterKueueMissingLabels = "RayClusterKueueMissingLabels"
)

func NewKueueLabelsRayClusterCheck() *KueueLabelCheck {
	return NewCheck(CheckConfig{
		Kind:                      constants.ComponentKueue,
		Component:                 constants.ComponentKueue,
		Resource:                  resources.RayCluster,
		ConditionType:             ConditionTypeRayClusterKueueLabels,
		MissingLabelConditionType: ConditionTypeRayClusterKueueMissingLabels,
		KindLabel:                 "RayCluster",
		CheckID:                   "workloads.ray.kueue-labels-raycluster",
		CheckName:                 "Workloads :: Ray :: RayCluster Kueue Labels",
	})
}
