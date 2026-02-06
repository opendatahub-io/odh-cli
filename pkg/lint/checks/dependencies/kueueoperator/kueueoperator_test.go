package kueueoperator_test

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
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/dependencies/kueueoperator"
	"github.com/lburgazzoli/odh-cli/pkg/util/client"

	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

func TestKueueOperatorCheck_NotInstalled(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	scheme := runtime.NewScheme()
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, nil)
	olmClient := operatorfake.NewSimpleClientset()

	c := client.NewForTesting(client.TestClientConfig{
		Dynamic: dynamicClient,
		OLM:     olmClient,
	})

	ver := semver.MustParse("2.17.0")
	target := check.Target{
		Client:        c,
		TargetVersion: &ver,
	}

	kueueOperatorCheck := kueueoperator.NewCheck()
	result, err := kueueOperatorCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeAvailable),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonResourceNotFound),
		"Message": ContainSubstring("not installed"),
	}))
}

func TestKueueOperatorCheck_Installed(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	sub := &operatorsv1alpha1.Subscription{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kueue-operator",
			Namespace: "kueue-system",
		},
		Status: operatorsv1alpha1.SubscriptionStatus{
			InstalledCSV: "kueue-operator.v0.6.0",
		},
	}

	scheme := runtime.NewScheme()
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, nil)
	olmClient := operatorfake.NewSimpleClientset(sub)

	c := client.NewForTesting(client.TestClientConfig{
		Dynamic: dynamicClient,
		OLM:     olmClient,
	})

	ver := semver.MustParse("2.17.0")
	target := check.Target{
		Client:        c,
		TargetVersion: &ver,
	}

	kueueOperatorCheck := kueueoperator.NewCheck()
	result, err := kueueOperatorCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeAvailable),
		"Status":  Equal(metav1.ConditionTrue),
		"Reason":  Equal(check.ReasonResourceFound),
		"Message": ContainSubstring("kueue-operator.v0.6.0"),
	}))
	g.Expect(result.Annotations).To(HaveKeyWithValue("operator.opendatahub.io/installed-version", "kueue-operator.v0.6.0"))
}

func TestKueueOperatorCheck_Metadata(t *testing.T) {
	g := NewWithT(t)

	kueueOperatorCheck := kueueoperator.NewCheck()

	g.Expect(kueueOperatorCheck.ID()).To(Equal("dependencies.kueueoperator.installed"))
	g.Expect(kueueOperatorCheck.Name()).To(Equal("Dependencies :: KueueOperator :: Installed"))
	g.Expect(kueueOperatorCheck.Group()).To(Equal(check.GroupDependency))
	g.Expect(kueueOperatorCheck.Description()).ToNot(BeEmpty())
}
