package notebook_test

import (
	"fmt"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	resultpkg "github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/testutil"
	"github.com/opendatahub-io/odh-cli/pkg/lint/checks/workloads/notebook"
	"github.com/opendatahub-io/odh-cli/pkg/resources"

	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

//nolint:gochecknoglobals
var connIntegrityListKinds = map[schema.GroupVersionResource]string{
	resources.Notebook.GVR():           resources.Notebook.ListKind(),
	resources.Secret.GVR():             resources.Secret.ListKind(),
	resources.DataScienceCluster.GVR(): resources.DataScienceCluster.ListKind(),
}

func newSecret(name, namespace string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.Secret.APIVersion(),
			"kind":       resources.Secret.Kind,
			"metadata": map[string]any{
				"name":      name,
				"namespace": namespace,
			},
		},
	}
}

func TestConnectionIntegrityCheck_Metadata(t *testing.T) {
	g := NewWithT(t)

	chk := notebook.NewConnectionIntegrityCheck()

	g.Expect(chk.ID()).To(Equal("workloads.notebook.connection-integrity"))
	g.Expect(chk.Name()).To(Equal("Workloads :: Notebook :: Connection Integrity"))
	g.Expect(chk.Group()).To(Equal(check.GroupWorkload))
	g.Expect(chk.CheckKind()).To(Equal("notebook"))
	g.Expect(chk.CheckType()).To(Equal(string(check.CheckTypeDataIntegrity)))
	g.Expect(chk.Description()).ToNot(BeEmpty())
	g.Expect(chk.Remediation()).To(ContainSubstring("missing connection Secret"))
}

func TestConnectionIntegrityCheck_CanApply_WorkbenchesManaged(t *testing.T) {
	g := NewWithT(t)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      connIntegrityListKinds,
		Objects:        []*unstructured.Unstructured{testutil.NewDSC(map[string]string{"workbenches": "Managed"})},
		CurrentVersion: "3.0.0",
		TargetVersion:  "3.0.0",
	})

	chk := notebook.NewConnectionIntegrityCheck()
	canApply, err := chk.CanApply(t.Context(), target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeTrue())
}

func TestConnectionIntegrityCheck_CanApply_WorkbenchesRemoved(t *testing.T) {
	g := NewWithT(t)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      connIntegrityListKinds,
		Objects:        []*unstructured.Unstructured{testutil.NewDSC(map[string]string{"workbenches": "Removed"})},
		CurrentVersion: "3.0.0",
		TargetVersion:  "3.0.0",
	})

	chk := notebook.NewConnectionIntegrityCheck()
	canApply, err := chk.CanApply(t.Context(), target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeFalse())
}

func TestConnectionIntegrityCheck_NoNotebooks(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      connIntegrityListKinds,
		CurrentVersion: "3.0.0",
		TargetVersion:  "3.0.0",
	})

	chk := notebook.NewConnectionIntegrityCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(notebook.ConditionTypeConnectionIntegrity),
		"Status":  Equal(metav1.ConditionTrue),
		"Reason":  Equal(check.ReasonRequirementsMet),
		"Message": Equal(notebook.MsgAllConnectionsValid),
	}))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactNone))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "0"))
	g.Expect(result.ImpactedObjects).To(BeEmpty())
}

func TestConnectionIntegrityCheck_NotebookWithoutConnectionAnnotation(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	nb := newNotebook("plain-notebook", "user-ns", notebookOptions{})

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      connIntegrityListKinds,
		Objects:        []*unstructured.Unstructured{nb},
		CurrentVersion: "3.0.0",
		TargetVersion:  "3.0.0",
	})

	chk := notebook.NewConnectionIntegrityCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions[0].Status).To(Equal(metav1.ConditionTrue))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "0"))
	g.Expect(result.ImpactedObjects).To(BeEmpty())
}

func TestConnectionIntegrityCheck_AllSecretsExist(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	secret1 := newSecret("s3-connection", "user-ns")
	secret2 := newSecret("db-connection", "user-ns")

	nb := newNotebook("connected-notebook", "user-ns", notebookOptions{
		Annotations: map[string]any{
			notebook.AnnotationConnections: "user-ns/s3-connection,user-ns/db-connection",
		},
	})

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      connIntegrityListKinds,
		Objects:        []*unstructured.Unstructured{nb, secret1, secret2},
		CurrentVersion: "3.0.0",
		TargetVersion:  "3.0.0",
	})

	chk := notebook.NewConnectionIntegrityCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions[0].Status).To(Equal(metav1.ConditionTrue))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "0"))
	g.Expect(result.ImpactedObjects).To(BeEmpty())
}

