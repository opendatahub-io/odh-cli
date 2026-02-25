package kserve

import (
	"github.com/opendatahub-io/odh-cli/pkg/constants"
	"github.com/opendatahub-io/odh-cli/pkg/lint/checks/workloads/kueue"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
)

const (
	ConditionTypeISVCKueueLabels        = "ISVCKueueLabels"
	ConditionTypeISVCKueueMissingLabels = "ISVCKueueMissingLabels"
)

func NewKueueLabelsISVCCheck() *kueue.KueueLabelCheck {
	return kueue.NewCheck(kueue.CheckConfig{
		Kind:                      constants.ComponentKServe,
		Component:                 constants.ComponentKServe,
		Resource:                  resources.InferenceService,
		ConditionType:             ConditionTypeISVCKueueLabels,
		MissingLabelConditionType: ConditionTypeISVCKueueMissingLabels,
		KindLabel:                 "InferenceService",
		CheckID:                   "workloads.kserve.kueue-labels-isvc",
		CheckName:                 "Workloads :: KServe :: InferenceService Kueue Labels",
	})
}
