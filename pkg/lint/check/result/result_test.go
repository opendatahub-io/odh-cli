package result_test

import (
	"testing"

	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/resources"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

// T014: DiagnosticResult struct creation tests

func TestNewDiagnosticResult_ValidCreation(t *testing.T) {
	g := NewWithT(t)

	dr := result.New(
		"components",
		"kserve",
		"version-compatibility",
		"Validates KServe version compatibility",
	)

	g.Expect(dr).ToNot(BeNil())
	g.Expect(dr.Group).To(Equal("components"))
	g.Expect(dr.Kind).To(Equal("kserve"))
	g.Expect(dr.Name).To(Equal("version-compatibility"))
	g.Expect(dr.Spec.Description).To(Equal("Validates KServe version compatibility"))
	g.Expect(dr.Annotations).ToNot(BeNil())
	g.Expect(dr.Status.Conditions).ToNot(BeNil())
	g.Expect(dr.Status.Conditions).To(BeEmpty())
}

func TestDiagnosticResult_EmptyDescription(t *testing.T) {
	g := NewWithT(t)

	dr := result.New(
		"components",
		"kserve",
		"configuration-valid",
		"",
	)

	g.Expect(dr.Spec.Description).To(BeEmpty())
}

func TestDiagnosticResult_WithAnnotations(t *testing.T) {
	g := NewWithT(t)

	dr := result.New(
		"components",
		"kserve",
		"version-compatibility",
		"Validates KServe version compatibility",
	)

	dr.Annotations["check.opendatahub.io/source-version"] = "2.15"
	dr.Annotations["check.opendatahub.io/target-version"] = "3.0"

	g.Expect(dr.Annotations).To(HaveLen(2))
	g.Expect(dr.Annotations["check.opendatahub.io/source-version"]).To(Equal("2.15"))
	g.Expect(dr.Annotations["check.opendatahub.io/target-version"]).To(Equal("3.0"))
}

func TestDiagnosticResult_WithSingleCondition(t *testing.T) {
	g := NewWithT(t)

	dr := result.New(
		"components",
		"kserve",
		"version-compatibility",
		"Validates KServe version compatibility",
	)

	dr.Status.Conditions = append(dr.Status.Conditions, result.Condition{Condition: metav1.Condition{
		Type:               check.ConditionTypeValidated,
		Status:             metav1.ConditionTrue,
		Reason:             check.ReasonRequirementsMet,
		Message:            "All version requirements met",
		LastTransitionTime: metav1.Now(),
	}})

	g.Expect(dr.Status.Conditions).To(HaveLen(1))
	g.Expect(dr.Status.Conditions[0].Type).To(Equal(check.ConditionTypeValidated))
	g.Expect(dr.Status.Conditions[0].Status).To(Equal(metav1.ConditionTrue))
	g.Expect(dr.Status.Conditions[0].Reason).To(Equal(check.ReasonRequirementsMet))
}

func TestDiagnosticResult_WithMultipleConditions(t *testing.T) {
	g := NewWithT(t)

	dr := result.New(
		"services",
		"auth",
		"readiness-check",
		"Validates authentication service readiness",
	)

	dr.Status.Conditions = []result.Condition{
		{Condition: metav1.Condition{
			Type:               check.ConditionTypeAvailable,
			Status:             metav1.ConditionTrue,
			Reason:             check.ReasonResourceFound,
			Message:            "Authentication service deployment found",
			LastTransitionTime: metav1.Now(),
		}},
		{Condition: metav1.Condition{
			Type:               check.ConditionTypeReady,
			Status:             metav1.ConditionTrue,
			Reason:             "PodsReady",
			Message:            "All auth service pods are ready (3/3)",
			LastTransitionTime: metav1.Now(),
		}},
		{Condition: metav1.Condition{
			Type:               check.ConditionTypeConfigured,
			Status:             metav1.ConditionTrue,
			Reason:             "ConfigValid",
			Message:            "Authentication provider configuration is valid",
			LastTransitionTime: metav1.Now(),
		}},
	}

	g.Expect(dr.Status.Conditions).To(HaveLen(3))
	g.Expect(dr.Status.Conditions[0].Type).To(Equal(check.ConditionTypeAvailable))
	g.Expect(dr.Status.Conditions[1].Type).To(Equal(check.ConditionTypeReady))
	g.Expect(dr.Status.Conditions[2].Type).To(Equal(check.ConditionTypeConfigured))
}

func TestDiagnosticResult_Validate_Success(t *testing.T) {
	g := NewWithT(t)

	dr := result.New(
		"components",
		"kserve",
		"version-compatibility",
		"Validates KServe version compatibility",
	)

	dr.Status.Conditions = append(dr.Status.Conditions, result.Condition{Condition: metav1.Condition{
		Type:               check.ConditionTypeValidated,
		Status:             metav1.ConditionTrue,
		Reason:             check.ReasonRequirementsMet,
		Message:            "All version requirements met",
		LastTransitionTime: metav1.Now(),
	}})

	err := dr.Validate()
	g.Expect(err).ToNot(HaveOccurred())
}

func TestDiagnosticResult_Validate_EmptyGroup(t *testing.T) {
	g := NewWithT(t)

	dr := result.New(
		"",
		"kserve",
		"version-compatibility",
		"Validates KServe version compatibility",
	)

	dr.Status.Conditions = append(dr.Status.Conditions, result.Condition{Condition: metav1.Condition{
		Type:               check.ConditionTypeValidated,
		Status:             metav1.ConditionTrue,
		Reason:             check.ReasonRequirementsMet,
		Message:            "All version requirements met",
		LastTransitionTime: metav1.Now(),
	}})

	err := dr.Validate()
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("group must not be empty"))
}

