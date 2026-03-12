package check_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/result"

	. "github.com/onsi/gomega"
)

func TestDefaultVerboseFormatter_NamespacedObjects(t *testing.T) {
	g := NewWithT(t)

	dr := result.New("workload", "kserve", "impacted-workloads", "test description")
	dr.ImpactedObjects = []metav1.PartialObjectMetadata{
		{
			TypeMeta:   metav1.TypeMeta{Kind: "InferenceService"},
			ObjectMeta: metav1.ObjectMeta{Namespace: "ns-b", Name: "isvc-2"},
		},
		{
			TypeMeta:   metav1.TypeMeta{Kind: "InferenceService"},
			ObjectMeta: metav1.ObjectMeta{Namespace: "ns-a", Name: "isvc-1"},
		},
		{
			TypeMeta:   metav1.TypeMeta{Kind: "InferenceService"},
			ObjectMeta: metav1.ObjectMeta{Namespace: "ns-a", Name: "isvc-3"},
		},
	}

	formatter := &check.DefaultVerboseFormatter{}

	var buf bytes.Buffer
	formatter.FormatVerboseOutput(&buf, dr)

	expected := "" +
		"    ns-a:\n" +
		"      - isvc-1 (InferenceService)\n" +
		"      - isvc-3 (InferenceService)\n" +
		"    ns-b:\n" +
		"      - isvc-2 (InferenceService)\n"

	g.Expect(buf.String()).To(Equal(expected))
}

func TestDefaultVerboseFormatter_ClusterScopedObjects(t *testing.T) {
	g := NewWithT(t)

	dr := result.New("dependency", "cert-manager", "installed", "test description")
	dr.ImpactedObjects = []metav1.PartialObjectMetadata{
		{
			TypeMeta:   metav1.TypeMeta{Kind: "ClusterRole"},
			ObjectMeta: metav1.ObjectMeta{Name: "cluster-resource"},
		},
	}

	formatter := &check.DefaultVerboseFormatter{}

	var buf bytes.Buffer
	formatter.FormatVerboseOutput(&buf, dr)

	expected := "    - cluster-resource (ClusterRole)\n"
	g.Expect(buf.String()).To(Equal(expected))
}

func TestDefaultVerboseFormatter_WithNamespaceRequesters(t *testing.T) {
	g := NewWithT(t)

	dr := result.New("workload", "notebook", "impacted-workloads", "test description")
	dr.ImpactedObjects = []metav1.PartialObjectMetadata{
		{
			TypeMeta:   metav1.TypeMeta{Kind: "Notebook"},
			ObjectMeta: metav1.ObjectMeta{Namespace: "user-ns", Name: "my-notebook"},
		},
	}

	formatter := &check.DefaultVerboseFormatter{
		NamespaceRequesters: map[string]string{
			"user-ns": "jdoe",
		},
	}

	var buf bytes.Buffer
	formatter.FormatVerboseOutput(&buf, dr)

	expected := "" +
		"    user-ns (requester: jdoe):\n" +
		"      - my-notebook (Notebook)\n"

	g.Expect(buf.String()).To(Equal(expected))
}

func TestDefaultVerboseFormatter_EmptyKind(t *testing.T) {
	g := NewWithT(t)

	dr := result.New("workload", "test", "check", "test description")
	dr.ImpactedObjects = []metav1.PartialObjectMetadata{
		{
			ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "obj-name"},
		},
	}

	formatter := &check.DefaultVerboseFormatter{}

	var buf bytes.Buffer
	formatter.FormatVerboseOutput(&buf, dr)

	expected := "" +
		"    ns:\n" +
		"      - obj-name\n"

	g.Expect(buf.String()).To(Equal(expected))
}

func TestDefaultVerboseFormatter_EmptyObjects(t *testing.T) {
	g := NewWithT(t)

	dr := result.New("workload", "test", "check", "test description")

	formatter := &check.DefaultVerboseFormatter{}

	var buf bytes.Buffer
	formatter.FormatVerboseOutput(&buf, dr)

	g.Expect(buf.String()).To(BeEmpty())
}

