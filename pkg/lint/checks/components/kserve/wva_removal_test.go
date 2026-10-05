package kserve_test

import (
	"testing"

	"github.com/onsi/gomega"
	"github.com/onsi/gomega/gstruct"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	resultpkg "github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/testutil"
	"github.com/opendatahub-io/odh-cli/pkg/lint/checks/components/kserve"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
)

func wvaListKinds() map[schema.GroupVersionResource]string {
	return map[schema.GroupVersionResource]string{
		resources.DataScienceCluster.GVR():       resources.DataScienceCluster.ListKind(),
		resources.DSCInitialization.GVR():        resources.DSCInitialization.ListKind(),
		resources.Deployment.GVR():               resources.Deployment.ListKind(),
		resources.ConfigMap.GVR():                resources.ConfigMap.ListKind(),
		resources.CustomResourceDefinition.GVR(): resources.CustomResourceDefinition.ListKind(),
		variantAutoscalingGVR():                  "VariantAutoscalingList",
	}
}

func variantAutoscalingGVR() schema.GroupVersionResource {
	return schema.GroupVersionResource{Group: "llmd.ai", Version: "v1alpha1", Resource: "variantautoscalings"}
}

func wvaDSC(wvaState string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.DataScienceCluster.APIVersion(),
			"kind":       resources.DataScienceCluster.Kind,
			"metadata": map[string]any{
				"name": "default-dsc",
			},
			"spec": map[string]any{
				"components": map[string]any{
					"kserve": map[string]any{
						"managementState": "Managed",
						"wva": map[string]any{
							"managementState": wvaState,
						},
					},
				},
			},
		},
	}
}

func wvaTarget(t *testing.T, current, target string, objects ...*unstructured.Unstructured) check.Target {
	t.Helper()
	all := []*unstructured.Unstructured{testutil.NewDSCI("opendatahub")}
	all = append(all, objects...)

	return testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      wvaListKinds(),
		Objects:        all,
		CurrentVersion: current,
		TargetVersion:  target,
	})
}

func TestWVARemovalCheck_CanApply(t *testing.T) {
	g := gomega.NewWithT(t)
	chk := kserve.NewWVARemovalCheck()

	cases := []struct {
		name    string
		current string
		target  string
		want    bool
	}{
		{name: "into 3.6", current: "3.5.0", target: "3.6.0", want: true},
		{name: "already 3.6", current: "3.6.0", target: "3.6.1", want: false},
		{name: "older target", current: "3.4.0", target: "3.5.0", want: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			target := wvaTarget(t, tc.current, tc.target, wvaDSC("Removed"))
			got, err := chk.CanApply(t.Context(), target)
			g.Expect(err).NotTo(gomega.HaveOccurred())
			g.Expect(got).To(gomega.Equal(tc.want))
		})
	}
}

func TestWVARemovalCheck_Clean(t *testing.T) {
	g := gomega.NewWithT(t)
	target := wvaTarget(t, "3.5.0", "3.6.0", wvaDSC("Removed"))

	got, err := kserve.NewWVARemovalCheck().Validate(t.Context(), target)
	g.Expect(err).NotTo(gomega.HaveOccurred())
	g.Expect(got.Status.Conditions).To(gomega.HaveLen(1))
	g.Expect(got.Status.Conditions[0].Condition).To(gstruct.MatchFields(gstruct.IgnoreExtras, gstruct.Fields{
		"Type":   gomega.Equal(check.ConditionTypeCompatible),
		"Status": gomega.Equal(metav1.ConditionTrue),
		"Reason": gomega.Equal(check.ReasonVersionCompatible),
	}))
}

func TestWVARemovalCheck_ManagedBlocks(t *testing.T) {
	g := gomega.NewWithT(t)
	target := wvaTarget(t, "3.5.0", "3.6.0", wvaDSC("Managed"))

	got, err := kserve.NewWVARemovalCheck().Validate(t.Context(), target)
	g.Expect(err).NotTo(gomega.HaveOccurred())
	g.Expect(got.Status.Conditions[0].Status).To(gomega.Equal(metav1.ConditionFalse))
	g.Expect(got.Status.Conditions[0].Impact).To(gomega.Equal(resultpkg.ImpactBlocking))
	g.Expect(got.Status.Conditions[0].Message).To(gomega.ContainSubstring("managementState is Managed"))
}

func TestWVARemovalCheck_DeploymentBlocks(t *testing.T) {
	g := gomega.NewWithT(t)
	deployment := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata": map[string]any{
				"name":      "workload-variant-autoscaler-controller-manager",
				"namespace": "opendatahub",
			},
		},
	}
	target := wvaTarget(t, "3.5.0", "3.6.0", wvaDSC("Removed"), deployment)

	got, err := kserve.NewWVARemovalCheck().Validate(t.Context(), target)
	g.Expect(err).NotTo(gomega.HaveOccurred())
	g.Expect(got.Status.Conditions[0].Impact).To(gomega.Equal(resultpkg.ImpactBlocking))
	g.Expect(got.Status.Conditions[0].Message).To(gomega.ContainSubstring("workload-variant-autoscaler-controller-manager"))
}

func TestWVARemovalCheck_CRDAloneIsAdvisory(t *testing.T) {
	g := gomega.NewWithT(t)
	crd := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "apiextensions.k8s.io/v1",
			"kind":       "CustomResourceDefinition",
			"metadata": map[string]any{
				"name": "variantautoscalings.llmd.ai",
			},
		},
	}
	target := wvaTarget(t, "3.5.0", "3.6.0", wvaDSC("Removed"), crd)

	got, err := kserve.NewWVARemovalCheck().Validate(t.Context(), target)
	g.Expect(err).NotTo(gomega.HaveOccurred())
	g.Expect(got.Status.Conditions[0].Impact).To(gomega.Equal(resultpkg.ImpactAdvisory))
	g.Expect(got.Status.Conditions[0].Message).To(gomega.ContainSubstring("variantautoscalings.llmd.ai"))
}
