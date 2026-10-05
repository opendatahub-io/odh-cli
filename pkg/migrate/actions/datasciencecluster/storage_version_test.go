package datasciencecluster_test

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/blang/semver/v4"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	extensionsfake "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/fake"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	discoveryfake "k8s.io/client-go/discovery/fake"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	clienttesting "k8s.io/client-go/testing"

	"github.com/opendatahub-io/odh-cli/pkg/migrate/action"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/action/result"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/actions/datasciencecluster"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
	"github.com/opendatahub-io/odh-cli/pkg/util/iostreams"
	"github.com/opendatahub-io/odh-cli/pkg/util/jq"

	. "github.com/onsi/gomega"
)

const (
	requestName              = "dsc-storage-migration"
	storageVersionAnnotation = "odh-cli.opendatahub.io/storage-version"
	declined                 = "n\n"

	// Long enough for every non-waiting path, short enough for the pending case.
	failureTimeout = 200 * time.Millisecond
)

//nolint:gochecknoglobals // Test fixtures - shared across test functions
var (
	historyV1   = []string{"v1"}
	historyV1V2 = []string{"v1", "v2"}
	historyV2V3 = []string{"v2", "v3"}
	historyV2   = []string{"v2"}

	migrationSucceeded = metav1.Condition{
		Type:   "Succeeded",
		Status: metav1.ConditionTrue,
		Reason: "Complete",
	}

	migrationFailed = metav1.Condition{
		Type:    "Failed",
		Status:  metav1.ConditionTrue,
		Reason:  "RewriteError",
		Message: "blocked",
	}

	allAPIs = []resources.ResourceType{
		resources.OpenShiftStorageVersionMigration,
		resources.StorageVersionMigrationAlpha,
		resources.StorageVersionMigrationBeta,
		resources.StorageVersionMigration,
	}
)

func TestAction(t *testing.T) {
	g := NewWithT(t)

	migration := &datasciencecluster.StorageVersionAction{}
	target := semver.MustParse("3.6.0")

	g.Expect(migration.ID()).To(Equal("dsc.storage-version.migrate"))
	g.Expect(migration.Phase()).To(Equal(action.PhasePreUpgrade))
	g.Expect(migration.Prepare()).To(BeNil())
	g.Expect(migration.CanApply(action.Target{TargetVersion: &target})).To(BeTrue())
	g.Expect(migration.CanApply(action.Target{})).To(BeFalse())
}

func TestExecuteWithoutWrites(t *testing.T) {
	tests := []struct {
		name     string
		storage  string
		history  []string
		dryRun   bool
		declined bool
	}{
		{
			name:    "v1 only",
			storage: "v1",
			history: historyV1,
		},
		{
			name:    "already current",
			storage: "v2",
			history: historyV2,
		},
		{
			name:    "dry run",
			storage: "v2",
			history: historyV1V2,
			dryRun:  true,
		},
		{
			name:     "confirmation declined",
			storage:  "v2",
			history:  historyV1V2,
			declined: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			f := newFixture(t, tc.storage, tc.history, resources.StorageVersionMigrationBeta)
			f.target.DryRun = tc.dryRun
			f.target.SkipConfirm = !tc.declined

			dr, err := f.run(t.Context())

			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(dr.HasFailedSteps()).To(BeFalse())
			g.Expect(f.requests.Actions()).To(BeEmpty())
			g.Expect(f.history(t)).To(Equal(tc.history))
		})
	}
}

func TestExecuteMigrates(t *testing.T) {
	tests := []struct {
		name    string
		storage string
		history []string
		served  []resources.ResourceType
		want    resources.ResourceType
		// wantVersion is set only for the APIs whose request names a version.
		wantVersion string
	}{
		{
			name:    "kubernetes v1",
			storage: "v2",
			history: historyV1V2,
			served:  []resources.ResourceType{resources.StorageVersionMigration},
			want:    resources.StorageVersionMigration,
		},
		{
			name:    "kubernetes v1beta1",
			storage: "v2",
			history: historyV1V2,
			served:  []resources.ResourceType{resources.StorageVersionMigrationBeta},
			want:    resources.StorageVersionMigrationBeta,
		},
		{
			name:        "kubernetes v1alpha1",
			storage:     "v2",
			history:     historyV1V2,
			served:      []resources.ResourceType{resources.StorageVersionMigrationAlpha},
			want:        resources.StorageVersionMigrationAlpha,
			wantVersion: "v2",
		},
		{
			name:        "openshift v1alpha1 from v2 to v3",
			storage:     "v3",
			history:     historyV2V3,
			served:      []resources.ResourceType{resources.OpenShiftStorageVersionMigration},
			want:        resources.OpenShiftStorageVersionMigration,
			wantVersion: "v3",
		},
		{
			name:    "prefers kubernetes v1",
			storage: "v2",
			history: historyV1V2,
			served:  allAPIs,
			want:    resources.StorageVersionMigration,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			f := newFixture(t, tc.storage, tc.history, tc.served...)
			f.controllerReports(migrationSucceeded)

			dr, err := f.run(t.Context())

			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(dr.HasFailedSteps()).To(BeFalse())
			g.Expect(f.history(t)).To(Equal([]string{tc.storage}))
			g.Expect(f.requestExists(tc.want)).To(BeFalse())

			wantResource := map[string]string{
				"group":    resources.DataScienceCluster.Group,
				"resource": resources.DataScienceCluster.Resource,
			}
			if tc.wantVersion != "" {
				wantResource["version"] = tc.wantVersion
			}

			create := f.created(t)
			request := create.GetObject().(*unstructured.Unstructured)

			g.Expect(create.GetResource()).To(Equal(tc.want.GVR()))
			g.Expect(request.GetName()).To(Equal(requestName))
			g.Expect(request.GetAnnotations()).To(HaveKeyWithValue(storageVersionAnnotation, tc.storage))
			g.Expect(jq.Query[map[string]string](request, ".spec.resource")).To(Equal(wantResource))
		})
	}
}

