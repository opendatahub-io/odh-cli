package components_test

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/opendatahub-io/odh-cli/pkg/constants"
	"github.com/opendatahub-io/odh-cli/pkg/util/components"

	. "github.com/onsi/gomega"
)

// newDSCWithSubComponent builds an unstructured DSC with one sub-component nested under parent.
func newDSCWithSubComponent(parent, sub, state string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "datasciencecluster.opendatahub.io/v2",
			"kind":       "DataScienceCluster",
			"metadata": map[string]any{
				"name": "default-dsc",
			},
			"spec": map[string]any{
				"components": map[string]any{
					parent: map[string]any{
						"managementState": "Managed",
						sub: map[string]any{
							"managementState": state,
						},
					},
				},
			},
		},
	}
}

// newDSCWithoutSubComponent builds a DSC where parent exists but sub is absent.
func newDSCWithoutSubComponent(parent string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "datasciencecluster.opendatahub.io/v2",
			"kind":       "DataScienceCluster",
			"metadata": map[string]any{
				"name": "default-dsc",
			},
			"spec": map[string]any{
				"components": map[string]any{
					parent: map[string]any{
						"managementState": "Managed",
					},
				},
			},
		},
	}
}

func TestGetSubComponentManagementState_Found(t *testing.T) {
	tests := []struct {
		name  string
		state string
	}{
		{"Managed", constants.ManagementStateManaged},
		{"Unmanaged", constants.ManagementStateUnmanaged},
		{"Removed", constants.ManagementStateRemoved},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			dsc := newDSCWithSubComponent("aigateway", "modelsAsAService", tt.state)
			state, err := components.GetSubComponentManagementState(dsc, "aigateway", "modelsAsAService")

			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(state).To(Equal(tt.state))
		})
	}
}

func TestGetSubComponentManagementState_NotFound_ReturnsRemoved(t *testing.T) {
	g := NewWithT(t)

	dsc := newDSCWithoutSubComponent("aigateway")
	state, err := components.GetSubComponentManagementState(dsc, "aigateway", "modelsAsAService")

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(state).To(Equal(constants.ManagementStateRemoved))
}

func TestGetSubComponentManagementState_ParentAbsent_ReturnsRemoved(t *testing.T) {
	g := NewWithT(t)

	dsc := &unstructured.Unstructured{
		Object: map[string]any{
			"spec": map[string]any{
				"components": map[string]any{},
			},
		},
	}
	state, err := components.GetSubComponentManagementState(dsc, "aigateway", "modelsAsAService")

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(state).To(Equal(constants.ManagementStateRemoved))
}

func TestHasSubComponentManagementState_MatchingState(t *testing.T) {
	g := NewWithT(t)

	dsc := newDSCWithSubComponent("aigateway", "modelsAsAService", constants.ManagementStateManaged)
	result := components.HasSubComponentManagementState(dsc, "aigateway", "modelsAsAService", constants.ManagementStateManaged)

	g.Expect(result).To(BeTrue())
}

func TestHasSubComponentManagementState_NonMatchingState(t *testing.T) {
	g := NewWithT(t)

	dsc := newDSCWithSubComponent("aigateway", "modelsAsAService", constants.ManagementStateRemoved)
	result := components.HasSubComponentManagementState(dsc, "aigateway", "modelsAsAService", constants.ManagementStateManaged)

	g.Expect(result).To(BeFalse())
}

func TestHasSubComponentManagementState_NoStates_AlwaysTrue(t *testing.T) {
	g := NewWithT(t)

	dsc := newDSCWithSubComponent("aigateway", "modelsAsAService", constants.ManagementStateRemoved)
	result := components.HasSubComponentManagementState(dsc, "aigateway", "modelsAsAService")

	g.Expect(result).To(BeTrue())
}

func TestHasSubComponentManagementState_NotFound_ReturnsFalse(t *testing.T) {
	g := NewWithT(t)

	dsc := newDSCWithoutSubComponent("aigateway")
	result := components.HasSubComponentManagementState(dsc, "aigateway", "modelsAsAService", constants.ManagementStateManaged)

	g.Expect(result).To(BeFalse())
}

func TestHasSubComponentManagementState_MultipleStates(t *testing.T) {
	g := NewWithT(t)

	dsc := newDSCWithSubComponent("aigateway", "modelsAsAService", constants.ManagementStateUnmanaged)
	result := components.HasSubComponentManagementState(
		dsc, "aigateway", "modelsAsAService",
		constants.ManagementStateManaged, constants.ManagementStateUnmanaged,
	)

	g.Expect(result).To(BeTrue())
}
