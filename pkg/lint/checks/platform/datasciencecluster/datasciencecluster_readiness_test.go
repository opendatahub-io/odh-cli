package datasciencecluster_test

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	resultpkg "github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/testutil"
	"github.com/opendatahub-io/odh-cli/pkg/lint/checks/platform/datasciencecluster"
	"github.com/opendatahub-io/odh-cli/pkg/resources"

	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

// Test fixtures for DataScienceCluster readiness check.
//
//nolint:gochecknoglobals // Test fixtures shared across test functions in this file.
var dscReadinessListKinds = map[schema.GroupVersionResource]string{
	resources.DataScienceCluster.GVR(): resources.DataScienceCluster.ListKind(),
}

func newDSCWithoutConditions() *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.DataScienceCluster.APIVersion(),
			"kind":       resources.DataScienceCluster.Kind,
			"metadata": map[string]any{
				"name": "default-dsc",
			},
			"status": map[string]any{},
		},
	}
}

func newDSCWithReadyCondition(status string, message string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.DataScienceCluster.APIVersion(),
			"kind":       resources.DataScienceCluster.Kind,
			"metadata": map[string]any{
				"name": "default-dsc",
			},
			"status": map[string]any{
				"conditions": []any{
					map[string]any{
						"type":    "Ready",
						"status":  status,
						"reason":  "ReconcileCompleted",
						"message": message,
					},
				},
			},
		},
	}
}

func TestDataScienceClusterReadinessCheck_NoDSC(t *testing.T) {
	g := NewWithT(t)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds: dscReadinessListKinds,
	})

	chk := datasciencecluster.NewDataScienceClusterReadinessCheck()
	result, err := chk.Validate(t.Context(), target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(check.ConditionTypeAvailable),
		"Status": Equal(metav1.ConditionFalse),
		"Reason": Equal(check.ReasonResourceNotFound),
	}))
}

func TestDataScienceClusterReadinessCheck_NoReadyCondition(t *testing.T) {
	g := NewWithT(t)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds: dscReadinessListKinds,
		Objects:   []*unstructured.Unstructured{newDSCWithoutConditions()},
	})

	chk := datasciencecluster.NewDataScienceClusterReadinessCheck()
	result, err := chk.Validate(t.Context(), target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeReady),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonInsufficientData),
		"Message": ContainSubstring("Ready condition is missing"),
	}))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactBlocking))
}

func TestDataScienceClusterReadinessCheck_NotReady(t *testing.T) {
	g := NewWithT(t)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds: dscReadinessListKinds,
		Objects:   []*unstructured.Unstructured{newDSCWithReadyCondition("False", "reconcile failed")},
	})

	chk := datasciencecluster.NewDataScienceClusterReadinessCheck()
	result, err := chk.Validate(t.Context(), target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeReady),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonResourceUnavailable),
		"Message": And(ContainSubstring("DataScienceCluster"), ContainSubstring("reconcile failed")),
	}))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactBlocking))
}

func TestDataScienceClusterReadinessCheck_Ready(t *testing.T) {
	g := NewWithT(t)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds: dscReadinessListKinds,
		Objects:   []*unstructured.Unstructured{newDSCWithReadyCondition("True", "Reconcile completed successfully")},
	})

	chk := datasciencecluster.NewDataScienceClusterReadinessCheck()
	result, err := chk.Validate(t.Context(), target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeReady),
		"Status":  Equal(metav1.ConditionTrue),
		"Reason":  Equal(check.ReasonResourceAvailable),
		"Message": ContainSubstring("DataScienceCluster is ready"),
	}))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactNone))
}

func TestDataScienceClusterReadinessCheck_Metadata(t *testing.T) {
	g := NewWithT(t)

	chk := datasciencecluster.NewDataScienceClusterReadinessCheck()

	g.Expect(chk.ID()).To(Equal("platform.dsc.readiness"))
	g.Expect(chk.Name()).To(Equal("Platform :: DSC :: Readiness Check"))
	g.Expect(chk.Group()).To(Equal(check.GroupPlatform))
	g.Expect(chk.Description()).ToNot(BeEmpty())
}
