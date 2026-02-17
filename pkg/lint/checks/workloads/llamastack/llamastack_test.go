package llamastack_test

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/testutil"
	"github.com/opendatahub-io/odh-cli/pkg/lint/checks/workloads/llamastack"
	"github.com/opendatahub-io/odh-cli/pkg/resources"

	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

//nolint:gochecknoglobals
var listKinds = map[schema.GroupVersionResource]string{
	resources.LlamaStackDistribution.GVR(): resources.LlamaStackDistribution.ListKind(),
	resources.DataScienceCluster.GVR():     resources.DataScienceCluster.ListKind(),
}

func newDSC(componentStates map[string]string) *unstructured.Unstructured {
	components := make(map[string]any, len(componentStates))
	for name, state := range componentStates {
		components[name] = map[string]any{
			"managementState": state,
		}
	}

	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.DataScienceCluster.APIVersion(),
			"kind":       resources.DataScienceCluster.Kind,
			"metadata": map[string]any{
				"name": "default-dsc",
			},
			"spec": map[string]any{
				"components": components,
			},
		},
	}
}

func newLLSD(name, namespace string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.LlamaStackDistribution.APIVersion(),
			"kind":       resources.LlamaStackDistribution.Kind,
			"metadata": map[string]any{
				"name":      name,
				"namespace": namespace,
			},
			"spec": map[string]any{
				"server": map[string]any{},
			},
		},
	}
}

func TestLlamaStackConfigCheck_CanApply(t *testing.T) {
	g := NewWithT(t)

	testCases := []struct {
		name           string
		currentVersion string
		targetVersion  string
		componentState string
		expected       bool
	}{
		{
			name:           "2.25 to 3.3 upgrade with component Managed",
			currentVersion: "2.25.2",
			targetVersion:  "3.3.0",
			componentState: "Managed",
			expected:       true,
		},
		{
			name:           "2.17 to 3.0 upgrade with component Managed",
			currentVersion: "2.17.0",
			targetVersion:  "3.0.0",
			componentState: "Managed",
			expected:       true,
		},
		{
			name:           "2.25 to 3.3 upgrade with component Removed",
			currentVersion: "2.25.2",
			targetVersion:  "3.3.0",
			componentState: "Removed",
			expected:       false,
		},
		{
			name:           "2.25 to 3.3 upgrade with component Unmanaged",
			currentVersion: "2.25.2",
			targetVersion:  "3.3.0",
			componentState: "Unmanaged",
			expected:       false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			dsc := newDSC(map[string]string{"llamastackoperator": tc.componentState})
			target := testutil.NewTarget(t, testutil.TargetConfig{
				ListKinds:      listKinds,
				Objects:        []*unstructured.Unstructured{dsc},
				CurrentVersion: tc.currentVersion,
				TargetVersion:  tc.targetVersion,
			})

			chk := llamastack.NewConfigCheck()
			canApply, err := chk.CanApply(t.Context(), target)

			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(canApply).To(Equal(tc.expected))
		})
	}
}

func TestLlamaStackConfigCheck_NoWorkloads(t *testing.T) {
	g := NewWithT(t)

	dsc := newDSC(map[string]string{"llamastackoperator": "Managed"})
	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{dsc},
		CurrentVersion: "2.25.2",
		TargetVersion:  "3.3.0",
	})

	chk := llamastack.NewConfigCheck()
	res, err := chk.Validate(t.Context(), target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(res.Status.Conditions).To(HaveLen(1))
	g.Expect(res.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal("RequiresRecreation"),
		"Status": Equal(metav1.ConditionTrue),
		"Reason": Equal(check.ReasonResourceNotFound),
	}))
	g.Expect(res.Status.Conditions[0].Condition.Message).To(ContainSubstring("No LlamaStackDistribution resources found"))
	g.Expect(res.Status.Conditions[0].Impact).To(Equal(result.ImpactNone))
	g.Expect(res.ImpactedObjects).To(BeEmpty())
}

func TestLlamaStackConfigCheck_WorkloadsFound(t *testing.T) {
	g := NewWithT(t)

	dsc := newDSC(map[string]string{"llamastackoperator": "Managed"})
	llsd1 := newLLSD("llsd-1", "test-ns")
	llsd2 := newLLSD("llsd-2", "test-ns")

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{dsc, llsd1, llsd2},
		CurrentVersion: "2.25.2",
		TargetVersion:  "3.3.0",
	})

	chk := llamastack.NewConfigCheck()
	res, err := chk.Validate(t.Context(), target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(res.Status.Conditions).To(HaveLen(1))

	// Verify condition
	g.Expect(res.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal("RequiresRecreation"),
		"Status": Equal(metav1.ConditionFalse),
		"Reason": Equal("ArchitecturalIncompatibility"),
	}))
	g.Expect(res.Status.Conditions[0].Condition.Message).To(ContainSubstring("Found 2 LlamaStackDistribution(s)"))
	g.Expect(res.Status.Conditions[0].Condition.Message).To(ContainSubstring("must be deleted and recreated"))
	g.Expect(res.Status.Conditions[0].Condition.Message).To(ContainSubstring("ALL DATA WILL BE LOST"))
	g.Expect(res.Status.Conditions[0].Impact).To(Equal(result.ImpactBlocking))
	g.Expect(res.Status.Conditions[0].Remediation).To(ContainSubstring("backup script"))

	// Verify impacted objects
	g.Expect(res.ImpactedObjects).To(HaveLen(2))
	g.Expect(res.ImpactedObjects[0].Name).To(Equal("llsd-1"))
	g.Expect(res.ImpactedObjects[0].Namespace).To(Equal("test-ns"))
	g.Expect(res.ImpactedObjects[0].Annotations).To(HaveKeyWithValue("upgrade.action", "requires-recreation"))
	g.Expect(res.ImpactedObjects[1].Name).To(Equal("llsd-2"))
	g.Expect(res.ImpactedObjects[1].Namespace).To(Equal("test-ns"))
}

func TestLlamaStackConfigCheck_Metadata(t *testing.T) {
	g := NewWithT(t)

	chk := llamastack.NewConfigCheck()

	g.Expect(chk.ID()).To(Equal("workloads.llamastack.config"))
	g.Expect(chk.Name()).To(Equal("Workloads :: LlamaStack :: Upgrade Preparation (2.x to 3.3)"))
	g.Expect(chk.Group()).To(Equal(check.GroupWorkload))
	g.Expect(chk.Description()).ToNot(BeEmpty())
	g.Expect(chk.CheckKind()).To(Equal("llamastackdistribution"))
	g.Expect(chk.CheckType()).To(Equal("config"))
}
