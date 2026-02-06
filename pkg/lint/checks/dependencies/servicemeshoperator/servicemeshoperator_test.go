package servicemeshoperator_test

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
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/dependencies/servicemeshoperator"
	"github.com/lburgazzoli/odh-cli/pkg/util/client"

	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

func TestServiceMeshOperator2Check_NotInstalled(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	scheme := runtime.NewScheme()
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, nil)
	olmClient := operatorfake.NewSimpleClientset()

	c := client.NewForTesting(client.TestClientConfig{
		Dynamic: dynamicClient,
		OLM:     olmClient,
	})

	ver := semver.MustParse("3.0.0")
	target := check.Target{
		Client:        c,
		TargetVersion: &ver,
	}

	serviceMeshOperator2Check := servicemeshoperator.NewCheck()
	result, err := serviceMeshOperator2Check.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeCompatible),
		"Status":  Equal(metav1.ConditionTrue),
		"Reason":  Equal(check.ReasonVersionCompatible),
		"Message": And(ContainSubstring("not installed"), ContainSubstring("ready for RHOAI 3.x")),
	}))
}

func TestServiceMeshOperator2Check_InstalledBlocking(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	sub := &operatorsv1alpha1.Subscription{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "servicemeshoperator",
			Namespace: "openshift-operators",
		},
		Spec: &operatorsv1alpha1.SubscriptionSpec{
			Channel: "stable",
		},
		Status: operatorsv1alpha1.SubscriptionStatus{
			InstalledCSV: "servicemeshoperator.v2.5.0",
		},
	}

	scheme := runtime.NewScheme()
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, nil)
	olmClient := operatorfake.NewSimpleClientset(sub)

	c := client.NewForTesting(client.TestClientConfig{
		Dynamic: dynamicClient,
		OLM:     olmClient,
	})

	ver := semver.MustParse("3.0.0")
	target := check.Target{
		Client:        c,
		TargetVersion: &ver,
	}

	serviceMeshOperator2Check := servicemeshoperator.NewCheck()
	result, err := serviceMeshOperator2Check.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeCompatible),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonVersionIncompatible),
		"Message": And(ContainSubstring("installed but RHOAI 3.x requires v3")),
	}))
	g.Expect(result.Annotations).To(HaveKeyWithValue("operator.opendatahub.io/installed-version", "servicemeshoperator.v2.5.0"))
}

func TestServiceMeshOperator2Check_Metadata(t *testing.T) {
	g := NewWithT(t)

	serviceMeshOperator2Check := servicemeshoperator.NewCheck()

	g.Expect(serviceMeshOperator2Check.ID()).To(Equal("dependencies.servicemeshoperator2.upgrade"))
	g.Expect(serviceMeshOperator2Check.Name()).To(Equal("Dependencies :: ServiceMeshOperator2 :: Upgrade (3.x)"))
	g.Expect(serviceMeshOperator2Check.Group()).To(Equal(check.GroupDependency))
	g.Expect(serviceMeshOperator2Check.Description()).ToNot(BeEmpty())
}
