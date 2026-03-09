package kueue

import (
	"github.com/opendatahub-io/odh-cli/pkg/constants"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
)

const (
	ConditionTypePyTorchJobKueueLabels        = "PyTorchJobKueueLabels"
	ConditionTypePyTorchJobKueueMissingLabels = "PyTorchJobKueueMissingLabels"
)

func NewKueueLabelsPyTorchJobCheck() *KueueLabelCheck {
	return NewCheck(CheckConfig{
		Kind:                      constants.ComponentKueue,
		Resource:                  resources.PyTorchJob,
		ConditionType:             ConditionTypePyTorchJobKueueLabels,
		MissingLabelConditionType: ConditionTypePyTorchJobKueueMissingLabels,
		KindLabel:                 "PyTorchJob",
		CheckID:                   "workloads.kueue.pytorchjob-labels",
		CheckName:                 "Workloads :: Kueue :: PyTorchJob Labels",
	})
}
