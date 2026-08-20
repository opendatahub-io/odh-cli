package dashboard_test

import (
	"testing"

	"github.com/blang/semver/v4"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/opendatahub-io/odh-cli/pkg/constants"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/testutil"
	"github.com/opendatahub-io/odh-cli/pkg/lint/checks/components/dashboard"
	"github.com/opendatahub-io/odh-cli/pkg/resources"

	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

//nolint:gochecknoglobals // Test fixture - shared across test functions
var resourceCapacityListKinds = map[schema.GroupVersionResource]string{
	resources.DataScienceCluster.GVR():   resources.DataScienceCluster.ListKind(),
	resources.DataScienceClusterV1.GVR(): resources.DataScienceClusterV1.ListKind(),
	resources.DSCInitialization.GVR():    resources.DSCInitialization.ListKind(),
	resources.DSCInitializationV1.GVR():  resources.DSCInitializationV1.ListKind(),
	resources.ClusterAutoscaler.GVR():    resources.ClusterAutoscaler.ListKind(),
	resources.Node.GVR():                 resources.Node.ListKind(),
	resources.Deployment.GVR():           resources.Deployment.ListKind(),
	resources.Pod.GVR():                  resources.Pod.ListKind(),
}

func newNode(name, cpuAllocatable, memAllocatable string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "Node",
		"metadata":   map[string]any{"name": name},
		"status": map[string]any{
			"allocatable": map[string]any{
				"cpu":    cpuAllocatable,
				"memory": memAllocatable,
			},
		},
	}}
}

func newClusterAutoscaler() *unstructured.Unstructured {
	ca := &unstructured.Unstructured{}
	ca.SetGroupVersionKind(resources.ClusterAutoscaler.GVK())
	ca.SetName("default")

	return ca
}

type containerSpec struct {
	name      string
	cpuReq    string
	memoryReq string
}

func newDashboardDeployment(namespace string, containers []containerSpec, replicas int64, maxUnavailable string) *unstructured.Unstructured {
	ctrs := make([]any, len(containers))
	for i, c := range containers {
		ctrs[i] = map[string]any{
			"name": c.name,
			"resources": map[string]any{
				"requests": map[string]any{
					"cpu":    c.cpuReq,
					"memory": c.memoryReq,
				},
			},
		}
	}

	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"metadata": map[string]any{
			"name":      "rhods-dashboard",
			"namespace": namespace,
		},
		"spec": map[string]any{
			"replicas": replicas,
			"strategy": map[string]any{
				"type": "RollingUpdate",
				"rollingUpdate": map[string]any{
					"maxUnavailable": maxUnavailable,
				},
			},
			"template": map[string]any{
				"spec": map[string]any{
					"containers": ctrs,
				},
			},
		},
	}}
}

func newDashboardPodWithResources(name, namespace string, containers []containerSpec) *unstructured.Unstructured {
	ctrs := make([]any, len(containers))
	for i, c := range containers {
		ctrs[i] = map[string]any{
			"name": c.name,
			"resources": map[string]any{
				"requests": map[string]any{
					"cpu":    c.cpuReq,
					"memory": c.memoryReq,
				},
			},
		}
	}

	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "Pod",
		"metadata": map[string]any{
			"name":      name,
			"namespace": namespace,
			"labels":    map[string]any{"app": "rhods-dashboard"},
		},
		"spec": map[string]any{"containers": ctrs},
	}}
}

func TestResourceCapacityCheck_Metadata(t *testing.T) {
	g := NewWithT(t)

	chk := dashboard.NewResourceCapacityCheck()

	t.Run("should have correct ID", func(_ *testing.T) {
		g.Expect(chk.ID()).To(Equal("components.dashboard.resource-capacity"))
	})

	t.Run("should have correct Name", func(_ *testing.T) {
		g.Expect(chk.Name()).To(Equal("Components :: Dashboard :: Resource Capacity (3.x)"))
	})

	t.Run("should have correct Group", func(_ *testing.T) {
		g.Expect(chk.Group()).To(Equal(check.GroupComponent))
	})
}

