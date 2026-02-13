package dashboard_test

import (
	"testing"

	"github.com/blang/semver/v4"
	"github.com/opendatahub-io/odh-cli/pkg/constants"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/testutil"
	"github.com/opendatahub-io/odh-cli/pkg/lint/checks/components/dashboard"
	"github.com/opendatahub-io/odh-cli/pkg/resources"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

// Test data constants.
const (
	testAcceleratorProfileNamespace1 = "redhat-ods-applications"
	testAcceleratorProfileNamespace2 = "my-project"
	testAcceleratorProfile1          = "nvidia-gpu"
	testAcceleratorProfile2          = "amd-gpu"
)

//nolint:gochecknoglobals // Test fixture - shared across test functions
var acceleratorProfileListKinds = map[schema.GroupVersionResource]string{
	resources.AcceleratorProfile.GVR(): resources.AcceleratorProfile.ListKind(),
}

func TestAcceleratorProfileMigrationCheck_CanApply(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	chk := dashboard.NewAcceleratorProfileMigrationCheck()

	t.Run("should apply when upgrading to 3.x", func(_ *testing.T) {
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

	t.Run("should not apply for 2.x versions", func(_ *testing.T) {
		targetVer := semver.MustParse("2.17.0")
		currentVer := semver.MustParse("2.16.0")

		target := check.Target{
			CurrentVersion: &currentVer,
			TargetVersion:  &targetVer,
		}

		canApply, err := chk.CanApply(ctx, target)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(canApply).To(BeFalse())
	})

	t.Run("should not apply when target version is nil", func(_ *testing.T) {
		currentVer := semver.MustParse("2.17.0")

		target := check.Target{
			CurrentVersion: &currentVer,
			TargetVersion:  nil,
		}

		canApply, err := chk.CanApply(ctx, target)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(canApply).To(BeFalse())
	})
}

func TestAcceleratorProfileMigrationCheck_Validate_NoProfiles(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      acceleratorProfileListKinds,
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := dashboard.NewAcceleratorProfileMigrationCheck()
	dr, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(dr).ToNot(BeNil())
	g.Expect(dr).To(PointTo(MatchFields(IgnoreExtras, Fields{
		"Group": Equal(string(check.GroupComponent)),
		"Kind":  Equal(constants.ComponentDashboard),
		"Name":  Equal("acceleratorprofile-migration"),
	})))
	g.Expect(dr.Status.Conditions).To(HaveLen(1))
	g.Expect(dr.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(check.ConditionTypeMigrationRequired),
		"Status": Equal(metav1.ConditionTrue),
		"Reason": Equal(check.ReasonNoMigrationRequired),
	}))
	g.Expect(dr.ImpactedObjects).To(BeEmpty())
}

func TestAcceleratorProfileMigrationCheck_Validate_WithProfiles(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	profile1 := createAcceleratorProfile(testAcceleratorProfileNamespace1, testAcceleratorProfile1)
	profile2 := createAcceleratorProfile(testAcceleratorProfileNamespace2, testAcceleratorProfile2)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      acceleratorProfileListKinds,
		Objects:        []*unstructured.Unstructured{profile1, profile2},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := dashboard.NewAcceleratorProfileMigrationCheck()
	dr, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(dr).ToNot(BeNil())
	g.Expect(dr).To(PointTo(MatchFields(IgnoreExtras, Fields{
		"Group": Equal(string(check.GroupComponent)),
		"Kind":  Equal(constants.ComponentDashboard),
	})))
	g.Expect(dr.Status.Conditions).To(HaveLen(1))
	// Status=False (not yet migrated) with advisory impact since auto-migration is informational.
	g.Expect(dr.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeMigrationRequired),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonMigrationPending),
		"Message": ContainSubstring("2 legacy AcceleratorProfile"),
	}))
	g.Expect(dr.Status.Conditions[0].Impact).To(Equal(result.ImpactAdvisory))
	g.Expect(dr.Annotations[check.AnnotationImpactedWorkloadCount]).To(Equal("2"))
	g.Expect(dr.ImpactedObjects).To(HaveLen(2))
}

func TestAcceleratorProfileMigrationCheck_Validate_AnnotationsPresent(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      acceleratorProfileListKinds,
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.3.0",
	})

	chk := dashboard.NewAcceleratorProfileMigrationCheck()
	dr, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(dr.Annotations[check.AnnotationCheckTargetVersion]).To(Equal("3.3.0"))
}

func TestAcceleratorProfileMigrationCheck_Metadata(t *testing.T) {
	g := NewWithT(t)

	chk := dashboard.NewAcceleratorProfileMigrationCheck()

	t.Run("should have correct ID", func(_ *testing.T) {
		g.Expect(chk.ID()).To(Equal("components.dashboard.acceleratorprofile-migration"))
	})

	t.Run("should have correct Name", func(_ *testing.T) {
		g.Expect(chk.Name()).To(Equal("Components :: Dashboard :: AcceleratorProfile Migration (3.x)"))
	})

	t.Run("should have correct Group", func(_ *testing.T) {
		g.Expect(chk.Group()).To(Equal(check.GroupComponent))
	})

	t.Run("should have correct Description", func(_ *testing.T) {
		g.Expect(chk.Description()).To(ContainSubstring("AcceleratorProfiles"))
		g.Expect(chk.Description()).To(ContainSubstring("HardwareProfiles"))
	})
}

// createAcceleratorProfile creates an unstructured AcceleratorProfile for testing.
func createAcceleratorProfile(namespace string, name string) *unstructured.Unstructured {
	profile := &unstructured.Unstructured{}
	profile.SetGroupVersionKind(resources.AcceleratorProfile.GVK())
	profile.SetNamespace(namespace)
	profile.SetName(name)

	return profile
}
