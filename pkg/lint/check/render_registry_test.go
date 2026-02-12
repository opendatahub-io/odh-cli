package check_test

import (
	"bytes"
	"io"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/lburgazzoli/odh-cli/pkg/lint/check"

	. "github.com/onsi/gomega"
)

// Test constants to avoid magic strings.
const (
	// Groups for isolation tests.
	testGroupIsolatedObj   check.CheckGroup = "test-isolated-obj-group"
	testGroupDifferentObj  check.CheckGroup = "test-different-obj-group"
	testGroupIsolatedGrp   check.CheckGroup = "test-isolated-grp-group"
	testGroupDifferentGrp  check.CheckGroup = "test-different-grp-group"
	testGroupCoexist       check.CheckGroup = "test-coexist-group"
	testGroupUnregistered  check.CheckGroup = "test-unregistered-group"
	testGroupDefaultFormat check.CheckGroup = "test-default-format-group"

	// Kinds for isolation tests.
	testKindIsolatedObj  = "test-isolated-obj-kind"
	testKindDifferentObj = "test-different-obj-kind"
	testKindIsolatedGrp  = "test-isolated-grp-kind"
	testKindDifferentGrp = "test-different-grp-kind"
	testKindCoexist      = "test-coexist-kind"
	testKindUnregistered = "test-unregistered-kind"
	testKindWorkload     = "test-render-workload"
	testKindComponent    = "test-render-component"
	testKindGroupRender  = "test-group-render-kind"

	// Check types for isolation tests.
	testCheckTypeIsolatedObj  check.CheckType = "test-isolated-obj-check"
	testCheckTypeDifferentObj check.CheckType = "test-different-obj-check"
	testCheckTypeIsolatedGrp  check.CheckType = "test-isolated-grp-check"
	testCheckTypeDifferentGrp check.CheckType = "test-different-grp-check"
	testCheckTypeCoexist      check.CheckType = "test-coexist-check"
	testCheckTypeUnregistered check.CheckType = "test-unregistered-check"
	testCheckTypeDefault      check.CheckType = "test-default-check"
	testCheckTypeWorkload     check.CheckType = "test-workload-check"
	testCheckTypeComponent    check.CheckType = "test-component-check"
	testCheckTypeGroupRender  check.CheckType = "test-group-render-check"
)

func TestGetImpactedObjectRenderer_DefaultFormat(t *testing.T) {
	tests := []struct {
		name     string
		obj      metav1.PartialObjectMetadata
		expected string
	}{
		{
			name: "with namespace",
			obj: metav1.PartialObjectMetadata{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "my-namespace",
					Name:      "my-name",
				},
				TypeMeta: metav1.TypeMeta{
					Kind: "Notebook",
				},
			},
			expected: "my-namespace/my-name (Notebook)",
		},
		{
			name: "without namespace",
			obj: metav1.PartialObjectMetadata{
				ObjectMeta: metav1.ObjectMeta{
					Name: "cluster-resource",
				},
				TypeMeta: metav1.TypeMeta{
					Kind: "ClusterRole",
				},
			},
			expected: "cluster-resource (ClusterRole)",
		},
		{
			name: "empty kind",
			obj: metav1.PartialObjectMetadata{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns",
					Name:      "name",
				},
			},
			expected: "ns/name ()",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			// Use an unregistered group/kind/checkType to get the default renderer.
			renderer := check.GetImpactedObjectRenderer(testGroupDefaultFormat, tt.name, testCheckTypeDefault)
			result := renderer(tt.obj)
			g.Expect(result).To(Equal(tt.expected))
		})
	}
}

func TestGetImpactedObjectRenderer_Unregistered(t *testing.T) {
	g := NewWithT(t)

	// Unregistered group/kind/checkType should return default renderer (not nil).
	renderer := check.GetImpactedObjectRenderer(testGroupUnregistered, testKindUnregistered, testCheckTypeUnregistered)
	g.Expect(renderer).NotTo(BeNil())

	obj := metav1.PartialObjectMetadata{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "test-ns",
			Name:      "test-name",
		},
		TypeMeta: metav1.TypeMeta{
			Kind: "TestKind",
		},
	}

	result := renderer(obj)
	g.Expect(result).To(Equal("test-ns/test-name (TestKind)"))
}

func TestRegisterAndGetImpactedObjectRenderer(t *testing.T) {
	tests := []struct {
		name           string
		group          check.CheckGroup
		kind           string
		checkType      check.CheckType
		customRenderer check.ImpactedObjectRenderer
		expectedOutput string
	}{
		{
			name:      "custom renderer for workload",
			group:     check.GroupWorkload,
			kind:      testKindWorkload,
			checkType: testCheckTypeWorkload,
			customRenderer: func(obj metav1.PartialObjectMetadata) string {
				return "custom: " + obj.Name
			},
			expectedOutput: "custom: my-object",
		},
		{
			name:      "custom renderer for component",
			group:     check.GroupComponent,
			kind:      testKindComponent,
			checkType: testCheckTypeComponent,
			customRenderer: func(obj metav1.PartialObjectMetadata) string {
				return "component: " + obj.Namespace + "/" + obj.Name
			},
			expectedOutput: "component: ns/my-object",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			// Register custom renderer.
			check.RegisterImpactedObjectRenderer(tt.group, tt.kind, tt.checkType, tt.customRenderer)

			// Retrieve and verify.
			renderer := check.GetImpactedObjectRenderer(tt.group, tt.kind, tt.checkType)
			g.Expect(renderer).NotTo(BeNil())

			obj := metav1.PartialObjectMetadata{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns",
					Name:      "my-object",
				},
			}

			result := renderer(obj)
			g.Expect(result).To(Equal(tt.expectedOutput))
		})
	}
}

