package trustyai_test

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	resultpkg "github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/testutil"
	"github.com/opendatahub-io/odh-cli/pkg/lint/checks/workloads/trustyai"

	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

func TestDatabaseStorageMigrationCheck_NoTAS(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        nil,
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := trustyai.NewDatabaseStorageMigrationCheck()
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

func TestDatabaseStorageMigrationCheck_NoDatabaseStorage(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{newTestTAS("tas-pvc", "test-ns", "PVC")},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := trustyai.NewDatabaseStorageMigrationCheck()
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

func TestDatabaseStorageMigrationCheck_WithDatabaseStorage(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{newTestTAS("tas-db", "prod-ns", "DATABASE")},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := trustyai.NewDatabaseStorageMigrationCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeCompatible),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonMigrationPending),
		"Message": ContainSubstring("Found 1 TrustyAIService"),
	}))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactAdvisory))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "1"))
	g.Expect(result.ImpactedObjects).To(HaveLen(1))
	g.Expect(result.ImpactedObjects[0].Name).To(Equal("tas-db"))
	g.Expect(result.ImpactedObjects[0].Namespace).To(Equal("prod-ns"))
}

func TestDatabaseStorageMigrationCheck_Metadata(t *testing.T) {
	g := NewWithT(t)

	chk := trustyai.NewDatabaseStorageMigrationCheck()

	g.Expect(chk.ID()).To(Equal("workloads.trustyai.database-storage-migration"))
	g.Expect(chk.Name()).To(Equal("Workloads :: TrustyAI :: Database Storage Migration (3.x)"))
	g.Expect(chk.Group()).To(Equal(check.GroupWorkload))
	g.Expect(chk.Description()).ToNot(BeEmpty())
}

func TestDatabaseStorageMigrationCheck_CanApply_NilVersions(t *testing.T) {
	g := NewWithT(t)

	chk := trustyai.NewDatabaseStorageMigrationCheck()
	canApply, err := chk.CanApply(t.Context(), check.Target{})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeFalse())
}

func TestDatabaseStorageMigrationCheck_CanApply_LintMode(t *testing.T) {
	g := NewWithT(t)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{testutil.NewDSC(map[string]string{"trustyai": "Managed"})},
		CurrentVersion: "2.17.0",
		TargetVersion:  "2.17.0",
	})

	chk := trustyai.NewDatabaseStorageMigrationCheck()
	canApply, err := chk.CanApply(t.Context(), target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeFalse())
}

func TestDatabaseStorageMigrationCheck_CanApply_2xTo3x_Managed(t *testing.T) {
	g := NewWithT(t)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{testutil.NewDSC(map[string]string{"trustyai": "Managed"})},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := trustyai.NewDatabaseStorageMigrationCheck()
	canApply, err := chk.CanApply(t.Context(), target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeTrue())
}

func TestDatabaseStorageMigrationCheck_CanApply_2xTo3x_Removed(t *testing.T) {
	g := NewWithT(t)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{testutil.NewDSC(map[string]string{"trustyai": "Removed"})},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := trustyai.NewDatabaseStorageMigrationCheck()
	canApply, err := chk.CanApply(t.Context(), target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeFalse())
}

func TestDatabaseStorageMigrationCheck_CanApply_3xTo3x(t *testing.T) {
	g := NewWithT(t)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{testutil.NewDSC(map[string]string{"trustyai": "Managed"})},
		CurrentVersion: "3.0.0",
		TargetVersion:  "3.3.0",
	})

	chk := trustyai.NewDatabaseStorageMigrationCheck()
	canApply, err := chk.CanApply(t.Context(), target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeFalse())
}
