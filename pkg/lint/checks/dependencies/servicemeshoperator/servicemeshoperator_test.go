package servicemeshoperator_test

import (
	"testing"

	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/testutil"
	"github.com/opendatahub-io/odh-cli/pkg/lint/checks/dependencies/servicemeshoperator"
	operatorsv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	operatorfake "github.com/operator-framework/operator-lifecycle-manager/pkg/api/client/clientset/versioned/fake"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

func TestServiceMeshOperator2Check_NotInstalled(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	target := testutil.NewTarget(t, testutil.TargetConfig{
		OLM:           operatorfake.NewSimpleClientset(), //nolint:staticcheck // NewClientset requires generated apply configs not available in OLM
		TargetVersion: "3.0.0",
	})

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
	ctx := t.Context()

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

	target := testutil.NewTarget(t, testutil.TargetConfig{
		OLM:           operatorfake.NewSimpleClientset(sub), //nolint:staticcheck // NewClientset requires generated apply configs not available in OLM
		TargetVersion: "3.0.0",
	})

	serviceMeshOperator2Check := servicemeshoperator.NewCheck()
	result, err := serviceMeshOperator2Check.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeCompatible),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonVersionIncompatible),
		"Message": And(ContainSubstring("no longer required by RHOAI 3.x"), ContainSubstring("should be removed")),
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