func TestDiagnosticResult_Validate_EmptyKind(t *testing.T) {
	g := NewWithT(t)

	dr := result.New(
		"components",
		"",
		"version-compatibility",
		"Validates KServe version compatibility",
	)

	dr.Status.Conditions = append(dr.Status.Conditions, result.Condition{Condition: metav1.Condition{
		Type:               check.ConditionTypeValidated,
		Status:             metav1.ConditionTrue,
		Reason:             check.ReasonRequirementsMet,
		Message:            "All version requirements met",
		LastTransitionTime: metav1.Now(),
	}})

	err := dr.Validate()
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("kind must not be empty"))
}

func TestDiagnosticResult_Validate_EmptyName(t *testing.T) {
	g := NewWithT(t)

	dr := result.New(
		"components",
		"kserve",
		"",
		"Validates KServe version compatibility",
	)

	dr.Status.Conditions = append(dr.Status.Conditions, result.Condition{Condition: metav1.Condition{
		Type:               check.ConditionTypeValidated,
		Status:             metav1.ConditionTrue,
		Reason:             check.ReasonRequirementsMet,
		Message:            "All version requirements met",
		LastTransitionTime: metav1.Now(),
	}})

	err := dr.Validate()
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("name must not be empty"))
}

func TestDiagnosticResult_Validate_EmptyConditions(t *testing.T) {
	g := NewWithT(t)

	dr := result.New(
		"components",
		"kserve",
		"version-compatibility",
		"Validates KServe version compatibility",
	)

	// No conditions added
	err := dr.Validate()
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("status.conditions must contain at least one condition"))
}

func TestDiagnosticResult_Validate_EmptyConditionType(t *testing.T) {
	g := NewWithT(t)

	dr := result.New(
		"components",
		"kserve",
		"version-compatibility",
		"Validates KServe version compatibility",
	)

	dr.Status.Conditions = append(dr.Status.Conditions, result.Condition{Condition: metav1.Condition{
		Type:               "",
		Status:             metav1.ConditionTrue,
		Reason:             check.ReasonRequirementsMet,
		Message:            "All version requirements met",
		LastTransitionTime: metav1.Now(),
	}})

	err := dr.Validate()
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("condition with empty type found"))
}

