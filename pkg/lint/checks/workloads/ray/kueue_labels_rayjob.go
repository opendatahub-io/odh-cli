package ray

import (
	"github.com/opendatahub-io/odh-cli/pkg/constants"
	"github.com/opendatahub-io/odh-cli/pkg/lint/checks/workloads/kueue"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
)

const (
	ConditionTypeRayJobKueueLabels        = "RayJobKueueLabels"
	ConditionTypeRayJobKueueMissingLabels = "RayJobKueueMissingLabels"
)

func NewKueueLabelsRayJobCheck() *kueue.KueueLabelCheck {
	return kueue.NewCheck(kueue.CheckConfig{
		Kind:                      kind,
		Component:                 constants.ComponentRay,
		Resource:                  resources.RayJob,
		ConditionType:             ConditionTypeRayJobKueueLabels,
		MissingLabelConditionType: ConditionTypeRayJobKueueMissingLabels,
		KindLabel:                 "RayJob",
		CheckID:                   "workloads.ray.kueue-labels-rayjob",
		CheckName:                 "Workloads :: Ray :: RayJob Kueue Labels",
	})
}
