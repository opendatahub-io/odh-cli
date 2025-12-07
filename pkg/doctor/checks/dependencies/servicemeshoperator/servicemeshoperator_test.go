package servicemeshoperator_test

import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	"github.com/lburgazzoli/odh-cli/pkg/doctor/check"
	"github.com/lburgazzoli/odh-cli/pkg/doctor/checks/dependencies/servicemeshoperator"
	"github.com/lburgazzoli/odh-cli/pkg/doctor/version"
	"github.com/lburgazzoli/odh-cli/pkg/resources"
	"github.com/lburgazzoli/odh-cli/pkg/util/client"

	. "github.com/onsi/gomega"
)

//nolint:gochecknoglobals
var listKinds = map[schema.GroupVersionResource]string{
	resources.Subscription.GVR(): resources.Subscription.ListKind(),
}

func TestServiceMeshOperator2Check_NotInstalled(t *testing.T) {
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
			Version: "3.0.0",
		},
	}

	serviceMeshOperator2Check := &servicemeshoperator.Check{}
	result, err := serviceMeshOperator2Check.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).To(HaveField("Status", check.StatusPass))
	g.Expect(result.Severity).To(BeNil())
	g.Expect(result.Message).To(ContainSubstring("Not installed"))
	g.Expect(result.Message).To(ContainSubstring("ready for RHOAI 3.x"))
	g.Expect(result.Details).To(HaveKeyWithValue("installed", false))
	g.Expect(result.Details).To(HaveKeyWithValue("version", "Not installed"))
}

func TestServiceMeshOperator2Check_InstalledBlocking(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	sub := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.Subscription.APIVersion(),
			"kind":       resources.Subscription.Kind,
			"metadata": map[string]any{
				"name":      "servicemeshoperator2",
				"namespace": "openshift-operators",
			},
			"status": map[string]any{
				"installedCSV": "servicemeshoperator.v2.5.0",
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
			Version: "3.0.0",
		},
	}

	serviceMeshOperator2Check := &servicemeshoperator.Check{}
	result, err := serviceMeshOperator2Check.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).To(HaveField("Status", check.StatusFail))
	g.Expect(result.Severity).ToNot(BeNil())
	g.Expect(*result.Severity).To(Equal(check.SeverityCritical))
	g.Expect(result.Message).To(ContainSubstring("not supported in RHOAI 3.x"))
	g.Expect(result.Message).To(ContainSubstring("servicemeshoperator3"))
	g.Expect(result.Details).To(HaveKeyWithValue("installed", true))
	g.Expect(result.Details).To(HaveKeyWithValue("version", "servicemeshoperator.v2.5.0"))
	g.Expect(result.Details).To(HaveKeyWithValue("targetVersion", "3.0.0"))
}

func TestServiceMeshOperator2Check_Metadata(t *testing.T) {
	g := NewWithT(t)

	serviceMeshOperator2Check := &servicemeshoperator.Check{}

	g.Expect(serviceMeshOperator2Check.ID()).To(Equal("dependencies.servicemeshoperator2.upgrade"))
	g.Expect(serviceMeshOperator2Check.Name()).To(Equal("Dependencies :: ServiceMeshOperator2 :: Upgrade (3.x)"))
	g.Expect(serviceMeshOperator2Check.Category()).To(Equal(check.CategoryDependency))
	g.Expect(serviceMeshOperator2Check.Description()).ToNot(BeEmpty())
}
