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
	resources.LocalQueue.GVR():          resources.LocalQueue.ListKind(),
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

func newLocalQueue(namespace, name string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.LocalQueue.APIVersion(),
			"kind":       resources.LocalQueue.Kind,
			"metadata": map[string]any{
				"name":      name,
				"namespace": namespace,
			},
		},
	}
}

// newDSCWithDefaultQueue builds a DSC with the given kueue managementState and, when
// defaultQueueName is non-empty, sets spec.components.kueue.defaultLocalQueueName.
func newDSCWithDefaultQueue(state, defaultQueueName string) *unstructured.Unstructured {
	dsc := testutil.NewDSC(map[string]string{"kueue": state})
	if defaultQueueName != "" {
		if err := unstructured.SetNestedField(
			dsc.Object, defaultQueueName,
			"spec", "components", "kueue", "defaultLocalQueueName",
		); err != nil {
			panic(err)
		}
	}

	return dsc
}

// conditionByType returns the first condition of the given type and whether it was found.
func conditionByType(r *resultpkg.DiagnosticResult, condType string) (resultpkg.Condition, bool) {
	for _, c := range r.Status.Conditions {
		if c.Type == condType {
			return c, true
		}
	}

	return resultpkg.Condition{}, false
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
	// No default-LocalQueue hazard present → base condition only.
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeCompatible),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonVersionIncompatible),
		"Message": And(ContainSubstring("does not support the Kueue managementState of Managed"), ContainSubstring("Migrate Kueue to Unmanaged")),
	}))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactProhibited))
	g.Expect(result.Status.Conditions[0].Remediation).ToNot(BeEmpty())
	g.Expect(result.GetImpact()).To(Equal(resultpkg.ImpactProhibited))
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

func TestManagementStateCheck_UnmanagedInUsePass(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	// Kueue is Unmanaged AND there is a workload labeled for kueue. Unmanaged is a supported
	// target state, so the base condition passes (the RHBoK operator requirement is enforced
	// by the separate operator-installed check).
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
		"Type":   Equal(check.ConditionTypeCompatible),
		"Status": Equal(metav1.ConditionTrue),
		"Reason": Equal(check.ReasonVersionCompatible),
	}))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactNone))
	g.Expect(result.GetImpact()).To(Equal(resultpkg.ImpactNone))
}

func TestManagementStateCheck_UnmanagedNotInUsePass(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	// Kueue is Unmanaged and NOT in use — still a supported target state → pass.
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
		"Status": Equal(metav1.ConditionTrue),
		"Reason": Equal(check.ReasonVersionCompatible),
	}))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactNone))
	g.Expect(result.GetImpact()).To(Equal(resultpkg.ImpactNone))
}

// TestManagementStateCheck_ManagedInUse_DefaultQueueName_Warns verifies the Managed+in-use path
// layers an advisory default-LocalQueue warning on top of the prohibited base condition when
// DSC spec.components.kueue.defaultLocalQueueName is "default". Overall impact stays Prohibited.
func TestManagementStateCheck_ManagedInUse_DefaultQueueName_Warns(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	ns := newNamespace("team-a", map[string]string{constants.LabelKueueManaged: "true"})

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{newDSCWithDefaultQueue("Managed", "default"), ns},
		CurrentVersion: "2.25.0",
		TargetVersion:  "3.3.2",
	})

	chk := kueue.NewManagementStateCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(2))

	base, found := conditionByType(result, check.ConditionTypeCompatible)
	g.Expect(found).To(BeTrue())
	g.Expect(base.Impact).To(Equal(resultpkg.ImpactProhibited))

	warning, found := conditionByType(result, check.ConditionTypeConfigured)
	g.Expect(found).To(BeTrue())
	g.Expect(warning.Status).To(Equal(metav1.ConditionFalse))
	g.Expect(warning.Reason).To(Equal(check.ReasonDefaultLocalQueueConflict))
	g.Expect(warning.Impact).To(Equal(resultpkg.ImpactAdvisory))
	g.Expect(warning.Remediation).ToNot(BeEmpty())

	// Prohibited base dominates the advisory warning.
	g.Expect(result.GetImpact()).To(Equal(resultpkg.ImpactProhibited))
}

// TestManagementStateCheck_ManagedInUse_DefaultQueueResource_Warns verifies the warning also fires
// when a LocalQueue named "default" exists on the cluster, and that the LocalQueue is recorded as
// an impacted object.
func TestManagementStateCheck_ManagedInUse_DefaultQueueResource_Warns(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	ns := newNamespace("team-a", map[string]string{constants.LabelKueueManaged: "true"})
	lq := newLocalQueue("team-a", "default")

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{testutil.NewDSC(map[string]string{"kueue": "Managed"}), ns, lq},
		CurrentVersion: "2.25.0",
		TargetVersion:  "3.3.2",
	})

	chk := kueue.NewManagementStateCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(2))

	warning, found := conditionByType(result, check.ConditionTypeConfigured)
	g.Expect(found).To(BeTrue())
	g.Expect(warning.Reason).To(Equal(check.ReasonDefaultLocalQueueConflict))
	g.Expect(warning.Impact).To(Equal(resultpkg.ImpactAdvisory))

	g.Expect(result.ImpactedObjects).To(HaveLen(1))
	g.Expect(result.ImpactedObjects[0].Name).To(Equal("default"))
	g.Expect(result.ImpactedObjects[0].Namespace).To(Equal("team-a"))
	g.Expect(result.GetImpact()).To(Equal(resultpkg.ImpactProhibited))
}

