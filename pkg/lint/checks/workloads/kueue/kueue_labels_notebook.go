package kueue

import (
	"github.com/opendatahub-io/odh-cli/pkg/constants"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
)

const (
	ConditionTypeNotebookKueueLabels        = "KueueLabels"
	ConditionTypeNotebookKueueMissingLabels = "KueueMissingLabels"
)

func NewKueueLabelsNotebookCheck() *KueueLabelCheck {
	return NewCheck(CheckConfig{
		Kind:                      constants.ComponentKueue,
		Component:                 constants.ComponentKueue,
		Resource:                  resources.Notebook,
		ConditionType:             ConditionTypeNotebookKueueLabels,
		MissingLabelConditionType: ConditionTypeNotebookKueueMissingLabels,
		KindLabel:                 "Notebook",
		CheckID:                   "workloads.notebook.kueue-labels",
		CheckName:                 "Workloads :: Notebook :: Kueue Labels",
	})
}
