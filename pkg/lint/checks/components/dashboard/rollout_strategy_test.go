package dashboard_test

import (
	"testing"

	"github.com/blang/semver/v4"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/opendatahub-io/odh-cli/pkg/constants"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/testutil"
	"github.com/opendatahub-io/odh-cli/pkg/lint/checks/components/dashboard"

	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

func TestRolloutStrategyCheck_Metadata(t *testing.T) {
	g := NewWithT(t)

	chk := dashboard.NewRolloutStrategyCheck()

	t.Run("should have correct ID", func(_ *testing.T) {
		g.Expect(chk.ID()).To(Equal("components.dashboard.rollout-strategy"))
	})

	t.Run("should have correct Name", func(_ *testing.T) {
		g.Expect(chk.Name()).To(Equal("Components :: Dashboard :: Rollout Strategy (3.x)"))
	})

	t.Run("should have correct Group", func(_ *testing.T) {
		g.Expect(chk.Group()).To(Equal(check.GroupComponent))
	})
}

func TestRolloutStrategyCheck_CanApply(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	chk := dashboard.NewRolloutStrategyCheck()

	t.Run("should apply when upgrading from 2.x to 3.x", func(_ *testing.T) {
		targetVer := semver.MustParse("3.0.0")
		currentVer := semver.MustParse("2.17.0")

		canApply, err := chk.CanApply(ctx, check.Target{
			CurrentVersion: &currentVer,
			TargetVersion:  &targetVer,
		})
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(canApply).To(BeTrue())
	})

	t.Run("should not apply when upgrading from 3.x to 3.x", func(_ *testing.T) {
		targetVer := semver.MustParse("3.3.0")
		currentVer := semver.MustParse("3.0.0")

		canApply, err := chk.CanApply(ctx, check.Target{
			CurrentVersion: &currentVer,
			TargetVersion:  &targetVer,
		})
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(canApply).To(BeFalse())
	})
}

// runRolloutStrategyCheck builds a target holding a dashboard deployment with the
// given replica count and maxUnavailable, then runs the check against it.
func runRolloutStrategyCheck(
	t *testing.T,
	objects []*unstructured.Unstructured,
) *result.DiagnosticResult {
	t.Helper()

	g := NewWithT(t)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      resourceCapacityListKinds,
		Objects:        objects,
		CurrentVersion: "2.25.0",
		TargetVersion:  "3.5.0",
	})

	dr, err := dashboard.NewRolloutStrategyCheck().Validate(t.Context(), target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(dr).ToNot(BeNil())

	return dr
}

func TestRolloutStrategyCheck_Deadlock(t *testing.T) {
	g := NewWithT(t)

	ns := "redhat-ods-applications"
	deploy := newDashboardDeployment(ns, []containerSpec{
		{name: "dashboard", cpuReq: "500m", memoryReq: "1Gi"},
	}, 2, "25%")

	dr := runRolloutStrategyCheck(t, []*unstructured.Unstructured{
		testutil.NewDSC(map[string]string{"dashboard": "Managed"}),
		testutil.NewDSCI(ns),
		deploy,
	})

	g.Expect(dr).To(PointTo(MatchFields(IgnoreExtras, Fields{
		"Group": Equal(string(check.GroupComponent)),
		"Kind":  Equal(constants.ComponentDashboard),
	})))

	cond := findCondition(dr, "RolloutStrategy")
	g.Expect(cond).ToNot(BeNil())
	g.Expect(cond.Status).To(Equal(metav1.ConditionFalse))
	g.Expect(cond.Reason).To(Equal(check.ReasonWorkloadsImpacted))
	g.Expect(cond.Impact).To(Equal(result.ImpactAdvisory))
	g.Expect(cond.Message).To(ContainSubstring("resolves to 0"))
	g.Expect(cond.Message).To(ContainSubstring("surging 1 extra pod(s)"))
}

// newDeploymentWithStrategy builds a dashboard deployment with an arbitrary
// .spec.strategy, so tests can omit rollingUpdate fields entirely.
func newDeploymentWithStrategy(namespace string, replicas int64, strategy map[string]any) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"metadata":   map[string]any{"name": "rhods-dashboard", "namespace": namespace},
		"spec": map[string]any{
			"replicas": replicas,
			"strategy": strategy,
			"template": map[string]any{
				"spec": map[string]any{
					"containers": []any{map[string]any{"name": "dashboard"}},
				},
			},
		},
	}}
}

// An omitted maxUnavailable means 25% in Kubernetes, so it must be reported the
// same way as setting 25% explicitly.
func TestRolloutStrategyCheck_OmittedMaxUnavailableMatchesExplicitDefault(t *testing.T) {
	g := NewWithT(t)

	ns := "redhat-ods-applications"
	deploy := newDeploymentWithStrategy(ns, 2, map[string]any{"type": "RollingUpdate"})

	dr := runRolloutStrategyCheck(t, []*unstructured.Unstructured{
		testutil.NewDSC(map[string]string{"dashboard": "Managed"}),
		testutil.NewDSCI(ns),
		deploy,
	})

	cond := findCondition(dr, "RolloutStrategy")
	g.Expect(cond).ToNot(BeNil())
	g.Expect(cond.Status).To(Equal(metav1.ConditionFalse))
	g.Expect(cond.Reason).To(Equal(check.ReasonWorkloadsImpacted))
	g.Expect(cond.Message).To(ContainSubstring("Kubernetes default"))
}

