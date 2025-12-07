package certmanager_test

import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	"github.com/lburgazzoli/odh-cli/pkg/doctor/check"
	"github.com/lburgazzoli/odh-cli/pkg/doctor/checks/dependencies/certmanager"
	"github.com/lburgazzoli/odh-cli/pkg/doctor/version"
	"github.com/lburgazzoli/odh-cli/pkg/resources"
	"github.com/lburgazzoli/odh-cli/pkg/util/client"

	. "github.com/onsi/gomega"
)

//nolint:gochecknoglobals
var listKinds = map[schema.GroupVersionResource]string{
	resources.Subscription.GVR(): resources.Subscription.ListKind(),
}

func TestCertManagerCheck_NotInstalled(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	scheme := runtime.NewScheme()
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, listKinds)

	c := &client.Client{
		Dynamic: dynamicClient,
	}

	target := &check.CheckTarget{
		Client: c,
		Version: &version.ClusterVersion{
			Version: "2.17.0",
		},
	}

	certManagerCheck := &certmanager.Check{}
	result, err := certManagerCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).To(HaveField("Status", check.StatusPass))
	g.Expect(result.Severity).To(BeNil())
	g.Expect(result.Message).To(ContainSubstring("Not installed"))
	g.Expect(result.Details).To(HaveKeyWithValue("installed", false))
	g.Expect(result.Details).To(HaveKeyWithValue("version", "Not installed"))
}

func TestCertManagerCheck_InstalledCertManager(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	sub := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.Subscription.APIVersion(),
			"kind":       resources.Subscription.Kind,
			"metadata": map[string]any{
				"name":      "cert-manager",
				"namespace": "cert-manager",
			},
			"status": map[string]any{
				"installedCSV": "cert-manager.v1.13.0",
			},
		},
	}

	scheme := runtime.NewScheme()
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, listKinds, sub)

	c := &client.Client{
		Dynamic: dynamicClient,
	}

	target := &check.CheckTarget{
		Client: c,
		Version: &version.ClusterVersion{
			Version: "2.17.0",
		},
	}

	certManagerCheck := &certmanager.Check{}
	result, err := certManagerCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).To(HaveField("Status", check.StatusPass))
	g.Expect(result.Severity).To(BeNil())
	g.Expect(result.Message).To(ContainSubstring("cert-manager.v1.13.0"))
	g.Expect(result.Details).To(HaveKeyWithValue("installed", true))
	g.Expect(result.Details).To(HaveKeyWithValue("version", "cert-manager.v1.13.0"))
}

func TestCertManagerCheck_InstalledOpenShiftCertManager(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	sub := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.Subscription.APIVersion(),
			"kind":       resources.Subscription.Kind,
			"metadata": map[string]any{
				"name":      "openshift-cert-manager-operator",
				"namespace": "cert-manager-operator",
			},
			"status": map[string]any{
				"installedCSV": "cert-manager-operator.v1.12.0",
			},
		},
	}

	scheme := runtime.NewScheme()
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, listKinds, sub)

	c := &client.Client{
		Dynamic: dynamicClient,
	}

	target := &check.CheckTarget{
		Client: c,
		Version: &version.ClusterVersion{
			Version: "2.17.0",
		},
	}

	certManagerCheck := &certmanager.Check{}
	result, err := certManagerCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).To(HaveField("Status", check.StatusPass))
	g.Expect(result.Severity).To(BeNil())
	g.Expect(result.Message).To(ContainSubstring("cert-manager-operator.v1.12.0"))
	g.Expect(result.Details).To(HaveKeyWithValue("installed", true))
	g.Expect(result.Details).To(HaveKeyWithValue("version", "cert-manager-operator.v1.12.0"))
}

func TestCertManagerCheck_Metadata(t *testing.T) {
	g := NewWithT(t)

	certManagerCheck := &certmanager.Check{}

	g.Expect(certManagerCheck.ID()).To(Equal("dependencies.certmanager.installed"))
	g.Expect(certManagerCheck.Name()).To(Equal("Dependencies :: CertManager :: Installed"))
	g.Expect(certManagerCheck.Category()).To(Equal(check.CategoryDependency))
	g.Expect(certManagerCheck.Description()).ToNot(BeEmpty())
}
