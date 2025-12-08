package check_test

import (
	"testing"

	"github.com/lburgazzoli/odh-cli/pkg/doctor/check"
	mocks "github.com/lburgazzoli/odh-cli/pkg/util/test/mocks/check"

	. "github.com/onsi/gomega"
)

func TestCheckRegistry_ListByPattern(t *testing.T) {
	g := NewWithT(t)

	registry := check.NewRegistry()

	// Register test checks
	mockChecks := []struct {
		id       string
		name     string
		category check.CheckCategory
	}{
		{id: "components.dashboard", name: "Dashboard Component", category: check.CategoryComponent},
		{id: "components.workbench", name: "Workbench Component", category: check.CategoryComponent},
		{id: "services.oauth", name: "OAuth Service", category: check.CategoryService},
		{id: "workloads.limits", name: "Resource Limits", category: check.CategoryWorkload},
	}

	for _, mc := range mockChecks {
		mockCheck := mocks.NewMockCheck()
		mockCheck.On("ID").Return(mc.id)
		mockCheck.On("Name").Return(mc.name)
		mockCheck.On("Category").Return(mc.category)
		g.Expect(registry.Register(mockCheck)).To(Succeed())
	}

	tests := []struct {
		name     string
		pattern  string
		category check.CheckCategory
		wantIDs  []string
	}{
		{
			name:     "wildcard all checks",
			pattern:  "*",
			category: "",
			wantIDs:  []string{"components.dashboard", "components.workbench", "services.oauth", "workloads.limits"},
		},
		{
			name:     "category shortcut components",
			pattern:  "components",
			category: "",
			wantIDs:  []string{"components.dashboard", "components.workbench"},
		},
		{
			name:     "category shortcut services",
			pattern:  "services",
			category: "",
			wantIDs:  []string{"services.oauth"},
		},
		{
			name:     "category shortcut workloads",
			pattern:  "workloads",
			category: "",
			wantIDs:  []string{"workloads.limits"},
		},
		{
			name:     "glob components.*",
			pattern:  "components.*",
			category: "",
			wantIDs:  []string{"components.dashboard", "components.workbench"},
		},
		{
			name:     "glob *dashboard*",
			pattern:  "*dashboard*",
			category: "",
			wantIDs:  []string{"components.dashboard"},
		},
		{
			name:     "glob *.dashboard",
			pattern:  "*.dashboard",
			category: "",
			wantIDs:  []string{"components.dashboard"},
		},
		{
			name:     "exact match",
			pattern:  "components.dashboard",
			category: "",
			wantIDs:  []string{"components.dashboard"},
		},
		{
			name:     "pattern with category filter",
			pattern:  "*",
			category: check.CategoryComponent,
			wantIDs:  []string{"components.dashboard", "components.workbench"},
		},
		{
			name:     "glob with category filter",
			pattern:  "*dashboard*",
			category: check.CategoryComponent,
			wantIDs:  []string{"components.dashboard"},
		},
		{
			name:     "no matches",
			pattern:  "nonexistent.*",
			category: "",
			wantIDs:  []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, err := registry.ListByPattern(tt.pattern, tt.category)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(results).To(HaveLen(len(tt.wantIDs)))

			gotIDs := make([]string, len(results))
			for i, c := range results {
				gotIDs[i] = c.ID()
			}

			g.Expect(gotIDs).To(ConsistOf(tt.wantIDs))
		})
	}
}

func TestCheckRegistry_ListByPattern_InvalidPattern(t *testing.T) {
	g := NewWithT(t)

	registry := check.NewRegistry()

	mockCheck := mocks.NewMockCheck()
	mockCheck.On("ID").Return("components.dashboard")
	mockCheck.On("Category").Return(check.CategoryComponent)

	g.Expect(registry.Register(mockCheck)).To(Succeed())

	// Invalid glob pattern should return error
	_, err := registry.ListByPattern("[", "")
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("pattern matching"))
}
