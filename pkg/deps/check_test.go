package deps_test

import (
	"testing"

	semver "github.com/blang/semver/v4"
	operatorversion "github.com/operator-framework/api/pkg/lib/version"
	operatorsv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	"github.com/stretchr/testify/mock"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"
	crfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	discoveryfake "k8s.io/client-go/discovery/fake"
	coretesting "k8s.io/client-go/testing"

	"github.com/opendatahub-io/odh-cli/pkg/deps"
	utilclient "github.com/opendatahub-io/odh-cli/pkg/util/client"
	mockclient "github.com/opendatahub-io/odh-cli/pkg/util/test/mocks/client"

	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

const (
	testOLMV1DependencyKey       = "certManager"
	testOLMV1DependencyPackage   = "cert-manager"
	testOLMV1DependencyNamespace = "cert-manager"
	testOLMV1ExtensionName       = "cert-manager-extension"
	testOLMV1DependencyVersion   = "1.14.0"
	testV0DependencyCSV          = "cert-manager.v1.14.0"
)

type dependencyCheckClient struct {
	utilclient.Client

	olm               utilclient.OLMReader
	discovery         discovery.DiscoveryInterface
	controllerRuntime crclient.Client
}

func (c *dependencyCheckClient) OLM() utilclient.OLMReader {
	return c.olm
}

func (c *dependencyCheckClient) Discovery() discovery.DiscoveryInterface {
	return c.discovery
}

func (c *dependencyCheckClient) ControllerRuntime() crclient.Client {
	return c.controllerRuntime
}

func newV0DependencyCheckClient(olmReader utilclient.OLMReader) utilclient.Client {
	fakeDiscovery := &discoveryfake.FakeDiscovery{Fake: &coretesting.Fake{}}
	fakeDiscovery.Resources = []*metav1.APIResourceList{{
		GroupVersion: "operators.coreos.com/v1alpha1",
		APIResources: []metav1.APIResource{{Name: "subscriptions", Kind: "Subscription"}},
	}}

	return &dependencyCheckClient{olm: olmReader, discovery: fakeDiscovery}
}

func newV1DependencyCheckClient(objects ...runtime.Object) *dependencyCheckClient {
	fakeDiscovery := &discoveryfake.FakeDiscovery{Fake: &coretesting.Fake{}}
	fakeDiscovery.Resources = []*metav1.APIResourceList{{
		GroupVersion: "olm.operatorframework.io/v1",
		APIResources: []metav1.APIResource{{Name: "clusterextensions", Kind: "ClusterExtension"}},
	}}

	scheme := runtime.NewScheme()
	for _, gvk := range []schema.GroupVersionKind{
		{Group: "operators.coreos.com", Version: "v2", Kind: "OperatorCondition"},
		{Group: "olm.operatorframework.io", Version: "v1", Kind: "ClusterExtension"},
	} {
		scheme.AddKnownTypeWithName(gvk, &unstructured.Unstructured{})
		scheme.AddKnownTypeWithName(gvk.GroupVersion().WithKind(gvk.Kind+"List"), &unstructured.UnstructuredList{})
	}

	controllerRuntime := crfake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(objects...).Build()

	return &dependencyCheckClient{discovery: fakeDiscovery, controllerRuntime: controllerRuntime}
}

func TestCheckDependencies_V0SubscriptionOnMixedOLM(t *testing.T) {
	for _, tt := range []struct {
		name         string
		enabled      string
		installedCSV string
		csvPhase     operatorsv1alpha1.ClusterServiceVersionPhase
		wantStatus   deps.Status
		wantVersion  string
	}{
		{name: "pending request", enabled: "true", wantStatus: deps.StatusInstalled},
		{name: "optional pending request", enabled: "auto", wantStatus: deps.StatusInstalled},
		{name: "CSV pending", enabled: "true", installedCSV: testV0DependencyCSV,
			csvPhase: operatorsv1alpha1.CSVPhasePending, wantStatus: deps.StatusInstalled,
			wantVersion: testOLMV1DependencyVersion},
		{name: "CSV succeeded", enabled: "true", installedCSV: testV0DependencyCSV,
			csvPhase: operatorsv1alpha1.CSVPhaseSucceeded, wantStatus: deps.StatusInstalled,
			wantVersion: testOLMV1DependencyVersion},
	} {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			mockSubReader := &mockclient.MockSubscriptionReader{}
			mockSubReader.On("List", mock.Anything, mock.Anything).
				Return(&operatorsv1alpha1.SubscriptionList{Items: []operatorsv1alpha1.Subscription{{
					ObjectMeta: metav1.ObjectMeta{Name: testOLMV1DependencyPackage, Namespace: testOLMV1DependencyNamespace},
					Spec:       &operatorsv1alpha1.SubscriptionSpec{Package: testOLMV1DependencyPackage},
					Status:     operatorsv1alpha1.SubscriptionStatus{InstalledCSV: tt.installedCSV},
				}}}, nil)

			mockCSVReader := &mockclient.MockCSVReader{}
			if tt.installedCSV == "" {
				mockCSVReader.On("List", mock.Anything, mock.Anything).
					Return(&operatorsv1alpha1.ClusterServiceVersionList{}, nil)
			} else {
				mockCSVReader.On("Get", mock.Anything, testV0DependencyCSV, mock.Anything).
					Return(&operatorsv1alpha1.ClusterServiceVersion{
						Spec: operatorsv1alpha1.ClusterServiceVersionSpec{
							Version: operatorversion.OperatorVersion{Version: semver.MustParse(testOLMV1DependencyVersion)},
						},
						Status: operatorsv1alpha1.ClusterServiceVersionStatus{Phase: tt.csvPhase},
					}, nil)
			}

			mockOLM := &mockclient.MockOLMReader{}
			mockOLM.On("Subscriptions", testOLMV1DependencyNamespace).Return(mockSubReader)
			mockOLM.On("ClusterServiceVersions", testOLMV1DependencyNamespace).Return(mockCSVReader)

			kubeClient := newV1DependencyCheckClient()
			fakeDiscovery := kubeClient.discovery.(*discoveryfake.FakeDiscovery)
			fakeDiscovery.Resources = append(fakeDiscovery.Resources, &metav1.APIResourceList{
				GroupVersion: "operators.coreos.com/v1alpha1",
				APIResources: []metav1.APIResource{{Name: "subscriptions", Kind: "Subscription"}},
			})
			kubeClient.olm = mockOLM
			manifest := &deps.Manifest{Dependencies: map[string]deps.Dependency{
				testOLMV1DependencyKey: {
					Enabled: tt.enabled,
					OLM: deps.OLMConfig{
						Name:      testOLMV1DependencyPackage,
						Namespace: testOLMV1DependencyNamespace,
					},
				},
			}}

			statuses, err := deps.CheckDependencies(t.Context(), kubeClient, manifest)

			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(statuses).To(HaveLen(1))
			g.Expect(statuses[0]).To(MatchFields(IgnoreExtras, Fields{
				"Status":  Equal(tt.wantStatus),
				"Version": Equal(tt.wantVersion),
			}))
			mockOLM.AssertExpectations(t)
			mockSubReader.AssertExpectations(t)
			mockCSVReader.AssertExpectations(t)
		})
	}
}

