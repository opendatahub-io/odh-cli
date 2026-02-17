package notebook_test

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/opendatahub-io/odh-cli/pkg/lint/checks/workloads/notebook"

	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

const (
	testImage          = "quay.io/modh/jupyter-datascience:2025.2"
	testOAuthProxyName = "oauth-proxy"
	testOAuthProxyImg  = "registry.redhat.io/openshift4/ose-oauth-proxy-rhel9:v4.14"
)

func TestExtractWorkloadContainers_SingleContainer(t *testing.T) {
	g := NewWithT(t)

	nb := notebook.NewTestNotebook([]any{
		map[string]any{"name": "notebook", "image": testImage},
	})

	containers, err := notebook.ExtractWorkloadContainers(nb)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(containers).To(HaveLen(1))
	g.Expect(containers[0]).To(MatchFields(IgnoreExtras, Fields{
		"Name":  Equal("notebook"),
		"Image": Equal(testImage),
	}))
}

func TestExtractWorkloadContainers_MultipleContainers(t *testing.T) {
	g := NewWithT(t)

	nb := notebook.NewTestNotebook([]any{
		map[string]any{"name": "primary", "image": testImage},
		map[string]any{"name": "sidecar", "image": "quay.io/custom/sidecar:latest"},
	})

	containers, err := notebook.ExtractWorkloadContainers(nb)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(containers).To(HaveLen(2))
	g.Expect(containers[0]).To(HaveField("Name", "primary"))
	g.Expect(containers[1]).To(HaveField("Name", "sidecar"))
}

func TestExtractWorkloadContainers_FiltersOAuthProxy(t *testing.T) {
	g := NewWithT(t)

	nb := notebook.NewTestNotebook([]any{
		map[string]any{"name": "notebook", "image": testImage},
		map[string]any{"name": testOAuthProxyName, "image": testOAuthProxyImg},
	})

	containers, err := notebook.ExtractWorkloadContainers(nb)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(containers).To(HaveLen(1))
	g.Expect(containers[0]).To(HaveField("Name", "notebook"))
}

func TestExtractWorkloadContainers_EmptyContainersList(t *testing.T) {
	g := NewWithT(t)

	nb := notebook.NewTestNotebook([]any{})

	containers, err := notebook.ExtractWorkloadContainers(nb)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(containers).To(BeEmpty())
}

func TestExtractWorkloadContainers_NoContainersField(t *testing.T) {
	g := NewWithT(t)

	// Notebook with no .spec.template.spec.containers path.
	nb := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "kubeflow.org/v1",
			"kind":       "Notebook",
			"metadata": map[string]any{
				"name":      "test-nb",
				"namespace": "test-ns",
			},
			"spec": map[string]any{},
		},
	}

	containers, err := notebook.ExtractWorkloadContainers(nb)
	g.Expect(err).To(HaveOccurred())
	g.Expect(containers).To(BeNil())
}

func TestExtractWorkloadContainers_MissingNameAndImage(t *testing.T) {
	g := NewWithT(t)

	// Container map entries without name or image fields are still returned with zero-value strings.
	nb := notebook.NewTestNotebook([]any{
		map[string]any{"resources": map[string]any{}},
	})

	containers, err := notebook.ExtractWorkloadContainers(nb)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(containers).To(HaveLen(1))
	g.Expect(containers[0]).To(MatchFields(IgnoreExtras, Fields{
		"Name":  Equal(""),
		"Image": Equal(""),
	}))
}

func TestExtractWorkloadContainers_NonMapEntriesSkipped(t *testing.T) {
	g := NewWithT(t)

	// Non-map entries in the containers array are silently skipped.
	nb := notebook.NewTestNotebook([]any{
		"not-a-map",
		42,
		map[string]any{"name": "valid", "image": testImage},
	})

	containers, err := notebook.ExtractWorkloadContainers(nb)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(containers).To(HaveLen(1))
	g.Expect(containers[0]).To(HaveField("Name", "valid"))
}

func TestExtractWorkloadContainers_OnlyInfraContainers(t *testing.T) {
	g := NewWithT(t)

	// All containers are infrastructure sidecars — returns empty.
	nb := notebook.NewTestNotebook([]any{
		map[string]any{"name": testOAuthProxyName, "image": testOAuthProxyImg},
	})

	containers, err := notebook.ExtractWorkloadContainers(nb)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(containers).To(BeEmpty())
}

func TestIsInfrastructureContainer(t *testing.T) {
	tests := []struct {
		name      string
		container string
		image     string
		expected  bool
	}{
		{
			name:      "BothMatch",
			container: "oauth-proxy",
			image:     "registry.redhat.io/openshift4/ose-oauth-proxy-rhel9:v4.14",
			expected:  true,
		},
		{
			name:      "NameMatchesImageDoesNot",
			container: "oauth-proxy",
			image:     "quay.io/custom/auth-proxy:latest",
			expected:  false,
		},
		{
			name:      "ImageMatchesNameDoesNot",
			container: "my-auth-proxy",
			image:     "registry.redhat.io/openshift4/ose-oauth-proxy-rhel9:v4.14",
			expected:  false,
		},
		{
			name:      "NeitherMatches",
			container: "sidecar",
			image:     "quay.io/custom/sidecar:latest",
			expected:  false,
		},
		{
			name:      "EmptyStrings",
			container: "",
			image:     "",
			expected:  false,
		},
		{
			name:      "ImageContainsSubstring",
			container: "oauth-proxy",
			image:     "registry.example.com/custom/ose-oauth-proxy-rhel9-clone:v1",
			expected:  true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)
			g.Expect(notebook.IsInfrastructureContainer(tc.container, tc.image)).To(Equal(tc.expected))
		})
	}
}
