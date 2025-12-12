package certmanager_test

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	"github.com/lburgazzoli/odh-cli/pkg/lint/check"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/dependencies/certmanager"
	"github.com/lburgazzoli/odh-cli/pkg/lint/version"
	"github.com/lburgazzoli/odh-cli/pkg/resources"
	"github.com/lburgazzoli/odh-cli/pkg/util/client"

	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
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
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0]).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeAvailable),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonResourceNotFound),
		"Message": ContainSubstring("not installed"),
	}))
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
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0]).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeAvailable),
		"Status":  Equal(metav1.ConditionTrue),
		"Reason":  Equal(check.ReasonResourceFound),
		"Message": ContainSubstring("cert-manager.v1.13.0"),
	}))
	g.Expect(result.Metadata.Annotations).To(HaveKeyWithValue("operator.opendatahub.io/installed-version", "cert-manager.v1.13.0"))
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
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0]).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeAvailable),
		"Status":  Equal(metav1.ConditionTrue),
		"Reason":  Equal(check.ReasonResourceFound),
		"Message": ContainSubstring("cert-manager-operator.v1.12.0"),
	}))
	g.Expect(result.Metadata.Annotations).To(HaveKeyWithValue("operator.opendatahub.io/installed-version", "cert-manager-operator.v1.12.0"))
}

func TestCertManagerCheck_Metadata(t *testing.T) {
	g := NewWithT(t)

	certManagerCheck := &certmanager.Check{}

	g.Expect(certManagerCheck.ID()).To(Equal("dependencies.certmanager.installed"))
	g.Expect(certManagerCheck.Name()).To(Equal("Dependencies :: CertManager :: Installed"))
	g.Expect(certManagerCheck.Group()).To(Equal(check.GroupDependency))
	g.Expect(certManagerCheck.Description()).ToNot(BeEmpty())
}