func TestCheckDependencies_OLMNotAvailable(t *testing.T) {
	g := NewWithT(t)

	mockOLM := &mockclient.MockOLMReader{}
	mockOLM.On("Available").Return(false)

	manifest := &deps.Manifest{
		Dependencies: map[string]deps.Dependency{
			"certManager": {Enabled: "true"},
		},
	}

	_, err := deps.CheckDependencies(t.Context(), newV0DependencyCheckClient(mockOLM), manifest)

	g.Expect(err).To(MatchError(deps.ErrOLMNotAvailable))
	mockOLM.AssertExpectations(t)
}

func TestCheckDependencies_MissingDependency(t *testing.T) {
	g := NewWithT(t)

	mockSubReader := &mockclient.MockSubscriptionReader{}
	mockSubReader.On("Get", mock.Anything, "cert-manager", mock.Anything).
		Return(nil, apierrors.NewNotFound(schema.GroupResource{Group: "operators.coreos.com", Resource: "subscriptions"}, "cert-manager"))

	mockOLM := &mockclient.MockOLMReader{}
	mockOLM.On("Available").Return(true)
	mockOLM.On("Subscriptions", "cert-manager").Return(mockSubReader)

	manifest := &deps.Manifest{
		Dependencies: map[string]deps.Dependency{
			"certManager": {
				Enabled: "true",
				OLM: deps.OLMConfig{
					Name:      "cert-manager",
					Namespace: "cert-manager",
				},
			},
		},
	}

	statuses, err := deps.CheckDependencies(t.Context(), newV0DependencyCheckClient(mockOLM), manifest)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(statuses).To(HaveLen(1))
	g.Expect(statuses[0].Status).To(Equal(deps.StatusMissing))

	mockOLM.AssertExpectations(t)
	mockSubReader.AssertExpectations(t)
}

