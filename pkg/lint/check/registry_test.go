package check_test

import (
	"testing"

	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	mocks "github.com/opendatahub-io/odh-cli/pkg/util/test/mocks/check"

	. "github.com/onsi/gomega"
)

func TestCheckRegistry_ListByPattern(t *testing.T) {
	g := NewWithT(t)

	registry := check.NewRegistry()

	// Register test checks
	mockChecks := []struct {
		id    string
		name  string
		group check.CheckGroup
	}{
		{id: "components.dashboard", name: "Dashboard Component", group: check.GroupComponent},
		{id: "components.workbench", name: "Workbench Component", group: check.GroupComponent},
		{id: "services.oauth", name: "OAuth Service", group: check.GroupService},
		{id: "workloads.limits", name: "Resource Limits", group: check.GroupWorkload},
	}

	for _, mc := range mockChecks {
		mockCheck := mocks.NewMockCheck()
		mockCheck.On("ID").Return(mc.id)
		mockCheck.On("Name").Return(mc.name)
		mockCheck.On("Group").Return(mc.group)
		g.Expect(registry.Register(mockCheck)).To(Succeed())
	}

	tests := []struct {
		name    string
		pattern string
		group   check.CheckGroup
		wantIDs []string
	}{
		{
			name:    "wildcard all checks",
			pattern: "*",
			group:   "",
			wantIDs: []string{"components.dashboard", "components.workbench", "services.oauth", "workloads.limits"},
		},
		{
			name:    "group shortcut components",
			pattern: "components",
			group:   "",
			wantIDs: []string{"components.dashboard", "components.workbench"},
		},
		{
			name:    "group shortcut services",
			pattern: "services",
			group:   "",
			wantIDs: []string{"services.oauth"},
		},
		{
			name:    "group shortcut workloads",
			pattern: "workloads",
			group:   "",
			wantIDs: []string{"workloads.limits"},
		},
		{
			name:    "glob components.*",
			pattern: "components.*",
			group:   "",
			wantIDs: []string{"components.dashboard", "components.workbench"},
		},
		{
			name:    "glob *dashboard*",
			pattern: "*dashboard*",
			group:   "",
			wantIDs: []string{"components.dashboard"},
		},
		{
			name:    "glob *.dashboard",
			pattern: "*.dashboard",
			group:   "",
			wantIDs: []string{"components.dashboard"},
		},
		{
			name:    "exact match",
			pattern: "components.dashboard",
			group:   "",
			wantIDs: []string{"components.dashboard"},
		},
		{
			name:    "pattern with group filter",
			pattern: "*",
			group:   check.GroupComponent,
			wantIDs: []string{"components.dashboard", "components.workbench"},
		},
		{
			name:    "glob with group filter",
			pattern: "*dashboard*",
			group:   check.GroupComponent,
			wantIDs: []string{"components.dashboard"},
		},
		{
			name:    "no matches",
			pattern: "nonexistent.*",
			group:   "",
			wantIDs: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, err := registry.ListByPattern(tt.pattern, tt.group)
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

func TestCheckRegistry_ListByPatterns_MultiplePatterns(t *testing.T) {
	g := NewWithT(t)

	registry := check.NewRegistry()

	// Register test checks
	mockChecks := []struct {
		id    string
		name  string
		group check.CheckGroup
	}{
		{id: "components.dashboard", name: "Dashboard Component", group: check.GroupComponent},
		{id: "components.workbench", name: "Workbench Component", group: check.GroupComponent},
		{id: "services.oauth", name: "OAuth Service", group: check.GroupService},
		{id: "workloads.limits", name: "Resource Limits", group: check.GroupWorkload},
		{id: "dependencies.certmanager", name: "Cert Manager", group: check.GroupDependency},
	}

	for _, mc := range mockChecks {
		mockCheck := mocks.NewMockCheck()
		mockCheck.On("ID").Return(mc.id)
		mockCheck.On("Name").Return(mc.name)
		mockCheck.On("Group").Return(mc.group)
		g.Expect(registry.Register(mockCheck)).To(Succeed())
	}

	tests := []struct {
		name     string
		patterns []string
		group    check.CheckGroup
		wantIDs  []string
	}{
		{
			name:     "single wildcard pattern",
			patterns: []string{"*"},
			group:    "",
			wantIDs:  []string{"components.dashboard", "components.workbench", "services.oauth", "workloads.limits", "dependencies.certmanager"},
		},
		{
			name:     "two group shortcuts",
			patterns: []string{"components", "services"},
			group:    "",
			wantIDs:  []string{"components.dashboard", "components.workbench", "services.oauth"},
		},
		{
			name:     "exact IDs",
			patterns: []string{"components.dashboard", "workloads.limits"},
			group:    "",
			wantIDs:  []string{"components.dashboard", "workloads.limits"},
		},
		{
			name:     "glob and exact combined",
			patterns: []string{"services.*", "components.dashboard"},
			group:    "",
			wantIDs:  []string{"components.dashboard", "services.oauth"},
		},
		{
			name:     "overlapping patterns deduplicate",
			patterns: []string{"components.*", "*dashboard*"},
			group:    "",
			wantIDs:  []string{"components.dashboard", "components.workbench"},
		},
		{
			name:     "multiple patterns with group filter",
			patterns: []string{"*dashboard*", "*workbench*"},
			group:    check.GroupComponent,
			wantIDs:  []string{"components.dashboard", "components.workbench"},
		},
		{
			name:     "no matches across multiple patterns",
			patterns: []string{"nonexistent.*", "also.nonexistent"},
			group:    "",
			wantIDs:  []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, err := registry.ListByPatterns(tt.patterns, tt.group)
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

func TestCheckRegistry_ListByPatterns_InvalidPattern(t *testing.T) {
	g := NewWithT(t)

	registry := check.NewRegistry()

	mockCheck := mocks.NewMockCheck()
	mockCheck.On("ID").Return("components.dashboard")
	mockCheck.On("Group").Return(check.GroupComponent)

	g.Expect(registry.Register(mockCheck)).To(Succeed())

	// Invalid glob pattern in any position should return error
	_, err := registry.ListByPatterns([]string{"services.*", "["}, "")
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("pattern matching"))
}

func TestCheckRegistry_ListByPattern_InvalidPattern(t *testing.T) {
	g := NewWithT(t)

	registry := check.NewRegistry()

	mockCheck := mocks.NewMockCheck()
	mockCheck.On("ID").Return("components.dashboard")
	mockCheck.On("Group").Return(check.GroupComponent)

	g.Expect(registry.Register(mockCheck)).To(Succeed())

	// Invalid glob pattern should return error
	_, err := registry.ListByPattern("[", "")
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("pattern matching"))
}
