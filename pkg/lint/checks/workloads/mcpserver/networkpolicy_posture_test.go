package mcpserver_test

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	resultpkg "github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/testutil"
	"github.com/opendatahub-io/odh-cli/pkg/lint/checks/workloads/mcpserver"
	"github.com/opendatahub-io/odh-cli/pkg/resources"

	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

//nolint:gochecknoglobals // Test fixture - shared across test functions
var listKinds = map[schema.GroupVersionResource]string{
	resources.MCPServer.GVR(): resources.MCPServer.ListKind(),
}

func newMCPServer(name, namespace string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.MCPServer.APIVersion(),
			"kind":       resources.MCPServer.Kind,
			"metadata": map[string]any{
				"name":      name,
				"namespace": namespace,
			},
		},
	}
}

func TestNetworkPolicyPostureCheck_NoResources(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		CurrentVersion: "3.5.0",
		TargetVersion:  "3.6.0",
	})

	chk := mcpserver.NewNetworkPolicyPostureCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeCompatible),
		"Status":  Equal(metav1.ConditionTrue),
		"Reason":  Equal(check.ReasonRequirementsMet),
		"Message": ContainSubstring("No MCPServer resources found"),
	}))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "0"))
	g.Expect(result.ImpactedObjects).To(BeEmpty())
}

func TestNetworkPolicyPostureCheck_WithResources(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{newMCPServer("server-a", "ns1")},
		CurrentVersion: "3.5.0",
		TargetVersion:  "3.6.0",
	})

	chk := mcpserver.NewNetworkPolicyPostureCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeCompatible),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonWorkloadsImpacted),
		"Message": ContainSubstring("Found 1 MCPServer(s)"),
	}))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactAdvisory))
	g.Expect(result.Status.Conditions[0].Remediation).ToNot(BeEmpty())
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "1"))
	g.Expect(result.ImpactedObjects).To(ContainElement(MatchFields(IgnoreExtras, Fields{
		"ObjectMeta": MatchFields(IgnoreExtras, Fields{
			"Name":      Equal("server-a"),
			"Namespace": Equal("ns1"),
		}),
	})))
}

func TestNetworkPolicyPostureCheck_MultipleResources(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds: listKinds,
		Objects: []*unstructured.Unstructured{
			newMCPServer("server-a", "ns1"),
			newMCPServer("server-b", "ns2"),
		},
		CurrentVersion: "3.5.0",
		TargetVersion:  "3.6.0",
	})

	chk := mcpserver.NewNetworkPolicyPostureCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Status":  Equal(metav1.ConditionFalse),
		"Message": ContainSubstring("Found 2 MCPServer(s)"),
	}))
	g.Expect(result.Annotations).To(HaveKeyWithValue(check.AnnotationImpactedWorkloadCount, "2"))
}

func TestNetworkPolicyPostureCheck_Metadata(t *testing.T) {
	g := NewWithT(t)

	chk := mcpserver.NewNetworkPolicyPostureCheck()

	g.Expect(chk.ID()).To(Equal("workloads.mcpserver.networkpolicy-posture"))
	g.Expect(chk.Name()).To(Equal("Workloads :: MCPServer :: NetworkPolicy Posture"))
	g.Expect(chk.Group()).To(Equal(check.GroupWorkload))
	g.Expect(chk.Description()).ToNot(BeEmpty())
}

func TestNetworkPolicyPostureCheck_CanApply_NilVersions(t *testing.T) {
	g := NewWithT(t)

	chk := mcpserver.NewNetworkPolicyPostureCheck()
	canApply, err := chk.CanApply(t.Context(), check.Target{})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeFalse())
}

func TestNetworkPolicyPostureCheck_CanApply_CrossesIntoGA(t *testing.T) {
	g := NewWithT(t)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		CurrentVersion: "3.5.0",
		TargetVersion:  "3.6.0",
	})

	chk := mcpserver.NewNetworkPolicyPostureCheck()
	canApply, err := chk.CanApply(t.Context(), target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeTrue())
}

func TestNetworkPolicyPostureCheck_CanApply_AlreadyPastGA(t *testing.T) {
	g := NewWithT(t)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		CurrentVersion: "3.6.0",
		TargetVersion:  "3.7.0",
	})

	chk := mcpserver.NewNetworkPolicyPostureCheck()
	canApply, err := chk.CanApply(t.Context(), target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeFalse())
}

func TestNetworkPolicyPostureCheck_CanApply_BeforeGA(t *testing.T) {
	g := NewWithT(t)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		CurrentVersion: "3.4.0",
		TargetVersion:  "3.5.0",
	})

	chk := mcpserver.NewNetworkPolicyPostureCheck()
	canApply, err := chk.CanApply(t.Context(), target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeFalse())
}
