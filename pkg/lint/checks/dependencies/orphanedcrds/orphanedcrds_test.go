package orphanedcrds_test

import (
	"errors"
	"testing"

	"github.com/blang/semver/v4"
	operatorsv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	operatorfake "github.com/operator-framework/operator-lifecycle-manager/pkg/api/client/clientset/versioned/fake"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8stesting "k8s.io/client-go/testing"

	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	resultpkg "github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/testutil"
	"github.com/opendatahub-io/odh-cli/pkg/lint/checks/dependencies/orphanedcrds"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"

	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

func listKinds() map[schema.GroupVersionResource]string {
	return map[schema.GroupVersionResource]string{
		resources.CustomResourceDefinition.GVR():                                              resources.CustomResourceDefinition.ListKind(),
		{Group: "networking.istio.io", Version: versionV1beta1, Resource: "virtualservices"}:  "VirtualServiceList",
		{Group: "networking.istio.io", Version: versionV1beta1, Resource: "destinationrules"}: "DestinationRuleList",
		{Group: groupSecurityIstio, Version: versionV1beta1, Resource: resourceAuthzPolicies}: "AuthorizationPolicyList",
	}
}

const (
	crdVirtualServices      = "virtualservices.networking.istio.io"
	crdDestinationRules     = "destinationrules.networking.istio.io"
	crdMaistraIO            = "servicemeshcontrolplanes.maistra.io"
	crdAuthorizationPolicy  = "authorizationpolicies.security.istio.io"
	groupSecurityIstio      = "security.istio.io"
	resourceAuthzPolicies   = "authorizationpolicies"
	versionV1               = "v1"
	versionV1beta1          = "v1beta1"
)

func newIstioCRD(name string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.CustomResourceDefinition.APIVersion(),
			"kind":       resources.CustomResourceDefinition.Kind,
			"metadata": map[string]any{
				"name": name,
				"labels": map[string]any{
					"maistra-version": "2.6",
				},
			},
			"spec": map[string]any{
				"group": "networking.istio.io",
				"names": map[string]any{
					"plural": name[:len(name)-len(".networking.istio.io")],
				},
				"versions": []any{
					map[string]any{
						"name":   "v1beta1",
						"served": true,
					},
				},
			},
		},
	}
}

func newMaistraCRD(name string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.CustomResourceDefinition.APIVersion(),
			"kind":       resources.CustomResourceDefinition.Kind,
			"metadata": map[string]any{
				"name": name,
				"labels": map[string]any{
					"maistra-version": "2.6",
				},
			},
			"spec": map[string]any{
				"group": "maistra.io",
				"names": map[string]any{
					"plural": "servicemeshcontrolplanes",
				},
				"versions": []any{
					map[string]any{
						"name":   "v2",
						"served": true,
					},
				},
			},
		},
	}
}

func newIstioCRDWithGroup(name, group string) *unstructured.Unstructured {
	plural := name[:len(name)-len("."+group)]

	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.CustomResourceDefinition.APIVersion(),
			"kind":       resources.CustomResourceDefinition.Kind,
			"metadata": map[string]any{
				"name": name,
				"labels": map[string]any{
					"maistra-version": "2.6",
				},
			},
			"spec": map[string]any{
				"group": group,
				"names": map[string]any{
					"plural": plural,
				},
				"versions": []any{
					map[string]any{
						"name":   versionV1beta1,
						"served": true,
					},
				},
			},
		},
	}
}

func newIstioCRDMultiVersion(name string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.CustomResourceDefinition.APIVersion(),
			"kind":       resources.CustomResourceDefinition.Kind,
			"metadata": map[string]any{
				"name": name,
				"labels": map[string]any{
					"maistra-version": "2.6",
				},
			},
			"spec": map[string]any{
				"group": "networking.istio.io",
				"names": map[string]any{
					"plural": name[:len(name)-len(".networking.istio.io")],
				},
				"versions": []any{
					map[string]any{
						"name":   versionV1beta1,
						"served": true,
					},
					map[string]any{
						"name":   versionV1,
						"served": true,
					},
				},
			},
		},
	}
}

