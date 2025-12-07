package kueueoperator_test

import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	"github.com/lburgazzoli/odh-cli/pkg/doctor/check"
	"github.com/lburgazzoli/odh-cli/pkg/doctor/checks/dependencies/kueueoperator"
	"github.com/lburgazzoli/odh-cli/pkg/doctor/version"
	"github.com/lburgazzoli/odh-cli/pkg/resources"
	"github.com/lburgazzoli/odh-cli/pkg/util/client"

	. "github.com/onsi/gomega"
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
	g.Expect(result).To(HaveField("Status", check.StatusPass))
	g.Expect(result.Severity).To(BeNil())
	g.Expect(result.Message).To(ContainSubstring("Not installed"))
	g.Expect(result.Details).To(HaveKeyWithValue("installed", false))
	g.Expect(result.Details).To(HaveKeyWithValue("version", "Not installed"))
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
	g.Expect(result).To(HaveField("Status", check.StatusPass))
	g.Expect(result.Severity).To(BeNil())
	g.Expect(result.Message).To(ContainSubstring("kueue-operator.v0.6.0"))
	g.Expect(result.Details).To(HaveKeyWithValue("installed", true))
	g.Expect(result.Details).To(HaveKeyWithValue("version", "kueue-operator.v0.6.0"))
}

func TestKueueOperatorCheck_Metadata(t *testing.T) {
	g := NewWithT(t)

	kueueOperatorCheck := &kueueoperator.Check{}

	g.Expect(kueueOperatorCheck.ID()).To(Equal("dependencies.kueueoperator.installed"))
	g.Expect(kueueOperatorCheck.Name()).To(Equal("Dependencies :: KueueOperator :: Installed"))
	g.Expect(kueueOperatorCheck.Category()).To(Equal(check.CategoryDependency))
	g.Expect(kueueOperatorCheck.Description()).ToNot(BeEmpty())
}
