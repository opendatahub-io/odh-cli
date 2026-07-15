package sharedserverless_test

import (
	"testing"

	"github.com/blang/semver/v4"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	resultpkg "github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/testutil"
	"github.com/opendatahub-io/odh-cli/pkg/lint/checks/dependencies/sharedserverless"
	"github.com/opendatahub-io/odh-cli/pkg/resources"

	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

func listKinds() map[schema.GroupVersionResource]string {
	return map[schema.GroupVersionResource]string{
		resources.KnativeService.GVR():    resources.KnativeService.ListKind(),
		resources.KnativeServing.GVR():    resources.KnativeServing.ListKind(),
		resources.KnativeEventing.GVR():   resources.KnativeEventing.ListKind(),
		resources.DSCInitialization.GVR(): resources.DSCInitialization.ListKind(),
	}
}

func newKnativeService(name, namespace string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.KnativeService.APIVersion(),
			"kind":       resources.KnativeService.Kind,
			"metadata": map[string]any{
				"name":      name,
				"namespace": namespace,
			},
		},
	}
}

func newKnativeServing(name, namespace string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.KnativeServing.APIVersion(),
			"kind":       resources.KnativeServing.Kind,
			"metadata": map[string]any{
				"name":      name,
				"namespace": namespace,
			},
		},
	}
}

func newKnativeEventing(name, namespace string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.KnativeEventing.APIVersion(),
			"kind":       resources.KnativeEventing.Kind,
			"metadata": map[string]any{
				"name":      name,
				"namespace": namespace,
			},
		},
	}
}

func TestSharedServerlessCheck_NoSharedResources(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds(),
		Objects:        []*unstructured.Unstructured{testutil.NewDSCI("redhat-ods-applications")},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := sharedserverless.NewCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(check.ConditionTypeValidated),
		"Status": Equal(metav1.ConditionTrue),
		"Reason": Equal(check.ReasonRequirementsMet),
	}))
}

func TestSharedServerlessCheck_RHOAIResourcesOnly(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	ksvc := newKnativeService("predictor", "redhat-ods-applications")

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds(),
		Objects:        []*unstructured.Unstructured{testutil.NewDSCI("redhat-ods-applications"), ksvc},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := sharedserverless.NewCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(check.ConditionTypeValidated),
		"Status": Equal(metav1.ConditionTrue),
		"Reason": Equal(check.ReasonRequirementsMet),
	}))
}

func TestSharedServerlessCheck_SharedKServiceDetected(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	ksvc := newKnativeService("my-app", "team-a")

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds(),
		Objects:        []*unstructured.Unstructured{testutil.NewDSCI("redhat-ods-applications"), ksvc},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := sharedserverless.NewCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(check.ConditionTypeValidated),
		"Status": Equal(metav1.ConditionFalse),
		"Reason": Equal(check.ReasonWorkloadsImpacted),
	}))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactAdvisory))
	g.Expect(result.Status.Conditions[0].Message).To(ContainSubstring("team-a"))
	g.Expect(result.Status.Conditions[0].Message).To(ContainSubstring("1 Serverless resource"))
	g.Expect(result.ImpactedObjects).To(HaveLen(1))
}

func TestSharedServerlessCheck_MultipleSharedResources(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	ksvc1 := newKnativeService("app-1", "team-a")
	ksvc2 := newKnativeService("app-2", "team-b")
	serving := newKnativeServing("custom-serving", "team-a")
	eventing := newKnativeEventing("custom-eventing", "team-c")

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds: listKinds(),
		Objects: []*unstructured.Unstructured{
			testutil.NewDSCI("redhat-ods-applications"),
			ksvc1, ksvc2, serving, eventing,
		},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := sharedserverless.NewCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(check.ConditionTypeValidated),
		"Status": Equal(metav1.ConditionFalse),
		"Reason": Equal(check.ReasonWorkloadsImpacted),
	}))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactAdvisory))
	g.Expect(result.Status.Conditions[0].Message).To(ContainSubstring("4 Serverless resource"))
	g.Expect(result.ImpactedObjects).To(HaveLen(4))
}

func TestSharedServerlessCheck_KnativeServingInManagedNS(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	serving := newKnativeServing("knative-serving", "knative-serving")

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds(),
		Objects:        []*unstructured.Unstructured{testutil.NewDSCI("redhat-ods-applications"), serving},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := sharedserverless.NewCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(check.ConditionTypeValidated),
		"Status": Equal(metav1.ConditionTrue),
		"Reason": Equal(check.ReasonRequirementsMet),
	}))
}

func TestSharedServerlessCheck_OnlyServingAndEventingDetected(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	serving := newKnativeServing("custom-serving", "team-a")
	eventing := newKnativeEventing("custom-eventing", "team-b")

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds: listKinds(),
		Objects: []*unstructured.Unstructured{
			testutil.NewDSCI("redhat-ods-applications"),
			serving, eventing,
		},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := sharedserverless.NewCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(check.ConditionTypeValidated),
		"Status": Equal(metav1.ConditionFalse),
		"Reason": Equal(check.ReasonWorkloadsImpacted),
	}))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactAdvisory))
	g.Expect(result.Status.Conditions[0].Message).To(ContainSubstring("2 Serverless resource"))
	g.Expect(result.ImpactedObjects).To(HaveLen(2))
}

func TestSharedServerlessCheck_NoDSCI_FallsBackToWellKnownNamespaces(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	ksvcManaged := newKnativeService("predictor", "knative-serving")
	ksvcExternal := newKnativeService("my-app", "team-a")

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds(),
		Objects:        []*unstructured.Unstructured{ksvcManaged, ksvcExternal},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := sharedserverless.NewCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(check.ConditionTypeValidated),
		"Status": Equal(metav1.ConditionFalse),
		"Reason": Equal(check.ReasonWorkloadsImpacted),
	}))
	g.Expect(result.Status.Conditions[0].Message).To(ContainSubstring("1 Serverless resource"))
	g.Expect(result.Status.Conditions[0].Message).To(ContainSubstring("team-a"))
	g.Expect(result.ImpactedObjects).To(HaveLen(1))
}

func TestSharedServerlessCheck_CanApply_2xTo3x(t *testing.T) {
	g := NewWithT(t)

	chk := sharedserverless.NewCheck()

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

func TestSharedServerlessCheck_CanApply_3xTo3x(t *testing.T) {
	g := NewWithT(t)

	chk := sharedserverless.NewCheck()

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

func TestSharedServerlessCheck_CanApply_NilVersions(t *testing.T) {
	g := NewWithT(t)

	chk := sharedserverless.NewCheck()

	canApply, err := chk.CanApply(t.Context(), check.Target{})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeFalse())
}

func TestSharedServerlessCheck_Metadata(t *testing.T) {
	g := NewWithT(t)

	chk := sharedserverless.NewCheck()

	g.Expect(chk.ID()).To(Equal("dependencies.shared-serverless.shared-usage"))
	g.Expect(chk.Name()).To(Equal("Dependencies :: Shared Serverless :: Shared Usage Detection"))
	g.Expect(chk.Group()).To(Equal(check.GroupDependency))
	g.Expect(chk.Description()).ToNot(BeEmpty())
}
