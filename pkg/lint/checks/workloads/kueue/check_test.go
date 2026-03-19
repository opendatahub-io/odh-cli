package kueue_test

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	resultpkg "github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/testutil"
	kueuecheck "github.com/opendatahub-io/odh-cli/pkg/lint/checks/workloads/kueue"
	"github.com/opendatahub-io/odh-cli/pkg/resources"

	. "github.com/onsi/gomega"
)

//nolint:gochecknoglobals // Test fixture - shared across test functions.
var listKinds = map[schema.GroupVersionResource]string{
	resources.DataScienceCluster.GVR():  resources.DataScienceCluster.ListKind(),
	resources.Namespace.GVR():           resources.Namespace.ListKind(),
	resources.Notebook.GVR():            resources.Notebook.ListKind(),
	resources.InferenceService.GVR():    resources.InferenceService.ListKind(),
	resources.LLMInferenceService.GVR(): resources.LLMInferenceService.ListKind(),
	resources.RayCluster.GVR():          resources.RayCluster.ListKind(),
	resources.RayJob.GVR():              resources.RayJob.ListKind(),
	resources.PyTorchJob.GVR():          resources.PyTorchJob.ListKind(),
	resources.Pod.GVR():                 resources.Pod.ListKind(),
	resources.Deployment.GVR():          resources.Deployment.ListKind(),
	resources.StatefulSet.GVR():         resources.StatefulSet.ListKind(),
	resources.ReplicaSet.GVR():          resources.ReplicaSet.ListKind(),
	resources.DaemonSet.GVR():           resources.DaemonSet.ListKind(),
	resources.Job.GVR():                 resources.Job.ListKind(),
	resources.CronJob.GVR():             resources.CronJob.ListKind(),
}

func newNamespace(name string, labels map[string]string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.Namespace.APIVersion(),
			"kind":       resources.Namespace.Kind,
			"metadata": map[string]any{
				"name":   name,
				"labels": toAnyMap(labels),
			},
		},
	}
}

func newWorkload(
	rt resources.ResourceType,
	namespace string,
	name string,
	uid string,
	labels map[string]string,
) *unstructured.Unstructured {
	meta := map[string]any{
		"name":      name,
		"namespace": namespace,
		"uid":       uid,
	}
	if labels != nil {
		meta["labels"] = toAnyMap(labels)
	}

	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": rt.APIVersion(),
			"kind":       rt.Kind,
			"metadata":   meta,
		},
	}
}

func newOwnedResource(
	rt resources.ResourceType,
	namespace string,
	name string,
	uid string,
	ownerUID string,
	ownerKind string,
	labels map[string]string,
) *unstructured.Unstructured {
	meta := map[string]any{
		"name":      name,
		"namespace": namespace,
		"uid":       uid,
		"ownerReferences": []any{
			map[string]any{
				"apiVersion": "v1",
				"kind":       ownerKind,
				"name":       "owner",
				"uid":        ownerUID,
			},
		},
	}
	if labels != nil {
		meta["labels"] = toAnyMap(labels)
	}

	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": rt.APIVersion(),
			"kind":       rt.Kind,
			"metadata":   meta,
		},
	}
}

func toAnyMap(m map[string]string) map[string]any {
	result := make(map[string]any, len(m))
	for k, v := range m {
		result[k] = v
	}

	return result
}

