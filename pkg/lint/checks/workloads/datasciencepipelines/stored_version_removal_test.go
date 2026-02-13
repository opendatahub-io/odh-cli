package datasciencepipelines_test

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/testutil"
	"github.com/opendatahub-io/odh-cli/pkg/lint/checks/workloads/datasciencepipelines"
	"github.com/opendatahub-io/odh-cli/pkg/resources"

	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

//nolint:gochecknoglobals // Test fixture - shared across test functions
var storedVersionListKinds = map[schema.GroupVersionResource]string{
	resources.CustomResourceDefinition.GVR(): resources.CustomResourceDefinition.ListKind(),
}

func newDSPACRD(storedVersions ...string) *unstructured.Unstructured {
	versions := make([]any, 0, len(storedVersions))
	for _, v := range storedVersions {
		versions = append(versions, v)
	}

	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.CustomResourceDefinition.APIVersion(),
			"kind":       resources.CustomResourceDefinition.Kind,
			"metadata": map[string]any{
				"name": "datasciencepipelinesapplications.datasciencepipelinesapplications.opendatahub.io",
			},
			"status": map[string]any{
				"storedVersions": versions,
			},
		},
	}
}

func TestStoredVersionRemovalCheck_Metadata(t *testing.T) {
	g := NewWithT(t)

	chk := datasciencepipelines.NewStoredVersionRemovalCheck()

	g.Expect(chk.ID()).To(Equal("workloads.datasciencepipelines.stored-version-removal"))
	g.Expect(chk.Name()).To(Equal("Workloads :: DataSciencePipelines :: v1alpha1 StoredVersion Removal (3.x)"))
	g.Expect(chk.Group()).To(Equal(check.GroupWorkload))
	g.Expect(chk.Description()).ToNot(BeEmpty())
}

func TestStoredVersionRemovalCheck_CanApply(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	chk := datasciencepipelines.NewStoredVersionRemovalCheck()

	// Should not apply in lint mode (same version)
	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      storedVersionListKinds,
		CurrentVersion: "2.17.0",
		TargetVersion:  "2.17.0",
	})
	canApply, err := chk.CanApply(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeFalse())

	// Should apply for 2.x -> 3.x upgrade
	target = testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      storedVersionListKinds,
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})
	canApply, err = chk.CanApply(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeTrue())

	// Should not apply for 3.x -> 3.x upgrade
	target = testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      storedVersionListKinds,
		CurrentVersion: "3.0.0",
		TargetVersion:  "3.1.0",
	})
	canApply, err = chk.CanApply(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeFalse())

	// Should not apply with nil versions
	target = testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds: storedVersionListKinds,
	})
	canApply, err = chk.CanApply(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeFalse())
}

func TestStoredVersionRemovalCheck_WithV1Alpha1(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	crd := newDSPACRD("v1alpha1", "v1")
	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      storedVersionListKinds,
		Objects:        []*unstructured.Unstructured{crd},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := datasciencepipelines.NewStoredVersionRemovalCheck()
	dr, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(dr.Status.Conditions).To(HaveLen(1))
	g.Expect(dr.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(check.ConditionTypeCompatible),
		"Status": Equal(metav1.ConditionFalse),
		"Reason": Equal(check.ReasonVersionIncompatible),
	}))
	g.Expect(dr.Status.Conditions[0].Impact).To(Equal(result.ImpactBlocking))
	g.Expect(dr.Status.Conditions[0].Message).To(ContainSubstring("v1alpha1"))
	g.Expect(dr.Status.Conditions[0].Remediation).ToNot(BeEmpty())
}

func TestStoredVersionRemovalCheck_WithoutV1Alpha1(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	crd := newDSPACRD("v1")
	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      storedVersionListKinds,
		Objects:        []*unstructured.Unstructured{crd},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := datasciencepipelines.NewStoredVersionRemovalCheck()
	dr, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(dr.Status.Conditions).To(HaveLen(1))
	g.Expect(dr.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(check.ConditionTypeCompatible),
		"Status": Equal(metav1.ConditionTrue),
		"Reason": Equal(check.ReasonVersionCompatible),
	}))
	g.Expect(dr.Status.Conditions[0].Message).To(ContainSubstring("No DataSciencePipelinesApplication"))
}

func TestStoredVersionRemovalCheck_CRDNotFound(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	// No CRD objects registered
	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      storedVersionListKinds,
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := datasciencepipelines.NewStoredVersionRemovalCheck()
	dr, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(dr.Status.Conditions).To(HaveLen(1))
	g.Expect(dr.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(check.ConditionTypeCompatible),
		"Status": Equal(metav1.ConditionTrue),
		"Reason": Equal(check.ReasonResourceNotFound),
	}))
}

func TestStoredVersionRemovalCheck_OnlyV1Alpha1(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	crd := newDSPACRD("v1alpha1")
	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      storedVersionListKinds,
		Objects:        []*unstructured.Unstructured{crd},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := datasciencepipelines.NewStoredVersionRemovalCheck()
	dr, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(dr.Status.Conditions).To(HaveLen(1))
	g.Expect(dr.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(check.ConditionTypeCompatible),
		"Status": Equal(metav1.ConditionFalse),
		"Reason": Equal(check.ReasonVersionIncompatible),
	}))
	g.Expect(dr.Status.Conditions[0].Impact).To(Equal(result.ImpactBlocking))
}
