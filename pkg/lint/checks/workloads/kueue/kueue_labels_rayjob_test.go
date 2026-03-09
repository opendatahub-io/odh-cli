package kueue_test

import (
	"fmt"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/opendatahub-io/odh-cli/pkg/constants"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	resultpkg "github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/testutil"
	"github.com/opendatahub-io/odh-cli/pkg/lint/checks/workloads/kueue"
	"github.com/opendatahub-io/odh-cli/pkg/resources"

	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

//nolint:gochecknoglobals
var kueueLabelsRayJobListKinds = map[schema.GroupVersionResource]string{
	resources.RayJob.GVR():             resources.RayJob.ListKind(),
	resources.Namespace.GVR():          resources.Namespace.ListKind(),
	resources.DataScienceCluster.GVR(): resources.DataScienceCluster.ListKind(),
}

func newRayJob(name string, namespace string, labels map[string]any) *unstructured.Unstructured {
	metadata := map[string]any{
		"name":      name,
		"namespace": namespace,
	}

	if len(labels) > 0 {
		metadata["labels"] = labels
	}

	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.RayJob.APIVersion(),
			"kind":       resources.RayJob.Kind,
			"metadata":   metadata,
		},
	}
}

func TestKueueLabelsRayJobCheck_Metadata(t *testing.T) {
	g := NewWithT(t)

	chk := kueue.NewKueueLabelsRayJobCheck()

	g.Expect(chk.ID()).To(Equal("workloads.kueue.rayjob-labels"))
	g.Expect(chk.Name()).To(Equal("Workloads :: Kueue :: RayJob Labels"))
	g.Expect(chk.Group()).To(Equal(check.GroupWorkload))
	g.Expect(chk.CheckKind()).To(Equal("kueue"))
	g.Expect(chk.Remediation()).To(ContainSubstring("kueue.x-k8s.io/queue-name"))
}

func TestKueueLabelsRayJobCheck_CanApply_KueueManaged(t *testing.T) {
	g := NewWithT(t)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      kueueLabelsRayJobListKinds,
		Objects:        []*unstructured.Unstructured{testutil.NewDSC(map[string]string{"kueue": "Managed"})},
		CurrentVersion: "3.0.0",
		TargetVersion:  "3.0.0",
	})

	chk := kueue.NewKueueLabelsRayJobCheck()
	canApply, err := chk.CanApply(t.Context(), target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeTrue())
}

func TestKueueLabelsRayJobCheck_CanApply_KueueRemoved(t *testing.T) {
	g := NewWithT(t)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      kueueLabelsRayJobListKinds,
		Objects:        []*unstructured.Unstructured{testutil.NewDSC(map[string]string{"kueue": "Removed"})},
		CurrentVersion: "3.0.0",
		TargetVersion:  "3.0.0",
	})

	chk := kueue.NewKueueLabelsRayJobCheck()
	canApply, err := chk.CanApply(t.Context(), target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeFalse())
}

func TestKueueLabelsRayJobCheck_CanApply_KueueUnmanaged(t *testing.T) {
	g := NewWithT(t)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      kueueLabelsRayJobListKinds,
		Objects:        []*unstructured.Unstructured{testutil.NewDSC(map[string]string{"kueue": "Unmanaged"})},
		CurrentVersion: "3.0.0",
		TargetVersion:  "3.0.0",
	})

	chk := kueue.NewKueueLabelsRayJobCheck()
	canApply, err := chk.CanApply(t.Context(), target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeTrue())
}

func TestKueueLabelsRayJobCheck_NoRayJobs(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      kueueLabelsRayJobListKinds,
		CurrentVersion: "3.0.0",
		TargetVersion:  "3.0.0",
	})

	chk := kueue.NewKueueLabelsRayJobCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(2))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(kueue.ConditionTypeRayJobKueueLabels),
		"Status":  Equal(metav1.ConditionTrue),
		"Message": Equal(fmt.Sprintf(kueue.MsgNoWorkloads, "RayJob")),
	}))
	g.Expect(result.Status.Conditions[1].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(kueue.ConditionTypeRayJobKueueMissingLabels),
		"Status":  Equal(metav1.ConditionTrue),
		"Message": Equal(fmt.Sprintf(kueue.MsgNoWorkloadsInKueueNs, "RayJob")),
	}))
	g.Expect(result.ImpactedObjects).To(BeEmpty())
}

func TestKueueLabelsRayJobCheck_WithQueueLabelInKueueNamespace(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	ns := newKueueNs("kueue-ns", map[string]any{constants.LabelKueueManaged: "true"})
	rj := newRayJob("good-rj", "kueue-ns", map[string]any{constants.LabelKueueQueueName: "default"})

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      kueueLabelsRayJobListKinds,
		Objects:        []*unstructured.Unstructured{ns, rj},
		CurrentVersion: "3.0.0",
		TargetVersion:  "3.0.0",
	})

	chk := kueue.NewKueueLabelsRayJobCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions[0].Status).To(Equal(metav1.ConditionTrue))
	g.Expect(result.Status.Conditions[0].Message).To(Equal(fmt.Sprintf(kueue.MsgAllValid, 1, "RayJob")))
	g.Expect(result.Status.Conditions[1].Status).To(Equal(metav1.ConditionTrue))
	g.Expect(result.Status.Conditions[1].Message).To(Equal(fmt.Sprintf(kueue.MsgAllInKueueNsLabeled, 1, "RayJob")))
	g.Expect(result.ImpactedObjects).To(BeEmpty())
}

func TestKueueLabelsRayJobCheck_LabeledInNonKueueNamespace(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	ns := newKueueNs("plain-ns", nil)
	rj := newRayJob("bad-rj", "plain-ns", map[string]any{constants.LabelKueueQueueName: "default"})

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      kueueLabelsRayJobListKinds,
		Objects:        []*unstructured.Unstructured{ns, rj},
		CurrentVersion: "3.0.0",
		TargetVersion:  "3.0.0",
	})

	chk := kueue.NewKueueLabelsRayJobCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions[0].Status).To(Equal(metav1.ConditionFalse))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactBlocking))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "1"))
	g.Expect(result.ImpactedObjects).To(HaveLen(1))
	g.Expect(result.ImpactedObjects[0].Name).To(Equal("bad-rj"))
	g.Expect(result.Status.Conditions[1].Status).To(Equal(metav1.ConditionTrue))
	g.Expect(result.Status.Conditions[1].Message).To(Equal(fmt.Sprintf(kueue.MsgNoWorkloadsInKueueNs, "RayJob")))
}

func TestKueueLabelsRayJobCheck_WithoutQueueLabel(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	ns := newKueueNs("plain-ns", nil)
	rj := newRayJob("unlabeled-rj", "plain-ns", nil)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      kueueLabelsRayJobListKinds,
		Objects:        []*unstructured.Unstructured{ns, rj},
		CurrentVersion: "3.0.0",
		TargetVersion:  "3.0.0",
	})

	chk := kueue.NewKueueLabelsRayJobCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions[0].Status).To(Equal(metav1.ConditionTrue))
	g.Expect(result.Status.Conditions[0].Message).To(Equal(fmt.Sprintf(kueue.MsgNoLabeledWorkloads, "RayJob")))
	g.Expect(result.Status.Conditions[1].Status).To(Equal(metav1.ConditionTrue))
	g.Expect(result.Status.Conditions[1].Message).To(Equal(fmt.Sprintf(kueue.MsgNoWorkloadsInKueueNs, "RayJob")))
	g.Expect(result.ImpactedObjects).To(BeEmpty())
}
