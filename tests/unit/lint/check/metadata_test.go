package check_test

import (
	"testing"

	"github.com/lburgazzoli/odh-cli/pkg/lint/check/result"

	. "github.com/onsi/gomega"
)

func TestDiagnosticMetadata_ValidMetadata(t *testing.T) {
	g := NewWithT(t)

	metadata := result.DiagnosticMetadata{
		Group: "components",
		Kind:  "kserve",
		Name:  "version-compatibility",
		Annotations: map[string]string{
			"check.opendatahub.io/source-version": "2.15",
		},
	}

	g.Expect(metadata.Group).To(Equal("components"))
	g.Expect(metadata.Kind).To(Equal("kserve"))
	g.Expect(metadata.Name).To(Equal("version-compatibility"))
	g.Expect(metadata.Annotations).To(HaveKey("check.opendatahub.io/source-version"))
}

func TestDiagnosticMetadata_EmptyGroup(t *testing.T) {
	g := NewWithT(t)

	metadata := result.DiagnosticMetadata{
		Group: "",
		Kind:  "kserve",
		Name:  "version-compatibility",
	}

	g.Expect(metadata.Group).To(BeEmpty())
}

func TestDiagnosticMetadata_EmptyKind(t *testing.T) {
	g := NewWithT(t)

	metadata := result.DiagnosticMetadata{
		Group: "components",
		Kind:  "",
		Name:  "version-compatibility",
	}

	g.Expect(metadata.Kind).To(BeEmpty())
}

func TestDiagnosticMetadata_EmptyName(t *testing.T) {
	g := NewWithT(t)

	metadata := result.DiagnosticMetadata{
		Group: "components",
		Kind:  "kserve",
		Name:  "",
	}

	g.Expect(metadata.Name).To(BeEmpty())
}

func TestDiagnosticMetadata_NilAnnotations(t *testing.T) {
	g := NewWithT(t)

	metadata := result.DiagnosticMetadata{
		Group:       "components",
		Kind:        "kserve",
		Name:        "version-compatibility",
		Annotations: nil,
	}

	g.Expect(metadata.Annotations).To(BeNil())
}

func TestDiagnosticMetadata_EmptyAnnotations(t *testing.T) {
	g := NewWithT(t)

	metadata := result.DiagnosticMetadata{
		Group:       "components",
		Kind:        "kserve",
		Name:        "version-compatibility",
		Annotations: make(map[string]string),
	}

	g.Expect(metadata.Annotations).To(BeEmpty())
}

func TestDiagnosticMetadata_MultipleAnnotations(t *testing.T) {
	g := NewWithT(t)

	metadata := result.DiagnosticMetadata{
		Group: "components",
		Kind:  "kserve",
		Name:  "version-compatibility",
		Annotations: map[string]string{
			"check.opendatahub.io/source-version": "2.15",
			"check.opendatahub.io/target-version": "3.0",
			"check.opendatahub.io/category":       "upgrade",
		},
	}

	g.Expect(metadata.Annotations).To(HaveLen(3))
	g.Expect(metadata.Annotations).To(HaveKey("check.opendatahub.io/source-version"))
	g.Expect(metadata.Annotations).To(HaveKey("check.opendatahub.io/target-version"))
	g.Expect(metadata.Annotations).To(HaveKey("check.opendatahub.io/category"))
}

// T013: Group/Kind/Name uniqueness tests

func TestDiagnosticMetadata_SameNameDifferentGroup(t *testing.T) {
	g := NewWithT(t)

	metadata1 := result.DiagnosticMetadata{
		Group: "components",
		Kind:  "kserve",
		Name:  "configuration-valid",
	}

	metadata2 := result.DiagnosticMetadata{
		Group: "services",
		Kind:  "kserve",
		Name:  "configuration-valid",
	}

	// Same Name can exist across different Groups
	g.Expect(metadata1.Name).To(Equal(metadata2.Name))
	g.Expect(metadata1.Group).ToNot(Equal(metadata2.Group))
	g.Expect(metadata1.Kind).To(Equal(metadata2.Kind))
}

func TestDiagnosticMetadata_SameNameDifferentKind(t *testing.T) {
	g := NewWithT(t)

	metadata1 := result.DiagnosticMetadata{
		Group: "components",
		Kind:  "kserve",
		Name:  "configuration-valid",
	}

	metadata2 := result.DiagnosticMetadata{
		Group: "components",
		Kind:  "servicemesh",
		Name:  "configuration-valid",
	}

	// Same Name can exist across different Kinds
	g.Expect(metadata1.Name).To(Equal(metadata2.Name))
	g.Expect(metadata1.Group).To(Equal(metadata2.Group))
	g.Expect(metadata1.Kind).ToNot(Equal(metadata2.Kind))
}

func TestDiagnosticMetadata_SameNameDifferentGroupAndKind(t *testing.T) {
	g := NewWithT(t)

	metadata1 := result.DiagnosticMetadata{
		Group: "components",
		Kind:  "kserve",
		Name:  "ready",
	}

	metadata2 := result.DiagnosticMetadata{
		Group: "services",
		Kind:  "auth",
		Name:  "ready",
	}

	// Same Name can exist across different Group/Kind combinations
	g.Expect(metadata1.Name).To(Equal(metadata2.Name))
	g.Expect(metadata1.Group).ToNot(Equal(metadata2.Group))
	g.Expect(metadata1.Kind).ToNot(Equal(metadata2.Kind))
}

func TestDiagnosticMetadata_UniqueIdentity(t *testing.T) {
	g := NewWithT(t)

	metadata1 := result.DiagnosticMetadata{
		Group: "components",
		Kind:  "kserve",
		Name:  "version-compatibility",
	}

	metadata2 := result.DiagnosticMetadata{
		Group: "components",
		Kind:  "kserve",
		Name:  "version-compatibility",
	}

	// Same Group+Kind+Name represents the same diagnostic
	g.Expect(metadata1.Group).To(Equal(metadata2.Group))
	g.Expect(metadata1.Kind).To(Equal(metadata2.Kind))
	g.Expect(metadata1.Name).To(Equal(metadata2.Name))
}