func TestImpactedObjectRenderer_IndependentKeys(t *testing.T) {
	g := NewWithT(t)

	const isolatedOutput = "isolated-obj-renderer"

	// Register renderer for one group/kind/checkType.
	check.RegisterImpactedObjectRenderer(testGroupIsolatedObj, testKindIsolatedObj, testCheckTypeIsolatedObj, func(obj metav1.PartialObjectMetadata) string {
		return isolatedOutput
	})

	// Different group/kind/checkType should still return default.
	renderer := check.GetImpactedObjectRenderer(testGroupDifferentObj, testKindDifferentObj, testCheckTypeDifferentObj)

	obj := metav1.PartialObjectMetadata{
		ObjectMeta: metav1.ObjectMeta{Name: "test"},
		TypeMeta:   metav1.TypeMeta{Kind: "Test"},
	}

	result := renderer(obj)
	g.Expect(result).To(Equal("test (Test)"))

	// Original should return custom renderer.
	originalRenderer := check.GetImpactedObjectRenderer(testGroupIsolatedObj, testKindIsolatedObj, testCheckTypeIsolatedObj)
	g.Expect(originalRenderer(obj)).To(Equal(isolatedOutput))
}

func TestGetImpactedGroupRenderer_Unregistered(t *testing.T) {
	g := NewWithT(t)

	// Unregistered group/kind/checkType should return nil.
	renderer := check.GetImpactedGroupRenderer(testGroupUnregistered, testKindUnregistered, testCheckTypeUnregistered)
	g.Expect(renderer).To(BeNil())
}

func TestRegisterAndGetImpactedGroupRenderer(t *testing.T) {
	g := NewWithT(t)

	const groupRenderedOutput = "group rendered"

	var capturedObjects []metav1.PartialObjectMetadata
	var capturedMaxDisplay int

	// Register group renderer.
	check.RegisterImpactedGroupRenderer(check.GroupWorkload, testKindGroupRender, testCheckTypeGroupRender, func(
		out io.Writer,
		objects []metav1.PartialObjectMetadata,
		maxDisplay int,
	) {
		capturedObjects = objects
		capturedMaxDisplay = maxDisplay
		_, _ = out.Write([]byte(groupRenderedOutput))
	})

	// Retrieve renderer.
	renderer := check.GetImpactedGroupRenderer(check.GroupWorkload, testKindGroupRender, testCheckTypeGroupRender)
	g.Expect(renderer).NotTo(BeNil())

	// Call renderer and verify behavior.
	objects := []metav1.PartialObjectMetadata{
		{ObjectMeta: metav1.ObjectMeta{Name: "obj1"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "obj2"}},
	}

	var buf bytes.Buffer
	renderer(&buf, objects, 50)

	g.Expect(buf.String()).To(Equal(groupRenderedOutput))
	g.Expect(capturedObjects).To(HaveLen(2))
	g.Expect(capturedMaxDisplay).To(Equal(50))
}

func TestImpactedGroupRenderer_IndependentKeys(t *testing.T) {
	g := NewWithT(t)

	const isolatedGrpOutput = "isolated-grp-output"

	// Register renderer for one group/kind/checkType.
	check.RegisterImpactedGroupRenderer(testGroupIsolatedGrp, testKindIsolatedGrp, testCheckTypeIsolatedGrp, func(
		out io.Writer,
		objects []metav1.PartialObjectMetadata,
		maxDisplay int,
	) {
		_, _ = out.Write([]byte(isolatedGrpOutput))
	})

	// Different group/kind/checkType should return nil.
	renderer := check.GetImpactedGroupRenderer(testGroupDifferentGrp, testKindDifferentGrp, testCheckTypeDifferentGrp)
	g.Expect(renderer).To(BeNil())

	// Original should still work.
	originalRenderer := check.GetImpactedGroupRenderer(testGroupIsolatedGrp, testKindIsolatedGrp, testCheckTypeIsolatedGrp)
	g.Expect(originalRenderer).NotTo(BeNil())
}

func TestBothRenderersCanCoexist(t *testing.T) {
	g := NewWithT(t)

	const (
		objectRendererOutput = "object renderer output"
		groupRendererOutput  = "group renderer output"
	)

	// Register both object and group renderer for same key.
	check.RegisterImpactedObjectRenderer(testGroupCoexist, testKindCoexist, testCheckTypeCoexist, func(obj metav1.PartialObjectMetadata) string {
		return objectRendererOutput
	})

	check.RegisterImpactedGroupRenderer(testGroupCoexist, testKindCoexist, testCheckTypeCoexist, func(
		out io.Writer,
		objects []metav1.PartialObjectMetadata,
		maxDisplay int,
	) {
		_, _ = out.Write([]byte(groupRendererOutput))
	})

	// Both should be retrievable (caller decides precedence).
	objectRenderer := check.GetImpactedObjectRenderer(testGroupCoexist, testKindCoexist, testCheckTypeCoexist)
	groupRenderer := check.GetImpactedGroupRenderer(testGroupCoexist, testKindCoexist, testCheckTypeCoexist)

	g.Expect(objectRenderer).NotTo(BeNil())
	g.Expect(groupRenderer).NotTo(BeNil())

	// Verify each works independently.
	obj := metav1.PartialObjectMetadata{ObjectMeta: metav1.ObjectMeta{Name: "test"}}
	g.Expect(objectRenderer(obj)).To(Equal(objectRendererOutput))

	var buf bytes.Buffer
	groupRenderer(&buf, []metav1.PartialObjectMetadata{obj}, 10)
	g.Expect(buf.String()).To(Equal(groupRendererOutput))
}
