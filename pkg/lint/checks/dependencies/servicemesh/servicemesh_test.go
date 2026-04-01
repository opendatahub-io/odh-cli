package servicemesh_test

import (
	"errors"
	"testing"

	"github.com/blang/semver/v4"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8stesting "k8s.io/client-go/testing"

	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	resultpkg "github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/testutil"
	"github.com/opendatahub-io/odh-cli/pkg/lint/checks/dependencies/servicemesh"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"

	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

func listKinds() map[schema.GroupVersionResource]string {
	return map[schema.GroupVersionResource]string{
		resources.Deployment.GVR():      resources.Deployment.ListKind(),
		resources.PackageManifest.GVR(): resources.PackageManifest.ListKind(),
	}
}

func newIngressOperatorDeployment(envVars map[string]string) *unstructured.Unstructured {
	envList := make([]any, 0, len(envVars))
	for k, v := range envVars {
		envList = append(envList, map[string]any{
			"name":  k,
			"value": v,
		})
	}

	obj := map[string]any{
		"apiVersion": resources.Deployment.APIVersion(),
		"kind":       resources.Deployment.Kind,
		"metadata": map[string]any{
			"name":      "ingress-operator",
			"namespace": "openshift-ingress-operator",
		},
		"spec": map[string]any{
			"template": map[string]any{
				"spec": map[string]any{
					"containers": []any{
						map[string]any{
							"name": "ingress-operator",
							"env":  envList,
						},
					},
				},
			},
		},
	}

	return &unstructured.Unstructured{Object: obj}
}

func newPackageManifest(catalogSource string, channel string, csvNames []string) *unstructured.Unstructured {
	entries := make([]any, 0, len(csvNames))
	for _, csv := range csvNames {
		entries = append(entries, map[string]any{
			"name": csv,
		})
	}

	channels := []any{
		map[string]any{
			"name":    channel,
			"entries": entries,
		},
	}

	obj := map[string]any{
		"apiVersion": resources.PackageManifest.APIVersion(),
		"kind":       resources.PackageManifest.Kind,
		"metadata": map[string]any{
			"name":      "servicemeshoperator3",
			"namespace": "openshift-marketplace",
		},
		"status": map[string]any{
			"catalogSource": catalogSource,
			"channels":      channels,
		},
	}

	return &unstructured.Unstructured{Object: obj}
}

func TestServiceMeshV3Check_VersionAvailable(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	deploy := newIngressOperatorDeployment(map[string]string{
		"GATEWAY_API_OPERATOR_VERSION": "servicemeshoperator3.v3.1.0",
		"GATEWAY_API_OPERATOR_CHANNEL": "stable",
	})
	pm := newPackageManifest("redhat-operators", "stable", []string{"servicemeshoperator3.v3.1.0"})

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds(),
		Objects:        []*unstructured.Unstructured{deploy, pm},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	smCheck := servicemesh.NewCheck()
	result, err := smCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(check.ConditionTypeAvailable),
		"Status": Equal(metav1.ConditionTrue),
		"Reason": Equal(check.ReasonResourceFound),
	}))
	g.Expect(result.Status.Conditions[0].Message).To(And(
		ContainSubstring("servicemeshoperator3.v3.1.0"),
		ContainSubstring("'stable' channel"),
		ContainSubstring("redhat-operators"),
	))
}

func TestServiceMeshV3Check_DeploymentNotFound(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	pm := newPackageManifest("redhat-operators", "stable", []string{"servicemeshoperator3.v3.1.0"})

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds(),
		Objects:        []*unstructured.Unstructured{pm},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	smCheck := servicemesh.NewCheck()
	result, err := smCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(check.ConditionTypeAvailable),
		"Status": Equal(metav1.ConditionFalse),
		"Reason": Equal(check.ReasonResourceNotFound),
	}))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactBlocking))
}

func TestServiceMeshV3Check_InsufficientPermissions(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	// testutil.NewTarget does not support reactor injection, so we construct the client manually.
	scheme := runtime.NewScheme()
	_ = metav1.AddMetaToScheme(scheme)

	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		scheme,
		listKinds(),
	)

	// Simulate a Forbidden error when trying to get the ingress-operator deployment.
	dynamicClient.PrependReactor("get", "deployments", func(action k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewForbidden(
			schema.GroupResource{Group: "apps", Resource: "deployments"},
			"ingress-operator",
			errors.New("forbidden"),
		)
	})

	currentVer := semver.MustParse("2.17.0")
	targetVer := semver.MustParse("3.0.0")

	target := check.Target{
		Client:         client.NewForTesting(client.TestClientConfig{Dynamic: dynamicClient}),
		CurrentVersion: &currentVer,
		TargetVersion:  &targetVer,
	}

	smCheck := servicemesh.NewCheck()
	result, err := smCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(check.ConditionTypeAvailable),
		"Status": Equal(metav1.ConditionFalse),
		"Reason": Equal(check.ReasonInsufficientData),
	}))
	g.Expect(result.Status.Conditions[0].Message).To(ContainSubstring("insufficient permissions"))
	g.Expect(result.Status.Conditions[0].Remediation).To(ContainSubstring("read access"))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactBlocking))
}