func TestDataIntegrityCheck_CanApply_KueueNotActive(t *testing.T) {
	g := NewWithT(t)

	dsc := testutil.NewDSC(map[string]string{
		"kueue": "Removed",
	})

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{dsc},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := kueuecheck.NewDataIntegrityCheck()
	applies, err := chk.CanApply(t.Context(), target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(applies).To(BeFalse())
}

func TestDataIntegrityCheck_CanApply_KueueManaged(t *testing.T) {
	g := NewWithT(t)

	dsc := testutil.NewDSC(map[string]string{
		"kueue": "Managed",
	})

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{dsc},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := kueuecheck.NewDataIntegrityCheck()
	applies, err := chk.CanApply(t.Context(), target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(applies).To(BeFalse())
}

func TestDataIntegrityCheck_CanApply_KueueUnmanaged(t *testing.T) {
	g := NewWithT(t)

	dsc := testutil.NewDSC(map[string]string{
		"kueue": "Unmanaged",
	})

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{dsc},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := kueuecheck.NewDataIntegrityCheck()
	applies, err := chk.CanApply(t.Context(), target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(applies).To(BeTrue())
}

func TestDataIntegrityCheck_BenignWorkload_NoViolations(t *testing.T) {
	g := NewWithT(t)

	dsc := testutil.NewDSC(map[string]string{
		"kueue": "Unmanaged",
	})

	// Non-kueue namespace.
	ns := newNamespace("team-b", nil)

	// Workload without queue-name label in non-kueue namespace — completely benign.
	nb := newWorkload(resources.Notebook, "team-b", "my-notebook", "nb-uid-1", nil)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{dsc, ns, nb},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := kueuecheck.NewDataIntegrityCheck()
	result, err := chk.Validate(t.Context(), target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0]).To(And(
		HaveField("Type", Equal("KueueConsistency")),
		HaveField("Status", Equal(metav1.ConditionTrue)),
	))
	g.Expect(result.ImpactedObjects).To(BeEmpty())
}

func TestDataIntegrityCheck_NoRelevantNamespaces(t *testing.T) {
	g := NewWithT(t)

	dsc := testutil.NewDSC(map[string]string{
		"kueue": "Unmanaged",
	})

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{dsc},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := kueuecheck.NewDataIntegrityCheck()
	result, err := chk.Validate(t.Context(), target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0]).To(And(
		HaveField("Type", Equal("KueueConsistency")),
		HaveField("Status", Equal(metav1.ConditionTrue)),
	))
	g.Expect(result.ImpactedObjects).To(BeEmpty())
}

func TestDataIntegrityCheck_FullyConsistent(t *testing.T) {
	g := NewWithT(t)

	dsc := testutil.NewDSC(map[string]string{
		"kueue": "Unmanaged",
	})

	ns := newNamespace("team-a", map[string]string{
		"kueue-managed": "true",
	})

	nb := newWorkload(resources.Notebook, "team-a", "my-notebook", "nb-uid-1",
		map[string]string{"kueue.x-k8s.io/queue-name": "default-queue"})

	// Pod owned by the notebook with matching label.
	pod := newOwnedResource(resources.Pod, "team-a", "my-notebook-0", "pod-uid-1",
		"nb-uid-1", "Notebook",
		map[string]string{"kueue.x-k8s.io/queue-name": "default-queue"})

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{dsc, ns, nb, pod},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := kueuecheck.NewDataIntegrityCheck()
	result, err := chk.Validate(t.Context(), target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0]).To(And(
		HaveField("Type", Equal("KueueConsistency")),
		HaveField("Status", Equal(metav1.ConditionTrue)),
		HaveField("Reason", Equal(check.ReasonRequirementsMet)),
	))
	g.Expect(result.ImpactedObjects).To(BeEmpty())
}

func TestDataIntegrityCheck_Invariant1_MissingLabelInKueueNamespace(t *testing.T) {
	g := NewWithT(t)

	dsc := testutil.NewDSC(map[string]string{
		"kueue": "Unmanaged",
	})

	ns := newNamespace("team-a", map[string]string{
		"kueue-managed": "true",
	})

	// Notebook in kueue-managed namespace but WITHOUT queue-name label.
	nb := newWorkload(resources.Notebook, "team-a", "my-notebook", "nb-uid-1", nil)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{dsc, ns, nb},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := kueuecheck.NewDataIntegrityCheck()
	result, err := chk.Validate(t.Context(), target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0]).To(And(
		HaveField("Type", Equal("KueueConsistency")),
		HaveField("Status", Equal(metav1.ConditionFalse)),
	))
	g.Expect(result.ImpactedObjects).To(HaveLen(1))
	g.Expect(result.ImpactedObjects[0]).To(And(
		HaveField("Namespace", Equal("team-a")),
		HaveField("Name", Equal("my-notebook")),
	))
}

func TestDataIntegrityCheck_Invariant2_LabeledInNonKueueNamespace(t *testing.T) {
	g := NewWithT(t)

	dsc := testutil.NewDSC(map[string]string{
		"kueue": "Unmanaged",
	})

	// Namespace WITHOUT kueue-managed label.
	ns := newNamespace("team-b", nil)

	// Notebook WITH queue-name label in non-kueue namespace.
	nb := newWorkload(resources.Notebook, "team-b", "my-notebook", "nb-uid-1",
		map[string]string{"kueue.x-k8s.io/queue-name": "default-queue"})

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{dsc, ns, nb},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := kueuecheck.NewDataIntegrityCheck()
	result, err := chk.Validate(t.Context(), target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0]).To(And(
		HaveField("Type", Equal("KueueConsistency")),
		HaveField("Status", Equal(metav1.ConditionFalse)),
	))
	g.Expect(result.ImpactedObjects).To(HaveLen(1))
}

