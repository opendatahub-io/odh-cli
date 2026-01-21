package notebook_test

import (
	"context"
	"testing"

	"github.com/blang/semver/v4"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	metadatafake "k8s.io/client-go/metadata/fake"

	"github.com/lburgazzoli/odh-cli/pkg/lint/check"
	resultpkg "github.com/lburgazzoli/odh-cli/pkg/lint/check/result"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/workloads/notebook"
	"github.com/lburgazzoli/odh-cli/pkg/resources"
	"github.com/lburgazzoli/odh-cli/pkg/util/client"

	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

//nolint:gochecknoglobals
var listKinds = map[schema.GroupVersionResource]string{
	resources.Notebook.GVR(): resources.Notebook.ListKind(),
}

func toPartialObjectMetadata(objs ...*unstructured.Unstructured) []runtime.Object {
	result := make([]runtime.Object, 0, len(objs))
	for _, obj := range objs {
		pom := &metav1.PartialObjectMetadata{
			TypeMeta: metav1.TypeMeta{
				APIVersion: obj.GetAPIVersion(),
				Kind:       obj.GetKind(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:        obj.GetName(),
				Namespace:   obj.GetNamespace(),
				Labels:      obj.GetLabels(),
				Annotations: obj.GetAnnotations(),
				Finalizers:  obj.GetFinalizers(),
			},
		}
		result = append(result, pom)
	}

	return result
}

func TestImpactedWorkloadsCheck_NoNotebooks(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	scheme := runtime.NewScheme()
	_ = metav1.AddMetaToScheme(scheme)
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, listKinds)
	metadataClient := metadatafake.NewSimpleMetadataClient(scheme)

	c := &client.Client{
		Dynamic:  dynamicClient,
		Metadata: metadataClient,
	}

	currentVer := semver.MustParse("2.17.0")
	targetVer := semver.MustParse("3.0.0")
	target := check.Target{
		Client:         c,
		CurrentVersion: &currentVer,
		TargetVersion:  &targetVer,
	}

	impactedCheck := notebook.NewImpactedWorkloadsCheck()
	result, err := impactedCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(notebook.ConditionTypeNotebooksCompatible),
		"Status":  Equal(metav1.ConditionTrue),
		"Reason":  Equal(check.ReasonVersionCompatible),
		"Message": ContainSubstring("No Notebooks found"),
	}))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "0"))
	g.Expect(result.ImpactedObjects).To(BeEmpty())
}

func TestImpactedWorkloadsCheck_SingleNotebook(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	notebook1 := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.Notebook.APIVersion(),
			"kind":       resources.Notebook.Kind,
			"metadata": map[string]any{
				"name":      "test-notebook",
				"namespace": "test-ns",
			},
		},
	}

	scheme := runtime.NewScheme()
	_ = metav1.AddMetaToScheme(scheme)
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, listKinds, notebook1)
	metadataClient := metadatafake.NewSimpleMetadataClient(scheme, toPartialObjectMetadata(notebook1)...)

	c := &client.Client{
		Dynamic:  dynamicClient,
		Metadata: metadataClient,
	}

	currentVer := semver.MustParse("2.17.0")
	targetVer := semver.MustParse("3.0.0")
	target := check.Target{
		Client:         c,
		CurrentVersion: &currentVer,
		TargetVersion:  &targetVer,
	}

	impactedCheck := notebook.NewImpactedWorkloadsCheck()
	result, err := impactedCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(notebook.ConditionTypeNotebooksCompatible),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonWorkloadsImpacted),
		"Message": And(ContainSubstring("Found 1 Notebook(s)"), ContainSubstring("will be impacted")),
	}))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactBlocking))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "1"))
	g.Expect(result.ImpactedObjects).To(HaveLen(1))
	g.Expect(result.ImpactedObjects[0].Name).To(Equal("test-notebook"))
	g.Expect(result.ImpactedObjects[0].Namespace).To(Equal("test-ns"))
}

