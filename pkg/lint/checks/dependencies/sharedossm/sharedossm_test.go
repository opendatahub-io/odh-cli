package sharedossm_test

import (
	"testing"

	"github.com/blang/semver/v4"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	resultpkg "github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/testutil"
	"github.com/opendatahub-io/odh-cli/pkg/lint/checks/dependencies/sharedossm"
	"github.com/opendatahub-io/odh-cli/pkg/resources"

	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

func listKinds() map[schema.GroupVersionResource]string {
	return map[schema.GroupVersionResource]string{
		resources.ServiceMeshControlPlane.GVR(): resources.ServiceMeshControlPlane.ListKind(),
		resources.ServiceMeshMemberRoll.GVR():   resources.ServiceMeshMemberRoll.ListKind(),
		resources.ServiceMeshMember.GVR():       resources.ServiceMeshMember.ListKind(),
		resources.DSCInitialization.GVR():       resources.DSCInitialization.ListKind(),
	}
}

func newSMCP(name, namespace string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.ServiceMeshControlPlane.APIVersion(),
			"kind":       resources.ServiceMeshControlPlane.Kind,
			"metadata": map[string]any{
				"name":      name,
				"namespace": namespace,
			},
		},
	}
}

func newSMMR(name, namespace string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.ServiceMeshMemberRoll.APIVersion(),
			"kind":       resources.ServiceMeshMemberRoll.Kind,
			"metadata": map[string]any{
				"name":      name,
				"namespace": namespace,
			},
		},
	}
}

func newSMM(name, namespace string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.ServiceMeshMember.APIVersion(),
			"kind":       resources.ServiceMeshMember.Kind,
			"metadata": map[string]any{
				"name":      name,
				"namespace": namespace,
			},
		},
	}
}

func TestSharedOSSMCheck_NoSharedResources(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds(),
		Objects:        []*unstructured.Unstructured{testutil.NewDSCI("redhat-ods-applications")},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := sharedossm.NewCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(check.ConditionTypeValidated),
		"Status": Equal(metav1.ConditionTrue),
		"Reason": Equal(check.ReasonRequirementsMet),
	}))
}

func TestSharedOSSMCheck_RHOAIResourcesOnly(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	smcp := newSMCP("basic", "istio-system")

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds(),
		Objects:        []*unstructured.Unstructured{testutil.NewDSCI("redhat-ods-applications"), smcp},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := sharedossm.NewCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(check.ConditionTypeValidated),
		"Status": Equal(metav1.ConditionTrue),
		"Reason": Equal(check.ReasonRequirementsMet),
	}))
}

func TestSharedOSSMCheck_SharedSMCPDetected(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	smcp := newSMCP("custom-mesh", "my-app-namespace")

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds(),
		Objects:        []*unstructured.Unstructured{testutil.NewDSCI("redhat-ods-applications"), smcp},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := sharedossm.NewCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(check.ConditionTypeValidated),
		"Status": Equal(metav1.ConditionFalse),
		"Reason": Equal(check.ReasonWorkloadsImpacted),
	}))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactAdvisory))
	g.Expect(result.Status.Conditions[0].Message).To(ContainSubstring("my-app-namespace"))
	g.Expect(result.Status.Conditions[0].Message).To(ContainSubstring("1 OSSM resource"))
	g.Expect(result.ImpactedObjects).To(HaveLen(1))
}

func TestSharedOSSMCheck_MultipleSharedResources(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	smcp := newSMCP("custom-mesh", "team-a")
	smmr := newSMMR("default", "team-a")
	smm := newSMM("default", "team-b")

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds: listKinds(),
		Objects: []*unstructured.Unstructured{
			testutil.NewDSCI("redhat-ods-applications"),
			smcp, smmr, smm,
		},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := sharedossm.NewCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(check.ConditionTypeValidated),
		"Status": Equal(metav1.ConditionFalse),
		"Reason": Equal(check.ReasonWorkloadsImpacted),
	}))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactAdvisory))
	g.Expect(result.Status.Conditions[0].Message).To(ContainSubstring("3 OSSM resource"))
	g.Expect(result.ImpactedObjects).To(HaveLen(3))
}

func TestSharedOSSMCheck_OnlySMMRsDetected(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	smmr := newSMMR("default", "team-a")
	smm := newSMM("default", "team-b")

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds: listKinds(),
		Objects: []*unstructured.Unstructured{
			testutil.NewDSCI("redhat-ods-applications"),
			smmr, smm,
		},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := sharedossm.NewCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(check.ConditionTypeValidated),
		"Status": Equal(metav1.ConditionFalse),
		"Reason": Equal(check.ReasonWorkloadsImpacted),
	}))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactAdvisory))
	g.Expect(result.Status.Conditions[0].Message).To(ContainSubstring("2 OSSM resource"))
	g.Expect(result.ImpactedObjects).To(HaveLen(2))
}

func TestSharedOSSMCheck_NoDSCI_FallsBackToWellKnownNamespaces(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	smcpManaged := newSMCP("basic", "istio-system")
	smcpExternal := newSMCP("custom-mesh", "team-a")

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds(),
		Objects:        []*unstructured.Unstructured{smcpManaged, smcpExternal},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := sharedossm.NewCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(check.ConditionTypeValidated),
		"Status": Equal(metav1.ConditionFalse),
		"Reason": Equal(check.ReasonWorkloadsImpacted),
	}))
	g.Expect(result.Status.Conditions[0].Message).To(ContainSubstring("1 OSSM resource"))
	g.Expect(result.Status.Conditions[0].Message).To(ContainSubstring("team-a"))
	g.Expect(result.ImpactedObjects).To(HaveLen(1))
}

func TestSharedOSSMCheck_CanApply_2xTo3x(t *testing.T) {
	g := NewWithT(t)

	chk := sharedossm.NewCheck()

	currentVer := semver.MustParse("2.17.0")
	targetVer := semver.MustParse("3.0.0")
	target := check.Target{
		CurrentVersion: &currentVer,
		TargetVersion:  &targetVer,
	}

	canApply, err := chk.CanApply(t.Context(), target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeTrue())
}

func TestSharedOSSMCheck_CanApply_3xTo3x(t *testing.T) {
	g := NewWithT(t)

	chk := sharedossm.NewCheck()

	currentVer := semver.MustParse("3.0.0")
	targetVer := semver.MustParse("3.1.0")
	target := check.Target{
		CurrentVersion: &currentVer,
		TargetVersion:  &targetVer,
	}

	canApply, err := chk.CanApply(t.Context(), target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeFalse())
}

func TestSharedOSSMCheck_CanApply_NilVersions(t *testing.T) {
	g := NewWithT(t)

	chk := sharedossm.NewCheck()

	canApply, err := chk.CanApply(t.Context(), check.Target{})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeFalse())
}

func TestSharedOSSMCheck_Metadata(t *testing.T) {
	g := NewWithT(t)

	chk := sharedossm.NewCheck()

	g.Expect(chk.ID()).To(Equal("dependencies.shared-ossm.shared-usage"))
	g.Expect(chk.Name()).To(Equal("Dependencies :: Shared OSSM :: Shared Usage Detection"))
	g.Expect(chk.Group()).To(Equal(check.GroupDependency))
	g.Expect(chk.Description()).ToNot(BeEmpty())
}
