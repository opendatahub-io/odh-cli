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
var resourceCapacityListKinds = map[schema.GroupVersionResource]string{
	resources.DataScienceCluster.GVR():   resources.DataScienceCluster.ListKind(),
	resources.DataScienceClusterV1.GVR(): resources.DataScienceClusterV1.ListKind(),
	resources.ClusterAutoscaler.GVR():    resources.ClusterAutoscaler.ListKind(),
}

func newClusterAutoscaler() *unstructured.Unstructured {
	ca := &unstructured.Unstructured{}
	ca.SetGroupVersionKind(resources.ClusterAutoscaler.GVK())
	ca.SetName("default")

	return ca
}

func TestResourceCapacityCheck_Metadata(t *testing.T) {
	g := NewWithT(t)

	chk := dashboard.NewResourceCapacityCheck()

	t.Run("should have correct ID", func(_ *testing.T) {
		g.Expect(chk.ID()).To(Equal("components.dashboard.resource-capacity"))
	})

	t.Run("should have correct Name", func(_ *testing.T) {
		g.Expect(chk.Name()).To(Equal("Components :: Dashboard :: Resource Capacity (3.x)"))
	})

	t.Run("should have correct Group", func(_ *testing.T) {
		g.Expect(chk.Group()).To(Equal(check.GroupComponent))
	})
}

func TestResourceCapacityCheck_CanApply(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	chk := dashboard.NewResourceCapacityCheck()

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

func TestResourceCapacityCheck_WithAutoscaler(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	dsc := testutil.NewDSC(map[string]string{"dashboard": "Managed"})
	ca := newClusterAutoscaler()

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      resourceCapacityListKinds,
		Objects:        []*unstructured.Unstructured{dsc, ca},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := dashboard.NewResourceCapacityCheck()
	dr, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(dr).ToNot(BeNil())
	g.Expect(dr).To(PointTo(MatchFields(IgnoreExtras, Fields{
		"Group": Equal(string(check.GroupComponent)),
		"Kind":  Equal(constants.ComponentDashboard),
		"Name":  Equal("resource-capacity"),
	})))
	g.Expect(dr.Status.Conditions).To(HaveLen(1))
	g.Expect(dr.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal("ResourceCapacity"),
		"Status": Equal(metav1.ConditionFalse),
		"Reason": Equal(check.ReasonWorkloadsImpacted),
	}))
	g.Expect(dr.Status.Conditions[0].Condition.Message).To(ContainSubstring("autoscaler detected"))
	g.Expect(dr.Status.Conditions[0].Impact).To(Equal(result.ImpactAdvisory))
}

func TestResourceCapacityCheck_WithoutAutoscaler(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	dsc := testutil.NewDSC(map[string]string{"dashboard": "Managed"})

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      resourceCapacityListKinds,
		Objects:        []*unstructured.Unstructured{dsc},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := dashboard.NewResourceCapacityCheck()
	dr, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(dr).ToNot(BeNil())
	g.Expect(dr.Status.Conditions).To(HaveLen(1))
	g.Expect(dr.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal("ResourceCapacity"),
		"Status": Equal(metav1.ConditionFalse),
		"Reason": Equal(check.ReasonWorkloadsImpacted),
	}))
	g.Expect(dr.Status.Conditions[0].Condition.Message).To(ContainSubstring("No cluster autoscaler"))
	g.Expect(dr.Status.Conditions[0].Impact).To(Equal(result.ImpactAdvisory))
}