func newIstioCRDNoServedVersion(name string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.CustomResourceDefinition.APIVersion(),
			"kind":       resources.CustomResourceDefinition.Kind,
			"metadata": map[string]any{
				"name": name,
				"labels": map[string]any{
					"maistra-version": "2.6",
				},
			},
			"spec": map[string]any{
				"group": "networking.istio.io",
				"names": map[string]any{
					"plural": name[:len(name)-len(".networking.istio.io")],
				},
				"versions": []any{
					map[string]any{
						"name":   versionV1beta1,
						"served": false,
					},
				},
			},
		},
	}
}

func newSM2Subscription() *operatorsv1alpha1.Subscription {
	return &operatorsv1alpha1.Subscription{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "servicemeshoperator",
			Namespace: "openshift-operators",
		},
		Spec: &operatorsv1alpha1.SubscriptionSpec{
			Channel: "stable",
			Package: "servicemeshoperator",
		},
	}
}

func TestOrphanedCRDsCheck_NoCRDs(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds(),
		OLM:            operatorfake.NewSimpleClientset(), //nolint:staticcheck // NewClientset requires generated apply configs not available in OLM
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := orphanedcrds.NewCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(check.ConditionTypeAvailable),
		"Status": Equal(metav1.ConditionTrue),
		"Reason": Equal(check.ReasonRequirementsMet),
	}))
}

func TestOrphanedCRDsCheck_OnlyMaistraCRDs(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds(),
		Objects:        []*unstructured.Unstructured{newMaistraCRD(crdMaistraIO)},
		OLM:            operatorfake.NewSimpleClientset(), //nolint:staticcheck // NewClientset requires generated apply configs not available in OLM
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := orphanedcrds.NewCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(check.ConditionTypeAvailable),
		"Status": Equal(metav1.ConditionTrue),
		"Reason": Equal(check.ReasonRequirementsMet),
	}))
}

func TestOrphanedCRDsCheck_SM2OperatorInstalled(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	sub := newSM2Subscription()

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds(),
		Objects:        []*unstructured.Unstructured{newIstioCRD(crdVirtualServices)},
		OLM:            operatorfake.NewSimpleClientset(sub), //nolint:staticcheck // NewClientset requires generated apply configs not available in OLM
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := orphanedcrds.NewCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(check.ConditionTypeAvailable),
		"Status": Equal(metav1.ConditionTrue),
		"Reason": Equal(check.ReasonRequirementsMet),
	}))
	g.Expect(result.Status.Conditions[0].Message).To(ContainSubstring("managed by the operator"))
}

func TestOrphanedCRDsCheck_OrphanedCRDsDetected(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds: listKinds(),
		Objects: []*unstructured.Unstructured{
			newIstioCRD(crdVirtualServices),
			newIstioCRD(crdDestinationRules),
		},
		OLM:            operatorfake.NewSimpleClientset(), //nolint:staticcheck // NewClientset requires generated apply configs not available in OLM
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := orphanedcrds.NewCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(check.ConditionTypeAvailable),
		"Status": Equal(metav1.ConditionFalse),
		"Reason": Equal(check.ReasonDependencyUnavailable),
	}))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactBlocking))
	g.Expect(result.Status.Conditions[0].Message).To(ContainSubstring("2 orphaned"))
	g.Expect(result.Status.Conditions[0].Message).To(ContainSubstring(crdVirtualServices))
	g.Expect(result.Status.Conditions[0].Message).To(ContainSubstring(crdDestinationRules))
	g.Expect(result.ImpactedObjects).To(HaveLen(2))
}

func TestOrphanedCRDsCheck_NoOLM_TreatedAsOrphaned(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds(),
		Objects:        []*unstructured.Unstructured{newIstioCRD(crdVirtualServices)},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := orphanedcrds.NewCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(check.ConditionTypeAvailable),
		"Status": Equal(metav1.ConditionFalse),
		"Reason": Equal(check.ReasonDependencyUnavailable),
	}))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactBlocking))
}

func TestOrphanedCRDsCheck_SM3SubscriptionNotTreatedAsSM2(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	sm3Sub := &operatorsv1alpha1.Subscription{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "servicemeshoperator3",
			Namespace: "openshift-operators",
		},
		Spec: &operatorsv1alpha1.SubscriptionSpec{
			Channel: "stable",
			Package: "servicemeshoperator3",
		},
	}

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds(),
		Objects:        []*unstructured.Unstructured{newIstioCRD(crdVirtualServices)},
		OLM:            operatorfake.NewSimpleClientset(sm3Sub), //nolint:staticcheck // NewClientset requires generated apply configs not available in OLM
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := orphanedcrds.NewCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(check.ConditionTypeAvailable),
		"Status": Equal(metav1.ConditionFalse),
		"Reason": Equal(check.ReasonDependencyUnavailable),
	}))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactBlocking))
}

