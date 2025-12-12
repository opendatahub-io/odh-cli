package servicemeshoperator_test

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	"github.com/lburgazzoli/odh-cli/pkg/lint/check"
	"github.com/lburgazzoli/odh-cli/pkg/lint/checks/dependencies/servicemeshoperator"
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
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0]).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeCompatible),
		"Status":  Equal(metav1.ConditionTrue),
		"Reason":  Equal(check.ReasonVersionCompatible),
		"Message": And(ContainSubstring("not installed"), ContainSubstring("ready for RHOAI 3.x")),
	}))
}

func TestServiceMeshOperator2Check_InstalledBlocking(t *testing.T) {
	g := NewWithT(t)
	ctx := context.Background()

	sub := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.Subscription.APIVersion(),
			"kind":       resources.Subscription.Kind,
			"metadata": map[string]any{
				"name":      "servicemeshoperator",
				"namespace": "openshift-operators",
			},
			"spec": map[string]any{
				"channel": "stable",
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
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0]).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeCompatible),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonVersionIncompatible),
		"Message": And(ContainSubstring("installed but RHOAI 3.x requires v3")),
	}))
	g.Expect(result.Metadata.Annotations).To(HaveKeyWithValue("operator.opendatahub.io/installed-version", "servicemeshoperator.v2.5.0"))
}

func TestServiceMeshOperator2Check_Metadata(t *testing.T) {
	g := NewWithT(t)

	serviceMeshOperator2Check := &servicemeshoperator.Check{}

	g.Expect(serviceMeshOperator2Check.ID()).To(Equal("dependencies.servicemeshoperator2.upgrade"))
	g.Expect(serviceMeshOperator2Check.Name()).To(Equal("Dependencies :: ServiceMeshOperator2 :: Upgrade (3.x)"))
	g.Expect(serviceMeshOperator2Check.Group()).To(Equal(check.GroupDependency))
	g.Expect(serviceMeshOperator2Check.Description()).ToNot(BeEmpty())
}
