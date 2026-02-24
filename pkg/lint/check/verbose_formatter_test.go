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