func TestServiceMeshV3Check_EnvVarMissing(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	deploy := newIngressOperatorDeployment(map[string]string{})
	pm := newPackageManifest("redhat-operators", "stable", []string{"servicemeshoperator3.v3.1.0"})

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds(),
		Objects:        []*unstructured.Unstructured{deploy, pm},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	smCheck := servicemesh.NewCheck()
	result, err := smCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(check.ConditionTypeAvailable),
		"Status": Equal(metav1.ConditionFalse),
		"Reason": Equal(check.ReasonDependencyUnavailable),
	}))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactBlocking))
}

func TestServiceMeshV3Check_PackageManifestNotFound(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	deploy := newIngressOperatorDeployment(map[string]string{
		"GATEWAY_API_OPERATOR_VERSION": "servicemeshoperator3.v3.1.0",
		"GATEWAY_API_OPERATOR_CHANNEL": "stable",
	})

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds(),
		Objects:        []*unstructured.Unstructured{deploy},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	smCheck := servicemesh.NewCheck()
	result, err := smCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(check.ConditionTypeAvailable),
		"Status": Equal(metav1.ConditionFalse),
		"Reason": Equal(check.ReasonResourceNotFound),
	}))
	g.Expect(result.Status.Conditions[0].Message).To(And(
		ContainSubstring("servicemeshoperator3.v3.1.0"),
		ContainSubstring("'stable' channel"),
		ContainSubstring("redhat-operators"),
	))
	g.Expect(result.Status.Conditions[0].Remediation).To(And(
		ContainSubstring("servicemeshoperator3.v3.1.0"),
		ContainSubstring("'stable' channel"),
		ContainSubstring("redhat-operators"),
		ContainSubstring("openshift-marketplace"),
		ContainSubstring("upgrade guide"),
	))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactBlocking))
}

func TestServiceMeshV3Check_WrongCatalogSource(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	deploy := newIngressOperatorDeployment(map[string]string{
		"GATEWAY_API_OPERATOR_VERSION": "servicemeshoperator3.v3.1.0",
		"GATEWAY_API_OPERATOR_CHANNEL": "stable",
	})
	// PackageManifest exists but from a non-redhat-operators catalog; the check should not find it.
	pm := newPackageManifest("custom-operators", "stable", []string{"servicemeshoperator3.v3.1.0"})

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds(),
		Objects:        []*unstructured.Unstructured{deploy, pm},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	smCheck := servicemesh.NewCheck()
	result, err := smCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(check.ConditionTypeAvailable),
		"Status": Equal(metav1.ConditionFalse),
		"Reason": Equal(check.ReasonResourceNotFound),
	}))
	g.Expect(result.Status.Conditions[0].Message).To(And(
		ContainSubstring("servicemeshoperator3.v3.1.0"),
		ContainSubstring("'stable' channel"),
		ContainSubstring("redhat-operators"),
	))
	g.Expect(result.Status.Conditions[0].Remediation).To(And(
		ContainSubstring("servicemeshoperator3.v3.1.0"),
		ContainSubstring("'stable' channel"),
		ContainSubstring("redhat-operators"),
		ContainSubstring("openshift-marketplace"),
		ContainSubstring("upgrade guide"),
	))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactBlocking))
}

func TestServiceMeshV3Check_VersionNotAvailable(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	deploy := newIngressOperatorDeployment(map[string]string{
		"GATEWAY_API_OPERATOR_VERSION": "servicemeshoperator3.v3.1.0",
		"GATEWAY_API_OPERATOR_CHANNEL": "stable",
	})
	pm := newPackageManifest("redhat-operators", "stable", []string{"servicemeshoperator3.v3.0.0"})

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds(),
		Objects:        []*unstructured.Unstructured{deploy, pm},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	smCheck := servicemesh.NewCheck()
	result, err := smCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(check.ConditionTypeAvailable),
		"Status": Equal(metav1.ConditionFalse),
		"Reason": Equal(check.ReasonDependencyUnavailable),
	}))
	g.Expect(result.Status.Conditions[0].Message).To(ContainSubstring("'stable' channel"))
	g.Expect(result.Status.Conditions[0].Remediation).To(And(
		ContainSubstring("Mirror"),
		ContainSubstring("'stable' channel"),
		ContainSubstring("redhat-operators"),
		ContainSubstring("openshift-marketplace"),
	))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactBlocking))
}

