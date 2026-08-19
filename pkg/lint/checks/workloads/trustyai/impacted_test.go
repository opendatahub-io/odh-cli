package trustyai_test

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	resultpkg "github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/testutil"
	"github.com/opendatahub-io/odh-cli/pkg/lint/checks/workloads/trustyai"
	"github.com/opendatahub-io/odh-cli/pkg/resources"

	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

//nolint:gochecknoglobals
var listKinds = map[schema.GroupVersionResource]string{
	resources.TrustyAIService.GVR():    resources.TrustyAIService.ListKind(),
	resources.DataScienceCluster.GVR(): resources.DataScienceCluster.ListKind(),
}

func newTestTAS(name, namespace, format string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.TrustyAIService.APIVersion(),
			"kind":       resources.TrustyAIService.Kind,
			"metadata": map[string]any{
				"name":      name,
				"namespace": namespace,
			},
			"spec": map[string]any{
				"storage": map[string]any{
					"format": format,
				},
			},
		},
	}
}

func TestImpactedWorkloadsCheck_NoTAS(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        nil,
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := trustyai.NewImpactedWorkloadsCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(check.ConditionTypeCompatible),
		"Status": Equal(metav1.ConditionTrue),
		"Reason": Equal(check.ReasonResourceNotFound),
	}))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "0"))
	g.Expect(result.ImpactedObjects).To(BeEmpty())
}

func TestImpactedWorkloadsCheck_WithOneTAS(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	tas := newTestTAS("tas-1", "test-ns", "PVC")

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{tas},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := trustyai.NewImpactedWorkloadsCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeCompatible),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonWorkloadsImpacted),
		"Message": ContainSubstring("Found 1 TrustyAIService"),
	}))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactAdvisory))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "1"))
	g.Expect(result.ImpactedObjects).To(HaveLen(1))
	g.Expect(result.ImpactedObjects[0].Name).To(Equal("tas-1"))
	g.Expect(result.ImpactedObjects[0].Namespace).To(Equal("test-ns"))
}

func TestImpactedWorkloadsCheck_WithMultipleTAS(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds: listKinds,
		Objects: []*unstructured.Unstructured{
			newTestTAS("tas-1", "ns1", "PVC"),
			newTestTAS("tas-2", "ns2", "DATABASE"),
		},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := trustyai.NewImpactedWorkloadsCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Status":  Equal(metav1.ConditionFalse),
		"Message": ContainSubstring("Found 2 TrustyAIService"),
	}))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "2"))
	g.Expect(result.ImpactedObjects).To(HaveLen(2))
}

func TestImpactedWorkloadsCheck_Metadata(t *testing.T) {
	g := NewWithT(t)

	chk := trustyai.NewImpactedWorkloadsCheck()

	g.Expect(chk.ID()).To(Equal("workloads.trustyai.impacted-workloads"))
	g.Expect(chk.Name()).To(Equal("Workloads :: TrustyAI :: Impacted Workloads (3.x)"))
	g.Expect(chk.Group()).To(Equal(check.GroupWorkload))
	g.Expect(chk.Description()).ToNot(BeEmpty())
}

func TestImpactedWorkloadsCheck_CanApply_NilVersions(t *testing.T) {
	g := NewWithT(t)

	chk := trustyai.NewImpactedWorkloadsCheck()
	canApply, err := chk.CanApply(t.Context(), check.Target{})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeFalse())
}

func TestImpactedWorkloadsCheck_CanApply_LintMode(t *testing.T) {
	g := NewWithT(t)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{testutil.NewDSC(map[string]string{"trustyai": "Managed"})},
		CurrentVersion: "2.17.0",
		TargetVersion:  "2.17.0",
	})

	chk := trustyai.NewImpactedWorkloadsCheck()
	canApply, err := chk.CanApply(t.Context(), target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeFalse())
}

func TestImpactedWorkloadsCheck_CanApply_2xTo3x_Managed(t *testing.T) {
	g := NewWithT(t)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{testutil.NewDSC(map[string]string{"trustyai": "Managed"})},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := trustyai.NewImpactedWorkloadsCheck()
	canApply, err := chk.CanApply(t.Context(), target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeTrue())
}

func TestImpactedWorkloadsCheck_CanApply_2xTo3x_Removed(t *testing.T) {
	g := NewWithT(t)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{testutil.NewDSC(map[string]string{"trustyai": "Removed"})},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := trustyai.NewImpactedWorkloadsCheck()
	canApply, err := chk.CanApply(t.Context(), target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeFalse())
}

func TestImpactedWorkloadsCheck_CanApply_3xTo3x(t *testing.T) {
	g := NewWithT(t)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{testutil.NewDSC(map[string]string{"trustyai": "Managed"})},
		CurrentVersion: "3.0.0",
		TargetVersion:  "3.3.0",
	})

	chk := trustyai.NewImpactedWorkloadsCheck()
	canApply, err := chk.CanApply(t.Context(), target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeFalse())
}

func TestImpactedWorkloadsCheck_AnnotationTargetVersion(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        nil,
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := trustyai.NewImpactedWorkloadsCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationCheckTargetVersion, "3.0.0"))
}