func TestVerboseOutputFormatter_TypeAssertion_CustomCheck(t *testing.T) {
	g := NewWithT(t)

	customCheck := &mockFormatterCheck{}

	var c check.Check = customCheck
	f, ok := c.(check.VerboseOutputFormatter)
	g.Expect(ok).To(BeTrue())
	g.Expect(f).NotTo(BeNil())

	dr := result.New("workload", "test", "check", "test description")
	dr.ImpactedObjects = []metav1.PartialObjectMetadata{
		{ObjectMeta: metav1.ObjectMeta{Name: "obj1"}},
	}

	var buf bytes.Buffer
	f.FormatVerboseOutput(&buf, dr)
	g.Expect(buf.String()).To(Equal("custom: 1 objects\n"))
}

func TestVerboseOutputFormatter_TypeAssertion_PlainCheck(t *testing.T) {
	g := NewWithT(t)

	plainCheck := &mockPlainCheck{}

	var c check.Check = plainCheck
	_, ok := c.(check.VerboseOutputFormatter)
	g.Expect(ok).To(BeFalse())
}

// --- EnhancedVerboseFormatter tests ---

func TestEnhancedVerboseFormatter_NamespacedObjects(t *testing.T) {
	g := NewWithT(t)

	dr := result.New("workload", "notebook", "impacted-workloads", "test description")
	dr.Annotations[result.AnnotationResourceCRDName] = "notebooks.kubeflow.org"
	dr.ImpactedObjects = []metav1.PartialObjectMetadata{
		{ObjectMeta: metav1.ObjectMeta{Namespace: "ns-b", Name: "nb-2"}},
		{ObjectMeta: metav1.ObjectMeta{Namespace: "ns-a", Name: "nb-1"}},
		{ObjectMeta: metav1.ObjectMeta{Namespace: "ns-a", Name: "nb-3"}},
	}

	formatter := &check.EnhancedVerboseFormatter{}

	var buf bytes.Buffer
	formatter.FormatVerboseOutput(&buf, dr)

	expected := "" +
		"      namespace: ns-a\n" +
		"        - notebooks.kubeflow.org/nb-1\n" +
		"        - notebooks.kubeflow.org/nb-3\n" +
		"\n" +
		"      namespace: ns-b\n" +
		"        - notebooks.kubeflow.org/nb-2\n"

	g.Expect(buf.String()).To(Equal(expected))
}

func TestEnhancedVerboseFormatter_ClusterScopedObjects(t *testing.T) {
	g := NewWithT(t)

	dr := result.New("workload", "test", "check", "test description")
	dr.Annotations[result.AnnotationResourceCRDName] = "widgets.example.io"
	dr.ImpactedObjects = []metav1.PartialObjectMetadata{
		{ObjectMeta: metav1.ObjectMeta{Name: "cluster-widget"}},
	}

	formatter := &check.EnhancedVerboseFormatter{}

	var buf bytes.Buffer
	formatter.FormatVerboseOutput(&buf, dr)

	expected := "      - widgets.example.io/cluster-widget\n"
	g.Expect(buf.String()).To(Equal(expected))
}

func TestEnhancedVerboseFormatter_WithNamespaceRequesters(t *testing.T) {
	g := NewWithT(t)

	dr := result.New("workload", "notebook", "check", "test description")
	dr.Annotations[result.AnnotationResourceCRDName] = "notebooks.kubeflow.org"
	dr.ImpactedObjects = []metav1.PartialObjectMetadata{
		{ObjectMeta: metav1.ObjectMeta{Namespace: "user-ns", Name: "my-nb"}},
	}

	formatter := &check.EnhancedVerboseFormatter{}
	formatter.SetNamespaceRequesters(map[string]string{
		"user-ns": "jdoe",
	})

	var buf bytes.Buffer
	formatter.FormatVerboseOutput(&buf, dr)

	expected := "" +
		"      namespace: user-ns | requester: jdoe\n" +
		"        - notebooks.kubeflow.org/my-nb\n"

	g.Expect(buf.String()).To(Equal(expected))
}

