package openshift_test

import (
	"testing"

	"github.com/blang/semver/v4"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	resultpkg "github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/testutil"
	"github.com/opendatahub-io/odh-cli/pkg/lint/checks/dependencies/openshift"
	"github.com/opendatahub-io/odh-cli/pkg/resources"

	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

func createClusterVersion(version string) *unstructured.Unstructured {
	cv := &unstructured.Unstructured{}
	cv.SetAPIVersion("config.openshift.io/v1")
	cv.SetKind("ClusterVersion")
	cv.SetName("version")

	_ = unstructured.SetNestedField(cv.Object, version, "status", "desired", "version")

	return cv
}

func TestOpenShiftCheck_VersionMeetsRequirement(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	cv := createClusterVersion("4.19.9")
	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds: map[schema.GroupVersionResource]string{
			resources.ClusterVersion.GVR(): "ClusterVersionList",
		},
		Objects:        []*unstructured.Unstructured{cv},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	openshiftCheck := openshift.NewCheck()
	result, err := openshiftCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeCompatible),
		"Status":  Equal(metav1.ConditionTrue),
		"Reason":  Equal(check.ReasonVersionCompatible),
		"Message": ContainSubstring("4.19.9 meets RHOAI 3.x minimum version requirement"),
	}))
}

func TestOpenShiftCheck_VersionAboveRequirement(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	cv := createClusterVersion("4.20.5")
	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds: map[schema.GroupVersionResource]string{
			resources.ClusterVersion.GVR(): "ClusterVersionList",
		},
		Objects:        []*unstructured.Unstructured{cv},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	openshiftCheck := openshift.NewCheck()
	result, err := openshiftCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(check.ConditionTypeCompatible),
		"Status": Equal(metav1.ConditionTrue),
		"Reason": Equal(check.ReasonVersionCompatible),
	}))
}

func TestOpenShiftCheck_VersionBelowRequirement(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	cv := createClusterVersion("4.18.5")
	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds: map[schema.GroupVersionResource]string{
			resources.ClusterVersion.GVR(): "ClusterVersionList",
		},
		Objects:        []*unstructured.Unstructured{cv},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	openshiftCheck := openshift.NewCheck()
	result, err := openshiftCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(check.ConditionTypeCompatible),
		"Status": Equal(metav1.ConditionFalse),
		"Reason": Equal(check.ReasonVersionIncompatible),
		"Message": And(
			ContainSubstring("4.18.5 does not meet RHOAI 3.x minimum version requirement"),
			ContainSubstring("4.19"),
		),
	}))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactBlocking))
}

func TestOpenShiftCheck_PatchVersionBelowRequirement(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	cv := createClusterVersion("4.19.8")
	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds: map[schema.GroupVersionResource]string{
			resources.ClusterVersion.GVR(): "ClusterVersionList",
		},
		Objects:        []*unstructured.Unstructured{cv},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	openshiftCheck := openshift.NewCheck()
	result, err := openshiftCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(check.ConditionTypeCompatible),
		"Status": Equal(metav1.ConditionFalse),
		"Reason": Equal(check.ReasonVersionIncompatible),
		"Message": And(
			ContainSubstring("4.19.8 does not meet RHOAI 3.x minimum version requirement"),
			ContainSubstring("4.19.9"),
		),
	}))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactBlocking))
}

func TestOpenShiftCheck_VersionNotDetectable(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	target := testutil.NewTarget(t, testutil.TargetConfig{
		Objects:        nil,
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	openshiftCheck := openshift.NewCheck()
	result, err := openshiftCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeCompatible),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonInsufficientData),
		"Message": ContainSubstring("Unable to detect OpenShift version"),
	}))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactBlocking))
}

func TestOpenShiftCheck_CanApply_2xTo3x(t *testing.T) {
	g := NewWithT(t)

	openshiftCheck := openshift.NewCheck()

	currentVer := semver.MustParse("2.17.0")
	targetVer := semver.MustParse("3.0.0")
	target := check.Target{
		CurrentVersion: &currentVer,
		TargetVersion:  &targetVer,
	}

	canApply, err := openshiftCheck.CanApply(t.Context(), target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeTrue())
}

func TestOpenShiftCheck_CanApply_2xTo2x(t *testing.T) {
	g := NewWithT(t)

	openshiftCheck := openshift.NewCheck()

	currentVer := semver.MustParse("2.17.0")
	targetVer := semver.MustParse("2.18.0")
	target := check.Target{
		CurrentVersion: &currentVer,
		TargetVersion:  &targetVer,
	}

	canApply, err := openshiftCheck.CanApply(t.Context(), target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeFalse())
}

func TestOpenShiftCheck_CanApply_3xTo3x(t *testing.T) {
	g := NewWithT(t)

	openshiftCheck := openshift.NewCheck()

	currentVer := semver.MustParse("3.0.0")
	targetVer := semver.MustParse("3.1.0")
	target := check.Target{
		CurrentVersion: &currentVer,
		TargetVersion:  &targetVer,
	}

	canApply, err := openshiftCheck.CanApply(t.Context(), target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeTrue())
}

func TestOpenShiftCheck_CanApply_3xCurrent(t *testing.T) {
	g := NewWithT(t)

	openshiftCheck := openshift.NewCheck()

	currentVer := semver.MustParse("3.0.0")
	targetVer := semver.MustParse("3.0.0")
	target := check.Target{
		CurrentVersion: &currentVer,
		TargetVersion:  &targetVer,
	}

	canApply, err := openshiftCheck.CanApply(t.Context(), target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeTrue())
}

func TestOpenShiftCheck_Metadata(t *testing.T) {
	g := NewWithT(t)

	openshiftCheck := openshift.NewCheck()

	g.Expect(openshiftCheck.ID()).To(Equal("dependencies.openshift.version-requirement"))
	g.Expect(openshiftCheck.Name()).To(Equal("Dependencies :: OpenShift :: Version Requirement (3.x)"))
	g.Expect(openshiftCheck.Group()).To(Equal(check.GroupDependency))
	g.Expect(openshiftCheck.Description()).ToNot(BeEmpty())
}
