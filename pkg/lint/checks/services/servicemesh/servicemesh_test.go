package servicemesh_test

import (
	"testing"

	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	resultpkg "github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/testutil"
	"github.com/opendatahub-io/odh-cli/pkg/lint/checks/services/servicemesh"
	"github.com/opendatahub-io/odh-cli/pkg/resources"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

//nolint:gochecknoglobals // Test fixture - shared across test functions
var listKinds = map[schema.GroupVersionResource]string{
	resources.DSCInitialization.GVR(): resources.DSCInitialization.ListKind(),
}

func TestServiceMeshRemovalCheck_NoDSCI(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	// Create empty cluster (no DSCInitialization)
	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:     listKinds,
		TargetVersion: "3.0.0",
	})

	servicemeshCheck := servicemesh.NewRemovalCheck()
	result, err := servicemeshCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeAvailable),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonResourceNotFound),
		"Message": ContainSubstring("No DSCInitialization"),
	}))
}

func TestServiceMeshRemovalCheck_NotConfigured(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	// Create DSCInitialization without serviceMesh
	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:     listKinds,
		Objects:       []*unstructured.Unstructured{testutil.NewDSCI("opendatahub")},
		TargetVersion: "3.0.0",
	})

	servicemeshCheck := servicemesh.NewRemovalCheck()
	result, err := servicemeshCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeConfigured),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonResourceNotFound),
		"Message": ContainSubstring("not configured"),
	}))
}

func TestServiceMeshRemovalCheck_ManagedBlocking(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	// Create DSCInitialization with serviceMesh Managed (blocking upgrade)
	dsci := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.DSCInitialization.APIVersion(),
			"kind":       resources.DSCInitialization.Kind,
			"metadata": map[string]any{
				"name": "default-dsci",
			},
			"spec": map[string]any{
				"applicationsNamespace": "opendatahub",
				"serviceMesh": map[string]any{
					"managementState": "Managed",
				},
			},
		},
	}

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:     listKinds,
		Objects:       []*unstructured.Unstructured{dsci},
		TargetVersion: "3.0.0",
	})

	servicemeshCheck := servicemesh.NewRemovalCheck()
	result, err := servicemeshCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeCompatible),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonVersionIncompatible),
		"Message": And(ContainSubstring("enabled"), ContainSubstring("no longer required by RHOAI 3.x")),
	}))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactBlocking))
}

func TestServiceMeshRemovalCheck_UnmanagedBlocking(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	// Create DSCInitialization with serviceMesh Unmanaged (also blocking)
	dsci := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.DSCInitialization.APIVersion(),
			"kind":       resources.DSCInitialization.Kind,
			"metadata": map[string]any{
				"name": "default-dsci",
			},
			"spec": map[string]any{
				"applicationsNamespace": "opendatahub",
				"serviceMesh": map[string]any{
					"managementState": "Unmanaged",
				},
			},
		},
	}

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:     listKinds,
		Objects:       []*unstructured.Unstructured{dsci},
		TargetVersion: "3.1.0",
	})

	servicemeshCheck := servicemesh.NewRemovalCheck()
	result, err := servicemeshCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeCompatible),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonVersionIncompatible),
		"Message": ContainSubstring("state: Unmanaged"),
	}))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactBlocking))
}

func TestServiceMeshRemovalCheck_RemovedReady(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	// Create DSCInitialization with serviceMesh Removed (ready for upgrade)
	dsci := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.DSCInitialization.APIVersion(),
			"kind":       resources.DSCInitialization.Kind,
			"metadata": map[string]any{
				"name": "default-dsci",
			},
			"spec": map[string]any{
				"applicationsNamespace": "opendatahub",
				"serviceMesh": map[string]any{
					"managementState": "Removed",
				},
			},
		},
	}

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:     listKinds,
		Objects:       []*unstructured.Unstructured{dsci},
		TargetVersion: "3.0.0",
	})

	servicemeshCheck := servicemesh.NewRemovalCheck()
	result, err := servicemeshCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeCompatible),
		"Status":  Equal(metav1.ConditionTrue),
		"Reason":  Equal(check.ReasonVersionCompatible),
		"Message": And(ContainSubstring("disabled"), ContainSubstring("ready for RHOAI 3.x upgrade")),
	}))
}

func TestServiceMeshRemovalCheck_Metadata(t *testing.T) {
	g := NewWithT(t)

	servicemeshCheck := servicemesh.NewRemovalCheck()

	g.Expect(servicemeshCheck.ID()).To(Equal("services.servicemesh.removal"))
	g.Expect(servicemeshCheck.Name()).To(Equal("Services :: ServiceMesh :: Removal (3.x)"))
	g.Expect(servicemeshCheck.Group()).To(Equal(check.GroupService))
	g.Expect(servicemeshCheck.Description()).ToNot(BeEmpty())
}
