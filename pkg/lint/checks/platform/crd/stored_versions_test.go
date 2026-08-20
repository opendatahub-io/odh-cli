package crd_test

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/testutil"
	platformcrd "github.com/opendatahub-io/odh-cli/pkg/lint/checks/platform/crd"
	"github.com/opendatahub-io/odh-cli/pkg/resources"

	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

//nolint:gochecknoglobals // Test fixture - shared across test functions
var storedVersionsListKinds = map[schema.GroupVersionResource]string{
	resources.CustomResourceDefinition.GVR(): resources.CustomResourceDefinition.ListKind(),
}

func newCRDWithVersions(name string, storedVersions []string, specVersions []string) *unstructured.Unstructured {
	stored := make([]any, len(storedVersions))
	for i, v := range storedVersions {
		stored[i] = v
	}

	versions := make([]any, len(specVersions))
	for i, v := range specVersions {
		versions[i] = map[string]any{"name": v, "served": true, "storage": i == 0}
	}

	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": resources.CustomResourceDefinition.APIVersion(),
		"kind":       resources.CustomResourceDefinition.Kind,
		"metadata":   map[string]any{"name": name},
		"spec":       map[string]any{"versions": versions},
		"status":     map[string]any{"storedVersions": stored},
	}}
}

func TestStoredVersionsCheck_Metadata(t *testing.T) {
	g := NewWithT(t)

	chk := platformcrd.NewStoredVersionsCheck()

	g.Expect(chk.ID()).To(Equal("platform.crd.stored-versions"))
	g.Expect(chk.Group()).To(Equal(check.GroupPlatform))
	g.Expect(chk.Description()).ToNot(BeEmpty())
}

func TestStoredVersionsCheck_CanApply(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	chk := platformcrd.NewStoredVersionsCheck()

	// Should apply for 2.x -> 3.x upgrade
	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      storedVersionsListKinds,
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})
	canApply, err := chk.CanApply(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeTrue())

	// Should not apply for 3.x -> 3.x upgrade
	target = testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      storedVersionsListKinds,
		CurrentVersion: "3.0.0",
		TargetVersion:  "3.1.0",
	})
	canApply, err = chk.CanApply(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeFalse())

	// Should not apply with nil versions
	target = testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds: storedVersionsListKinds,
	})
	canApply, err = chk.CanApply(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeFalse())
}

func TestStoredVersionsCheck_Compatible(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	crd := newCRDWithVersions(
		"datascienceclusters.datasciencecluster.opendatahub.io",
		[]string{"v1", "v2"},
		[]string{"v1", "v2"},
	)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      storedVersionsListKinds,
		Objects:        []*unstructured.Unstructured{crd},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := platformcrd.NewStoredVersionsCheck()
	dr, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(dr.Status.Conditions).To(HaveLen(1))
	g.Expect(dr.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal("StoredVersionsCompatible"),
		"Status": Equal(metav1.ConditionTrue),
		"Reason": Equal(check.ReasonVersionCompatible),
	}))
}

func TestStoredVersionsCheck_Incompatible(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	crd := newCRDWithVersions(
		"datascienceclusters.datasciencecluster.opendatahub.io",
		[]string{"v1alpha1", "v1"},
		[]string{"v1", "v2"},
	)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      storedVersionsListKinds,
		Objects:        []*unstructured.Unstructured{crd},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := platformcrd.NewStoredVersionsCheck()
	dr, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(dr.Status.Conditions).To(HaveLen(1))
	g.Expect(dr.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal("StoredVersionsCompatible"),
		"Status": Equal(metav1.ConditionFalse),
		"Reason": Equal(check.ReasonVersionIncompatible),
	}))
	g.Expect(dr.Status.Conditions[0].Impact).To(Equal(result.ImpactBlocking))
	g.Expect(dr.Status.Conditions[0].Message).To(ContainSubstring("v1alpha1"))
}

func TestStoredVersionsCheck_CRDNotFound(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	// No CRDs registered
	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      storedVersionsListKinds,
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := platformcrd.NewStoredVersionsCheck()
	dr, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(dr.Status.Conditions).To(HaveLen(1))
	g.Expect(dr.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal("StoredVersionsCompatible"),
		"Status": Equal(metav1.ConditionTrue),
		"Reason": Equal(check.ReasonResourceNotFound),
	}))
}
