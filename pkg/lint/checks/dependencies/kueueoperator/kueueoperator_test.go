package kueueoperator_test

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	"github.com/lburgazzoli/odh-cli/pkg/lint/check"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/dependencies/kueueoperator"
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

func TestKueueOperatorCheck_NotInstalled(t *testing.T) {
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

	kueueOperatorCheck := &kueueoperator.Check{}
	result, err := kueueOperatorCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0]).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeAvailable),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonResourceNotFound),
		"Message": ContainSubstring("not installed"),
	}))
}

func TestKueueOperatorCheck_Installed(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	sub := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.Subscription.APIVersion(),
			"kind":       resources.Subscription.Kind,
			"metadata": map[string]any{
				"name":      "kueue-operator",
				"namespace": "kueue-system",
			},
			"status": map[string]any{
				"installedCSV": "kueue-operator.v0.6.0",
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

	kueueOperatorCheck := &kueueoperator.Check{}
	result, err := kueueOperatorCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0]).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeAvailable),
		"Status":  Equal(metav1.ConditionTrue),
		"Reason":  Equal(check.ReasonResourceFound),
		"Message": ContainSubstring("kueue-operator.v0.6.0"),
	}))
	g.Expect(result.Metadata.Annotations).To(HaveKeyWithValue("operator.opendatahub.io/installed-version", "kueue-operator.v0.6.0"))
}

func TestKueueOperatorCheck_Metadata(t *testing.T) {
	g := NewWithT(t)

	kueueOperatorCheck := &kueueoperator.Check{}

	g.Expect(kueueOperatorCheck.ID()).To(Equal("dependencies.kueueoperator.installed"))
	g.Expect(kueueOperatorCheck.Name()).To(Equal("Dependencies :: KueueOperator :: Installed"))
	g.Expect(kueueOperatorCheck.Group()).To(Equal(check.GroupDependency))
	g.Expect(kueueOperatorCheck.Description()).ToNot(BeEmpty())
}