func TestDiagnosticResult_Validate_InvalidConditionStatus(t *testing.T) {
	g := NewWithT(t)

	dr := result.New(
		"components",
		"kserve",
		"version-compatibility",
		"Validates KServe version compatibility",
	)

	dr.Status.Conditions = append(dr.Status.Conditions, result.Condition{Condition: metav1.Condition{
		Type:               check.ConditionTypeValidated,
		Status:             "Invalid",
		Reason:             check.ReasonRequirementsMet,
		Message:            "All version requirements met",
		LastTransitionTime: metav1.Now(),
	}})

	err := dr.Validate()
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("has invalid status"))
}

func TestDiagnosticResult_Validate_EmptyConditionReason(t *testing.T) {
	g := NewWithT(t)

	dr := result.New(
		"components",
		"kserve",
		"version-compatibility",
		"Validates KServe version compatibility",
	)

	dr.Status.Conditions = append(dr.Status.Conditions, result.Condition{Condition: metav1.Condition{
		Type:               check.ConditionTypeValidated,
		Status:             metav1.ConditionTrue,
		Reason:             "",
		Message:            "All version requirements met",
		LastTransitionTime: metav1.Now(),
	}})

	err := dr.Validate()
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("has empty reason"))
}

// T023: Multiple conditions array tests

func TestMultipleConditions_OrderedByExecutionSequence(t *testing.T) {
	g := NewWithT(t)

	dr := result.New(
		"services",
		"auth",
		"comprehensive-check",
		"Validates authentication service health",
	)

	// Conditions should be added in execution order
	dr.Status.Conditions = []result.Condition{
		{Condition: metav1.Condition{
			Type:               check.ConditionTypeAvailable,
			Status:             metav1.ConditionTrue,
			Reason:             check.ReasonResourceFound,
			Message:            "Step 1: Resource found",
			LastTransitionTime: metav1.Now(),
		}},
		{Condition: metav1.Condition{
			Type:               check.ConditionTypeReady,
			Status:             metav1.ConditionTrue,
			Reason:             "PodsReady",
			Message:            "Step 2: Pods ready",
			LastTransitionTime: metav1.Now(),
		}},
		{Condition: metav1.Condition{
			Type:               check.ConditionTypeConfigured,
			Status:             metav1.ConditionTrue,
			Reason:             check.ReasonConfigurationValid,
			Message:            "Step 3: Configuration valid",
			LastTransitionTime: metav1.Now(),
		}},
	}

	g.Expect(dr.Status.Conditions).To(HaveLen(3))
	g.Expect(dr.Status.Conditions[0].Type).To(Equal(check.ConditionTypeAvailable))
	g.Expect(dr.Status.Conditions[1].Type).To(Equal(check.ConditionTypeReady))
	g.Expect(dr.Status.Conditions[2].Type).To(Equal(check.ConditionTypeConfigured))
}

func TestMultipleConditions_MixedStatusValues(t *testing.T) {
	g := NewWithT(t)

	dr := result.New(
		"components",
		"kserve",
		"health-check",
		"Validates KServe health",
	)

	dr.Status.Conditions = []result.Condition{
		{Condition: metav1.Condition{
			Type:               check.ConditionTypeAvailable,
			Status:             metav1.ConditionTrue,
			Reason:             check.ReasonResourceFound,
			Message:            "KServe deployment found",
			LastTransitionTime: metav1.Now(),
		}},
		{Condition: metav1.Condition{
			Type:               check.ConditionTypeReady,
			Status:             metav1.ConditionFalse,
			Reason:             "PodsNotReady",
			Message:            "2 of 3 pods not ready",
			LastTransitionTime: metav1.Now(),
		}},
		{Condition: metav1.Condition{
			Type:               check.ConditionTypeConfigured,
			Status:             metav1.ConditionTrue,
			Reason:             check.ReasonConfigurationValid,
			Message:            "Configuration is valid",
			LastTransitionTime: metav1.Now(),
		}},
	}

	g.Expect(dr.Status.Conditions).To(HaveLen(3))
	g.Expect(dr.Status.Conditions[0].Status).To(Equal(metav1.ConditionTrue))
	g.Expect(dr.Status.Conditions[1].Status).To(Equal(metav1.ConditionFalse))
	g.Expect(dr.Status.Conditions[2].Status).To(Equal(metav1.ConditionTrue))
}

