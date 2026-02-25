package trainingoperator

import (
	"github.com/opendatahub-io/odh-cli/pkg/constants"
	"github.com/opendatahub-io/odh-cli/pkg/lint/checks/workloads/kueue"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
)

const (
	ConditionTypePyTorchJobKueueLabels        = "PyTorchJobKueueLabels"
	ConditionTypePyTorchJobKueueMissingLabels = "PyTorchJobKueueMissingLabels"
)

func NewKueueLabelsPyTorchJobCheck() *kueue.KueueLabelCheck {
	return kueue.NewCheck(kueue.CheckConfig{
		Kind:                      constants.ComponentTrainingOperator,
		Component:                 constants.ComponentTrainingOperator,
		Resource:                  resources.PyTorchJob,
		ConditionType:             ConditionTypePyTorchJobKueueLabels,
		MissingLabelConditionType: ConditionTypePyTorchJobKueueMissingLabels,
		KindLabel:                 "PyTorchJob",
		CheckID:                   "workloads.trainingoperator.kueue-labels-pytorchjob",
		CheckName:                 "Workloads :: TrainingOperator :: PyTorchJob Kueue Labels",
	})
}