func TestResourceCapacityCheck_CanApply(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	chk := dashboard.NewResourceCapacityCheck()

	t.Run("should apply when upgrading from 2.x to 3.x", func(_ *testing.T) {
		targetVer := semver.MustParse("3.0.0")
		currentVer := semver.MustParse("2.17.0")

		target := check.Target{
			CurrentVersion: &currentVer,
			TargetVersion:  &targetVer,
		}

		canApply, err := chk.CanApply(ctx, target)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(canApply).To(BeTrue())
	})

	t.Run("should not apply when upgrading from 3.x to 3.x", func(_ *testing.T) {
		targetVer := semver.MustParse("3.3.0")
		currentVer := semver.MustParse("3.0.0")

		target := check.Target{
			CurrentVersion: &currentVer,
			TargetVersion:  &targetVer,
		}

		canApply, err := chk.CanApply(ctx, target)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(canApply).To(BeFalse())
	})
}

func TestResourceCapacityCheck_BlockingCPUInsufficient(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	ns := "redhat-ods-applications"
	dsc := testutil.NewDSC(map[string]string{"dashboard": "Managed"})
	dsci := testutil.NewDSCI(ns)

	deploy := newDashboardDeployment(ns, []containerSpec{
		{name: "dashboard", cpuReq: "2000m", memoryReq: "1Gi"},
		{name: "kube-rbac-proxy", cpuReq: "1000m", memoryReq: "1Gi"},
		{name: "model-registry-ui", cpuReq: "700m", memoryReq: "512Mi"},
	}, 2, "25%")

	node1 := newNode("node-1", "3500m", "16Gi")
	node2 := newNode("node-2", "3500m", "16Gi")

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      resourceCapacityListKinds,
		Objects:        []*unstructured.Unstructured{dsc, dsci, deploy, node1, node2},
		CurrentVersion: "2.25.0",
		TargetVersion:  "3.5.0",
	})

	chk := dashboard.NewResourceCapacityCheck()
	dr, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(dr).ToNot(BeNil())
	g.Expect(dr).To(PointTo(MatchFields(IgnoreExtras, Fields{
		"Group": Equal(string(check.GroupComponent)),
		"Kind":  Equal(constants.ComponentDashboard),
	})))

	capacityCond := findCondition(dr, "ResourceCapacity")
	g.Expect(capacityCond).ToNot(BeNil())
	g.Expect(capacityCond.Status).To(Equal(metav1.ConditionFalse))
	g.Expect(capacityCond.Reason).To(Equal(check.ReasonInsufficientCapacity))
	g.Expect(capacityCond.Impact).To(Equal(result.ImpactBlocking))
	g.Expect(capacityCond.Message).To(ContainSubstring("3700m"))
	g.Expect(capacityCond.Message).To(ContainSubstring("3500m"))
}

func TestResourceCapacityCheck_BlockingMemoryInsufficient(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	ns := "redhat-ods-applications"
	dsc := testutil.NewDSC(map[string]string{"dashboard": "Managed"})
	dsci := testutil.NewDSCI(ns)

	deploy := newDashboardDeployment(ns, []containerSpec{
		{name: "dashboard", cpuReq: "500m", memoryReq: "4Gi"},
		{name: "proxy", cpuReq: "500m", memoryReq: "4Gi"},
	}, 2, "1")

	node := newNode("node-1", "8000m", "4Gi")

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      resourceCapacityListKinds,
		Objects:        []*unstructured.Unstructured{dsc, dsci, deploy, node},
		CurrentVersion: "2.25.0",
		TargetVersion:  "3.5.0",
	})

	chk := dashboard.NewResourceCapacityCheck()
	dr, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())

	capacityCond := findCondition(dr, "ResourceCapacity")
	g.Expect(capacityCond).ToNot(BeNil())
	g.Expect(capacityCond.Status).To(Equal(metav1.ConditionFalse))
	g.Expect(capacityCond.Reason).To(Equal(check.ReasonInsufficientCapacity))
	g.Expect(capacityCond.Impact).To(Equal(result.ImpactBlocking))
}