func TestEnhancedVerboseFormatter_FallsBackToTypeMeta(t *testing.T) {
	g := NewWithT(t)

	// No AnnotationResourceCRDName — should derive from TypeMeta.
	dr := result.New("workload", "notebook", "check", "test description")
	dr.ImpactedObjects = []metav1.PartialObjectMetadata{
		{
			TypeMeta:   metav1.TypeMeta{Kind: "Notebook", APIVersion: "kubeflow.org/v1"},
			ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "nb-1"},
		},
	}

	formatter := &check.EnhancedVerboseFormatter{}

	var buf bytes.Buffer
	formatter.FormatVerboseOutput(&buf, dr)

	expected := "" +
		"      namespace: ns\n" +
		"        - notebooks.kubeflow.org/nb-1\n"

	g.Expect(buf.String()).To(Equal(expected))
}

func TestEnhancedVerboseFormatter_MixedKinds(t *testing.T) {
	g := NewWithT(t)

	// Mixed resource types — each object should get its own CRD FQN prefix.
	dr := result.New("workload", "kueue", "data-integrity", "test description")
	dr.ImpactedObjects = []metav1.PartialObjectMetadata{
		{
			TypeMeta:   metav1.TypeMeta{Kind: "Notebook", APIVersion: "kubeflow.org/v1"},
			ObjectMeta: metav1.ObjectMeta{Namespace: "ns-a", Name: "my-nb"},
		},
		{
			TypeMeta:   metav1.TypeMeta{Kind: "RayCluster", APIVersion: "ray.io/v1"},
			ObjectMeta: metav1.ObjectMeta{Namespace: "ns-a", Name: "my-ray"},
		},
		{
			TypeMeta:   metav1.TypeMeta{Kind: "Notebook", APIVersion: "kubeflow.org/v1"},
			ObjectMeta: metav1.ObjectMeta{Namespace: "ns-b", Name: "other-nb"},
		},
	}

	formatter := &check.EnhancedVerboseFormatter{}

	var buf bytes.Buffer
	formatter.FormatVerboseOutput(&buf, dr)

	expected := "" +
		"      namespace: ns-a\n" +
		"        - notebooks.kubeflow.org/my-nb\n" +
		"        - rayclusters.ray.io/my-ray\n" +
		"\n" +
		"      namespace: ns-b\n" +
		"        - notebooks.kubeflow.org/other-nb\n"

	g.Expect(buf.String()).To(Equal(expected))
}

func TestEnhancedVerboseFormatter_MixedKindsIgnoresAnnotation(t *testing.T) {
	g := NewWithT(t)

	// When multiple kinds are present, the AnnotationResourceCRDName annotation
	// should be ignored — it only applies to single-kind results.
	dr := result.New("workload", "kueue", "data-integrity", "test description")
	dr.Annotations[result.AnnotationResourceCRDName] = "notebooks.kubeflow.org"
	dr.ImpactedObjects = []metav1.PartialObjectMetadata{
		{
			TypeMeta:   metav1.TypeMeta{Kind: "Notebook", APIVersion: "kubeflow.org/v1"},
			ObjectMeta: metav1.ObjectMeta{Namespace: "ns-a", Name: "my-nb"},
		},
		{
			TypeMeta:   metav1.TypeMeta{Kind: "RayCluster", APIVersion: "ray.io/v1"},
			ObjectMeta: metav1.ObjectMeta{Namespace: "ns-a", Name: "my-ray"},
		},
	}

	formatter := &check.EnhancedVerboseFormatter{}

	var buf bytes.Buffer
	formatter.FormatVerboseOutput(&buf, dr)

	output := buf.String()

	// RayCluster should use derived FQN, not the annotation value.
	g.Expect(output).To(ContainSubstring("rayclusters.ray.io/my-ray"))
	g.Expect(output).To(ContainSubstring("notebooks.kubeflow.org/my-nb"))
}