func TestMultipleConditions_AllPassing(t *testing.T) {
	g := NewWithT(t)

	dr := result.New(
		"components",
		"kserve",
		"readiness",
		"Validates KServe readiness",
	)

	dr.Status.Conditions = []result.Condition{
		{Condition: metav1.Condition{
			Type:               check.ConditionTypeAvailable,
			Status:             metav1.ConditionTrue,
			Reason:             check.ReasonResourceFound,
			Message:            "Deployment found",
			LastTransitionTime: metav1.Now(),
		}},
		{Condition: metav1.Condition{
			Type:               check.ConditionTypeReady,
			Status:             metav1.ConditionTrue,
			Reason:             "PodsReady",
			Message:            "All pods ready",
			LastTransitionTime: metav1.Now(),
		}},
		{Condition: metav1.Condition{
			Type:               check.ConditionTypeConfigured,
			Status:             metav1.ConditionTrue,
			Reason:             check.ReasonConfigurationValid,
			Message:            "Configuration valid",
			LastTransitionTime: metav1.Now(),
		}},
	}

	allPassing := true
	for _, cond := range dr.Status.Conditions {
		if cond.Status != metav1.ConditionTrue {
			allPassing = false

			break
		}
	}

	g.Expect(allPassing).To(BeTrue())
}

func TestMultipleConditions_AllFailing(t *testing.T) {
	g := NewWithT(t)

	dr := result.New(
		"components",
		"kserve",
		"readiness",
		"Validates KServe readiness",
	)

	dr.Status.Conditions = []result.Condition{
		{Condition: metav1.Condition{
			Type:               check.ConditionTypeAvailable,
			Status:             metav1.ConditionFalse,
			Reason:             check.ReasonResourceNotFound,
			Message:            "Deployment not found",
			LastTransitionTime: metav1.Now(),
		}},
		{Condition: metav1.Condition{
			Type:               check.ConditionTypeReady,
			Status:             metav1.ConditionFalse,
			Reason:             "PodsNotReady",
			Message:            "No pods ready",
			LastTransitionTime: metav1.Now(),
		}},
		{Condition: metav1.Condition{
			Type:               check.ConditionTypeConfigured,
			Status:             metav1.ConditionFalse,
			Reason:             check.ReasonConfigurationInvalid,
			Message:            "Configuration invalid",
			LastTransitionTime: metav1.Now(),
		}},
	}

	allFailing := true
	for _, cond := range dr.Status.Conditions {
		if cond.Status != metav1.ConditionFalse {
			allFailing = false

			break
		}
	}

	g.Expect(allFailing).To(BeTrue())
}

