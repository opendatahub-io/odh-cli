package dscinitialization_test

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	resultpkg "github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/testutil"
	"github.com/opendatahub-io/odh-cli/pkg/lint/checks/platform/dscinitialization"
	"github.com/opendatahub-io/odh-cli/pkg/resources"

	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

// Test constants for DSCInitialization readiness check.
//
//nolint:gochecknoglobals // Test fixtures shared across test functions in this file.
var dsciReadinessListKinds = map[schema.GroupVersionResource]string{
	resources.DSCInitialization.GVR(): resources.DSCInitialization.ListKind(),
}

func newDSCIWithPhase(phase string) *unstructured.Unstructured {
	obj := map[string]any{
		"apiVersion": resources.DSCInitialization.APIVersion(),
		"kind":       resources.DSCInitialization.Kind,
		"metadata": map[string]any{
			"name": "default-dsci",
		},
	}
	if phase != "" {
		obj["status"] = map[string]any{
			"phase": phase,
		}
	}

	return &unstructured.Unstructured{Object: obj}
}

func newDSCIWithEmptyPhase() *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.DSCInitialization.APIVersion(),
			"kind":       resources.DSCInitialization.Kind,
			"metadata": map[string]any{
				"name": "default-dsci",
			},
			"status": map[string]any{
				"phase": "",
			},
		},
	}
}

func newDSCIWithoutPhase() *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.DSCInitialization.APIVersion(),
			"kind":       resources.DSCInitialization.Kind,
			"metadata": map[string]any{
				"name": "default-dsci",
			},
			"status": map[string]any{},
		},
	}
}

func TestDSCInitializationReadinessCheck_NoDSCI(t *testing.T) {
	g := NewWithT(t)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds: dsciReadinessListKinds,
	})

	chk := dscinitialization.NewDSCInitializationReadinessCheck()
	result, err := chk.Validate(t.Context(), target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(check.ConditionTypeAvailable),
		"Status": Equal(metav1.ConditionFalse),
		"Reason": Equal(check.ReasonResourceNotFound),
	}))
}

func TestDSCInitializationReadinessCheck_PhaseMissing(t *testing.T) {
	g := NewWithT(t)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds: dsciReadinessListKinds,
		Objects:   []*unstructured.Unstructured{newDSCIWithoutPhase()},
	})

	chk := dscinitialization.NewDSCInitializationReadinessCheck()
	result, err := chk.Validate(t.Context(), target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeReady),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonInsufficientData),
		"Message": ContainSubstring("phase field is missing"),
	}))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactBlocking))
}

func TestDSCInitializationReadinessCheck_PhaseEmpty(t *testing.T) {
	g := NewWithT(t)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds: dsciReadinessListKinds,
		Objects:   []*unstructured.Unstructured{newDSCIWithEmptyPhase()},
	})

	chk := dscinitialization.NewDSCInitializationReadinessCheck()
	result, err := chk.Validate(t.Context(), target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeReady),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonInsufficientData),
		"Message": ContainSubstring("phase is empty"),
	}))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactBlocking))
}

func TestDSCInitializationReadinessCheck_NotReady(t *testing.T) {
	g := NewWithT(t)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds: dsciReadinessListKinds,
		Objects:   []*unstructured.Unstructured{newDSCIWithPhase("Degraded")},
	})

	chk := dscinitialization.NewDSCInitializationReadinessCheck()
	result, err := chk.Validate(t.Context(), target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeReady),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonResourceUnavailable),
		"Message": And(ContainSubstring("DSCInitialization"), ContainSubstring("Degraded")),
	}))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactBlocking))
}

func TestDSCInitializationReadinessCheck_NotReadyWithUnknownCondition(t *testing.T) {
	g := NewWithT(t)

	dsci := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.DSCInitialization.APIVersion(),
			"kind":       resources.DSCInitialization.Kind,
			"metadata": map[string]any{
				"name": "default-dsci",
			},
			"status": map[string]any{
				"phase": "Ready",
				"conditions": []any{
					map[string]any{"type": "ReconcileComplete", "status": "Unknown", "message": "reconcile state unknown"},
				},
			},
		},
	}

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds: dsciReadinessListKinds,
		Objects:   []*unstructured.Unstructured{dsci},
	})

	chk := dscinitialization.NewDSCInitializationReadinessCheck()
	result, err := chk.Validate(t.Context(), target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeReady),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonResourceUnavailable),
		"Message": ContainSubstring("ReconcileComplete: reconcile state unknown"),
	}))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactBlocking))
}

