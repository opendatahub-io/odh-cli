package kueue

import (
	"github.com/opendatahub-io/odh-cli/pkg/constants"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
)

const (
	ConditionTypeISVCKueueLabels        = "ISVCKueueLabels"
	ConditionTypeISVCKueueMissingLabels = "ISVCKueueMissingLabels"
)

func NewKueueLabelsISVCCheck() *KueueLabelCheck {
	return NewCheck(CheckConfig{
		Kind:                      constants.ComponentKueue,
		Resource:                  resources.InferenceService,
		ConditionType:             ConditionTypeISVCKueueLabels,
		MissingLabelConditionType: ConditionTypeISVCKueueMissingLabels,
		KindLabel:                 "InferenceService",
		CheckID:                   "workloads.kueue.inferenceservice-labels",
		CheckName:                 "Workloads :: Kueue :: InferenceService Labels",
	})
}
