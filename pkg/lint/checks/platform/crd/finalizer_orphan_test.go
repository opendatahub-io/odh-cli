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
var finalizerListKinds = map[schema.GroupVersionResource]string{
	resources.DataScienceCluster.GVR(): resources.DataScienceCluster.ListKind(),
	resources.DSCInitialization.GVR():  resources.DSCInitialization.ListKind(),
	resources.Deployment.GVR():         resources.Deployment.ListKind(),
}

func toAnySlice(ss []string) []any {
	out := make([]any, len(ss))
	for i, s := range ss {
		out[i] = s
	}

	return out
}

func newDSCWithFinalizers(finalizers []string) *unstructured.Unstructured {
	metadata := map[string]any{
		"name": "default-dsc",
	}

	if len(finalizers) > 0 {
		metadata["finalizers"] = toAnySlice(finalizers)
	}

	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": resources.DataScienceCluster.APIVersion(),
		"kind":       resources.DataScienceCluster.Kind,
		"metadata":   metadata,
		"spec":       map[string]any{"components": map[string]any{}},
	}}
}

func newOperatorDeployment(availableReplicas int64) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": resources.Deployment.APIVersion(),
		"kind":       resources.Deployment.Kind,
		"metadata": map[string]any{
			"name":      "rhods-operator",
			"namespace": "redhat-ods-operator",
			"labels":    map[string]any{"operators.coreos.com/rhods-operator.redhat-ods-operator": ""},
		},
		"status": map[string]any{
			"replicas":          int64(1),
			"availableReplicas": availableReplicas,
		},
	}}
}

func TestFinalizerOrphanCheck_Metadata(t *testing.T) {
	g := NewWithT(t)

	chk := platformcrd.NewFinalizerOrphanCheck()

	g.Expect(chk.ID()).To(Equal("platform.crd.finalizer-orphan"))
	g.Expect(chk.Group()).To(Equal(check.GroupPlatform))
	g.Expect(chk.Description()).ToNot(BeEmpty())
}

func TestFinalizerOrphanCheck_CanApply(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	chk := platformcrd.NewFinalizerOrphanCheck()

	// Should apply for 2.x -> 3.x upgrade
	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      finalizerListKinds,
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})
	canApply, err := chk.CanApply(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeTrue())

	// Should not apply for 3.x -> 3.x upgrade
	target = testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      finalizerListKinds,
		CurrentVersion: "3.0.0",
		TargetVersion:  "3.1.0",
	})
	canApply, err = chk.CanApply(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeFalse())

	// Should not apply with nil versions
	target = testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds: finalizerListKinds,
	})
	canApply, err = chk.CanApply(ctx, target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeFalse())
}

func TestFinalizerOrphanCheck_NoFinalizers(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	dsc := testutil.NewDSC(map[string]string{"dashboard": "Managed"})

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      finalizerListKinds,
		Objects:        []*unstructured.Unstructured{dsc},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := platformcrd.NewFinalizerOrphanCheck()
	dr, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(dr.Status.Conditions).To(HaveLen(1))
	g.Expect(dr.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal("FinalizerHealth"),
		"Status": Equal(metav1.ConditionTrue),
		"Reason": Equal(check.ReasonRequirementsMet),
	}))
}

func TestFinalizerOrphanCheck_FinalizersWithHealthyOperator(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	dsc := newDSCWithFinalizers([]string{"datasciencecluster.opendatahub.io/finalizer"})
	deploy := newOperatorDeployment(1)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      finalizerListKinds,
		Objects:        []*unstructured.Unstructured{dsc, deploy},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := platformcrd.NewFinalizerOrphanCheck()
	dr, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(dr.Status.Conditions).To(HaveLen(1))
	g.Expect(dr.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal("FinalizerHealth"),
		"Status": Equal(metav1.ConditionTrue),
		"Reason": Equal(check.ReasonRequirementsMet),
	}))
}

func TestFinalizerOrphanCheck_FinalizersWithUnhealthyOperator(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	dsc := newDSCWithFinalizers([]string{"datasciencecluster.opendatahub.io/finalizer"})
	deploy := newOperatorDeployment(0)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      finalizerListKinds,
		Objects:        []*unstructured.Unstructured{dsc, deploy},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := platformcrd.NewFinalizerOrphanCheck()
	dr, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(dr.Status.Conditions).To(HaveLen(1))
	g.Expect(dr.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal("FinalizerHealth"),
		"Status": Equal(metav1.ConditionFalse),
		"Reason": Equal(check.ReasonResourceUnavailable),
	}))
	g.Expect(dr.Status.Conditions[0].Impact).To(Equal(result.ImpactAdvisory))
}

func TestFinalizerOrphanCheck_FinalizersNoOperator(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	dsc := newDSCWithFinalizers([]string{"datasciencecluster.opendatahub.io/finalizer"})

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      finalizerListKinds,
		Objects:        []*unstructured.Unstructured{dsc},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := platformcrd.NewFinalizerOrphanCheck()
	dr, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(dr.Status.Conditions).To(HaveLen(1))
	g.Expect(dr.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal("FinalizerHealth"),
		"Status": Equal(metav1.ConditionFalse),
		"Reason": Equal(check.ReasonResourceNotFound),
	}))
	g.Expect(dr.Status.Conditions[0].Impact).To(Equal(result.ImpactAdvisory))
}
