package lint_test

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/opendatahub-io/odh-cli/pkg/lint"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/result"

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

func TestSeverityLevelValidate(t *testing.T) {
	g := NewWithT(t)

	tests := []struct {
		name    string
		level   lint.SeverityLevel
		wantErr bool
	}{
		{name: "critical valid", level: lint.SeverityLevelCritical, wantErr: false},
		{name: "warning valid", level: lint.SeverityLevelWarning, wantErr: false},
		{name: "info valid", level: lint.SeverityLevelInfo, wantErr: false},
		{name: "empty invalid", level: "", wantErr: true},
		{name: "unknown invalid", level: "high", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.level.Validate()

			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).ToNot(HaveOccurred())
			}
		})
	}
}

func makeCondition(impact result.Impact, msg string) result.Condition {
	status := metav1.ConditionTrue
	switch impact {
	case result.ImpactBlocking:
		status = metav1.ConditionFalse
	case result.ImpactAdvisory:
		status = metav1.ConditionFalse
	case result.ImpactNone:
		// status stays ConditionTrue
	}

	return result.Condition{
		Condition: metav1.Condition{
			Type:    "Validated",
			Status:  status,
			Reason:  "TestReason",
			Message: msg,
		},
		Impact: impact,
	}
}

func makeExec(kind string, conditions ...result.Condition) check.CheckExecution {
	return check.CheckExecution{
		Result: &result.DiagnosticResult{
			Group: "components",
			Kind:  kind,
			Name:  "test-check",
			Status: result.DiagnosticStatus{
				Conditions: conditions,
			},
		},
	}
}

func TestFilterBySeverity_InfoReturnsAll(t *testing.T) {
	g := NewWithT(t)

	results := []check.CheckExecution{
		makeExec("kserve", makeCondition(result.ImpactBlocking, "crit")),
		makeExec("dashboard", makeCondition(result.ImpactAdvisory, "warn")),
		makeExec("notebook", makeCondition(result.ImpactNone, "info")),
	}

	filtered := lint.FilterBySeverity(results, lint.SeverityLevelInfo)

	g.Expect(filtered).To(HaveLen(3))
}

func TestFilterBySeverity_WarningHidesInfo(t *testing.T) {
	g := NewWithT(t)

	results := []check.CheckExecution{
		makeExec("kserve", makeCondition(result.ImpactBlocking, "crit")),
		makeExec("dashboard", makeCondition(result.ImpactAdvisory, "warn")),
		makeExec("notebook", makeCondition(result.ImpactNone, "info")),
	}

	filtered := lint.FilterBySeverity(results, lint.SeverityLevelWarning)

	g.Expect(filtered).To(HaveLen(2))
	g.Expect(filtered[0].Result.Kind).To(Equal("kserve"))
	g.Expect(filtered[1].Result.Kind).To(Equal("dashboard"))
}

func TestFilterBySeverity_CriticalHidesWarningAndInfo(t *testing.T) {
	g := NewWithT(t)

	results := []check.CheckExecution{
		makeExec("kserve", makeCondition(result.ImpactBlocking, "crit")),
		makeExec("dashboard", makeCondition(result.ImpactAdvisory, "warn")),
		makeExec("notebook", makeCondition(result.ImpactNone, "info")),
	}

	filtered := lint.FilterBySeverity(results, lint.SeverityLevelCritical)

	g.Expect(filtered).To(HaveLen(1))
	g.Expect(filtered[0].Result.Kind).To(Equal("kserve"))
}

func TestFilterBySeverity_MixedConditionsPartiallyFiltered(t *testing.T) {
	g := NewWithT(t)

	results := []check.CheckExecution{
		makeExec("kserve",
			makeCondition(result.ImpactBlocking, "crit-condition"),
			makeCondition(result.ImpactNone, "info-condition"),
		),
	}

	filtered := lint.FilterBySeverity(results, lint.SeverityLevelWarning)

	g.Expect(filtered).To(HaveLen(1))
	g.Expect(filtered[0].Result.Status.Conditions).To(HaveLen(1))
	g.Expect(filtered[0].Result.Status.Conditions[0].Message).To(Equal("crit-condition"))
}

func TestFilterBySeverity_AllConditionsFilteredDropsResult(t *testing.T) {
	g := NewWithT(t)

	results := []check.CheckExecution{
		makeExec("dashboard",
			makeCondition(result.ImpactNone, "info-1"),
			makeCondition(result.ImpactNone, "info-2"),
		),
	}

	filtered := lint.FilterBySeverity(results, lint.SeverityLevelWarning)

	g.Expect(filtered).To(BeEmpty())
}

func TestFilterBySeverity_DoesNotMutateOriginal(t *testing.T) {
	g := NewWithT(t)

	results := []check.CheckExecution{
		makeExec("kserve",
			makeCondition(result.ImpactBlocking, "crit"),
			makeCondition(result.ImpactNone, "info"),
		),
	}

	_ = lint.FilterBySeverity(results, lint.SeverityLevelCritical)

	g.Expect(results[0].Result.Status.Conditions).To(HaveLen(2))
}

func TestFilterBySeverity_NilResultSkipped(t *testing.T) {
	g := NewWithT(t)

	results := []check.CheckExecution{
		{Result: nil},
		makeExec("kserve", makeCondition(result.ImpactBlocking, "crit")),
	}

	filtered := lint.FilterBySeverity(results, lint.SeverityLevelCritical)

	g.Expect(filtered).To(HaveLen(1))
	g.Expect(filtered[0].Result.Kind).To(Equal("kserve"))
}
