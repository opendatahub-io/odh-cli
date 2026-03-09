package kueue

import (
	"github.com/opendatahub-io/odh-cli/pkg/constants"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
)

const (
	ConditionTypeNotebookKueueLabels        = "NotebookKueueLabels"
	ConditionTypeNotebookKueueMissingLabels = "NotebookKueueMissingLabels"
)

func NewKueueLabelsNotebookCheck() *KueueLabelCheck {
	return NewCheck(CheckConfig{
		Kind:                      constants.ComponentKueue,
		Resource:                  resources.Notebook,
		ConditionType:             ConditionTypeNotebookKueueLabels,
		MissingLabelConditionType: ConditionTypeNotebookKueueMissingLabels,
		KindLabel:                 "Notebook",
		CheckID:                   "workloads.kueue.notebook-labels",
		CheckName:                 "Workloads :: Kueue :: Notebook Labels",
	})
}