func TestImpactedWorkloadsCheck_MultipleNotebooks(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	notebook1 := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.Notebook.APIVersion(),
			"kind":       resources.Notebook.Kind,
			"metadata": map[string]any{
				"name":      "notebook-1",
				"namespace": "ns1",
			},
		},
	}

	notebook2 := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.Notebook.APIVersion(),
			"kind":       resources.Notebook.Kind,
			"metadata": map[string]any{
				"name":      "notebook-2",
				"namespace": "ns2",
			},
		},
	}

	notebook3 := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.Notebook.APIVersion(),
			"kind":       resources.Notebook.Kind,
			"metadata": map[string]any{
				"name":      "notebook-3",
				"namespace": "ns1",
			},
		},
	}

	scheme := runtime.NewScheme()
	_ = metav1.AddMetaToScheme(scheme)
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		scheme,
		listKinds,
		notebook1,
		notebook2,
		notebook3,
	)
	metadataClient := metadatafake.NewSimpleMetadataClient(scheme, toPartialObjectMetadata(notebook1, notebook2, notebook3)...)

	c := &client.Client{
		Dynamic:  dynamicClient,
		Metadata: metadataClient,
	}

	currentVer := semver.MustParse("2.17.0")
	targetVer := semver.MustParse("3.0.0")
	target := check.Target{
		Client:         c,
		CurrentVersion: &currentVer,
		TargetVersion:  &targetVer,
	}

	impactedCheck := notebook.NewImpactedWorkloadsCheck()
	result, err := impactedCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(notebook.ConditionTypeNotebooksCompatible),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonWorkloadsImpacted),
		"Message": And(ContainSubstring("Found 3 Notebook(s)"), ContainSubstring("will be impacted")),
	}))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactBlocking))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "3"))
	g.Expect(result.ImpactedObjects).To(HaveLen(3))
}

func TestImpactedWorkloadsCheck_Metadata(t *testing.T) {
	g := NewWithT(t)

	impactedCheck := notebook.NewImpactedWorkloadsCheck()

	g.Expect(impactedCheck.ID()).To(Equal("workloads.notebook.impacted-workloads"))
	g.Expect(impactedCheck.Name()).To(Equal("Workloads :: Notebook :: Impacted Workloads (3.x)"))
	g.Expect(impactedCheck.Group()).To(Equal(check.GroupWorkload))
	g.Expect(impactedCheck.Description()).ToNot(BeEmpty())
}

func TestImpactedWorkloadsCheck_CanApply_LintMode(t *testing.T) {
	g := NewWithT(t)

	currentVer := semver.MustParse("2.17.0")
	target := check.Target{
		CurrentVersion: &currentVer,
		TargetVersion:  &currentVer,
	}

	impactedCheck := notebook.NewImpactedWorkloadsCheck()
	canApply := impactedCheck.CanApply(target)

	g.Expect(canApply).To(BeFalse())
}

func TestImpactedWorkloadsCheck_CanApply_Upgrade2xTo3x(t *testing.T) {
	g := NewWithT(t)

	currentVer := semver.MustParse("2.17.0")
	targetVer := semver.MustParse("3.0.0")
	target := check.Target{
		CurrentVersion: &currentVer,
		TargetVersion:  &targetVer,
	}

	impactedCheck := notebook.NewImpactedWorkloadsCheck()
	canApply := impactedCheck.CanApply(target)

	g.Expect(canApply).To(BeTrue())
}

func TestImpactedWorkloadsCheck_CanApply_Upgrade3xTo3x(t *testing.T) {
	g := NewWithT(t)

	currentVer := semver.MustParse("3.0.0")
	targetVer := semver.MustParse("3.1.0")
	target := check.Target{
		CurrentVersion: &currentVer,
		TargetVersion:  &targetVer,
	}

	impactedCheck := notebook.NewImpactedWorkloadsCheck()
	canApply := impactedCheck.CanApply(target)

	g.Expect(canApply).To(BeFalse())
}

func TestImpactedWorkloadsCheck_AnnotationTargetVersion(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	scheme := runtime.NewScheme()
	_ = metav1.AddMetaToScheme(scheme)
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, listKinds)
	metadataClient := metadatafake.NewSimpleMetadataClient(scheme)

	c := &client.Client{
		Dynamic:  dynamicClient,
		Metadata: metadataClient,
	}

	currentVer := semver.MustParse("2.17.0")
	targetVer := semver.MustParse("3.0.0")
	target := check.Target{
		Client:         c,
		CurrentVersion: &currentVer,
		TargetVersion:  &targetVer,
	}

	impactedCheck := notebook.NewImpactedWorkloadsCheck()
	result, err := impactedCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationCheckTargetVersion, "3.0.0"))
}
