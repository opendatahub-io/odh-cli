package notebook_test

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	resultpkg "github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/testutil"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/validate"
	"github.com/opendatahub-io/odh-cli/pkg/lint/checks/workloads/notebook"
	"github.com/opendatahub-io/odh-cli/pkg/resources"

	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

//nolint:gochecknoglobals
var containerNameListKinds = map[schema.GroupVersionResource]string{
	resources.Notebook.GVR():           resources.Notebook.ListKind(),
	resources.DataScienceCluster.GVR(): resources.DataScienceCluster.ListKind(),
}

// newContainerNameNotebook creates a test Notebook with the given name, container name, and optional annotations.
func newContainerNameNotebook(name, namespace, containerName string, annotations map[string]string) *unstructured.Unstructured {
	opts := notebookOptions{
		Containers: []any{
			map[string]any{
				"name":  containerName,
				"image": "quay.io/modh/jupyter-datascience:2025.2",
			},
		},
	}

	if annotations != nil {
		opts.Annotations = make(map[string]any, len(annotations))
		for k, v := range annotations {
			opts.Annotations[k] = v
		}
	}

	return newNotebook(name, namespace, opts)
}

// newContainerNameNotebookWithOAuthProxy creates a test Notebook with an oauth-proxy sidecar.
func newContainerNameNotebookWithOAuthProxy(name, namespace, containerName string, annotations map[string]string) *unstructured.Unstructured {
	opts := notebookOptions{
		Containers: []any{
			map[string]any{
				"name":  containerName,
				"image": "quay.io/modh/jupyter-datascience:2025.2",
			},
			map[string]any{
				"name":  "oauth-proxy",
				"image": "registry.redhat.io/openshift4/ose-oauth-proxy-rhel9:v4.14",
			},
		},
	}

	if annotations != nil {
		opts.Annotations = make(map[string]any, len(annotations))
		for k, v := range annotations {
			opts.Annotations[k] = v
		}
	}

	return newNotebook(name, namespace, opts)
}

func TestContainerNameCheck_NoNotebooks(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      containerNameListKinds,
		Objects:        nil,
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := notebook.NewContainerNameCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(notebook.ConditionTypeContainerNameValid),
		"Status": Equal(metav1.ConditionTrue),
		"Reason": Equal(check.ReasonConfigurationValid),
	}))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "0"))
	g.Expect(result.ImpactedObjects).To(BeEmpty())
}

func TestContainerNameCheck_MatchingContainerName(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	// Notebook where container name matches notebook name — no mismatch.
	nb := newContainerNameNotebook("my-workbench", "test-ns", "my-workbench", map[string]string{
		validate.AnnotationAcceleratorName: "gpu-large",
	})

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      containerNameListKinds,
		Objects:        []*unstructured.Unstructured{nb},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := notebook.NewContainerNameCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(notebook.ConditionTypeContainerNameValid),
		"Status": Equal(metav1.ConditionTrue),
		"Reason": Equal(check.ReasonConfigurationValid),
	}))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "0"))
	g.Expect(result.ImpactedObjects).To(BeEmpty())
}

func TestContainerNameCheck_MismatchedContainerName(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	// Notebook where container name differs from notebook name.
	nb := newContainerNameNotebook("my-workbench", "test-ns", "wrong-name", map[string]string{
		validate.AnnotationAcceleratorName: "gpu-large",
	})

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      containerNameListKinds,
		Objects:        []*unstructured.Unstructured{nb},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := notebook.NewContainerNameCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(notebook.ConditionTypeContainerNameValid),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonConfigurationInvalid),
		"Message": ContainSubstring("Found 1 Notebook"),
	}))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactAdvisory))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "1"))
	g.Expect(result.ImpactedObjects).To(HaveLen(1))
	g.Expect(result.ImpactedObjects[0].Name).To(Equal("my-workbench"))
	g.Expect(result.ImpactedObjects[0].Namespace).To(Equal("test-ns"))
}

func TestContainerNameCheck_NoDashboardAnnotation(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	// Notebook without any Dashboard annotation — should be filtered out.
	nb := newContainerNameNotebook("my-workbench", "test-ns", "wrong-name", nil)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      containerNameListKinds,
		Objects:        []*unstructured.Unstructured{nb},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := notebook.NewContainerNameCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(notebook.ConditionTypeContainerNameValid),
		"Status": Equal(metav1.ConditionTrue),
		"Reason": Equal(check.ReasonConfigurationValid),
	}))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "0"))
	g.Expect(result.ImpactedObjects).To(BeEmpty())
}

