package dashboard_test

import (
	"testing"

	"github.com/blang/semver/v4"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/opendatahub-io/odh-cli/pkg/constants"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/testutil"
	"github.com/opendatahub-io/odh-cli/pkg/lint/checks/components/dashboard"
	"github.com/opendatahub-io/odh-cli/pkg/resources"

	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

//nolint:gochecknoglobals // Test fixture - shared across test functions
var authModelListKinds = map[schema.GroupVersionResource]string{
	resources.DataScienceCluster.GVR():   resources.DataScienceCluster.ListKind(),
	resources.DataScienceClusterV1.GVR(): resources.DataScienceClusterV1.ListKind(),
	resources.DSCInitialization.GVR():    resources.DSCInitialization.ListKind(),
	resources.DSCInitializationV1.GVR():  resources.DSCInitializationV1.ListKind(),
	resources.Pod.GVR():                  resources.Pod.ListKind(),
}

func newDashboardPod(name, namespace string, containerNames ...string) *unstructured.Unstructured {
	containers := make([]any, len(containerNames))
	for i, cn := range containerNames {
		containers[i] = map[string]any{"name": cn}
	}

	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "Pod",
		"metadata": map[string]any{
			"name":      name,
			"namespace": namespace,
			"labels":    map[string]any{"app": "rhods-dashboard"},
		},
		"spec": map[string]any{"containers": containers},
	}}
}

func TestAuthModelMigrationCheck_Metadata(t *testing.T) {
	g := NewWithT(t)

	chk := dashboard.NewAuthModelMigrationCheck()

	t.Run("should have correct ID", func(_ *testing.T) {
		g.Expect(chk.ID()).To(Equal("components.dashboard.auth-model-migration"))
	})

	t.Run("should have correct Name", func(_ *testing.T) {
		g.Expect(chk.Name()).To(Equal("Components :: Dashboard :: Auth Model Migration (3.x)"))
	})

	t.Run("should have correct Group", func(_ *testing.T) {
		g.Expect(chk.Group()).To(Equal(check.GroupComponent))
	})
}

func TestAuthModelMigrationCheck_CanApply(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	chk := dashboard.NewAuthModelMigrationCheck()

	t.Run("should apply when upgrading from 2.x to 3.x", func(_ *testing.T) {
		targetVer := semver.MustParse("3.0.0")
		currentVer := semver.MustParse("2.17.0")

		target := check.Target{
			CurrentVersion: &currentVer,
			TargetVersion:  &targetVer,
		}

		canApply, err := chk.CanApply(ctx, target)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(canApply).To(BeTrue())
	})

	t.Run("should not apply when upgrading from 3.x to 3.x", func(_ *testing.T) {
		targetVer := semver.MustParse("3.3.0")
		currentVer := semver.MustParse("3.0.0")

		target := check.Target{
			CurrentVersion: &currentVer,
			TargetVersion:  &targetVer,
		}

		canApply, err := chk.CanApply(ctx, target)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(canApply).To(BeFalse())
	})
}

func TestAuthModelMigrationCheck_NoPods(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	dsc := testutil.NewDSC(map[string]string{"dashboard": "Managed"})
	dsci := testutil.NewDSCI("redhat-ods-applications")

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      authModelListKinds,
		Objects:        []*unstructured.Unstructured{dsc, dsci},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := dashboard.NewAuthModelMigrationCheck()
	dr, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(dr).ToNot(BeNil())
	g.Expect(dr).To(PointTo(MatchFields(IgnoreExtras, Fields{
		"Group": Equal(string(check.GroupComponent)),
		"Kind":  Equal(constants.ComponentDashboard),
		"Name":  Equal("auth-model-migration"),
	})))
	g.Expect(dr.Status.Conditions).To(HaveLen(1))
	g.Expect(dr.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal("AuthModelMigration"),
		"Status": Equal(metav1.ConditionTrue),
		"Reason": Equal(check.ReasonRequirementsMet),
	}))
}

func TestAuthModelMigrationCheck_WithOAuthProxy(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	dsc := testutil.NewDSC(map[string]string{"dashboard": "Managed"})
	dsci := testutil.NewDSCI("redhat-ods-applications")
	pod := newDashboardPod("dashboard-1", "redhat-ods-applications", "dashboard", "oauth-proxy")

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      authModelListKinds,
		Objects:        []*unstructured.Unstructured{dsc, dsci, pod},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := dashboard.NewAuthModelMigrationCheck()
	dr, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(dr).ToNot(BeNil())
	g.Expect(dr.Status.Conditions).To(HaveLen(1))
	g.Expect(dr.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal("AuthModelMigration"),
		"Status": Equal(metav1.ConditionFalse),
		"Reason": Equal(check.ReasonMigrationPending),
	}))
	g.Expect(dr.Status.Conditions[0].Condition.Message).To(ContainSubstring("1"))
	g.Expect(dr.Status.Conditions[0].Impact).To(Equal(result.ImpactAdvisory))
}

func TestAuthModelMigrationCheck_WithKubeRBACProxy(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	dsc := testutil.NewDSC(map[string]string{"dashboard": "Managed"})
	dsci := testutil.NewDSCI("redhat-ods-applications")
	pod := newDashboardPod("dashboard-1", "redhat-ods-applications", "dashboard", "kube-rbac-proxy")

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      authModelListKinds,
		Objects:        []*unstructured.Unstructured{dsc, dsci, pod},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := dashboard.NewAuthModelMigrationCheck()
	dr, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(dr).ToNot(BeNil())
	g.Expect(dr.Status.Conditions).To(HaveLen(1))
	g.Expect(dr.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal("AuthModelMigration"),
		"Status": Equal(metav1.ConditionTrue),
		"Reason": Equal(check.ReasonRequirementsMet),
	}))
}