func TestExecuteReusesExistingRequest(t *testing.T) {
	g := NewWithT(t)

	api := resources.StorageVersionMigrationBeta
	f := newFixture(t, "v2", historyV1V2, api)

	f.existingRequest(t, api, "v2", migrationSucceeded)

	_, err := f.run(t.Context())

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(f.history(t)).To(Equal(historyV2))
	g.Expect(f.requestExists(api)).To(BeFalse())
}

func TestExecuteFailures(t *testing.T) {
	tests := []struct {
		name        string
		noAPI       bool
		setup       func(t *testing.T, f *fixture)
		wantErr     string
		wantHistory []string
		wantRequest bool
	}{
		{
			name:        "no migration API served",
			noAPI:       true,
			wantErr:     "no supported StorageVersionMigration API",
			wantHistory: historyV1V2,
		},
		{
			name:        "CRD read forbidden",
			setup:       forbidCRDRead,
			wantErr:     "reading DataScienceCluster CRD",
			wantHistory: historyV1V2,
		},
		{
			name:        "discovery fails",
			setup:       forbidDiscovery,
			wantErr:     "discovering storage migration APIs",
			wantHistory: historyV1V2,
		},
		{
			name:        "existing request is for another storage version",
			setup:       staleRequest,
			wantErr:     `is for storage version "v1", not "v2"`,
			wantHistory: historyV1V2,
			wantRequest: true,
		},
		{
			name:        "request create forbidden",
			setup:       forbidRequestCreate,
			wantErr:     "forbidden",
			wantHistory: historyV1V2,
		},
		{
			name:        "controller reports failure",
			setup:       controllerFails,
			wantErr:     "migration failed: RewriteError: blocked",
			wantHistory: historyV1V2,
			wantRequest: true,
		},
		{
			name:        "controller never finishes",
			wantErr:     "retained for diagnosis",
			wantHistory: historyV1V2,
			wantRequest: true,
		},
		{
			name:        "CRD status update forbidden",
			setup:       forbidCRDStatusUpdate,
			wantErr:     "updating DataScienceCluster CRD storedVersions",
			wantHistory: historyV1V2,
			wantRequest: true,
		},
		{
			name:        "request delete forbidden",
			setup:       forbidRequestDelete,
			wantErr:     "deleting completed migration request",
			wantHistory: historyV2,
			wantRequest: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			api := resources.StorageVersionMigrationBeta
			served := []resources.ResourceType{api}
			if tc.noAPI {
				served = nil
			}

			f := newFixture(t, "v2", historyV1V2, served...)
			if tc.setup != nil {
				tc.setup(t, f)
			}

			ctx, cancel := context.WithTimeout(t.Context(), failureTimeout)
			defer cancel()

			dr, err := f.run(ctx)

			g.Expect(err).To(MatchError(ContainSubstring(tc.wantErr)))
			g.Expect(dr.HasFailedSteps()).To(BeTrue())
			g.Expect(f.history(t)).To(Equal(tc.wantHistory))
			g.Expect(f.requestExists(api)).To(Equal(tc.wantRequest))
		})
	}
}

// staleRequest leaves a succeeded request from an earlier storage version behind.
func staleRequest(t *testing.T, f *fixture) {
	t.Helper()

	f.existingRequest(t, resources.StorageVersionMigrationBeta, "v1", migrationSucceeded)
}

func forbidCRDRead(_ *testing.T, f *fixture) {
	f.crds.PrependReactor("get", "*", forbidden)
}

func forbidDiscovery(_ *testing.T, f *fixture) {
	f.discovery.PrependReactor("get", "*", forbidden)
}

func forbidRequestCreate(_ *testing.T, f *fixture) {
	f.requests.PrependReactor("create", "*", forbidden)
}

func controllerFails(_ *testing.T, f *fixture) {
	f.controllerReports(migrationFailed)
}

