package codeflare_test

import (
	"testing"

	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	resultpkg "github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/testutil"
	"github.com/opendatahub-io/odh-cli/pkg/lint/checks/workloads/codeflare"
	"github.com/opendatahub-io/odh-cli/pkg/resources"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

//nolint:gochecknoglobals // Test fixture - shared across test functions
var listKinds = map[schema.GroupVersionResource]string{
	resources.AppWrapper.GVR():         resources.AppWrapper.ListKind(),
	resources.DataScienceCluster.GVR(): resources.DataScienceCluster.ListKind(),
}

func TestImpactedWorkloadsCheck_NoAppWrappers(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{testutil.NewDSC(map[string]string{"codeflare": "Managed"})},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := codeflare.NewImpactedWorkloadsCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(codeflare.ConditionTypeAppWrapperCompatible),
		"Status":  Equal(metav1.ConditionTrue),
		"Reason":  Equal(check.ReasonVersionCompatible),
		"Message": ContainSubstring("No AppWrapper(s) found"),
	}))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "0"))
	g.Expect(result.ImpactedObjects).To(BeEmpty())
}

func TestImpactedWorkloadsCheck_WithAppWrappers(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	aw1 := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.AppWrapper.APIVersion(),
			"kind":       resources.AppWrapper.Kind,
			"metadata": map[string]any{
				"name":      "my-appwrapper",
				"namespace": "test-ns",
			},
		},
	}

	dsc := testutil.NewDSC(map[string]string{"codeflare": "Managed"})
	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{aw1, dsc},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := codeflare.NewImpactedWorkloadsCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(codeflare.ConditionTypeAppWrapperCompatible),
		"Status": Equal(metav1.ConditionFalse),
		"Reason": Equal(check.ReasonVersionIncompatible),
		"Message": And(
			ContainSubstring("Found 1 AppWrapper workload CRs"),
			ContainSubstring("AppWrapper controller has been removed"),
		),
	}))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactAdvisory))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "1"))
	g.Expect(result.ImpactedObjects).To(HaveLen(1))
	g.Expect(result.ImpactedObjects[0].Name).To(Equal("my-appwrapper"))
	g.Expect(result.ImpactedObjects[0].Namespace).To(Equal("test-ns"))
}

func TestImpactedWorkloadsCheck_MultipleAppWrappers(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	aw1 := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.AppWrapper.APIVersion(),
			"kind":       resources.AppWrapper.Kind,
			"metadata": map[string]any{
				"name":      "aw-1",
				"namespace": "ns1",
			},
		},
	}

	aw2 := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.AppWrapper.APIVersion(),
			"kind":       resources.AppWrapper.Kind,
			"metadata": map[string]any{
				"name":      "aw-2",
				"namespace": "ns2",
			},
		},
	}

	dsc := testutil.NewDSC(map[string]string{"codeflare": "Managed"})
	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{aw1, aw2, dsc},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := codeflare.NewImpactedWorkloadsCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(codeflare.ConditionTypeAppWrapperCompatible),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonVersionIncompatible),
		"Message": ContainSubstring("Found 2 AppWrapper workload CRs"),
	}))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "2"))
	g.Expect(result.ImpactedObjects).To(HaveLen(2))
}

func TestImpactedWorkloadsCheck_Metadata(t *testing.T) {
	g := NewWithT(t)

	chk := codeflare.NewImpactedWorkloadsCheck()

	g.Expect(chk.ID()).To(Equal("workloads.codeflare.impacted-workloads"))
	g.Expect(chk.Name()).To(Equal("Workloads :: CodeFlare :: Impacted Workloads (3.x)"))
	g.Expect(chk.Group()).To(Equal(check.GroupWorkload))
	g.Expect(chk.Description()).ToNot(BeEmpty())
	g.Expect(chk.Remediation()).To(ContainSubstring("AppWrapper"))
}

func TestImpactedWorkloadsCheck_CanApply(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	chk := codeflare.NewImpactedWorkloadsCheck()

	// Should not apply when versions are nil
	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds: listKinds,
		Objects:   []*unstructured.Unstructured{testutil.NewDSC(map[string]string{"codeflare": "Managed"})},
	})
	canApply, err := chk.CanApply(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeFalse())

	// Should not apply for 2.x to 2.x
	target = testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{testutil.NewDSC(map[string]string{"codeflare": "Managed"})},
		CurrentVersion: "2.15.0",
		TargetVersion:  "2.17.0",
	})
	canApply, err = chk.CanApply(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeFalse())

	// Should apply for 2.x to 3.x with codeflare Managed
	target = testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{testutil.NewDSC(map[string]string{"codeflare": "Managed"})},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})
	canApply, err = chk.CanApply(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeTrue())

	// Should not apply for 2.x to 3.x with codeflare Removed
	target = testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{testutil.NewDSC(map[string]string{"codeflare": "Removed"})},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})
	canApply, err = chk.CanApply(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeFalse())

	// Should not apply for 3.x to 3.x
	target = testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{testutil.NewDSC(map[string]string{"codeflare": "Managed"})},
		CurrentVersion: "3.0.0",
		TargetVersion:  "3.1.0",
	})
	canApply, err = chk.CanApply(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeFalse())
}