func TestConnectionIntegrityCheck_SecretMissing(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	nb := newNotebook("broken-notebook", "user-ns", notebookOptions{
		Annotations: map[string]any{
			notebook.AnnotationConnections: "user-ns/deleted-connection",
		},
	})

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      connIntegrityListKinds,
		Objects:        []*unstructured.Unstructured{nb},
		CurrentVersion: "3.0.0",
		TargetVersion:  "3.0.0",
	})

	chk := notebook.NewConnectionIntegrityCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(notebook.ConditionTypeConnectionIntegrity),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonResourceNotFound),
		"Message": Equal(fmt.Sprintf(notebook.MsgConnectionsMissing, 1)),
	}))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactBlocking))
	g.Expect(result.Status.Conditions[0].Remediation).To(ContainSubstring("missing connection Secret"))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "1"))
	g.Expect(result.ImpactedObjects).To(HaveLen(1))
	g.Expect(result.ImpactedObjects[0].Name).To(Equal("broken-notebook"))
	g.Expect(result.ImpactedObjects[0].Namespace).To(Equal("user-ns"))
}

func TestConnectionIntegrityCheck_OneOfMultipleSecretsMissing(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	// First secret exists, second does not
	secret := newSecret("s3-connection", "user-ns")

	nb := newNotebook("partial-notebook", "user-ns", notebookOptions{
		Annotations: map[string]any{
			notebook.AnnotationConnections: "user-ns/s3-connection,user-ns/deleted-secret",
		},
	})

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      connIntegrityListKinds,
		Objects:        []*unstructured.Unstructured{nb, secret},
		CurrentVersion: "3.0.0",
		TargetVersion:  "3.0.0",
	})

	chk := notebook.NewConnectionIntegrityCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions[0].Status).To(Equal(metav1.ConditionFalse))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "1"))
	g.Expect(result.ImpactedObjects).To(HaveLen(1))
	g.Expect(result.ImpactedObjects[0].Name).To(Equal("partial-notebook"))
}

func TestConnectionIntegrityCheck_MixedNotebooks(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	secret := newSecret("good-conn", "ns1")

	// Notebook with valid connection
	nbGood := newNotebook("good-notebook", "ns1", notebookOptions{
		Annotations: map[string]any{
			notebook.AnnotationConnections: "ns1/good-conn",
		},
	})

	// Notebook without annotation (should be skipped)
	nbPlain := newNotebook("plain-notebook", "ns2", notebookOptions{})

	// Notebook with missing connection
	nbBroken := newNotebook("broken-notebook", "ns3", notebookOptions{
		Annotations: map[string]any{
			notebook.AnnotationConnections: "ns3/gone-secret",
		},
	})

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      connIntegrityListKinds,
		Objects:        []*unstructured.Unstructured{secret, nbGood, nbPlain, nbBroken},
		CurrentVersion: "3.0.0",
		TargetVersion:  "3.0.0",
	})

	chk := notebook.NewConnectionIntegrityCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions[0].Status).To(Equal(metav1.ConditionFalse))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "1"))
	g.Expect(result.ImpactedObjects).To(HaveLen(1))
	g.Expect(result.ImpactedObjects[0].Name).To(Equal("broken-notebook"))
}

func TestConnectionIntegrityCheck_SecretInWrongNamespace(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	// Secret exists but in a different namespace than what the annotation references
	secret := newSecret("my-conn", "other-ns")

	nb := newNotebook("notebook-wrong-ns", "user-ns", notebookOptions{
		Annotations: map[string]any{
			notebook.AnnotationConnections: "user-ns/my-conn",
		},
	})

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      connIntegrityListKinds,
		Objects:        []*unstructured.Unstructured{secret, nb},
		CurrentVersion: "3.0.0",
		TargetVersion:  "3.0.0",
	})

	chk := notebook.NewConnectionIntegrityCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions[0].Status).To(Equal(metav1.ConditionFalse))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "1"))
	g.Expect(result.ImpactedObjects).To(HaveLen(1))
}

func TestConnectionIntegrityCheck_NotebookFlaggedOnlyOnce(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	// Notebook with multiple missing connections should only appear once in impacted
	nb := newNotebook("multi-broken", "user-ns", notebookOptions{
		Annotations: map[string]any{
			notebook.AnnotationConnections: "user-ns/missing1,user-ns/missing2,user-ns/missing3",
		},
	})

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      connIntegrityListKinds,
		Objects:        []*unstructured.Unstructured{nb},
		CurrentVersion: "3.0.0",
		TargetVersion:  "3.0.0",
	})

	chk := notebook.NewConnectionIntegrityCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "1"))
	g.Expect(result.ImpactedObjects).To(HaveLen(1))
}

func TestConnectionIntegrityCheck_AnnotationTargetVersion(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      connIntegrityListKinds,
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := notebook.NewConnectionIntegrityCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationCheckTargetVersion, "3.0.0"))
}