func TestCheckDependencies_OptionalStatus(t *testing.T) {
	tests := []struct {
		name    string
		enabled string
		want    deps.Status
	}{
		{"auto is optional", "auto", deps.StatusOptional},
		{"false is optional", "false", deps.StatusOptional},
		{"true is missing", "true", deps.StatusMissing},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			mockSubReader := &mockclient.MockSubscriptionReader{}
			mockSubReader.On("Get", mock.Anything, "servicemeshoperator", mock.Anything).
				Return(nil, apierrors.NewNotFound(schema.GroupResource{Group: "operators.coreos.com", Resource: "subscriptions"}, "servicemeshoperator"))

			mockOLM := &mockclient.MockOLMReader{}
			mockOLM.On("Available").Return(true)
			mockOLM.On("Subscriptions", "openshift-operators").Return(mockSubReader)

			manifest := &deps.Manifest{
				Dependencies: map[string]deps.Dependency{
					"serviceMesh": {
						Enabled: tt.enabled,
						OLM: deps.OLMConfig{
							Name:      "servicemeshoperator",
							Namespace: "openshift-operators",
						},
					},
				},
			}

			statuses, err := deps.CheckDependencies(t.Context(), newV0DependencyCheckClient(mockOLM), manifest)

			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(statuses[0].Status).To(Equal(tt.want))

			mockOLM.AssertExpectations(t)
		})
	}
}

func TestCheckDependencies_Installed(t *testing.T) {
	g := NewWithT(t)

	mockSubReader := &mockclient.MockSubscriptionReader{}
	mockSubReader.On("Get", mock.Anything, "cert-manager", mock.Anything).
		Return(&operatorsv1alpha1.Subscription{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "cert-manager",
				Namespace: "cert-manager",
			},
			Status: operatorsv1alpha1.SubscriptionStatus{
				InstalledCSV: "cert-manager.v1.14.0",
			},
		}, nil)

	mockCSVReader := &mockclient.MockCSVReader{}
	mockCSVReader.On("Get", mock.Anything, "cert-manager.v1.14.0", mock.Anything).
		Return(nil, apierrors.NewNotFound(schema.GroupResource{Group: "operators.coreos.com", Resource: "clusterserviceversions"}, "cert-manager.v1.14.0"))
	mockCSVReader.On("List", mock.Anything, mock.Anything).
		Return(&operatorsv1alpha1.ClusterServiceVersionList{Items: []operatorsv1alpha1.ClusterServiceVersion{}}, nil)

	mockOLM := &mockclient.MockOLMReader{}
	mockOLM.On("Available").Return(true)
	mockOLM.On("Subscriptions", "cert-manager").Return(mockSubReader)
	mockOLM.On("ClusterServiceVersions", "cert-manager").Return(mockCSVReader)

	manifest := &deps.Manifest{
		Dependencies: map[string]deps.Dependency{
			"certManager": {
				Enabled: "true",
				OLM: deps.OLMConfig{
					Name:      "cert-manager",
					Namespace: "cert-manager",
				},
			},
		},
	}

	statuses, err := deps.CheckDependencies(t.Context(), newV0DependencyCheckClient(mockOLM), manifest)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(statuses).To(HaveLen(1))
	g.Expect(statuses[0].Status).To(Equal(deps.StatusInstalled))
	g.Expect(statuses[0].Name).To(Equal("certManager"))
	g.Expect(statuses[0].DisplayName).To(Equal("Cert Manager"))

	mockOLM.AssertExpectations(t)
}

func TestCheckDependencies_InstalledWithOLMV1(t *testing.T) {
	g := NewWithT(t)

	extension := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "olm.operatorframework.io/v1",
		"kind":       "ClusterExtension",
		"metadata": map[string]any{
			"name": testOLMV1ExtensionName,
		},
		"spec": map[string]any{
			"namespace": testOLMV1DependencyNamespace,
			"source": map[string]any{
				"sourceType": "Catalog",
				"catalog": map[string]any{
					"packageName": testOLMV1DependencyPackage,
				},
			},
		},
		"status": map[string]any{
			"conditions": []any{map[string]any{
				"type":   "Installed",
				"status": "True",
				"reason": "Succeeded",
			}},
			"install": map[string]any{
				"bundle": map[string]any{"version": testOLMV1DependencyVersion},
			},
		},
	}}

	manifest := &deps.Manifest{Dependencies: map[string]deps.Dependency{
		testOLMV1DependencyKey: {
			Enabled: "true",
			OLM: deps.OLMConfig{
				Name:      testOLMV1DependencyPackage,
				Namespace: testOLMV1DependencyNamespace,
			},
		},
	}}

	statuses, err := deps.CheckDependencies(
		t.Context(),
		newV1DependencyCheckClient(extension),
		manifest,
	)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(statuses).To(HaveLen(1))
	g.Expect(statuses[0]).To(MatchFields(IgnoreExtras, Fields{
		"Status":  Equal(deps.StatusInstalled),
		"Version": Equal(testOLMV1DependencyVersion),
	}))
}