func TestContainerNameCheck_SizeSelectionAnnotationMismatch(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	// Notebook with last-size-selection annotation (no accelerator) and mismatched container name.
	nb := newContainerNameNotebook("my-workbench", "test-ns", "wrong-name", map[string]string{
		"notebooks.opendatahub.io/last-size-selection": "Small",
	})

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      containerNameListKinds,
		Objects:        []*unstructured.Unstructured{nb},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := notebook.NewContainerNameCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(notebook.ConditionTypeContainerNameValid),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonConfigurationInvalid),
		"Message": ContainSubstring("Found 1 Notebook"),
	}))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactAdvisory))
	g.Expect(result.ImpactedObjects).To(HaveLen(1))
	g.Expect(result.ImpactedObjects[0].Name).To(Equal("my-workbench"))
}

func TestContainerNameCheck_SizeSelectionAnnotationMatching(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	// Notebook with last-size-selection annotation and matching container name — no mismatch.
	nb := newContainerNameNotebook("my-workbench", "test-ns", "my-workbench", map[string]string{
		"notebooks.opendatahub.io/last-size-selection": "Large",
	})

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      containerNameListKinds,
		Objects:        []*unstructured.Unstructured{nb},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := notebook.NewContainerNameCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(notebook.ConditionTypeContainerNameValid),
		"Status": Equal(metav1.ConditionTrue),
		"Reason": Equal(check.ReasonConfigurationValid),
	}))
	g.Expect(result.ImpactedObjects).To(BeEmpty())
}

func TestContainerNameCheck_MixedNotebooks(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	// Matching name + accelerator.
	nbMatching := newContainerNameNotebook("matching-nb", "ns1", "matching-nb", map[string]string{
		validate.AnnotationAcceleratorName: "gpu-large",
	})

	// Mismatched name + accelerator.
	nbMismatched := newContainerNameNotebook("mismatched-nb", "ns2", "old-name", map[string]string{
		validate.AnnotationAcceleratorName: "gpu-small",
	})

	// Size-selection only, mismatched — should be detected.
	nbSizeOnly := newContainerNameNotebook("size-nb", "ns3", "old-name", map[string]string{
		"notebooks.opendatahub.io/last-size-selection": "Medium",
	})

	// No Dashboard annotation — should be filtered out even with mismatch.
	nbNoDashboard := newContainerNameNotebook("no-dashboard-nb", "ns4", "different-name", nil)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      containerNameListKinds,
		Objects:        []*unstructured.Unstructured{nbMatching, nbMismatched, nbSizeOnly, nbNoDashboard},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := notebook.NewContainerNameCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(notebook.ConditionTypeContainerNameValid),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonConfigurationInvalid),
		"Message": ContainSubstring("Found 2 Notebook"),
	}))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactAdvisory))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "2"))
	g.Expect(result.ImpactedObjects).To(HaveLen(2))
}

func TestContainerNameCheck_WithOAuthProxy(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	// Notebook with oauth-proxy sidecar and primary container mismatch.
	// Only the primary container should be compared.
	nb := newContainerNameNotebookWithOAuthProxy("my-workbench", "test-ns", "wrong-name", map[string]string{
		validate.AnnotationAcceleratorName: "gpu-large",
	})

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      containerNameListKinds,
		Objects:        []*unstructured.Unstructured{nb},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := notebook.NewContainerNameCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(notebook.ConditionTypeContainerNameValid),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonConfigurationInvalid),
		"Message": ContainSubstring("Found 1 Notebook"),
	}))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactAdvisory))
	g.Expect(result.ImpactedObjects).To(HaveLen(1))
}

func TestContainerNameCheck_CanApply_Managed(t *testing.T) {
	g := NewWithT(t)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      containerNameListKinds,
		Objects:        []*unstructured.Unstructured{testutil.NewDSC(map[string]string{"workbenches": "Managed"})},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := notebook.NewContainerNameCheck()
	canApply, err := chk.CanApply(t.Context(), target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeTrue())
}

func TestContainerNameCheck_CanApply_Removed(t *testing.T) {
	g := NewWithT(t)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      containerNameListKinds,
		Objects:        []*unstructured.Unstructured{testutil.NewDSC(map[string]string{"workbenches": "Removed"})},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := notebook.NewContainerNameCheck()
	canApply, err := chk.CanApply(t.Context(), target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeFalse())
}

func TestContainerNameCheck_Metadata(t *testing.T) {
	g := NewWithT(t)

	chk := notebook.NewContainerNameCheck()

	g.Expect(chk.ID()).To(Equal("workloads.notebook.container-name-mismatch"))
	g.Expect(chk.Name()).To(Equal("Workloads :: Notebook :: Container Name Mismatch"))
	g.Expect(chk.Group()).To(Equal(check.GroupWorkload))
	g.Expect(chk.Description()).ToNot(BeEmpty())
}