func TestEnhancedVerboseFormatter_MixedKindsSortedByCRDThenName(t *testing.T) {
	g := NewWithT(t)

	// Objects within the same namespace should sort by CRD FQN, then by name.
	dr := result.New("workload", "kueue", "data-integrity", "test description")
	dr.ImpactedObjects = []metav1.PartialObjectMetadata{
		{
			TypeMeta:   metav1.TypeMeta{Kind: "RayCluster", APIVersion: "ray.io/v1"},
			ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "rc-b"},
		},
		{
			TypeMeta:   metav1.TypeMeta{Kind: "Notebook", APIVersion: "kubeflow.org/v1"},
			ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "nb-a"},
		},
		{
			TypeMeta:   metav1.TypeMeta{Kind: "RayCluster", APIVersion: "ray.io/v1"},
			ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "rc-a"},
		},
	}

	formatter := &check.EnhancedVerboseFormatter{}

	var buf bytes.Buffer
	formatter.FormatVerboseOutput(&buf, dr)

	expected := "" +
		"      namespace: ns\n" +
		"        - notebooks.kubeflow.org/nb-a\n" +
		"        - rayclusters.ray.io/rc-a\n" +
		"        - rayclusters.ray.io/rc-b\n"

	g.Expect(buf.String()).To(Equal(expected))
}

func TestEnhancedVerboseFormatter_SingleKindPrefersAnnotation(t *testing.T) {
	g := NewWithT(t)

	// When all objects share one kind, the annotation should be preferred
	// over TypeMeta derivation for an accurate plural form.
	dr := result.New("workload", "kserve", "check", "test description")
	dr.Annotations[result.AnnotationResourceCRDName] = "inferenceservices.serving.kserve.io"
	dr.ImpactedObjects = []metav1.PartialObjectMetadata{
		{
			TypeMeta:   metav1.TypeMeta{Kind: "InferenceService", APIVersion: "serving.kserve.io/v1beta1"},
			ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "isvc-1"},
		},
	}

	formatter := &check.EnhancedVerboseFormatter{}

	var buf bytes.Buffer
	formatter.FormatVerboseOutput(&buf, dr)

	expected := "" +
		"      namespace: ns\n" +
		"        - inferenceservices.serving.kserve.io/isvc-1\n"

	g.Expect(buf.String()).To(Equal(expected))
}

func TestEnhancedVerboseFormatter_EmptyObjects(t *testing.T) {
	g := NewWithT(t)

	dr := result.New("workload", "test", "check", "test description")

	formatter := &check.EnhancedVerboseFormatter{}

	var buf bytes.Buffer
	formatter.FormatVerboseOutput(&buf, dr)

	g.Expect(buf.String()).To(BeEmpty())
}

func TestEnhancedVerboseFormatter_ImplementsVerboseOutputFormatter(t *testing.T) {
	g := NewWithT(t)

	// A check embedding EnhancedVerboseFormatter should satisfy VerboseOutputFormatter.
	chk := &mockEnhancedCheck{}

	var c check.Check = chk
	_, ok := c.(check.VerboseOutputFormatter)
	g.Expect(ok).To(BeTrue())
}

// --- AnnotationObjectContext tests ---