func TestServiceMeshV3Check_ContainerWithNoEnvKey(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	// Simulate a container with no env key at all (e.g. injected sidecar).
	deploy := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.Deployment.APIVersion(),
			"kind":       resources.Deployment.Kind,
			"metadata": map[string]any{
				"name":      "ingress-operator",
				"namespace": "openshift-ingress-operator",
			},
			"spec": map[string]any{
				"template": map[string]any{
					"spec": map[string]any{
						"containers": []any{
							map[string]any{
								"name": "sidecar",
							},
						},
					},
				},
			},
		},
	}
	pm := newPackageManifest("redhat-operators", "stable", []string{"servicemeshoperator3.v3.1.0"})

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds(),
		Objects:        []*unstructured.Unstructured{deploy, pm},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	smCheck := servicemesh.NewCheck()
	result, err := smCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(check.ConditionTypeAvailable),
		"Status": Equal(metav1.ConditionFalse),
		"Reason": Equal(check.ReasonDependencyUnavailable),
	}))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactBlocking))
}

func TestServiceMeshV3Check_EnvVarEmpty(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	deploy := newIngressOperatorDeployment(map[string]string{
		"GATEWAY_API_OPERATOR_VERSION": "",
	})
	pm := newPackageManifest("redhat-operators", "stable", []string{"servicemeshoperator3.v3.1.0"})

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds(),
		Objects:        []*unstructured.Unstructured{deploy, pm},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	smCheck := servicemesh.NewCheck()
	result, err := smCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(check.ConditionTypeAvailable),
		"Status": Equal(metav1.ConditionFalse),
		"Reason": Equal(check.ReasonDependencyUnavailable),
	}))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactBlocking))
}

func TestServiceMeshV3Check_VersionAvailableAmongMultipleEntries(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	deploy := newIngressOperatorDeployment(map[string]string{
		"GATEWAY_API_OPERATOR_VERSION": "servicemeshoperator3.v3.2.0",
		"GATEWAY_API_OPERATOR_CHANNEL": "stable",
	})
	pm := newPackageManifest("redhat-operators", "stable", []string{
		"servicemeshoperator3.v3.0.0",
		"servicemeshoperator3.v3.1.0",
		"servicemeshoperator3.v3.2.0",
	})

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds(),
		Objects:        []*unstructured.Unstructured{deploy, pm},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	smCheck := servicemesh.NewCheck()
	result, err := smCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(check.ConditionTypeAvailable),
		"Status": Equal(metav1.ConditionTrue),
		"Reason": Equal(check.ReasonResourceFound),
	}))
	g.Expect(result.Status.Conditions[0].Message).To(And(
		ContainSubstring("servicemeshoperator3.v3.2.0"),
		ContainSubstring("'stable' channel"),
		ContainSubstring("redhat-operators"),
	))
}

func TestServiceMeshV3Check_MissingCatalogSource(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	deploy := newIngressOperatorDeployment(map[string]string{
		"GATEWAY_API_OPERATOR_VERSION": "servicemeshoperator3.v3.1.0",
		"GATEWAY_API_OPERATOR_CHANNEL": "stable",
	})
	// PackageManifest with no catalogSource in status — won't match redhat-operators.
	pm := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.PackageManifest.APIVersion(),
			"kind":       resources.PackageManifest.Kind,
			"metadata": map[string]any{
				"name":      "servicemeshoperator3",
				"namespace": "openshift-marketplace",
			},
			"status": map[string]any{
				"channels": []any{
					map[string]any{
						"name": "stable",
						"entries": []any{
							map[string]any{
								"name": "servicemeshoperator3.v3.1.0",
							},
						},
					},
				},
			},
		},
	}

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds(),
		Objects:        []*unstructured.Unstructured{deploy, pm},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	smCheck := servicemesh.NewCheck()
	result, err := smCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(check.ConditionTypeAvailable),
		"Status": Equal(metav1.ConditionFalse),
		"Reason": Equal(check.ReasonResourceNotFound),
	}))
	g.Expect(result.Status.Conditions[0].Message).To(And(
		ContainSubstring("servicemeshoperator3.v3.1.0"),
		ContainSubstring("'stable' channel"),
		ContainSubstring("redhat-operators"),
	))
	g.Expect(result.Status.Conditions[0].Remediation).To(And(
		ContainSubstring("servicemeshoperator3.v3.1.0"),
		ContainSubstring("'stable' channel"),
		ContainSubstring("redhat-operators"),
		ContainSubstring("openshift-marketplace"),
		ContainSubstring("upgrade guide"),
	))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactBlocking))
}