func forbidCRDStatusUpdate(_ *testing.T, f *fixture) {
	f.controllerReports(migrationSucceeded)
	f.crds.PrependReactor("update", "*", forbidden)
}

func forbidRequestDelete(_ *testing.T, f *fixture) {
	f.controllerReports(migrationSucceeded)
	f.requests.PrependReactor("delete", "*", forbidden)
}

func forbidden(operation clienttesting.Action) (bool, runtime.Object, error) {
	resource := operation.GetResource().GroupResource()

	return true, nil, apierrors.NewForbidden(resource, requestName, nil)
}

// fixture is a DataScienceCluster CRD plus the fake clients the action talks to.
type fixture struct {
	target    action.Target
	crds      *extensionsfake.Clientset
	requests  *dynamicfake.FakeDynamicClient
	discovery *discoveryfake.FakeDiscovery
}

func newFixture(
	t *testing.T,
	storage string,
	history []string,
	served ...resources.ResourceType,
) *fixture {
	t.Helper()

	crd := &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name: resources.DataScienceCluster.CRDFQN(),
		},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Versions: []apiextensionsv1.CustomResourceDefinitionVersion{
				{
					Name:    storage,
					Served:  true,
					Storage: true,
				},
			},
		},
		Status: apiextensionsv1.CustomResourceDefinitionStatus{
			StoredVersions: history,
		},
	}

	f := &fixture{
		crds:     extensionsfake.NewSimpleClientset(crd),
		requests: dynamicfake.NewSimpleDynamicClient(runtime.NewScheme()),
		discovery: &discoveryfake.FakeDiscovery{
			Fake: &clienttesting.Fake{},
		},
	}

	for _, api := range served {
		f.discovery.Resources = append(f.discovery.Resources, &metav1.APIResourceList{
			GroupVersion: api.APIVersion(),
			APIResources: []metav1.APIResource{
				{
					Name: api.Resource,
					Kind: api.Kind,
				},
			},
		})
	}

	f.target = action.Target{
		Client: client.NewForTesting(client.TestClientConfig{
			APIExtensions: f.crds,
			Dynamic:       f.requests,
			Discovery:     f.discovery,
		}),
		Recorder:    action.NewRootRecorder(),
		SkipConfirm: true,
		IO:          iostreams.NewIOStreams(strings.NewReader(declined), io.Discard, io.Discard),
	}

	return f
}

func (f *fixture) run(ctx context.Context) (*result.ActionResult, error) {
	task := (&datasciencecluster.StorageVersionAction{}).Run()

	return task.Execute(ctx, f.target)
}

// controllerReports stands in for the migration controller: every request the
// action creates is stored with the given condition already set.
func (f *fixture) controllerReports(condition metav1.Condition) {
	f.requests.PrependReactor("create", "*", func(operation clienttesting.Action) (bool, runtime.Object, error) {
		create := operation.(clienttesting.CreateAction)
		request := create.GetObject().(*unstructured.Unstructured)

		setCondition(request, condition)

		// Not handled, so the default reactor stores the updated request.
		return false, nil, nil
	})
}

// history reads the tracker directly so it still works when API reads are made to fail.
func (f *fixture) history(t *testing.T) []string {
	t.Helper()

	gvr := apiextensionsv1.SchemeGroupVersion.WithResource("customresourcedefinitions")

	obj, err := f.crds.Tracker().Get(gvr, "", resources.DataScienceCluster.CRDFQN())
	NewWithT(t).Expect(err).ToNot(HaveOccurred())

	crd := obj.(*apiextensionsv1.CustomResourceDefinition)

	return crd.Status.StoredVersions
}

// existingRequest puts a request in the cluster as an earlier run would have
// left it: created for the given storage version and carrying a condition.
func (f *fixture) existingRequest(
	t *testing.T,
	api resources.ResourceType,
	storage string,
	condition metav1.Condition,
) {
	t.Helper()

	request := &unstructured.Unstructured{}
	request.SetGroupVersionKind(api.GVK())
	request.SetName(requestName)
	request.SetAnnotations(map[string]string{
		storageVersionAnnotation: storage,
	})

	setCondition(request, condition)

	NewWithT(t).Expect(f.requests.Tracker().Add(request)).To(Succeed())
}

func (f *fixture) requestExists(api resources.ResourceType) bool {
	_, err := f.requests.Tracker().Get(api.GVR(), "", requestName)

	return err == nil
}

func (f *fixture) created(t *testing.T) clienttesting.CreateAction {
	t.Helper()

	for _, operation := range f.requests.Actions() {
		if operation.GetVerb() == "create" {
			return operation.(clienttesting.CreateAction)
		}
	}

	t.Fatal("no migration request was created")

	return nil
}

func setCondition(request *unstructured.Unstructured, condition metav1.Condition) {
	conditions := []any{
		map[string]any{
			"type":    condition.Type,
			"status":  string(condition.Status),
			"reason":  condition.Reason,
			"message": condition.Message,
		},
	}

	request.Object["status"] = map[string]any{
		"conditions": conditions,
	}
}