func TestEnhancedVerboseFormatter_ObjectContextNamespaced(t *testing.T) {
	g := NewWithT(t)

	dr := result.New("workload", "kueue", "data-integrity", "test description")
	dr.ImpactedObjects = []metav1.PartialObjectMetadata{
		{
			TypeMeta:   metav1.TypeMeta{Kind: "Notebook", APIVersion: "kubeflow.org/v1"},
			ObjectMeta: metav1.ObjectMeta{Namespace: "ns-a", Name: "nb-1", Annotations: map[string]string{result.AnnotationObjectContext: "missing queue-name label in kueue-managed namespace"}},
		},
		{
			TypeMeta:   metav1.TypeMeta{Kind: "RayCluster", APIVersion: "ray.io/v1"},
			ObjectMeta: metav1.ObjectMeta{Namespace: "ns-a", Name: "rc-1", Annotations: map[string]string{result.AnnotationObjectContext: "queue-name label mismatch in owner tree"}},
		},
	}

	formatter := &check.EnhancedVerboseFormatter{}

	var buf bytes.Buffer
	formatter.FormatVerboseOutput(&buf, dr)

	expected := "" +
		"      namespace: ns-a\n" +
		"        - notebooks.kubeflow.org/nb-1\n" +
		"          — (missing queue-name label in kueue-managed namespace)\n" +
		"        - rayclusters.ray.io/rc-1\n" +
		"          — (queue-name label mismatch in owner tree)\n"

	g.Expect(buf.String()).To(Equal(expected))
}

func TestEnhancedVerboseFormatter_ObjectContextClusterScoped(t *testing.T) {
	g := NewWithT(t)

	dr := result.New("workload", "test", "check", "test description")
	dr.Annotations[result.AnnotationResourceCRDName] = "widgets.example.io"
	dr.ImpactedObjects = []metav1.PartialObjectMetadata{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "widget-1", Annotations: map[string]string{result.AnnotationObjectContext: "some context"}},
		},
	}

	formatter := &check.EnhancedVerboseFormatter{}

	var buf bytes.Buffer
	formatter.FormatVerboseOutput(&buf, dr)

	expected := "" +
		"      - widgets.example.io/widget-1\n" +
		"        — (some context)\n"

	g.Expect(buf.String()).To(Equal(expected))
}

func TestEnhancedVerboseFormatter_ObjectContextMixedPresence(t *testing.T) {
	g := NewWithT(t)

	// Only some objects have context — others should render without sub-bullet.
	dr := result.New("workload", "notebook", "check", "test description")
	dr.Annotations[result.AnnotationResourceCRDName] = "notebooks.kubeflow.org"
	dr.ImpactedObjects = []metav1.PartialObjectMetadata{
		{
			ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "nb-with-context", Annotations: map[string]string{result.AnnotationObjectContext: "has context"}},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "nb-without-context"},
		},
	}

	formatter := &check.EnhancedVerboseFormatter{}

	var buf bytes.Buffer
	formatter.FormatVerboseOutput(&buf, dr)

	expected := "" +
		"      namespace: ns\n" +
		"        - notebooks.kubeflow.org/nb-with-context\n" +
		"          — (has context)\n" +
		"        - notebooks.kubeflow.org/nb-without-context\n"

	g.Expect(buf.String()).To(Equal(expected))
}

// --- CRDFullyQualifiedName and DeriveCRDFQNFromTypeMeta tests ---

func TestCRDFullyQualifiedName_PrefersAnnotation(t *testing.T) {
	g := NewWithT(t)

	dr := result.New("workload", "notebook", "check", "test description")
	dr.Annotations[result.AnnotationResourceCRDName] = "notebooks.kubeflow.org"
	// TypeMeta would produce a different result — annotation should win.
	dr.ImpactedObjects = []metav1.PartialObjectMetadata{
		{
			TypeMeta:   metav1.TypeMeta{Kind: "Widget", APIVersion: "other.io/v1"},
			ObjectMeta: metav1.ObjectMeta{Name: "obj"},
		},
	}

	g.Expect(check.CRDFullyQualifiedName(dr)).To(Equal("notebooks.kubeflow.org"))
}

func TestCRDFullyQualifiedName_FallsBackToTypeMeta(t *testing.T) {
	g := NewWithT(t)

	dr := result.New("workload", "notebook", "check", "test description")
	dr.ImpactedObjects = []metav1.PartialObjectMetadata{
		{
			TypeMeta:   metav1.TypeMeta{Kind: "Notebook", APIVersion: "kubeflow.org/v1"},
			ObjectMeta: metav1.ObjectMeta{Name: "obj"},
		},
	}

	g.Expect(check.CRDFullyQualifiedName(dr)).To(Equal("notebooks.kubeflow.org"))
}

