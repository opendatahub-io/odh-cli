//nolint:testpackage // Tests internal implementation (depRegistry field)
package backup

import (
	"testing"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/cli-runtime/pkg/genericiooptions"

	. "github.com/onsi/gomega"
)

func TestCommandDefaults(t *testing.T) {
	g := NewWithT(t)

	cmd := NewCommand(genericiooptions.IOStreams{})

	g.Expect(cmd.Dependencies).To(BeTrue(), "Dependencies should default to true")
}

func TestCompleteWithDependenciesEnabled(t *testing.T) {
	g := NewWithT(t)

	cmd := NewCommand(genericiooptions.IOStreams{})
	cmd.Dependencies = true

	err := cmd.Complete()

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(cmd.depRegistry).ToNot(BeNil())

	// Verify notebook resolver is registered
	notebookGVR := schema.GroupVersionResource{
		Group:    "kubeflow.org",
		Version:  "v1",
		Resource: "notebooks",
	}
	resolver, err := cmd.depRegistry.GetResolver(notebookGVR)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(resolver).ToNot(BeNil())
}

func TestCompleteWithDependenciesDisabled(t *testing.T) {
	g := NewWithT(t)

	cmd := NewCommand(genericiooptions.IOStreams{})
	cmd.Dependencies = false

	err := cmd.Complete()

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(cmd.depRegistry).ToNot(BeNil())

	// Verify no resolvers registered
	notebookGVR := schema.GroupVersionResource{
		Group:    "kubeflow.org",
		Version:  "v1",
		Resource: "notebooks",
	}
	_, err = cmd.depRegistry.GetResolver(notebookGVR)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("no dependency resolver registered"))
}