func TestResourceCapacityCheck_PassSufficientCapacity(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	ns := "redhat-ods-applications"
	dsc := testutil.NewDSC(map[string]string{"dashboard": "Managed"})
	dsci := testutil.NewDSCI(ns)
	ca := newClusterAutoscaler()

	deploy := newDashboardDeployment(ns, []containerSpec{
		{name: "dashboard", cpuReq: "500m", memoryReq: "1Gi"},
		{name: "kube-rbac-proxy", cpuReq: "500m", memoryReq: "1Gi"},
	}, 2, "1")

	node := newNode("node-1", "8000m", "32Gi")

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      resourceCapacityListKinds,
		Objects:        []*unstructured.Unstructured{dsc, dsci, deploy, node, ca},
		CurrentVersion: "2.25.0",
		TargetVersion:  "3.5.0",
	})

	chk := dashboard.NewResourceCapacityCheck()
	dr, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())

	capacityCond := findCondition(dr, "ResourceCapacity")
	g.Expect(capacityCond).ToNot(BeNil())
	g.Expect(capacityCond.Status).To(Equal(metav1.ConditionTrue))
	g.Expect(capacityCond.Reason).To(Equal(check.ReasonRequirementsMet))
}

func TestResourceCapacityCheck_AdvisoryNoAutoscaler(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	ns := "redhat-ods-applications"
	dsc := testutil.NewDSC(map[string]string{"dashboard": "Managed"})
	dsci := testutil.NewDSCI(ns)

	deploy := newDashboardDeployment(ns, []containerSpec{
		{name: "dashboard", cpuReq: "500m", memoryReq: "1Gi"},
		{name: "kube-rbac-proxy", cpuReq: "500m", memoryReq: "1Gi"},
	}, 2, "1")

	node := newNode("node-1", "8000m", "32Gi")

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      resourceCapacityListKinds,
		Objects:        []*unstructured.Unstructured{dsc, dsci, deploy, node},
		CurrentVersion: "2.25.0",
		TargetVersion:  "3.5.0",
	})

	chk := dashboard.NewResourceCapacityCheck()
	dr, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())

	capacityCond := findCondition(dr, "ResourceCapacity")
	g.Expect(capacityCond).ToNot(BeNil())
	g.Expect(capacityCond.Status).To(Equal(metav1.ConditionFalse))
	g.Expect(capacityCond.Reason).To(Equal(check.ReasonWorkloadsImpacted))
	g.Expect(capacityCond.Impact).To(Equal(result.ImpactAdvisory))
}

func TestResourceCapacityCheck_FallbackToPods(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	ns := "redhat-ods-applications"
	dsc := testutil.NewDSC(map[string]string{"dashboard": "Managed"})
	dsci := testutil.NewDSCI(ns)
	ca := newClusterAutoscaler()

	pod := newDashboardPodWithResources("dashboard-1", ns, []containerSpec{
		{name: "dashboard", cpuReq: "500m", memoryReq: "1Gi"},
		{name: "oauth-proxy", cpuReq: "500m", memoryReq: "512Mi"},
	})

	node := newNode("node-1", "8000m", "32Gi")

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      resourceCapacityListKinds,
		Objects:        []*unstructured.Unstructured{dsc, dsci, pod, node, ca},
		CurrentVersion: "2.25.0",
		TargetVersion:  "3.5.0",
	})

	chk := dashboard.NewResourceCapacityCheck()
	dr, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())

	capacityCond := findCondition(dr, "ResourceCapacity")
	g.Expect(capacityCond).ToNot(BeNil())
	g.Expect(capacityCond.Status).To(Equal(metav1.ConditionTrue))
	g.Expect(capacityCond.Reason).To(Equal(check.ReasonRequirementsMet))
}

func TestResourceCapacityCheck_NoDeploymentNoPods(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	ns := "redhat-ods-applications"
	dsc := testutil.NewDSC(map[string]string{"dashboard": "Managed"})
	dsci := testutil.NewDSCI(ns)
	node := newNode("node-1", "8000m", "32Gi")

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      resourceCapacityListKinds,
		Objects:        []*unstructured.Unstructured{dsc, dsci, node},
		CurrentVersion: "2.25.0",
		TargetVersion:  "3.5.0",
	})

	chk := dashboard.NewResourceCapacityCheck()
	dr, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())

	capacityCond := findCondition(dr, "ResourceCapacity")
	g.Expect(capacityCond).ToNot(BeNil())
	g.Expect(capacityCond.Status).To(Equal(metav1.ConditionTrue))
	g.Expect(capacityCond.Reason).To(Equal(check.ReasonRequirementsMet))
}