func TestCheckDependencies_EmptyNamespace(t *testing.T) {
	g := NewWithT(t)

	mockOLM := &mockclient.MockOLMReader{}
	mockOLM.On("Available").Return(true)

	manifest := &deps.Manifest{
		Dependencies: map[string]deps.Dependency{
			"test": {
				Enabled: "true",
				OLM: deps.OLMConfig{
					Name:      "test-sub",
					Namespace: "",
				},
			},
		},
	}

	statuses, err := deps.CheckDependencies(t.Context(), newV0DependencyCheckClient(mockOLM), manifest)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(statuses[0].Status).To(Equal(deps.StatusMissing))

	mockOLM.AssertExpectations(t)
}

func TestCheckDependencies_EmptySubscription(t *testing.T) {
	g := NewWithT(t)

	mockOLM := &mockclient.MockOLMReader{}
	mockOLM.On("Available").Return(true)

	manifest := &deps.Manifest{
		Dependencies: map[string]deps.Dependency{
			"test": {
				Enabled: "true",
				OLM: deps.OLMConfig{
					Name:      "",
					Namespace: "test-ns",
				},
			},
		},
	}

	statuses, err := deps.CheckDependencies(t.Context(), newV0DependencyCheckClient(mockOLM), manifest)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(statuses[0].Status).To(Equal(deps.StatusMissing))

	mockOLM.AssertExpectations(t)
}

func TestNewDependencyList_EnvelopeFields(t *testing.T) {
	g := NewWithT(t)

	statuses := []deps.DependencyStatus{
		{Name: "certManager", DisplayName: "Cert Manager", Status: deps.StatusInstalled},
		{Name: "serviceMesh", DisplayName: "Service Mesh", Status: deps.StatusMissing},
	}

	list := deps.NewDependencyList(statuses)

	g.Expect(list.APIVersion).To(Equal("cli.opendatahub.io/v1"))
	g.Expect(list.Kind).To(Equal("DependencyList"))
	g.Expect(list.Metadata.Command).To(Equal("deps"))
	g.Expect(list.Metadata.CLIVersion).ToNot(BeEmpty())
	g.Expect(list.Metadata.GeneratedAt).ToNot(BeEmpty())
	g.Expect(list.Status).ToNot(BeNil())
	g.Expect(list.Dependencies).To(HaveLen(2))
}

func TestNewDependencyList_ComputesStatus(t *testing.T) {
	t.Run("success when all installed", func(t *testing.T) {
		g := NewWithT(t)

		statuses := []deps.DependencyStatus{
			{Name: "certManager", Status: deps.StatusInstalled},
			{Name: "serviceMesh", Status: deps.StatusOptional},
		}

		list := deps.NewDependencyList(statuses)

		g.Expect(list.Status.Result).To(Equal("success"))
		g.Expect(list.Status.Errors).To(Equal(0))
		g.Expect(list.Status.Warnings).To(Equal(0))
	})

	t.Run("failure when missing", func(t *testing.T) {
		g := NewWithT(t)

		statuses := []deps.DependencyStatus{
			{Name: "certManager", Status: deps.StatusMissing},
		}

		list := deps.NewDependencyList(statuses)

		g.Expect(list.Status.Result).To(Equal("failure"))
		g.Expect(list.Status.Errors).To(Equal(1))
	})

	t.Run("warning when unknown", func(t *testing.T) {
		g := NewWithT(t)

		statuses := []deps.DependencyStatus{
			{Name: "certManager", Status: deps.StatusUnknown},
		}

		list := deps.NewDependencyList(statuses)

		g.Expect(list.Status.Result).To(Equal("warning"))
		g.Expect(list.Status.Warnings).To(Equal(1))
	})
}
