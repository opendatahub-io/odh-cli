//nolint:testpackage // internal test: accesses unexported registry field on Command
package lint

import (
	"testing"

	. "github.com/onsi/gomega"
)

// TestNewCommand_RegistersAIGatewayMaaSCheck verifies that the MaaS field migration
// check is registered in the command's check registry and is discoverable by its ID.
func TestNewCommand_RegistersAIGatewayMaaSCheck(t *testing.T) {
	g := NewWithT(t)

	cmd := newTestCommand()
	ids := cmd.registry.AllCheckIDs()

	g.Expect(ids).To(ContainElement("components.aigateway.maas-field-migration"))
}

// TestNewCommand_AIGatewayCheckGroup verifies the check is in the component group.
func TestNewCommand_AIGatewayCheckGroup(t *testing.T) {
	g := NewWithT(t)

	cmd := newTestCommand()

	var found bool
	for _, chk := range cmd.registry.ListAll() {
		if chk.ID() == "components.aigateway.maas-field-migration" {
			g.Expect(string(chk.Group())).To(Equal("component"))
			found = true

			break
		}
	}

	g.Expect(found).To(BeTrue(), "check components.aigateway.maas-field-migration not found in registry")
}