func TestDSCInitializationReadinessCheck_NotReadyWithUnhappyConditions(t *testing.T) {
	g := NewWithT(t)

	dsci := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.DSCInitialization.APIVersion(),
			"kind":       resources.DSCInitialization.Kind,
			"metadata": map[string]any{
				"name": "default-dsci",
			},
			"status": map[string]any{
				"phase": "Degraded",
				"conditions": []any{
					map[string]any{"type": "ReconcileComplete", "status": "False", "message": "reconcile failed: some error"},
					map[string]any{"type": "Degraded", "status": "True", "message": "operator is degraded"},
					map[string]any{"type": "Progressing", "status": "False", "message": "Reconcile completed successfully"},
					map[string]any{"type": "Available", "status": "True", "message": "Reconcile completed successfully"},
				},
			},
		},
	}

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds: dsciReadinessListKinds,
		Objects:   []*unstructured.Unstructured{dsci},
	})

	chk := dscinitialization.NewDSCInitializationReadinessCheck()
	result, err := chk.Validate(t.Context(), target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(check.ConditionTypeReady),
		"Status": Equal(metav1.ConditionFalse),
		"Reason": Equal(check.ReasonResourceUnavailable),
		"Message": And(
			ContainSubstring("DSCInitialization"),
			ContainSubstring("Degraded"),
			ContainSubstring("ReconcileComplete: reconcile failed: some error"),
			ContainSubstring("Degraded: operator is degraded"),
		),
	}))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactBlocking))
}

func TestDSCInitializationReadinessCheck_ReadyWithRemovedCapabilities(t *testing.T) {
	g := NewWithT(t)

	dsci := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.DSCInitialization.APIVersion(),
			"kind":       resources.DSCInitialization.Kind,
			"metadata": map[string]any{
				"name": "default-dsci",
			},
			"status": map[string]any{
				"phase": "Ready",
				"conditions": []any{
					map[string]any{"type": "ReconcileComplete", "status": "True", "reason": "ReconcileCompleted", "message": "reconcile completed"},
					map[string]any{"type": "CapabilityServiceMesh", "status": "False", "reason": "Removed", "message": "service mesh removed"},
					map[string]any{"type": "CapabilityServiceMeshAuthorization", "status": "False", "reason": "Removed", "message": "service mesh authorization removed"},
				},
			},
		},
	}

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds: dsciReadinessListKinds,
		Objects:   []*unstructured.Unstructured{dsci},
	})

	chk := dscinitialization.NewDSCInitializationReadinessCheck()
	result, err := chk.Validate(t.Context(), target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeReady),
		"Status":  Equal(metav1.ConditionTrue),
		"Reason":  Equal(check.ReasonResourceAvailable),
		"Message": ContainSubstring("DSCInitialization is ready"),
	}))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactNone))
}

func TestDSCInitializationReadinessCheck_Ready(t *testing.T) {
	g := NewWithT(t)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds: dsciReadinessListKinds,
		Objects:   []*unstructured.Unstructured{newDSCIWithPhase("Ready")},
	})

	chk := dscinitialization.NewDSCInitializationReadinessCheck()
	result, err := chk.Validate(t.Context(), target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeReady),
		"Status":  Equal(metav1.ConditionTrue),
		"Reason":  Equal(check.ReasonResourceAvailable),
		"Message": ContainSubstring("DSCInitialization is ready"),
	}))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactNone))
}

func TestDSCInitializationReadinessCheck_ReadyWithUnhappyConditions(t *testing.T) {
	g := NewWithT(t)

	dsci := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.DSCInitialization.APIVersion(),
			"kind":       resources.DSCInitialization.Kind,
			"metadata": map[string]any{
				"name": "default-dsci",
			},
			"status": map[string]any{
				"phase": "Ready",
				"conditions": []any{
					map[string]any{"type": "ReconcileComplete", "status": "False", "message": "reconcile failed: some error"},
					map[string]any{"type": "Degraded", "status": "False", "message": "not degraded"},
					map[string]any{"type": "Progressing", "status": "False", "message": "not progressing"},
					map[string]any{"type": "Available", "status": "True", "message": "available"},
				},
			},
		},
	}

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds: dsciReadinessListKinds,
		Objects:   []*unstructured.Unstructured{dsci},
	})

	chk := dscinitialization.NewDSCInitializationReadinessCheck()
	result, err := chk.Validate(t.Context(), target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeReady),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonResourceUnavailable),
		"Message": And(ContainSubstring("phase is Ready"), ContainSubstring("ReconcileComplete: reconcile failed: some error")),
	}))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactBlocking))
}

func TestDSCInitializationReadinessCheck_Metadata(t *testing.T) {
	g := NewWithT(t)

	chk := dscinitialization.NewDSCInitializationReadinessCheck()

	g.Expect(chk.ID()).To(Equal("platform.dsci.readiness"))
	g.Expect(chk.Name()).To(Equal("Platform :: DSCI :: Readiness Check"))
	g.Expect(chk.Group()).To(Equal(check.GroupPlatform))
	g.Expect(chk.Description()).ToNot(BeEmpty())
}