func TestOrphanedCRDsCheck_CanApply_2xTo3x(t *testing.T) {
	g := NewWithT(t)

	chk := orphanedcrds.NewCheck()

	currentVer := semver.MustParse("2.17.0")
	targetVer := semver.MustParse("3.0.0")
	target := check.Target{
		CurrentVersion: &currentVer,
		TargetVersion:  &targetVer,
	}

	canApply, err := chk.CanApply(t.Context(), target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeTrue())
}

func TestOrphanedCRDsCheck_CanApply_3xTo3x(t *testing.T) {
	g := NewWithT(t)

	chk := orphanedcrds.NewCheck()

	currentVer := semver.MustParse("3.0.0")
	targetVer := semver.MustParse("3.1.0")
	target := check.Target{
		CurrentVersion: &currentVer,
		TargetVersion:  &targetVer,
	}

	canApply, err := chk.CanApply(t.Context(), target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeFalse())
}

func TestOrphanedCRDsCheck_CanApply_NilVersions(t *testing.T) {
	g := NewWithT(t)

	chk := orphanedcrds.NewCheck()

	canApply, err := chk.CanApply(t.Context(), check.Target{})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeFalse())
}

func TestOrphanedCRDsCheck_SM2CustomNameSubscription(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	sub := &operatorsv1alpha1.Subscription{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-custom-sm-subscription",
			Namespace: "openshift-operators",
		},
		Spec: &operatorsv1alpha1.SubscriptionSpec{
			Channel: "stable",
			Package: "servicemeshoperator",
		},
	}

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds(),
		Objects:        []*unstructured.Unstructured{newIstioCRD(crdVirtualServices)},
		OLM:            operatorfake.NewSimpleClientset(sub), //nolint:staticcheck // NewClientset requires generated apply configs not available in OLM
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := orphanedcrds.NewCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(check.ConditionTypeAvailable),
		"Status": Equal(metav1.ConditionTrue),
		"Reason": Equal(check.ReasonRequirementsMet),
	}))
	g.Expect(result.Status.Conditions[0].Message).To(ContainSubstring("managed by the operator"))
}

func TestOrphanedCRDsCheck_UnrelatedServiceMeshSubscription(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	sub := &operatorsv1alpha1.Subscription{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "servicemesh-monitoring",
			Namespace: "openshift-operators",
		},
		Spec: &operatorsv1alpha1.SubscriptionSpec{
			Channel: "stable",
			Package: "servicemesh-monitoring-operator",
		},
	}

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds(),
		Objects:        []*unstructured.Unstructured{newIstioCRD(crdVirtualServices)},
		OLM:            operatorfake.NewSimpleClientset(sub), //nolint:staticcheck // NewClientset requires generated apply configs not available in OLM
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := orphanedcrds.NewCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(check.ConditionTypeAvailable),
		"Status": Equal(metav1.ConditionFalse),
		"Reason": Equal(check.ReasonDependencyUnavailable),
	}))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactBlocking))
}

func TestOrphanedCRDsCheck_ListResourcesError(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	scheme := runtime.NewScheme()
	_ = metav1.AddMetaToScheme(scheme)

	crd := newIstioCRD(crdVirtualServices)

	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		scheme,
		listKinds(),
		crd,
	)

	// Inject a non-permission error when listing virtualservices to exercise
	// the ConditionUnknown / ReasonInsufficientData path.
	dynamicClient.PrependReactor("list", "virtualservices", func(_ k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("connection refused")
	})

	currentVer := semver.MustParse("2.17.0")
	targetVer := semver.MustParse("3.0.0")

	target := check.Target{
		Client:         client.NewForTesting(client.TestClientConfig{Dynamic: dynamicClient}),
		CurrentVersion: &currentVer,
		TargetVersion:  &targetVer,
	}

	chk := orphanedcrds.NewCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(check.ConditionTypeAvailable),
		"Status": Equal(metav1.ConditionUnknown),
		"Reason": Equal(check.ReasonInsufficientData),
	}))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactBlocking))
	g.Expect(result.Status.Conditions[0].Message).To(ContainSubstring("connection refused"))
	g.Expect(result.Status.Conditions[0].Message).To(ContainSubstring("unable to verify"))
	g.Expect(result.Status.Conditions[0].Remediation).To(ContainSubstring("Verify RBAC permissions"))
	g.Expect(result.Status.Conditions[0].Remediation).ToNot(ContainSubstring("Delete the orphaned"))
	g.Expect(result.ImpactedObjects).To(HaveLen(1))
}