func TestDataIntegrityCheck_Invariant3_OwnerTreeMismatch(t *testing.T) {
	g := NewWithT(t)

	dsc := testutil.NewDSC(map[string]string{
		"kueue": "Unmanaged",
	})

	ns := newNamespace("team-a", map[string]string{
		"kueue-managed": "true",
	})

	// Notebook with queue-name label.
	nb := newWorkload(resources.Notebook, "team-a", "my-notebook", "nb-uid-1",
		map[string]string{"kueue.x-k8s.io/queue-name": "default-queue"})

	// Pod owned by notebook but MISSING queue-name label.
	pod := newOwnedResource(resources.Pod, "team-a", "my-notebook-0", "pod-uid-1",
		"nb-uid-1", "Notebook", nil)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{dsc, ns, nb, pod},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := kueuecheck.NewDataIntegrityCheck()
	result, err := chk.Validate(t.Context(), target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0]).To(And(
		HaveField("Type", Equal("KueueConsistency")),
		HaveField("Status", Equal(metav1.ConditionFalse)),
	))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "1"))
	g.Expect(result.ImpactedObjects).To(HaveLen(1))
}

func TestDataIntegrityCheck_Invariant3_ChildHasLabelRootDoesNot(t *testing.T) {
	g := NewWithT(t)

	dsc := testutil.NewDSC(map[string]string{
		"kueue": "Unmanaged",
	})

	ns := newNamespace("team-a", map[string]string{
		"kueue-managed": "true",
	})

	// Notebook WITHOUT queue-name label (but in kueue namespace — so invariant 1 fires first).
	// To isolate invariant 3, the notebook needs the label but pod has a different value.
	nb := newWorkload(resources.Notebook, "team-a", "my-notebook", "nb-uid-1",
		map[string]string{"kueue.x-k8s.io/queue-name": "queue-a"})

	// Pod owned by notebook with DIFFERENT queue-name value.
	pod := newOwnedResource(resources.Pod, "team-a", "my-notebook-0", "pod-uid-1",
		"nb-uid-1", "Notebook",
		map[string]string{"kueue.x-k8s.io/queue-name": "queue-b"})

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{dsc, ns, nb, pod},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := kueuecheck.NewDataIntegrityCheck()
	result, err := chk.Validate(t.Context(), target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions[0]).To(And(
		HaveField("Type", Equal("KueueConsistency")),
		HaveField("Status", Equal(metav1.ConditionFalse)),
	))
	g.Expect(result.ImpactedObjects).To(HaveLen(1))
}

func TestDataIntegrityCheck_Invariant3_SingleNodeTreeConsistent(t *testing.T) {
	g := NewWithT(t)

	dsc := testutil.NewDSC(map[string]string{
		"kueue": "Unmanaged",
	})

	ns := newNamespace("team-a", map[string]string{
		"kueue-managed": "true",
	})

	// Notebook with label, no children — trivially consistent.
	nb := newWorkload(resources.Notebook, "team-a", "my-notebook", "nb-uid-1",
		map[string]string{"kueue.x-k8s.io/queue-name": "default-queue"})

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{dsc, ns, nb},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := kueuecheck.NewDataIntegrityCheck()
	result, err := chk.Validate(t.Context(), target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0]).To(And(
		HaveField("Type", Equal("KueueConsistency")),
		HaveField("Status", Equal(metav1.ConditionTrue)),
	))
	g.Expect(result.ImpactedObjects).To(BeEmpty())
}

func TestDataIntegrityCheck_MultiLevelOwnerChain(t *testing.T) {
	g := NewWithT(t)

	dsc := testutil.NewDSC(map[string]string{
		"kueue": "Unmanaged",
	})

	ns := newNamespace("team-a", map[string]string{
		"kueue-managed": "true",
	})

	queueLabels := map[string]string{"kueue.x-k8s.io/queue-name": "default-queue"}

	// InferenceService → Deployment → ReplicaSet → Pod, all with matching labels.
	isvc := newWorkload(resources.InferenceService, "team-a", "my-isvc", "isvc-uid-1", queueLabels)

	deploy := newOwnedResource(resources.Deployment, "team-a", "my-isvc-deploy", "deploy-uid-1",
		"isvc-uid-1", "InferenceService", queueLabels)

	rs := newOwnedResource(resources.ReplicaSet, "team-a", "my-isvc-rs", "rs-uid-1",
		"deploy-uid-1", "Deployment", queueLabels)

	pod := newOwnedResource(resources.Pod, "team-a", "my-isvc-pod", "pod-uid-1",
		"rs-uid-1", "ReplicaSet", queueLabels)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{dsc, ns, isvc, deploy, rs, pod},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := kueuecheck.NewDataIntegrityCheck()
	result, err := chk.Validate(t.Context(), target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions[0]).To(And(
		HaveField("Type", Equal("KueueConsistency")),
		HaveField("Status", Equal(metav1.ConditionTrue)),
	))
}