func TestResourceCapacityCheck_RolloutDeadlock(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	ns := "redhat-ods-applications"
	dsc := testutil.NewDSC(map[string]string{"dashboard": "Managed"})
	dsci := testutil.NewDSCI(ns)
	ca := newClusterAutoscaler()

	deploy := newDashboardDeployment(ns, []containerSpec{
		{name: "dashboard", cpuReq: "500m", memoryReq: "1Gi"},
	}, 2, "25%")

	node := newNode("node-1", "8000m", "32Gi")

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      resourceCapacityListKinds,
		Objects:        []*unstructured.Unstructured{dsc, dsci, deploy, node, ca},
		CurrentVersion: "2.25.0",
		TargetVersion:  "3.5.0",
	})

	chk := dashboard.NewResourceCapacityCheck()
	dr, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())

	rolloutCond := findCondition(dr, "RolloutStrategy")
	g.Expect(rolloutCond).ToNot(BeNil())
	g.Expect(rolloutCond.Status).To(Equal(metav1.ConditionFalse))
	g.Expect(rolloutCond.Reason).To(Equal(check.ReasonWorkloadsImpacted))
	g.Expect(rolloutCond.Impact).To(Equal(result.ImpactAdvisory))
	g.Expect(rolloutCond.Message).To(ContainSubstring("rounds to 0"))
}

func TestResourceCapacityCheck_RolloutOK(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	ns := "redhat-ods-applications"
	dsc := testutil.NewDSC(map[string]string{"dashboard": "Managed"})
	dsci := testutil.NewDSCI(ns)
	ca := newClusterAutoscaler()

	deploy := newDashboardDeployment(ns, []containerSpec{
		{name: "dashboard", cpuReq: "500m", memoryReq: "1Gi"},
	}, 2, "1")

	node := newNode("node-1", "8000m", "32Gi")

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      resourceCapacityListKinds,
		Objects:        []*unstructured.Unstructured{dsc, dsci, deploy, node, ca},
		CurrentVersion: "2.25.0",
		TargetVersion:  "3.5.0",
	})

	chk := dashboard.NewResourceCapacityCheck()
	dr, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())

	rolloutCond := findCondition(dr, "RolloutStrategy")
	g.Expect(rolloutCond).ToNot(BeNil())
	g.Expect(rolloutCond.Status).To(Equal(metav1.ConditionTrue))
	g.Expect(rolloutCond.Reason).To(Equal(check.ReasonRequirementsMet))
}

func TestResourceCapacityCheck_BlockingAndDeadlock(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	ns := "redhat-ods-applications"
	dsc := testutil.NewDSC(map[string]string{"dashboard": "Managed"})
	dsci := testutil.NewDSCI(ns)

	deploy := newDashboardDeployment(ns, []containerSpec{
		{name: "dashboard", cpuReq: "2000m", memoryReq: "1Gi"},
		{name: "proxy", cpuReq: "2000m", memoryReq: "1Gi"},
	}, 2, "25%")

	node := newNode("node-1", "3500m", "16Gi")

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      resourceCapacityListKinds,
		Objects:        []*unstructured.Unstructured{dsc, dsci, deploy, node},
		CurrentVersion: "2.25.0",
		TargetVersion:  "3.5.0",
	})

	chk := dashboard.NewResourceCapacityCheck()
	dr, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(dr.Status.Conditions).To(HaveLen(2))

	capacityCond := findCondition(dr, "ResourceCapacity")
	g.Expect(capacityCond).ToNot(BeNil())
	g.Expect(capacityCond.Impact).To(Equal(result.ImpactBlocking))

	rolloutCond := findCondition(dr, "RolloutStrategy")
	g.Expect(rolloutCond).ToNot(BeNil())
	g.Expect(rolloutCond.Impact).To(Equal(result.ImpactAdvisory))
}

func findCondition(dr *result.DiagnosticResult, condType string) *result.Condition {
	for i := range dr.Status.Conditions {
		if dr.Status.Conditions[i].Type == condType {
			return &dr.Status.Conditions[i]
		}
	}

	return nil
}