func TestOrphanedCRDsCheck_ActiveResources(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	vs := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "networking.istio.io/v1beta1",
			"kind":       "VirtualService",
			"metadata": map[string]any{
				"name":      "my-virtualservice",
				"namespace": "default",
			},
		},
	}

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds: listKinds(),
		Objects: []*unstructured.Unstructured{
			newIstioCRD(crdVirtualServices),
			vs,
		},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := orphanedcrds.NewCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(check.ConditionTypeAvailable),
		"Status": Equal(metav1.ConditionFalse),
		"Reason": Equal(check.ReasonDependencyUnavailable),
	}))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactBlocking))
	g.Expect(result.Status.Conditions[0].Message).To(ContainSubstring("active custom resource"))
	g.Expect(result.Status.Conditions[0].Remediation).To(ContainSubstring("Review and remove"))
	g.Expect(result.ImpactedObjects).To(HaveLen(1))
}

func TestOrphanedCRDsCheck_MultiVersionCRD_PicksFirst(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	multiVersionCRD := newIstioCRDMultiVersion(crdVirtualServices)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds(),
		Objects:        []*unstructured.Unstructured{multiVersionCRD},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := orphanedcrds.NewCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(check.ConditionTypeAvailable),
		"Status": Equal(metav1.ConditionFalse),
		"Reason": Equal(check.ReasonDependencyUnavailable),
	}))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactBlocking))
	g.Expect(result.Status.Conditions[0].Message).To(ContainSubstring("1 orphaned"))
	g.Expect(result.ImpactedObjects).To(HaveLen(1))
}

func TestOrphanedCRDsCheck_NoServedVersion_Skipped(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	noServedCRD := newIstioCRDNoServedVersion(crdVirtualServices)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds(),
		Objects:        []*unstructured.Unstructured{noServedCRD},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := orphanedcrds.NewCheck()
	result, err := chk.Validate(ctx, target)

	// The CRD is still detected as orphaned (it has the istio.io suffix and
	// maistra-version label), but countActiveResources silently skips it
	// because extractGVR returns false for a CRD with no served version.
	// The result should still report it as orphaned with zero active resources.
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(check.ConditionTypeAvailable),
		"Status": Equal(metav1.ConditionFalse),
		"Reason": Equal(check.ReasonDependencyUnavailable),
	}))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactBlocking))
	g.Expect(result.Status.Conditions[0].Message).To(ContainSubstring("1 orphaned"))
	g.Expect(result.ImpactedObjects).To(HaveLen(1))
}

func TestOrphanedCRDsCheck_NonNetworkingIstioGroup(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	securityCRD := newIstioCRDWithGroup(crdAuthorizationPolicy, groupSecurityIstio)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds(),
		Objects:        []*unstructured.Unstructured{securityCRD},
		OLM:            operatorfake.NewSimpleClientset(), //nolint:staticcheck // NewClientset requires generated apply configs not available in OLM
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	chk := orphanedcrds.NewCheck()
	result, err := chk.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(check.ConditionTypeAvailable),
		"Status": Equal(metav1.ConditionFalse),
		"Reason": Equal(check.ReasonDependencyUnavailable),
	}))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactBlocking))
	g.Expect(result.Status.Conditions[0].Message).To(ContainSubstring("1 orphaned"))
	g.Expect(result.Status.Conditions[0].Message).To(ContainSubstring(crdAuthorizationPolicy))
	g.Expect(result.ImpactedObjects).To(HaveLen(1))
}

func TestOrphanedCRDsCheck_Metadata(t *testing.T) {
	g := NewWithT(t)

	chk := orphanedcrds.NewCheck()

	g.Expect(chk.ID()).To(Equal("dependencies.orphaned-sm2-crds.readiness"))
	g.Expect(chk.Name()).To(Equal("Dependencies :: Orphaned SM2 CRDs :: Readiness"))
	g.Expect(chk.Group()).To(Equal(check.GroupDependency))
	g.Expect(chk.Description()).ToNot(BeEmpty())
}
