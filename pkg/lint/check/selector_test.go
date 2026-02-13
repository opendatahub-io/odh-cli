package check_test

import (
	"testing"

	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	mocks "github.com/opendatahub-io/odh-cli/pkg/util/test/mocks/check"

	. "github.com/onsi/gomega"
)

func TestMatchesPattern_Wildcard(t *testing.T) {
	g := NewWithT(t)

	tests := []struct {
		name    string
		checkID string
		group   check.CheckGroup
		pattern string
		want    bool
	}{
		{
			name:    "wildcard matches component check",
			checkID: "components.dashboard",
			group:   check.GroupComponent,
			pattern: "*",
			want:    true,
		},
		{
			name:    "wildcard matches service check",
			checkID: "services.oauth",
			group:   check.GroupService,
			pattern: "*",
			want:    true,
		},
		{
			name:    "wildcard matches workload check",
			checkID: "workloads.limits",
			group:   check.GroupWorkload,
			pattern: "*",
			want:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockCheck := mocks.NewMockCheck()
			mockCheck.On("ID").Return(tt.checkID)
			mockCheck.On("Group").Return(tt.group)

			// matchesPattern is not exported, so we test through ListByPattern
			registry := check.NewRegistry()
			g.Expect(registry.Register(mockCheck)).To(Succeed())

			results, err := registry.ListByPattern(tt.pattern, "")
			g.Expect(err).ToNot(HaveOccurred())

			if tt.want {
				g.Expect(results).To(HaveLen(1))
				g.Expect(results[0].ID()).To(Equal(tt.checkID))
			} else {
				g.Expect(results).To(BeEmpty())
			}
		})
	}
}

func TestMatchesPattern_GroupShortcuts(t *testing.T) {
	g := NewWithT(t)

	tests := []struct {
		name    string
		checkID string
		group   check.CheckGroup
		pattern string
		want    bool
	}{
		{
			name:    "components shortcut matches component check",
			checkID: "components.dashboard",
			group:   check.GroupComponent,
			pattern: "components",
			want:    true,
		},
		{
			name:    "components shortcut does not match service check",
			checkID: "services.oauth",
			group:   check.GroupService,
			pattern: "components",
			want:    false,
		},
		{
			name:    "services shortcut matches service check",
			checkID: "services.oauth",
			group:   check.GroupService,
			pattern: "services",
			want:    true,
		},
		{
			name:    "services shortcut does not match component check",
			checkID: "components.dashboard",
			group:   check.GroupComponent,
			pattern: "services",
			want:    false,
		},
		{
			name:    "workloads shortcut matches workload check",
			checkID: "workloads.limits",
			group:   check.GroupWorkload,
			pattern: "workloads",
			want:    true,
		},
		{
			name:    "workloads shortcut does not match component check",
			checkID: "components.dashboard",
			group:   check.GroupComponent,
			pattern: "workloads",
			want:    false,
		},
		{
			name:    "dependencies shortcut matches dependency check",
			checkID: "dependencies.certmanager",
			group:   check.GroupDependency,
			pattern: "dependencies",
			want:    true,
		},
		{
			name:    "dependencies shortcut does not match component check",
			checkID: "components.dashboard",
			group:   check.GroupComponent,
			pattern: "dependencies",
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockCheck := mocks.NewMockCheck()
			mockCheck.On("ID").Return(tt.checkID)
			mockCheck.On("Group").Return(tt.group)

			registry := check.NewRegistry()
			g.Expect(registry.Register(mockCheck)).To(Succeed())

			results, err := registry.ListByPattern(tt.pattern, "")
			g.Expect(err).ToNot(HaveOccurred())

			if tt.want {
				g.Expect(results).To(HaveLen(1))
				g.Expect(results[0].ID()).To(Equal(tt.checkID))
			} else {
				g.Expect(results).To(BeEmpty())
			}
		})
	}
}

func TestMatchesPattern_ExactMatch(t *testing.T) {
	g := NewWithT(t)

	tests := []struct {
		name    string
		checkID string
		group   check.CheckGroup
		pattern string
		want    bool
	}{
		{
			name:    "exact match success",
			checkID: "components.dashboard",
			group:   check.GroupComponent,
			pattern: "components.dashboard",
			want:    true,
		},
		{
			name:    "exact match fail",
			checkID: "components.dashboard",
			group:   check.GroupComponent,
			pattern: "components.workbench",
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockCheck := mocks.NewMockCheck()
			mockCheck.On("ID").Return(tt.checkID)
			mockCheck.On("Group").Return(tt.group)

			registry := check.NewRegistry()
			g.Expect(registry.Register(mockCheck)).To(Succeed())

			results, err := registry.ListByPattern(tt.pattern, "")
			g.Expect(err).ToNot(HaveOccurred())

			if tt.want {
				g.Expect(results).To(HaveLen(1))
				g.Expect(results[0].ID()).To(Equal(tt.checkID))
			} else {
				g.Expect(results).To(BeEmpty())
			}
		})
	}
}

func TestMatchesPattern_GlobPatterns(t *testing.T) {
	g := NewWithT(t)

	tests := []struct {
		name    string
		checkID string
		group   check.CheckGroup
		pattern string
		want    bool
	}{
		{
			name:    "prefix glob match",
			checkID: "components.dashboard",
			group:   check.GroupComponent,
			pattern: "components.*",
			want:    true,
		},
		{
			name:    "prefix glob no match",
			checkID: "services.oauth",
			group:   check.GroupService,
			pattern: "components.*",
			want:    false,
		},
		{
			name:    "suffix glob match",
			checkID: "components.dashboard",
			group:   check.GroupComponent,
			pattern: "*.dashboard",
			want:    true,
		},
		{
			name:    "suffix glob no match",
			checkID: "components.workbench",
			group:   check.GroupComponent,
			pattern: "*.dashboard",
			want:    false,
		},
		{
			name:    "contains glob match",
			checkID: "components.dashboard",
			group:   check.GroupComponent,
			pattern: "*dashboard*",
			want:    true,
		},
		{
			name:    "contains glob no match",
			checkID: "components.workbench",
			group:   check.GroupComponent,
			pattern: "*dashboard*",
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockCheck := mocks.NewMockCheck()
			mockCheck.On("ID").Return(tt.checkID)
			mockCheck.On("Group").Return(tt.group)

			registry := check.NewRegistry()
			g.Expect(registry.Register(mockCheck)).To(Succeed())

			results, err := registry.ListByPattern(tt.pattern, "")
			g.Expect(err).ToNot(HaveOccurred())

			if tt.want {
				g.Expect(results).To(HaveLen(1))
				g.Expect(results[0].ID()).To(Equal(tt.checkID))
			} else {
				g.Expect(results).To(BeEmpty())
			}
		})
	}
}

func TestMatchesPattern_InvalidPattern(t *testing.T) {
	g := NewWithT(t)

	mockCheck := mocks.NewMockCheck()
	mockCheck.On("ID").Return("components.dashboard")
	mockCheck.On("Group").Return(check.GroupComponent)

	registry := check.NewRegistry()
	g.Expect(registry.Register(mockCheck)).To(Succeed())

	// Invalid glob pattern should return error
	_, err := registry.ListByPattern("[", "")
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("invalid pattern"))
}