func TestMultipleConditions_WithUnknownStatus(t *testing.T) {
	g := NewWithT(t)

	dr := result.New(
		"components",
		"kserve",
		"health-check",
		"Validates KServe health",
	)

	dr.Status.Conditions = []result.Condition{
		{Condition: metav1.Condition{
			Type:               check.ConditionTypeAvailable,
			Status:             metav1.ConditionTrue,
			Reason:             check.ReasonResourceFound,
			Message:            "Deployment found",
			LastTransitionTime: metav1.Now(),
		}},
		{Condition: metav1.Condition{
			Type:               check.ConditionTypeReady,
			Status:             metav1.ConditionUnknown,
			Reason:             check.ReasonCheckExecutionFailed,
			Message:            "Failed to query pod status",
			LastTransitionTime: metav1.Now(),
		}},
		{Condition: metav1.Condition{
			Type:               check.ConditionTypeConfigured,
			Status:             metav1.ConditionTrue,
			Reason:             check.ReasonConfigurationValid,
			Message:            "Configuration valid",
			LastTransitionTime: metav1.Now(),
		}},
	}

	hasUnknown := false
	for _, cond := range dr.Status.Conditions {
		if cond.Status == metav1.ConditionUnknown {
			hasUnknown = true

			break
		}
	}

	g.Expect(hasUnknown).To(BeTrue())
}

func TestMultipleConditions_AppendingNewConditions(t *testing.T) {
	g := NewWithT(t)

	dr := result.New(
		"services",
		"auth",
		"multi-step-check",
		"Multi-step validation",
	)

	// Start with empty conditions
	g.Expect(dr.Status.Conditions).To(BeEmpty())

	// Add conditions incrementally
	dr.Status.Conditions = append(dr.Status.Conditions, result.Condition{Condition: metav1.Condition{
		Type:               check.ConditionTypeAvailable,
		Status:             metav1.ConditionTrue,
		Reason:             check.ReasonResourceFound,
		Message:            "First condition",
		LastTransitionTime: metav1.Now(),
	}})

	g.Expect(dr.Status.Conditions).To(HaveLen(1))

	dr.Status.Conditions = append(dr.Status.Conditions, result.Condition{Condition: metav1.Condition{
		Type:               check.ConditionTypeReady,
		Status:             metav1.ConditionTrue,
		Reason:             "PodsReady",
		Message:            "Second condition",
		LastTransitionTime: metav1.Now(),
	}})

	g.Expect(dr.Status.Conditions).To(HaveLen(2))

	dr.Status.Conditions = append(dr.Status.Conditions, result.Condition{Condition: metav1.Condition{
		Type:               check.ConditionTypeConfigured,
		Status:             metav1.ConditionTrue,
		Reason:             check.ReasonConfigurationValid,
		Message:            "Third condition",
		LastTransitionTime: metav1.Now(),
	}})

	g.Expect(dr.Status.Conditions).To(HaveLen(3))
}

func TestMultipleConditions_ValidationSucceedsWithMultiple(t *testing.T) {
	g := NewWithT(t)

	dr := result.New(
		"components",
		"kserve",
		"multi-check",
		"Multi-condition validation",
	)

	dr.Status.Conditions = []result.Condition{
		{Condition: metav1.Condition{
			Type:               check.ConditionTypeAvailable,
			Status:             metav1.ConditionTrue,
			Reason:             check.ReasonResourceFound,
			Message:            "Available",
			LastTransitionTime: metav1.Now(),
		}},
		{Condition: metav1.Condition{
			Type:               check.ConditionTypeReady,
			Status:             metav1.ConditionTrue,
			Reason:             "PodsReady",
			Message:            "Ready",
			LastTransitionTime: metav1.Now(),
		}},
	}

	err := dr.Validate()
	g.Expect(err).ToNot(HaveOccurred())
}

