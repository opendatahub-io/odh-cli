package lint_test

import (
	"testing"

	"github.com/opendatahub-io/odh-cli/pkg/lint"

	. "github.com/onsi/gomega"
)

func TestValidateCheckSelectors(t *testing.T) {
	g := NewWithT(t)

	tests := []struct {
		name      string
		selectors []string
		wantErr   bool
	}{
		{
			name:      "single wildcard valid",
			selectors: []string{"*"},
			wantErr:   false,
		},
		{
			name:      "multiple patterns valid",
			selectors: []string{"components.*", "services.*"},
			wantErr:   false,
		},
		{
			name:      "mixed patterns valid",
			selectors: []string{"components", "*dashboard*", "services.oauth"},
			wantErr:   false,
		},
		{
			name:      "empty slice invalid",
			selectors: []string{},
			wantErr:   true,
		},
		{
			name:      "nil slice invalid",
			selectors: nil,
			wantErr:   true,
		},
		{
			name:      "one invalid pattern fails all",
			selectors: []string{"components.*", "["},
			wantErr:   true,
		},
		{
			name:      "empty string in slice invalid",
			selectors: []string{"components.*", ""},
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := lint.ValidateCheckSelectors(tt.selectors)

			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).ToNot(HaveOccurred())
			}
		})
	}
}

func TestValidateCheckSelector(t *testing.T) {
	g := NewWithT(t)

	tests := []struct {
		name     string
		selector string
		wantErr  bool
	}{
		{
			name:     "wildcard valid",
			selector: "*",
			wantErr:  false,
		},
		{
			name:     "category components valid",
			selector: "components",
			wantErr:  false,
		},
		{
			name:     "category services valid",
			selector: "services",
			wantErr:  false,
		},
		{
			name:     "category workloads valid",
			selector: "workloads",
			wantErr:  false,
		},
		{
			name:     "category dependencies valid",
			selector: "dependencies",
			wantErr:  false,
		},
		{
			name:     "glob pattern components.* valid",
			selector: "components.*",
			wantErr:  false,
		},
		{
			name:     "glob pattern *dashboard* valid",
			selector: "*dashboard*",
			wantErr:  false,
		},
		{
			name:     "glob pattern *.dashboard valid",
			selector: "*.dashboard",
			wantErr:  false,
		},
		{
			name:     "exact ID valid",
			selector: "components.dashboard",
			wantErr:  false,
		},
		{
			name:     "complex glob valid",
			selector: "components.dash*",
			wantErr:  false,
		},
		{
			name:     "empty invalid",
			selector: "",
			wantErr:  true,
		},
		{
			name:     "invalid glob pattern [",
			selector: "[",
			wantErr:  true,
		},
		{
			name:     "invalid glob pattern \\",
			selector: "\\",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := lint.ValidateCheckSelector(tt.selector)

			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).ToNot(HaveOccurred())
			}
		})
	}
}
