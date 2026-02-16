package ray_test

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	resultpkg "github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/testutil"
	"github.com/opendatahub-io/odh-cli/pkg/lint/checks/workloads/ray"
	"github.com/opendatahub-io/odh-cli/pkg/resources"

	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

func TestAppWrapperCleanupCheck_NoAppWrappers(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{testutil.NewDSC(map[string]string{"codeflare": "Managed"})},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := ray.NewAppWrapperCleanupCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(ray.ConditionTypeAppWrapperCompatible),
		"Status":  Equal(metav1.ConditionTrue),
		"Reason":  Equal(check.ReasonVersionCompatible),
		"Message": ContainSubstring("No AppWrapper(s) found"),
	}))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "0"))
	g.Expect(result.ImpactedObjects).To(BeEmpty())
}

func TestAppWrapperCleanupCheck_WithAppWrappers(t *testing.T) {
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

	chk := ray.NewAppWrapperCleanupCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(ray.ConditionTypeAppWrapperCompatible),
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

func TestAppWrapperCleanupCheck_MultipleAppWrappers(t *testing.T) {
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

	chk := ray.NewAppWrapperCleanupCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(ray.ConditionTypeAppWrapperCompatible),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonVersionIncompatible),
		"Message": ContainSubstring("Found 2 AppWrapper workload CRs"),
	}))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "2"))
	g.Expect(result.ImpactedObjects).To(HaveLen(2))
}

func TestAppWrapperCleanupCheck_Metadata(t *testing.T) {
	g := NewWithT(t)

	chk := ray.NewAppWrapperCleanupCheck()

	g.Expect(chk.ID()).To(Equal("workloads.ray.appwrapper-cleanup"))
	g.Expect(chk.Name()).To(Equal("Workloads :: Ray :: AppWrapper Cleanup (3.x)"))
	g.Expect(chk.Group()).To(Equal(check.GroupWorkload))
	g.Expect(chk.CheckKind()).To(Equal("ray"))
	g.Expect(chk.Description()).ToNot(BeEmpty())
	g.Expect(chk.Remediation()).To(ContainSubstring("AppWrapper"))
}

func TestAppWrapperCleanupCheck_CanApply(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	chk := ray.NewAppWrapperCleanupCheck()

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