// T055: Test validation for annotation format.
func TestDiagnosticResult_ValidateAnnotationFormat(t *testing.T) {
	g := NewWithT(t)

	tests := []struct {
		name        string
		annotations map[string]string
		shouldError bool
		errorMsg    string
	}{
		{
			name: "valid annotation keys",
			annotations: map[string]string{
				"openshiftai.io/version": "2.17.0",
				"example.com/check-id":   "test-123",
			},
			shouldError: false,
		},
		{
			name: "invalid - missing domain",
			annotations: map[string]string{
				"version": "2.17.0",
			},
			shouldError: true,
			errorMsg:    "must be in domain/key format",
		},
		{
			name: "invalid - missing key",
			annotations: map[string]string{
				"openshiftai.io/": "value",
			},
			shouldError: true,
			errorMsg:    "must be in domain/key format",
		},
		{
			name: "invalid - no slash",
			annotations: map[string]string{
				"version-2.17.0": "value",
			},
			shouldError: true,
			errorMsg:    "must be in domain/key format",
		},
		{
			name: "invalid - domain without dot",
			annotations: map[string]string{
				"openshiftai/version": "2.17.0",
			},
			shouldError: true,
			errorMsg:    "must be in domain/key format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dr := result.New("components", "kserve", "version", "Test annotation validation")
			dr.Annotations = tt.annotations
			dr.Status.Conditions = append(dr.Status.Conditions, result.Condition{Condition: metav1.Condition{
				Type:               check.ConditionTypeValidated,
				Status:             metav1.ConditionTrue,
				Reason:             check.ReasonRequirementsMet,
				Message:            "Test condition",
				LastTransitionTime: metav1.Now(),
			}})

			err := dr.Validate()
			if tt.shouldError {
				g.Expect(err).To(HaveOccurred())
				g.Expect(err.Error()).To(ContainSubstring(tt.errorMsg))
			} else {
				g.Expect(err).ToNot(HaveOccurred())
			}
		})
	}
}

// T055: Test validation for required fields with annotations present.
func TestDiagnosticResult_ValidateRequiredFieldsWithAnnotations(t *testing.T) {
	g := NewWithT(t)

	tests := []struct {
		name     string
		group    string
		kind     string
		drName   string
		errorMsg string
	}{
		{
			name:     "missing group",
			group:    "",
			kind:     "kserve",
			drName:   "version",
			errorMsg: "group must not be empty",
		},
		{
			name:     "missing kind",
			group:    "components",
			kind:     "",
			drName:   "version",
			errorMsg: "kind must not be empty",
		},
		{
			name:     "missing name",
			group:    "components",
			kind:     "kserve",
			drName:   "",
			errorMsg: "name must not be empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dr := result.New(tt.group, tt.kind, tt.drName, "Test validation")
			dr.Annotations["valid.domain.io/key"] = "value"
			dr.Status.Conditions = append(dr.Status.Conditions, result.Condition{Condition: metav1.Condition{
				Type:               check.ConditionTypeValidated,
				Status:             metav1.ConditionTrue,
				Reason:             check.ReasonRequirementsMet,
				Message:            "Test condition",
				LastTransitionTime: metav1.Now(),
			}})

			err := dr.Validate()
			g.Expect(err).To(HaveOccurred())
			g.Expect(err.Error()).To(Equal(tt.errorMsg))
		})
	}
}

// SetCondition method tests

func TestSetCondition_AddNew(t *testing.T) {
	g := NewWithT(t)

	dr := result.New("component", "test", "check", "description")
	dr.SetCondition(check.NewCondition(check.ConditionTypeCompatible, metav1.ConditionTrue, check.WithReason("TestReason"), check.WithMessage("test message")))

	g.Expect(dr.Status.Conditions).To(HaveLen(1))
	g.Expect(dr.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeCompatible),
		"Status":  Equal(metav1.ConditionTrue),
		"Reason":  Equal("TestReason"),
		"Message": Equal("test message"),
	}))
}

func TestSetCondition_UpdateExisting(t *testing.T) {
	g := NewWithT(t)

	dr := result.New("component", "test", "check", "description")

	dr.SetCondition(check.NewCondition(check.ConditionTypeCompatible, metav1.ConditionTrue, check.WithReason("reason1"), check.WithMessage("message1")))
	g.Expect(dr.Status.Conditions).To(HaveLen(1))

	dr.SetCondition(check.NewCondition(check.ConditionTypeCompatible, metav1.ConditionFalse, check.WithReason("reason2"), check.WithMessage("message2")))
	g.Expect(dr.Status.Conditions).To(HaveLen(1))
	g.Expect(dr.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal("reason2"),
		"Message": Equal("message2"),
	}))
}

