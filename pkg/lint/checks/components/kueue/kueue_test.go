package kueue_test

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/opendatahub-io/odh-cli/pkg/constants"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	resultpkg "github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/testutil"
	"github.com/opendatahub-io/odh-cli/pkg/lint/checks/components/kueue"
	"github.com/opendatahub-io/odh-cli/pkg/resources"

	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

//nolint:gochecknoglobals // Test fixture - shared across test functions
var listKinds = map[schema.GroupVersionResource]string{
	resources.DataScienceCluster.GVR():  resources.DataScienceCluster.ListKind(),
	resources.Namespace.GVR():           resources.Namespace.ListKind(),
	resources.Notebook.GVR():            resources.Notebook.ListKind(),
	resources.InferenceService.GVR():    resources.InferenceService.ListKind(),
	resources.LLMInferenceService.GVR(): resources.LLMInferenceService.ListKind(),
	resources.RayCluster.GVR():          resources.RayCluster.ListKind(),
	resources.RayJob.GVR():              resources.RayJob.ListKind(),
	resources.PyTorchJob.GVR():          resources.PyTorchJob.ListKind(),
}

func newNamespace(name string, labels map[string]string) *unstructured.Unstructured {
	meta := map[string]any{
		"name": name,
	}
	if labels != nil {
		anyLabels := make(map[string]any, len(labels))
		for k, v := range labels {
			anyLabels[k] = v
		}

		meta["labels"] = anyLabels
	}

	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.Namespace.APIVersion(),
			"kind":       resources.Namespace.Kind,
			"metadata":   meta,
		},
	}
}

func newWorkload(
	rt resources.ResourceType,
	namespace, name string,
	labels map[string]string,
) *unstructured.Unstructured {
	meta := map[string]any{
		"name":      name,
		"namespace": namespace,
	}
	if labels != nil {
		anyLabels := make(map[string]any, len(labels))
		for k, v := range labels {
			anyLabels[k] = v
		}

		meta["labels"] = anyLabels
	}

	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": rt.APIVersion(),
			"kind":       rt.Kind,
			"metadata":   meta,
		},
	}
}

func TestManagementStateCheck_CanApply_NoDSC(t *testing.T) {
	g := NewWithT(t)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		CurrentVersion: "2.25.0",
		TargetVersion:  "3.3.2",
	})

	chk := kueue.NewManagementStateCheck()
	canApply, err := chk.CanApply(t.Context(), target)

	g.Expect(err).To(HaveOccurred())
	g.Expect(canApply).To(BeFalse())
}

func TestManagementStateCheck_CanApply_NotConfigured(t *testing.T) {
	g := NewWithT(t)

	// DSC without kueue component — state defaults to empty, not Managed/Unmanaged
	dsc := testutil.NewDSC(map[string]string{"dashboard": "Managed"})
	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{dsc},
		CurrentVersion: "2.25.0",
		TargetVersion:  "3.3.2",
	})

	chk := kueue.NewManagementStateCheck()
	canApply, err := chk.CanApply(t.Context(), target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeFalse())
}

func TestManagementStateCheck_ManagedProhibited(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	// Kueue is Managed AND there is a kueue-labeled namespace with a workload.
	ns := newNamespace("team-a", map[string]string{constants.LabelKueueManaged: "true"})
	nb := newWorkload(resources.Notebook, "team-a", "my-notebook",
		map[string]string{constants.LabelKueueQueueName: "default-queue"})

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{testutil.NewDSC(map[string]string{"kueue": "Managed"}), ns, nb},
		CurrentVersion: "2.25.0",
		TargetVersion:  "3.3.2",
	})

	chk := kueue.NewManagementStateCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeCompatible),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonVersionIncompatible),
		"Message": And(ContainSubstring("only supports the Kueue managementState of Removed"), ContainSubstring("migrated to the Red Hat build of Kueue Operator")),
	}))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactProhibited))
	g.Expect(result.Annotations).To(And(
		HaveKeyWithValue("component.opendatahub.io/management-state", "Managed"),
		HaveKeyWithValue("check.opendatahub.io/target-version", "3.3.2"),
	))
}

func TestManagementStateCheck_ManagedBlocking(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	// Kueue is Managed but NO namespaces or workloads are labeled for kueue.
	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{testutil.NewDSC(map[string]string{"kueue": "Managed"})},
		CurrentVersion: "2.25.0",
		TargetVersion:  "3.3.2",
	})

	chk := kueue.NewManagementStateCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(check.ConditionTypeCompatible),
		"Status": Equal(metav1.ConditionFalse),
		"Reason": Equal(check.ReasonVersionIncompatible),
	}))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactBlocking))
	g.Expect(result.Annotations).To(
		HaveKeyWithValue("component.opendatahub.io/management-state", "Managed"),
	)
}

