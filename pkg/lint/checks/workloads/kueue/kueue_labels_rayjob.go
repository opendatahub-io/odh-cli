package kueue

import (
	"github.com/opendatahub-io/odh-cli/pkg/constants"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
)

const (
	ConditionTypeRayJobKueueLabels        = "RayJobKueueLabels"
	ConditionTypeRayJobKueueMissingLabels = "RayJobKueueMissingLabels"
)

func NewKueueLabelsRayJobCheck() *KueueLabelCheck {
	return NewCheck(CheckConfig{
		Kind:                      constants.ComponentKueue,
		Component:                 constants.ComponentKueue,
		Resource:                  resources.RayJob,
		ConditionType:             ConditionTypeRayJobKueueLabels,
		MissingLabelConditionType: ConditionTypeRayJobKueueMissingLabels,
		KindLabel:                 "RayJob",
		CheckID:                   "workloads.ray.kueue-labels-rayjob",
		CheckName:                 "Workloads :: Ray :: RayJob Kueue Labels",
	})
}
