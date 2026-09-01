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
var configCompatibilityListKinds = map[schema.GroupVersionResource]string{
	resources.DataScienceCluster.GVR():   resources.DataScienceCluster.ListKind(),
	resources.DataScienceClusterV1.GVR(): resources.DataScienceClusterV1.ListKind(),
	resources.DSCInitialization.GVR():    resources.DSCInitialization.ListKind(),
	resources.DSCInitializationV1.GVR():  resources.DSCInitializationV1.ListKind(),
	resources.OdhDashboardConfig.GVR():   resources.OdhDashboardConfig.ListKind(),
}

func newOdhDashboardConfig(namespace, version string) *unstructured.Unstructured {
	cfg := &unstructured.Unstructured{}
	cfg.SetGroupVersionKind(resources.OdhDashboardConfig.GVK())
	cfg.SetName("odh-dashboard-config")
	cfg.SetNamespace(namespace)

	if version != "" {
		cfg.SetAnnotations(map[string]string{"platform.opendatahub.io/version": version})
	}

	return cfg
}

func TestConfigCompatibilityCheck_Metadata(t *testing.T) {
	g := NewWithT(t)

	chk := dashboard.NewConfigCompatibilityCheck()

	t.Run("should have correct ID", func(_ *testing.T) {
		g.Expect(chk.ID()).To(Equal("components.dashboard.config-compatibility"))
	})

	t.Run("should have correct Name", func(_ *testing.T) {
		g.Expect(chk.Name()).To(Equal("Components :: Dashboard :: Config Compatibility (3.x)"))
	})

	t.Run("should have correct Group", func(_ *testing.T) {
		g.Expect(chk.Group()).To(Equal(check.GroupComponent))
	})
}

func TestConfigCompatibilityCheck_CanApply(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	chk := dashboard.NewConfigCompatibilityCheck()

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

func TestConfigCompatibilityCheck_Found(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	dsc := testutil.NewDSC(map[string]string{"dashboard": "Managed"})
	dsci := testutil.NewDSCI("redhat-ods-applications")
	cfg := newOdhDashboardConfig("redhat-ods-applications", "2.17.0")

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      configCompatibilityListKinds,
		Objects:        []*unstructured.Unstructured{dsc, dsci, cfg},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := dashboard.NewConfigCompatibilityCheck()
	dr, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(dr).ToNot(BeNil())
	g.Expect(dr).To(PointTo(MatchFields(IgnoreExtras, Fields{
		"Group": Equal(string(check.GroupComponent)),
		"Kind":  Equal(constants.ComponentDashboard),
		"Name":  Equal("config-compatibility"),
	})))
	g.Expect(dr.Status.Conditions).To(HaveLen(1))
	g.Expect(dr.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal("ConfigCompatibility"),
		"Status": Equal(metav1.ConditionTrue),
		"Reason": Equal(check.ReasonResourceFound),
	}))
	g.Expect(dr.Status.Conditions[0].Condition.Message).To(ContainSubstring("2.17.0"))
}

func TestConfigCompatibilityCheck_NotFound(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	dsc := testutil.NewDSC(map[string]string{"dashboard": "Managed"})
	dsci := testutil.NewDSCI("redhat-ods-applications")

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      configCompatibilityListKinds,
		Objects:        []*unstructured.Unstructured{dsc, dsci},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := dashboard.NewConfigCompatibilityCheck()
	dr, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(dr).ToNot(BeNil())
	g.Expect(dr.Status.Conditions).To(HaveLen(1))
	g.Expect(dr.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal("ConfigCompatibility"),
		"Status": Equal(metav1.ConditionFalse),
		"Reason": Equal(check.ReasonResourceNotFound),
	}))
	g.Expect(dr.Status.Conditions[0].Impact).To(Equal(result.ImpactAdvisory))
}
