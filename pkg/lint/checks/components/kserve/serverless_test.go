package kserve_test

import (
	"testing"

	"github.com/opendatahub-io/odh-cli/pkg/constants"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	resultpkg "github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/testutil"
	"github.com/opendatahub-io/odh-cli/pkg/lint/checks/components/kserve"
	"github.com/opendatahub-io/odh-cli/pkg/resources"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

//nolint:gochecknoglobals // Test fixture - shared across test functions
var listKinds = map[schema.GroupVersionResource]string{
	resources.DataScienceCluster.GVR(): resources.DataScienceCluster.ListKind(),
}

func TestKServeServerlessRemovalCheck_CanApply_NoDSC(t *testing.T) {
	g := NewWithT(t)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := kserve.NewServerlessRemovalCheck()
	canApply, err := chk.CanApply(t.Context(), target)

	g.Expect(err).To(HaveOccurred())
	g.Expect(canApply).To(BeFalse())
}

func TestKServeServerlessRemovalCheck_CanApply_ManagementState(t *testing.T) {
	g := NewWithT(t)

	chk := kserve.NewServerlessRemovalCheck()

	testCases := []struct {
		name     string
		state    string
		expected bool
	}{
		{name: "Managed", state: "Managed", expected: true},
		{name: "Unmanaged", state: "Unmanaged", expected: false},
		{name: "Removed", state: "Removed", expected: false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			dsc := testutil.NewDSC(map[string]string{"kserve": tc.state})
			target := testutil.NewTarget(t, testutil.TargetConfig{
				ListKinds:      listKinds,
				Objects:        []*unstructured.Unstructured{dsc},
				CurrentVersion: "2.17.0",
				TargetVersion:  "3.0.0",
			})

			canApply, err := chk.CanApply(t.Context(), target)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(canApply).To(Equal(tc.expected))
		})
	}
}

func TestKServeServerlessRemovalCheck_ServerlessNotConfigured(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	// Create DataScienceCluster with kserve Managed but no serverless config
	dsc := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.DataScienceCluster.APIVersion(),
			"kind":       resources.DataScienceCluster.Kind,
			"metadata": map[string]any{
				"name": "default-dsc",
			},
			"spec": map[string]any{
				"components": map[string]any{
					"kserve": map[string]any{
						"managementState": "Managed",
						// No serving config
					},
				},
			},
		},
	}

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{dsc},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	kserveCheck := kserve.NewServerlessRemovalCheck()
	result, err := kserveCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeCompatible),
		"Status":  Equal(metav1.ConditionTrue),
		"Reason":  Equal(check.ReasonVersionCompatible),
		"Message": ContainSubstring("serverless mode is not configured"),
	}))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationComponentManagementState, constants.ManagementStateManaged))
}

func TestKServeServerlessRemovalCheck_ServerlessManagedBlocking(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	// Create DataScienceCluster with kserve serverless Managed (blocking upgrade)
	dsc := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.DataScienceCluster.APIVersion(),
			"kind":       resources.DataScienceCluster.Kind,
			"metadata": map[string]any{
				"name": "default-dsc",
			},
			"spec": map[string]any{
				"components": map[string]any{
					"kserve": map[string]any{
						"managementState": "Managed",
						"serving": map[string]any{
							"managementState": "Managed",
						},
					},
				},
			},
		},
	}

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{dsc},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	kserveCheck := kserve.NewServerlessRemovalCheck()
	result, err := kserveCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeCompatible),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonVersionIncompatible),
		"Message": And(ContainSubstring("serverless mode is enabled"), ContainSubstring("removed in RHOAI 3.x")),
	}))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactBlocking))
	g.Expect(result.Annotations).To(And(
		HaveKeyWithValue(check.AnnotationComponentManagementState, constants.ManagementStateManaged),
		HaveKeyWithValue(check.AnnotationCheckTargetVersion, "3.0.0"),
	))
}

func TestKServeServerlessRemovalCheck_ServerlessUnmanagedBlocking(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	// Create DataScienceCluster with kserve serverless Unmanaged (also blocking)
	dsc := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.DataScienceCluster.APIVersion(),
			"kind":       resources.DataScienceCluster.Kind,
			"metadata": map[string]any{
				"name": "default-dsc",
			},
			"spec": map[string]any{
				"components": map[string]any{
					"kserve": map[string]any{
						"managementState": "Managed",
						"serving": map[string]any{
							"managementState": "Unmanaged",
						},
					},
				},
			},
		},
	}

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{dsc},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.1.0",
	})

	kserveCheck := kserve.NewServerlessRemovalCheck()
	result, err := kserveCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeCompatible),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonVersionIncompatible),
		"Message": ContainSubstring("state: Unmanaged"),
	}))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactBlocking))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationComponentManagementState, constants.ManagementStateManaged))
}

func TestKServeServerlessRemovalCheck_ServerlessRemovedReady(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	// Create DataScienceCluster with kserve serverless Removed (ready for upgrade)
	dsc := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.DataScienceCluster.APIVersion(),
			"kind":       resources.DataScienceCluster.Kind,
			"metadata": map[string]any{
				"name": "default-dsc",
			},
			"spec": map[string]any{
				"components": map[string]any{
					"kserve": map[string]any{
						"managementState": "Managed",
						"serving": map[string]any{
							"managementState": "Removed",
						},
					},
				},
			},
		},
	}

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{dsc},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	kserveCheck := kserve.NewServerlessRemovalCheck()
	result, err := kserveCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeCompatible),
		"Status":  Equal(metav1.ConditionTrue),
		"Reason":  Equal(check.ReasonVersionCompatible),
		"Message": And(ContainSubstring("serverless mode is disabled"), ContainSubstring("ready for RHOAI 3.x upgrade")),
	}))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationComponentManagementState, constants.ManagementStateManaged))
}

func TestKServeServerlessRemovalCheck_Metadata(t *testing.T) {
	g := NewWithT(t)

	kserveCheck := kserve.NewServerlessRemovalCheck()

	g.Expect(kserveCheck.ID()).To(Equal("components.kserve.serverless-removal"))
	g.Expect(kserveCheck.Name()).To(Equal("Components :: KServe :: Serverless Removal (3.x)"))
	g.Expect(kserveCheck.Group()).To(Equal(check.GroupComponent))
	g.Expect(kserveCheck.Description()).ToNot(BeEmpty())
}