func TestServiceMeshV3Check_ChannelEnvVarMissing(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	deploy := newIngressOperatorDeployment(map[string]string{
		"GATEWAY_API_OPERATOR_VERSION": "servicemeshoperator3.v3.1.0",
	})
	pm := newPackageManifest("redhat-operators", "stable", []string{"servicemeshoperator3.v3.1.0"})

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds(),
		Objects:        []*unstructured.Unstructured{deploy, pm},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	smCheck := servicemesh.NewCheck()
	result, err := smCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(check.ConditionTypeAvailable),
		"Status": Equal(metav1.ConditionFalse),
		"Reason": Equal(check.ReasonDependencyUnavailable),
	}))
	g.Expect(result.Status.Conditions[0].Message).To(ContainSubstring("GATEWAY_API_OPERATOR_CHANNEL"))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactBlocking))
}

func TestServiceMeshV3Check_ChannelEnvVarEmpty(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	deploy := newIngressOperatorDeployment(map[string]string{
		"GATEWAY_API_OPERATOR_VERSION": "servicemeshoperator3.v3.1.0",
		"GATEWAY_API_OPERATOR_CHANNEL": "",
	})
	pm := newPackageManifest("redhat-operators", "stable", []string{"servicemeshoperator3.v3.1.0"})

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds(),
		Objects:        []*unstructured.Unstructured{deploy, pm},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	smCheck := servicemesh.NewCheck()
	result, err := smCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(check.ConditionTypeAvailable),
		"Status": Equal(metav1.ConditionFalse),
		"Reason": Equal(check.ReasonDependencyUnavailable),
	}))
	g.Expect(result.Status.Conditions[0].Message).To(ContainSubstring("GATEWAY_API_OPERATOR_CHANNEL"))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactBlocking))
}

func TestServiceMeshV3Check_VersionInDifferentChannel(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	deploy := newIngressOperatorDeployment(map[string]string{
		"GATEWAY_API_OPERATOR_VERSION": "servicemeshoperator3.v3.1.0",
		"GATEWAY_API_OPERATOR_CHANNEL": "stable",
	})
	// The CSV exists in the "candidate" channel, but the check requires "stable".
	pm := newPackageManifest("redhat-operators", "candidate", []string{"servicemeshoperator3.v3.1.0"})

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds(),
		Objects:        []*unstructured.Unstructured{deploy, pm},
		CurrentVersion: "2.17.0",
		TargetVersion:  "3.0.0",
	})

	smCheck := servicemesh.NewCheck()
	result, err := smCheck.Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result.Status.Conditions).To(HaveLen(1))
	g.Expect(result.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(check.ConditionTypeAvailable),
		"Status": Equal(metav1.ConditionFalse),
		"Reason": Equal(check.ReasonDependencyUnavailable),
	}))
	g.Expect(result.Status.Conditions[0].Message).To(ContainSubstring("'stable' channel"))
	g.Expect(result.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactBlocking))
}

func TestServiceMeshV3Check_CanApply_2xTo3x(t *testing.T) {
	g := NewWithT(t)

	smCheck := servicemesh.NewCheck()

	currentVer := semver.MustParse("2.17.0")
	targetVer := semver.MustParse("3.0.0")
	target := check.Target{
		CurrentVersion: &currentVer,
		TargetVersion:  &targetVer,
	}

	canApply, err := smCheck.CanApply(t.Context(), target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeTrue())
}

func TestServiceMeshV3Check_CanApply_2xTo2x(t *testing.T) {
	g := NewWithT(t)

	smCheck := servicemesh.NewCheck()

	currentVer := semver.MustParse("2.17.0")
	targetVer := semver.MustParse("2.18.0")
	target := check.Target{
		CurrentVersion: &currentVer,
		TargetVersion:  &targetVer,
	}

	canApply, err := smCheck.CanApply(t.Context(), target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeFalse())
}

func TestServiceMeshV3Check_CanApply_3xTo3x(t *testing.T) {
	g := NewWithT(t)

	smCheck := servicemesh.NewCheck()

	currentVer := semver.MustParse("3.0.0")
	targetVer := semver.MustParse("3.1.0")
	target := check.Target{
		CurrentVersion: &currentVer,
		TargetVersion:  &targetVer,
	}

	canApply, err := smCheck.CanApply(t.Context(), target)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeFalse())
}

func TestServiceMeshV3Check_CanApply_NilVersions(t *testing.T) {
	g := NewWithT(t)

	smCheck := servicemesh.NewCheck()

	canApply, err := smCheck.CanApply(t.Context(), check.Target{})
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeFalse())
}

func TestServiceMeshV3Check_Metadata(t *testing.T) {
	g := NewWithT(t)

	smCheck := servicemesh.NewCheck()

	g.Expect(smCheck.ID()).To(Equal("dependencies.servicemesh.installed"))
	g.Expect(smCheck.Name()).To(Equal("Dependencies :: Service Mesh v3 :: Installed"))
	g.Expect(smCheck.Group()).To(Equal(check.GroupDependency))
	g.Expect(smCheck.Description()).ToNot(BeEmpty())
}