func TestManagementStateCheck_UnmanagedProhibited(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	// Kueue is Unmanaged AND there is a workload labeled for kueue.
	nb := newWorkload(resources.Notebook, "team-b", "my-notebook",
		map[string]string{constants.LabelKueueQueueName: "default-queue"})

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{testutil.NewDSC(map[string]string{"kueue": "Unmanaged"}), nb},
		CurrentVersion: "2.25.0",
		TargetVersion:  "3.3.2",
	})

	chk := kueue.NewManagementStateCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeCompatible),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonVersionIncompatible),
		"Message": And(ContainSubstring("only supports the Kueue managementState of Removed"), Not(ContainSubstring("migrated to the Red Hat build of Kueue Operator"))),
	}))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactProhibited))
}

func TestManagementStateCheck_UnmanagedBlocking(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	// Kueue is Unmanaged but NO namespaces or workloads are labeled for kueue.
	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{testutil.NewDSC(map[string]string{"kueue": "Unmanaged"})},
		CurrentVersion: "2.25.0",
		TargetVersion:  "3.3.2",
	})

	chk := kueue.NewManagementStateCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(check.ConditionTypeCompatible),
		"Status": Equal(metav1.ConditionFalse),
		"Reason": Equal(check.ReasonVersionIncompatible),
	}))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactBlocking))
}

func TestManagementStateCheck_CanApply_ManagementState(t *testing.T) {
	g := NewWithT(t)

	chk := kueue.NewManagementStateCheck()

	testCases := []struct {
		name     string
		state    string
		expected bool
	}{
		{name: "Managed", state: "Managed", expected: true},
		{name: "Unmanaged", state: "Unmanaged", expected: true},
		{name: "Removed", state: "Removed", expected: false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			dsc := testutil.NewDSC(map[string]string{"kueue": tc.state})
			target := testutil.NewTarget(t, testutil.TargetConfig{
				ListKinds:      listKinds,
				Objects:        []*unstructured.Unstructured{dsc},
				CurrentVersion: "2.25.0",
				TargetVersion:  "3.3.2",
			})

			canApply, err := chk.CanApply(t.Context(), target)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(canApply).To(Equal(tc.expected))
		})
	}
}

func TestManagementStateCheck_Metadata(t *testing.T) {
	g := NewWithT(t)

	chk := kueue.NewManagementStateCheck()

	g.Expect(chk.ID()).To(Equal("components.kueue.management-state"))
	g.Expect(chk.Name()).To(Equal("Components :: Kueue :: Management State (3.x)"))
	g.Expect(chk.Group()).To(Equal(check.GroupComponent))
	g.Expect(chk.Description()).ToNot(BeEmpty())
}

func TestManagementStateCheck_KueueUsageViaNamespaceLabel(t *testing.T) {
	t.Run("kueue-managed label", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		// Kueue-managed namespace exists but no workloads with queue-name label — still "in use".
		ns := newNamespace("team-a", map[string]string{constants.LabelKueueManaged: "true"})

		target := testutil.NewTarget(t, testutil.TargetConfig{
			ListKinds:      listKinds,
			Objects:        []*unstructured.Unstructured{testutil.NewDSC(map[string]string{"kueue": "Managed"}), ns},
			CurrentVersion: "2.25.0",
			TargetVersion:  "3.3.2",
		})

		chk := kueue.NewManagementStateCheck()
		result, err := chk.Validate(ctx, target)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactProhibited))
	})

	t.Run("kueue.openshift.io/managed label", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		// Namespace with OpenShift kueue-managed label — still counts as "in use".
		ns := newNamespace("team-a", map[string]string{constants.LabelKueueOpenshiftManaged: "true"})

		target := testutil.NewTarget(t, testutil.TargetConfig{
			ListKinds:      listKinds,
			Objects:        []*unstructured.Unstructured{testutil.NewDSC(map[string]string{"kueue": "Managed"}), ns},
			CurrentVersion: "2.25.0",
			TargetVersion:  "3.3.2",
		})

		chk := kueue.NewManagementStateCheck()
		result, err := chk.Validate(ctx, target)

		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactProhibited))
	})
}

func TestManagementStateCheck_KueueUsageViaWorkloadLabel(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	// No kueue-managed namespace, but a workload has the queue-name label — still "in use".
	nb := newWorkload(resources.Notebook, "team-b", "my-notebook",
		map[string]string{constants.LabelKueueQueueName: "default-queue"})

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{testutil.NewDSC(map[string]string{"kueue": "Managed"}), nb},
		CurrentVersion: "2.25.0",
		TargetVersion:  "3.3.2",
	})

	chk := kueue.NewManagementStateCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactProhibited))
}
