package kueue

import (
	"github.com/opendatahub-io/odh-cli/pkg/constants"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
)

const (
	ConditionTypeLLMISVCKueueLabels        = "LLMISVCKueueLabels"
	ConditionTypeLLMISVCKueueMissingLabels = "LLMISVCKueueMissingLabels"
)

func NewKueueLabelsLLMCheck() *KueueLabelCheck {
	return NewCheck(CheckConfig{
		Kind:                      constants.ComponentKueue,
		Resource:                  resources.LLMInferenceService,
		ConditionType:             ConditionTypeLLMISVCKueueLabels,
		MissingLabelConditionType: ConditionTypeLLMISVCKueueMissingLabels,
		KindLabel:                 "LLMInferenceService",
		CheckID:                   "workloads.kueue.llminferenceservice-labels",
		CheckName:                 "Workloads :: Kueue :: LLMInferenceService Labels",
	})
}
