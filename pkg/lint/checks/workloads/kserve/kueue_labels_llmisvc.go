package kserve

import (
	"github.com/opendatahub-io/odh-cli/pkg/constants"
	"github.com/opendatahub-io/odh-cli/pkg/lint/checks/workloads/kueue"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
)

const (
	ConditionTypeLLMISVCKueueLabels        = "LLMISVCKueueLabels"
	ConditionTypeLLMISVCKueueMissingLabels = "LLMISVCKueueMissingLabels"
)

func NewKueueLabelsLLMCheck() *kueue.KueueLabelCheck {
	return kueue.NewCheck(kueue.CheckConfig{
		Kind:                      constants.ComponentKServe,
		Component:                 constants.ComponentKServe,
		Resource:                  resources.LLMInferenceService,
		ConditionType:             ConditionTypeLLMISVCKueueLabels,
		MissingLabelConditionType: ConditionTypeLLMISVCKueueMissingLabels,
		KindLabel:                 "LLMInferenceService",
		CheckID:                   "workloads.kserve.kueue-labels-llm",
		CheckName:                 "Workloads :: KServe :: LLMInferenceService Kueue Labels",
	})
}