func TestSetCondition_MultipleConditionTypes(t *testing.T) {
	g := NewWithT(t)

	dr := result.New("component", "test", "check", "description")

	dr.SetCondition(check.NewCondition(check.ConditionTypeCompatible, metav1.ConditionTrue, check.WithReason("reason1"), check.WithMessage("message1")))
	g.Expect(dr.Status.Conditions).To(HaveLen(1))

	dr.SetCondition(check.NewCondition(check.ConditionTypeAvailable, metav1.ConditionTrue, check.WithReason("reason2"), check.WithMessage("message2")))
	g.Expect(dr.Status.Conditions).To(HaveLen(2))

	dr.SetCondition(check.NewCondition(check.ConditionTypeCompatible, metav1.ConditionFalse, check.WithReason("reason3"), check.WithMessage("message3")))
	g.Expect(dr.Status.Conditions).To(HaveLen(2))

	g.Expect(dr.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(check.ConditionTypeCompatible),
		"Status": Equal(metav1.ConditionFalse),
	}))
	g.Expect(dr.Status.Conditions[1].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(check.ConditionTypeAvailable),
		"Status": Equal(metav1.ConditionTrue),
	}))
}

// SetImpactedObjects and AddImpactedObjects tests

func TestSetImpactedObjects(t *testing.T) {
	g := NewWithT(t)

	dr := result.New("component", "test", "check", "description")

	names := []types.NamespacedName{
		{Namespace: "ns1", Name: "obj1"},
		{Namespace: "ns2", Name: "obj2"},
	}

	dr.SetImpactedObjects(resources.Notebook, names)

	g.Expect(dr.ImpactedObjects).To(HaveLen(2))
	g.Expect(dr.ImpactedObjects[0].Name).To(Equal("obj1"))
	g.Expect(dr.ImpactedObjects[0].Namespace).To(Equal("ns1"))
	g.Expect(dr.ImpactedObjects[1].Name).To(Equal("obj2"))
	g.Expect(dr.ImpactedObjects[1].Namespace).To(Equal("ns2"))
}

func TestSetImpactedObjects_Replaces(t *testing.T) {
	g := NewWithT(t)

	dr := result.New("component", "test", "check", "description")

	dr.SetImpactedObjects(resources.Notebook, []types.NamespacedName{
		{Namespace: "ns1", Name: "obj1"},
	})
	g.Expect(dr.ImpactedObjects).To(HaveLen(1))

	dr.SetImpactedObjects(resources.Notebook, []types.NamespacedName{
		{Namespace: "ns2", Name: "obj2"},
		{Namespace: "ns3", Name: "obj3"},
	})
	g.Expect(dr.ImpactedObjects).To(HaveLen(2))
	g.Expect(dr.ImpactedObjects[0].Name).To(Equal("obj2"))
	g.Expect(dr.ImpactedObjects[1].Name).To(Equal("obj3"))
}

func TestAddImpactedObjects(t *testing.T) {
	g := NewWithT(t)

	dr := result.New("component", "test", "check", "description")

	dr.AddImpactedObjects(resources.Notebook, []types.NamespacedName{
		{Namespace: "ns1", Name: "obj1"},
	})
	g.Expect(dr.ImpactedObjects).To(HaveLen(1))

	dr.AddImpactedObjects(resources.Notebook, []types.NamespacedName{
		{Namespace: "ns2", Name: "obj2"},
		{Namespace: "ns3", Name: "obj3"},
	})
	g.Expect(dr.ImpactedObjects).To(HaveLen(3))
	g.Expect(dr.ImpactedObjects[0].Name).To(Equal("obj1"))
	g.Expect(dr.ImpactedObjects[1].Name).To(Equal("obj2"))
	g.Expect(dr.ImpactedObjects[2].Name).To(Equal("obj3"))
}