// Kubernetes clamps maxUnavailable to 1 when both fenceposts resolve to 0, so
// that combination cannot stall and must not be reported.
func TestRolloutStrategyCheck_BothFencepostsZeroIsClamped(t *testing.T) {
	g := NewWithT(t)

	ns := "redhat-ods-applications"
	deploy := newDeploymentWithStrategy(ns, 2, map[string]any{
		"type": "RollingUpdate",
		"rollingUpdate": map[string]any{
			"maxUnavailable": "0",
			"maxSurge":       "0",
		},
	})

	dr := runRolloutStrategyCheck(t, []*unstructured.Unstructured{
		testutil.NewDSC(map[string]string{"dashboard": "Managed"}),
		testutil.NewDSCI(ns),
		deploy,
	})

	cond := findCondition(dr, "RolloutStrategy")
	g.Expect(cond).ToNot(BeNil())
	g.Expect(cond.Status).To(Equal(metav1.ConditionTrue))
	g.Expect(cond.Reason).To(Equal(check.ReasonRequirementsMet))
}

func TestRolloutStrategyCheck_RecreateStrategy(t *testing.T) {
	g := NewWithT(t)

	ns := "redhat-ods-applications"
	deploy := newDeploymentWithStrategy(ns, 2, map[string]any{"type": "Recreate"})

	dr := runRolloutStrategyCheck(t, []*unstructured.Unstructured{
		testutil.NewDSC(map[string]string{"dashboard": "Managed"}),
		testutil.NewDSCI(ns),
		deploy,
	})

	cond := findCondition(dr, "RolloutStrategy")
	g.Expect(cond).ToNot(BeNil())
	g.Expect(cond.Status).To(Equal(metav1.ConditionTrue))
	g.Expect(cond.Reason).To(Equal(check.ReasonRequirementsMet))
	g.Expect(cond.Message).To(ContainSubstring("Recreate"))
}

func TestRolloutStrategyCheck_ExplicitMaxUnavailableOK(t *testing.T) {
	g := NewWithT(t)

	ns := "redhat-ods-applications"
	deploy := newDashboardDeployment(ns, []containerSpec{
		{name: "dashboard", cpuReq: "500m", memoryReq: "1Gi"},
	}, 2, "1")

	dr := runRolloutStrategyCheck(t, []*unstructured.Unstructured{
		testutil.NewDSC(map[string]string{"dashboard": "Managed"}),
		testutil.NewDSCI(ns),
		deploy,
	})

	cond := findCondition(dr, "RolloutStrategy")
	g.Expect(cond).ToNot(BeNil())
	g.Expect(cond.Status).To(Equal(metav1.ConditionTrue))
	g.Expect(cond.Reason).To(Equal(check.ReasonRequirementsMet))
}

func TestRolloutStrategyCheck_PercentageRoundsAboveZero(t *testing.T) {
	g := NewWithT(t)

	ns := "redhat-ods-applications"
	deploy := newDashboardDeployment(ns, []containerSpec{
		{name: "dashboard", cpuReq: "500m", memoryReq: "1Gi"},
	}, 4, "25%")

	dr := runRolloutStrategyCheck(t, []*unstructured.Unstructured{
		testutil.NewDSC(map[string]string{"dashboard": "Managed"}),
		testutil.NewDSCI(ns),
		deploy,
	})

	cond := findCondition(dr, "RolloutStrategy")
	g.Expect(cond).ToNot(BeNil())
	g.Expect(cond.Status).To(Equal(metav1.ConditionTrue))
	g.Expect(cond.Reason).To(Equal(check.ReasonRequirementsMet))
}

func TestRolloutStrategyCheck_SingleReplicaCannotDeadlock(t *testing.T) {
	g := NewWithT(t)

	ns := "redhat-ods-applications"
	deploy := newDashboardDeployment(ns, []containerSpec{
		{name: "dashboard", cpuReq: "500m", memoryReq: "1Gi"},
	}, 1, "25%")

	dr := runRolloutStrategyCheck(t, []*unstructured.Unstructured{
		testutil.NewDSC(map[string]string{"dashboard": "Managed"}),
		testutil.NewDSCI(ns),
		deploy,
	})

	cond := findCondition(dr, "RolloutStrategy")
	g.Expect(cond).ToNot(BeNil())
	g.Expect(cond.Status).To(Equal(metav1.ConditionTrue))
	g.Expect(cond.Reason).To(Equal(check.ReasonRequirementsMet))
}

func TestRolloutStrategyCheck_NoDeployment(t *testing.T) {
	g := NewWithT(t)

	ns := "redhat-ods-applications"

	dr := runRolloutStrategyCheck(t, []*unstructured.Unstructured{
		testutil.NewDSC(map[string]string{"dashboard": "Managed"}),
		testutil.NewDSCI(ns),
	})

	cond := findCondition(dr, "RolloutStrategy")
	g.Expect(cond).ToNot(BeNil())
	g.Expect(cond.Status).To(Equal(metav1.ConditionTrue))
	g.Expect(cond.Reason).To(Equal(check.ReasonResourceNotFound))
}