func TestDataIntegrityCheck_MixedViolationsAcrossNamespaces(t *testing.T) {
	g := NewWithT(t)

	dsc := testutil.NewDSC(map[string]string{
		"kueue": "Unmanaged",
	})

	kueueNs := newNamespace("team-a", map[string]string{
		"kueue-managed": "true",
	})

	// Non-kueue namespace.
	regularNs := newNamespace("team-b", nil)

	// Invariant 1 violation: notebook in kueue-ns missing label.
	nb1 := newWorkload(resources.Notebook, "team-a", "unlabeled-nb", "nb-uid-1", nil)

	// Invariant 2 violation: notebook in non-kueue-ns with label.
	nb2 := newWorkload(resources.Notebook, "team-b", "labeled-nb", "nb-uid-2",
		map[string]string{"kueue.x-k8s.io/queue-name": "some-queue"})

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{dsc, kueueNs, regularNs, nb1, nb2},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := kueuecheck.NewDataIntegrityCheck()
	result, err := chk.Validate(t.Context(), target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions[0]).To(And(
		HaveField("Type", Equal("KueueConsistency")),
		HaveField("Status", Equal(metav1.ConditionFalse)),
	))
	// Two distinct violations across two namespaces.
	g.Expect(result.ImpactedObjects).To(HaveLen(2))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "2"))
}

func TestDataIntegrityCheck_BlockingImpact(t *testing.T) {
	g := NewWithT(t)

	dsc := testutil.NewDSC(map[string]string{
		"kueue": "Unmanaged",
	})

	ns := newNamespace("team-a", map[string]string{
		"kueue-managed": "true",
	})

	// Invariant 1 violation.
	nb := newWorkload(resources.Notebook, "team-a", "my-notebook", "nb-uid-1", nil)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{dsc, ns, nb},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := kueuecheck.NewDataIntegrityCheck()
	result, err := chk.Validate(t.Context(), target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions[0]).To(HaveField("Impact", Equal(resultpkg.ImpactProhibited)))
}

func TestDataIntegrityCheck_AnnotationCheckTargetVersion(t *testing.T) {
	g := NewWithT(t)

	dsc := testutil.NewDSC(map[string]string{
		"kueue": "Unmanaged",
	})

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{dsc},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := kueuecheck.NewDataIntegrityCheck()
	result, err := chk.Validate(t.Context(), target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationCheckTargetVersion, "3.0.0"))
}

func TestDataIntegrityCheck_CheckMetadata(t *testing.T) {
	g := NewWithT(t)

	chk := kueuecheck.NewDataIntegrityCheck()

	g.Expect(chk.ID()).To(Equal("workloads.kueue.data-integrity"))
	g.Expect(chk.Name()).To(Equal("Workloads :: Kueue :: Data Integrity"))
	g.Expect(chk.Group()).To(Equal(check.GroupWorkload))
	g.Expect(chk.CheckKind()).To(Equal("kueue"))
	g.Expect(chk.CheckType()).To(Equal("data-integrity"))
	g.Expect(chk.Description()).ToNot(BeEmpty())
	g.Expect(chk.Remediation()).ToNot(BeEmpty())
}

func TestDataIntegrityCheck_OpenshiftManagedLabel(t *testing.T) {
	g := NewWithT(t)

	dsc := testutil.NewDSC(map[string]string{
		"kueue": "Unmanaged",
	})

	// Uses the kueue.openshift.io/managed label instead of kueue-managed.
	ns := newNamespace("team-a", map[string]string{
		"kueue.openshift.io/managed": "true",
	})

	nb := newWorkload(resources.Notebook, "team-a", "my-notebook", "nb-uid-1",
		map[string]string{"kueue.x-k8s.io/queue-name": "default-queue"})

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{dsc, ns, nb},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := kueuecheck.NewDataIntegrityCheck()
	result, err := chk.Validate(t.Context(), target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions[0]).To(And(
		HaveField("Type", Equal("KueueConsistency")),
		HaveField("Status", Equal(metav1.ConditionTrue)),
	))
}