func TestCRDFullyQualifiedName_EmptyObjects(t *testing.T) {
	g := NewWithT(t)

	dr := result.New("workload", "test", "check", "test description")

	g.Expect(check.CRDFullyQualifiedName(dr)).To(BeEmpty())
}

func TestDeriveCRDFQNFromTypeMeta_GroupedAPIVersion(t *testing.T) {
	g := NewWithT(t)

	objects := []metav1.PartialObjectMetadata{
		{TypeMeta: metav1.TypeMeta{Kind: "InferenceService", APIVersion: "serving.kserve.io/v1beta1"}},
	}

	g.Expect(check.DeriveCRDFQNFromTypeMeta(objects)).To(Equal("inferenceservices.serving.kserve.io"))
}

func TestDeriveCRDFQNFromTypeMeta_CoreAPIVersion(t *testing.T) {
	g := NewWithT(t)

	// Core API (e.g. "v1") has no group — should return plural only.
	objects := []metav1.PartialObjectMetadata{
		{TypeMeta: metav1.TypeMeta{Kind: "ConfigMap", APIVersion: "v1"}},
	}

	g.Expect(check.DeriveCRDFQNFromTypeMeta(objects)).To(Equal("configmaps"))
}

func TestDeriveCRDFQNFromTypeMeta_EmptyAPIVersion(t *testing.T) {
	g := NewWithT(t)

	objects := []metav1.PartialObjectMetadata{
		{TypeMeta: metav1.TypeMeta{Kind: "Notebook"}},
	}

	g.Expect(check.DeriveCRDFQNFromTypeMeta(objects)).To(Equal("notebooks"))
}

func TestDeriveCRDFQNFromTypeMeta_EmptyKind(t *testing.T) {
	g := NewWithT(t)

	objects := []metav1.PartialObjectMetadata{
		{TypeMeta: metav1.TypeMeta{APIVersion: "kubeflow.org/v1"}},
	}

	g.Expect(check.DeriveCRDFQNFromTypeMeta(objects)).To(BeEmpty())
}

func TestDeriveCRDFQNFromTypeMeta_EmptyObjects(t *testing.T) {
	g := NewWithT(t)

	g.Expect(check.DeriveCRDFQNFromTypeMeta(nil)).To(BeEmpty())
}

// mockEnhancedCheck is a check that embeds EnhancedVerboseFormatter.
type mockEnhancedCheck struct {
	check.BaseCheck
	check.EnhancedVerboseFormatter
}

func (c *mockEnhancedCheck) CanApply(_ context.Context, _ check.Target) (bool, error) {
	return true, nil
}

func (c *mockEnhancedCheck) Validate(_ context.Context, _ check.Target) (*result.DiagnosticResult, error) {
	return c.NewResult(), nil
}

// mockFormatterCheck is a check that implements VerboseOutputFormatter.
type mockFormatterCheck struct {
	check.BaseCheck
}

func (c *mockFormatterCheck) CanApply(_ context.Context, _ check.Target) (bool, error) {
	return true, nil
}

func (c *mockFormatterCheck) Validate(_ context.Context, _ check.Target) (*result.DiagnosticResult, error) {
	return c.NewResult(), nil
}

func (c *mockFormatterCheck) FormatVerboseOutput(out io.Writer, dr *result.DiagnosticResult) {
	_, _ = fmt.Fprintf(out, "custom: %d objects\n", len(dr.ImpactedObjects))
}

// mockPlainCheck is a check that does NOT implement VerboseOutputFormatter.
type mockPlainCheck struct {
	check.BaseCheck
}

func (c *mockPlainCheck) CanApply(_ context.Context, _ check.Target) (bool, error) {
	return true, nil
}

func (c *mockPlainCheck) Validate(_ context.Context, _ check.Target) (*result.DiagnosticResult, error) {
	return c.NewResult(), nil
}
