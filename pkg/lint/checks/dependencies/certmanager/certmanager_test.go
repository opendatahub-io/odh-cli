package certmanager_test

import (
	"context"
	"testing"

	"github.com/blang/semver/v4"
	operatorsv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	operatorfake "github.com/operator-framework/operator-lifecycle-manager/pkg/api/client/clientset/versioned/fake"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	"github.com/lburgazzoli/odh-cli/pkg/lint/check"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/dependencies/certmanager"
	"github.com/lburgazzoli/odh-cli/pkg/util/client"

	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

func TestCertManagerCheck_NotInstalled(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	scheme := runtime.NewScheme()
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, nil)
	olmClient := operatorfake.NewSimpleClientset() //nolint:staticcheck // NewClientset requires generated apply configs not available in OLM

	c := client.NewForTesting(client.TestClientConfig{
		Dynamic: dynamicClient,
		OLM:     olmClient,
	})

	ver := semver.MustParse("2.17.0")
	target := check.Target{
		Client:        c,
		TargetVersion: &ver,
	}

	certManagerCheck := certmanager.NewCheck()
	result, err := certManagerCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeAvailable),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonResourceNotFound),
		"Message": ContainSubstring("not installed"),
	}))
}

func TestCertManagerCheck_InstalledCertManager(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	sub := &operatorsv1alpha1.Subscription{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cert-manager",
			Namespace: "cert-manager",
		},
		Status: operatorsv1alpha1.SubscriptionStatus{
			InstalledCSV: "cert-manager.v1.13.0",
		},
	}

	scheme := runtime.NewScheme()
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, nil)
	olmClient := operatorfake.NewSimpleClientset(sub) //nolint:staticcheck // NewClientset requires generated apply configs not available in OLM

	c := client.NewForTesting(client.TestClientConfig{
		Dynamic: dynamicClient,
		OLM:     olmClient,
	})

	ver := semver.MustParse("2.17.0")
	target := check.Target{
		Client:        c,
		TargetVersion: &ver,
	}

	certManagerCheck := certmanager.NewCheck()
	result, err := certManagerCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeAvailable),
		"Status":  Equal(metav1.ConditionTrue),
		"Reason":  Equal(check.ReasonResourceFound),
		"Message": ContainSubstring("cert-manager.v1.13.0"),
	}))
	g.Expect(result.Annotations).To(HaveKeyWithValue("operator.opendatahub.io/installed-version", "cert-manager.v1.13.0"))
}

func TestCertManagerCheck_InstalledOpenShiftCertManager(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	sub := &operatorsv1alpha1.Subscription{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "openshift-cert-manager-operator",
			Namespace: "cert-manager-operator",
		},
		Status: operatorsv1alpha1.SubscriptionStatus{
			InstalledCSV: "cert-manager-operator.v1.12.0",
		},
	}

	scheme := runtime.NewScheme()
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, nil)
	olmClient := operatorfake.NewSimpleClientset(sub) //nolint:staticcheck // NewClientset requires generated apply configs not available in OLM

	c := client.NewForTesting(client.TestClientConfig{
		Dynamic: dynamicClient,
		OLM:     olmClient,
	})

	ver := semver.MustParse("2.17.0")
	target := check.Target{
		Client:        c,
		TargetVersion: &ver,
	}

	certManagerCheck := certmanager.NewCheck()
	result, err := certManagerCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeAvailable),
		"Status":  Equal(metav1.ConditionTrue),
		"Reason":  Equal(check.ReasonResourceFound),
		"Message": ContainSubstring("cert-manager-operator.v1.12.0"),
	}))
	g.Expect(result.Annotations).To(HaveKeyWithValue("operator.opendatahub.io/installed-version", "cert-manager-operator.v1.12.0"))
}

func TestCertManagerCheck_Metadata(t *testing.T) {
	g := NewWithT(t)

	certManagerCheck := certmanager.NewCheck()

	g.Expect(certManagerCheck.ID()).To(Equal("dependencies.certmanager.installed"))
	g.Expect(certManagerCheck.Name()).To(Equal("Dependencies :: CertManager :: Installed"))
	g.Expect(certManagerCheck.Group()).To(Equal(check.GroupDependency))
	g.Expect(certManagerCheck.Description()).ToNot(BeEmpty())
}