func TestDataIntegrityCheck_Invariant1_ObjectContextAnnotation(t *testing.T) {
	g := NewWithT(t)

	dsc := testutil.NewDSC(map[string]string{
		"kueue": "Unmanaged",
	})

	ns := newNamespace("team-a", map[string]string{
		"kueue-managed": "true",
	})

	// Invariant 1: notebook in kueue namespace missing label.
	nb := newWorkload(resources.Notebook, "team-a", "my-notebook", "nb-uid-1", nil)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{dsc, ns, nb},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := kueuecheck.NewDataIntegrityCheck()
	result, err := chk.Validate(t.Context(), target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.ImpactedObjects).To(HaveLen(1))
	g.Expect(result.ImpactedObjects[0].Annotations).To(
		HaveKeyWithValue(resultpkg.AnnotationObjectContext,
			"Notebook team-a/my-notebook is in kueue-managed namespace team-a but missing kueue.x-k8s.io/queue-name label"),
	)
}

func TestDataIntegrityCheck_Invariant2_ObjectContextAnnotation(t *testing.T) {
	g := NewWithT(t)

	dsc := testutil.NewDSC(map[string]string{
		"kueue": "Unmanaged",
	})

	ns := newNamespace("team-b", nil)

	// Invariant 2: notebook with label in non-kueue namespace.
	nb := newWorkload(resources.Notebook, "team-b", "my-notebook", "nb-uid-1",
		map[string]string{"kueue.x-k8s.io/queue-name": "default-queue"})

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{dsc, ns, nb},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := kueuecheck.NewDataIntegrityCheck()
	result, err := chk.Validate(t.Context(), target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.ImpactedObjects).To(HaveLen(1))
	g.Expect(result.ImpactedObjects[0].Annotations).To(
		HaveKeyWithValue(resultpkg.AnnotationObjectContext,
			"Notebook team-b/my-notebook has kueue.x-k8s.io/queue-name=default-queue but namespace is not kueue-managed"),
	)
}

func TestDataIntegrityCheck_Invariant3_ObjectContextAnnotation(t *testing.T) {
	g := NewWithT(t)

	dsc := testutil.NewDSC(map[string]string{
		"kueue": "Unmanaged",
	})

	ns := newNamespace("team-a", map[string]string{
		"kueue-managed": "true",
	})

	nb := newWorkload(resources.Notebook, "team-a", "my-notebook", "nb-uid-1",
		map[string]string{"kueue.x-k8s.io/queue-name": "default-queue"})

	// Pod missing the label — triggers invariant 3.
	pod := newOwnedResource(resources.Pod, "team-a", "my-notebook-0", "pod-uid-1",
		"nb-uid-1", "Notebook", nil)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{dsc, ns, nb, pod},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := kueuecheck.NewDataIntegrityCheck()
	result, err := chk.Validate(t.Context(), target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.ImpactedObjects).To(HaveLen(1))
	g.Expect(result.ImpactedObjects[0].Annotations).To(
		HaveKeyWithValue(resultpkg.AnnotationObjectContext,
			"Notebook team-a/my-notebook has kueue.x-k8s.io/queue-name=default-queue but descendant Pod team-a/my-notebook-0 is missing the label"),
	)
}

func TestDataIntegrityCheck_MultipleWorkloadTypes(t *testing.T) {
	g := NewWithT(t)

	dsc := testutil.NewDSC(map[string]string{
		"kueue": "Unmanaged",
	})

	ns := newNamespace("team-a", map[string]string{
		"kueue-managed": "true",
	})

	queueLabels := map[string]string{"kueue.x-k8s.io/queue-name": "default-queue"}

	nb := newWorkload(resources.Notebook, "team-a", "nb1", "nb-uid-1", queueLabels)
	isvc := newWorkload(resources.InferenceService, "team-a", "isvc1", "isvc-uid-1", queueLabels)
	ray := newWorkload(resources.RayCluster, "team-a", "ray1", "ray-uid-1", queueLabels)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{dsc, ns, nb, isvc, ray},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := kueuecheck.NewDataIntegrityCheck()
	result, err := chk.Validate(t.Context(), target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions[0]).To(HaveField("Status", Equal(metav1.ConditionTrue)))
}