// TestManagementStateCheck_Unmanaged_DefaultQueueName_Warns verifies that when Kueue is Unmanaged
// (whether or not workloads are in use) and defaultLocalQueueName is "default", the base passes and
// the advisory warning surfaces without blocking the upgrade (overall impact Advisory).
func TestManagementStateCheck_Unmanaged_DefaultQueueName_Warns(t *testing.T) {
	testCases := []struct {
		name    string
		objects []*unstructured.Unstructured
	}{
		{
			name: "in use",
			objects: []*unstructured.Unstructured{
				newDSCWithDefaultQueue("Unmanaged", "default"),
				newWorkload(resources.Notebook, "team-b", "my-notebook",
					map[string]string{constants.LabelKueueQueueName: "team-queue"}),
			},
		},
		{
			name: "not in use",
			objects: []*unstructured.Unstructured{
				newDSCWithDefaultQueue("Unmanaged", "default"),
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)
			ctx := t.Context()

			target := testutil.NewTarget(t, testutil.TargetConfig{
				ListKinds:      listKinds,
				Objects:        tc.objects,
				CurrentVersion: "2.25.0",
				TargetVersion:  "3.3.2",
			})

			chk := kueue.NewManagementStateCheck()
			result, err := chk.Validate(ctx, target)

			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(result.Status.Conditions).To(HaveLen(2))

			base, found := conditionByType(result, check.ConditionTypeCompatible)
			g.Expect(found).To(BeTrue())
			g.Expect(base.Status).To(Equal(metav1.ConditionTrue))
			g.Expect(base.Impact).To(Equal(resultpkg.ImpactNone))

			warning, found := conditionByType(result, check.ConditionTypeConfigured)
			g.Expect(found).To(BeTrue())
			g.Expect(warning.Reason).To(Equal(check.ReasonDefaultLocalQueueConflict))
			g.Expect(warning.Impact).To(Equal(resultpkg.ImpactAdvisory))

			// Advisory warning surfaces but does not block the upgrade.
			g.Expect(result.GetImpact()).To(Equal(resultpkg.ImpactAdvisory))
		})
	}
}

// TestManagementStateCheck_Unmanaged_DefaultQueueResource_Warns verifies the advisory fires for an
// Unmanaged cluster with a LocalQueue named "default" and records the impacted object.
func TestManagementStateCheck_Unmanaged_DefaultQueueResource_Warns(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	lq := newLocalQueue("team-b", "default")

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{testutil.NewDSC(map[string]string{"kueue": "Unmanaged"}), lq},
		CurrentVersion: "2.25.0",
		TargetVersion:  "3.3.2",
	})

	chk := kueue.NewManagementStateCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(2))

	warning, found := conditionByType(result, check.ConditionTypeConfigured)
	g.Expect(found).To(BeTrue())
	g.Expect(warning.Reason).To(Equal(check.ReasonDefaultLocalQueueConflict))
	g.Expect(warning.Impact).To(Equal(resultpkg.ImpactAdvisory))

	g.Expect(result.ImpactedObjects).To(HaveLen(1))
	g.Expect(result.ImpactedObjects[0].Name).To(Equal("default"))
	g.Expect(result.GetImpact()).To(Equal(resultpkg.ImpactAdvisory))
}

// TestManagementStateCheck_Unmanaged_NoDefaultQueue_NoWarning verifies that a non-"default"
// LocalQueue and no defaultLocalQueueName produce only the passing base condition.
func TestManagementStateCheck_Unmanaged_NoDefaultQueue_NoWarning(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	lq := newLocalQueue("team-b", "team-queue")

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{newDSCWithDefaultQueue("Unmanaged", "team-queue"), lq},
		CurrentVersion: "2.25.0",
		TargetVersion:  "3.3.2",
	})

	chk := kueue.NewManagementStateCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Type).To(Equal(check.ConditionTypeCompatible))

	_, found := conditionByType(result, check.ConditionTypeConfigured)
	g.Expect(found).To(BeFalse())
	g.Expect(result.GetImpact()).To(Equal(resultpkg.ImpactNone))
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
		g.Expect(result.GetImpact()).To(Equal(resultpkg.ImpactProhibited))
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
		g.Expect(result.GetImpact()).To(Equal(resultpkg.ImpactProhibited))
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
	g.Expect(result.GetImpact()).To(Equal(resultpkg.ImpactProhibited))
}
