package openshift_test

import (
	"context"
	"testing"

	"github.com/blang/semver/v4"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	"github.com/lburgazzoli/odh-cli/pkg/lint/check"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/dependencies/openshift"
	"github.com/lburgazzoli/odh-cli/pkg/resources"
	"github.com/lburgazzoli/odh-cli/pkg/util/client"

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
	ctx := context.Background()

	cv := createClusterVersion("4.19.9")

	scheme := runtime.NewScheme()
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		scheme,
		map[schema.GroupVersionResource]string{
			resources.ClusterVersion.GVR(): "ClusterVersionList",
		},
		cv,
	)

	c := client.NewForTesting(client.TestClientConfig{
		Dynamic: dynamicClient,
	})

	currentVer := semver.MustParse("2.17.0")
	targetVer := semver.MustParse("3.0.0")
	target := check.Target{
		Client:         c,
		CurrentVersion: &currentVer,
		TargetVersion:  &targetVer,
	}

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
	ctx := context.Background()

	cv := createClusterVersion("4.20.5")

	scheme := runtime.NewScheme()
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		scheme,
		map[schema.GroupVersionResource]string{
			resources.ClusterVersion.GVR(): "ClusterVersionList",
		},
		cv,
	)

	c := client.NewForTesting(client.TestClientConfig{
		Dynamic: dynamicClient,
	})

	currentVer := semver.MustParse("2.17.0")
	targetVer := semver.MustParse("3.0.0")
	target := check.Target{
		Client:         c,
		CurrentVersion: &currentVer,
		TargetVersion:  &targetVer,
	}

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
	ctx := context.Background()

	cv := createClusterVersion("4.18.5")

	scheme := runtime.NewScheme()
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		scheme,
		map[schema.GroupVersionResource]string{
			resources.ClusterVersion.GVR(): "ClusterVersionList",
		},
		cv,
	)

	c := client.NewForTesting(client.TestClientConfig{
		Dynamic: dynamicClient,
	})

	currentVer := semver.MustParse("2.17.0")
	targetVer := semver.MustParse("3.0.0")
	target := check.Target{
		Client:         c,
		CurrentVersion: &currentVer,
		TargetVersion:  &targetVer,
	}

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
}

func TestOpenShiftCheck_PatchVersionBelowRequirement(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	cv := createClusterVersion("4.19.8")

	scheme := runtime.NewScheme()
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		scheme,
		map[schema.GroupVersionResource]string{
			resources.ClusterVersion.GVR(): "ClusterVersionList",
		},
		cv,
	)

	c := client.NewForTesting(client.TestClientConfig{
		Dynamic: dynamicClient,
	})

	currentVer := semver.MustParse("2.17.0")
	targetVer := semver.MustParse("3.0.0")
	target := check.Target{
		Client:         c,
		CurrentVersion: &currentVer,
		TargetVersion:  &targetVer,
	}

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
}

func TestOpenShiftCheck_VersionNotDetectable(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	scheme := runtime.NewScheme()
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, nil)

	c := client.NewForTesting(client.TestClientConfig{
		Dynamic: dynamicClient,
	})

	currentVer := semver.MustParse("2.17.0")
	targetVer := semver.MustParse("3.0.0")
	target := check.Target{
		Client:         c,
		CurrentVersion: &currentVer,
		TargetVersion:  &targetVer,
	}

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

	g.Expect(openshiftCheck.CanApply(t.Context(), target)).To(BeTrue())
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

	g.Expect(openshiftCheck.CanApply(t.Context(), target)).To(BeFalse())
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

	g.Expect(openshiftCheck.CanApply(t.Context(), target)).To(BeFalse())
}

func TestOpenShiftCheck_Metadata(t *testing.T) {
	g := NewWithT(t)

	openshiftCheck := openshift.NewCheck()

	g.Expect(openshiftCheck.ID()).To(Equal("dependencies.openshift.version-requirement"))
	g.Expect(openshiftCheck.Name()).To(Equal("Dependencies :: OpenShift :: Version Requirement (3.x)"))
	g.Expect(openshiftCheck.Group()).To(Equal(check.GroupDependency))
	g.Expect(openshiftCheck.Description()).ToNot(BeEmpty())
}
