package notebook

import (
	"github.com/opendatahub-io/odh-cli/pkg/constants"
	"github.com/opendatahub-io/odh-cli/pkg/lint/checks/workloads/kueue"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
)

func NewKueueLabelsCheck() *kueue.KueueLabelCheck {
	return kueue.NewCheck(kueue.CheckConfig{
		Kind:                      kind,
		Component:                 constants.ComponentWorkbenches,
		Resource:                  resources.Notebook,
		ConditionType:             ConditionTypeKueueLabels,
		MissingLabelConditionType: ConditionTypeKueueMissingLabels,
		KindLabel:                 "Notebook",
		CheckID:                   "workloads.notebook.kueue-labels",
		CheckName:                 "Workloads :: Notebook :: Kueue Labels",
	})
}
