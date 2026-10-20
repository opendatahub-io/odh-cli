package modelregistry_test

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
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/testutil"
	"github.com/opendatahub-io/odh-cli/pkg/lint/checks/components/modelregistry"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"

	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

//nolint:gochecknoglobals // Test fixture - shared across test functions
var listKinds = map[schema.GroupVersionResource]string{
	resources.DataScienceCluster.GVR():   resources.DataScienceCluster.ListKind(),
	resources.DataScienceClusterV1.GVR(): resources.DataScienceClusterV1.ListKind(),
}

const (
	testCurrentVersion = "3.5.0"
	testTargetVersion  = "3.6.0"
	testDSCName        = "default-dsc"
)

func TestRenamingCheck_Metadata(t *testing.T) {
	g := NewWithT(t)

	renamingCheck := modelregistry.NewRenamingCheck()

	g.Expect(renamingCheck.ID()).To(Equal("components.modelregistry.renaming"))
	g.Expect(renamingCheck.Name()).To(Equal("Components :: Model Registry :: AI Hub Renaming (DSC v3)"))
	g.Expect(renamingCheck.Group()).To(Equal(check.GroupComponent))
	g.Expect(renamingCheck.CheckKind()).To(Equal("modelregistry"))
	g.Expect(renamingCheck.CheckType()).To(Equal("renaming"))
	g.Expect(renamingCheck.Description()).ToNot(BeEmpty())
}

func TestRenamingCheck_CanApply_VersionMatrix(t *testing.T) {
	chk := modelregistry.NewRenamingCheck()
	dsc := testutil.NewDSC(map[string]string{"modelregistry": "Managed"})

	tests := []struct {
		name    string
		current string
		target  string
		want    bool
	}{
		{name: "3.5 to 3.6 applies", current: "3.5.0", target: "3.6.0", want: true},
		{name: "3.4 to 3.6 applies", current: "3.4.0", target: "3.6.0", want: true},
		{name: "2.19 to 3.6 applies", current: "2.19.0", target: "3.6.0", want: true},
		{name: "3.6 to 3.7 applies", current: "3.6.0", target: "3.7.0", want: true},
		{name: "3.4 to 3.5 does not apply", current: "3.4.0", target: "3.5.0", want: false},
		{name: "target 2.x does not apply", current: "2.19.0", target: "2.20.0", want: false},
		{name: "target 4.x does not apply", current: "3.5.0", target: "4.0.0", want: false},
		{name: "same version does not apply", current: "3.6.0", target: "3.6.0", want: false},
		{name: "same major/minor patch change does not apply", current: "3.6.1", target: "3.6.2", want: false},
		{name: "downgrade does not apply", current: "3.7.0", target: "3.5.0", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			target := testutil.NewTarget(t, testutil.TargetConfig{
				ListKinds:      listKinds,
				Objects:        []*unstructured.Unstructured{dsc},
				CurrentVersion: tt.current,
				TargetVersion:  tt.target,
			})

			canApply, err := chk.CanApply(t.Context(), target)

			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(canApply).To(Equal(tt.want))
		})
	}
}

func TestRenamingCheck_CanApply_NilVersions(t *testing.T) {
	g := NewWithT(t)

	chk := modelregistry.NewRenamingCheck()

	canApply, err := chk.CanApply(t.Context(), check.Target{})

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeFalse())
}

func TestRenamingCheck_CanApply_States(t *testing.T) {
	tests := []struct {
		name  string
		state string
		want  bool
	}{
		{name: "managed applies", state: "Managed", want: true},
		{name: "unmanaged does not apply", state: "Unmanaged", want: false},
		{name: "removed does not apply", state: "Removed", want: false},
		{name: "unmatched state does not apply", state: "Bogus", want: false},
		{name: "empty state does not apply", state: "", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			target := testutil.NewTarget(t, testutil.TargetConfig{
				ListKinds:      listKinds,
				Objects:        []*unstructured.Unstructured{testutil.NewDSC(map[string]string{"modelregistry": tt.state})},
				CurrentVersion: testCurrentVersion,
				TargetVersion:  testTargetVersion,
			})

			chk := modelregistry.NewRenamingCheck()
			canApply, err := chk.CanApply(t.Context(), target)

			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(canApply).To(Equal(tt.want))
		})
	}
}

func TestRenamingCheck_CanApply_ComponentAbsent(t *testing.T) {
	g := NewWithT(t)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{testutil.NewDSC(map[string]string{"kserve": "Managed"})},
		CurrentVersion: testCurrentVersion,
		TargetVersion:  testTargetVersion,
	})

	chk := modelregistry.NewRenamingCheck()
	canApply, err := chk.CanApply(t.Context(), target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeFalse())
}

func TestRenamingCheck_CanApply_InapplicableVersionSkipsClusterAccess(t *testing.T) {
	g := NewWithT(t)

	chk := modelregistry.NewRenamingCheck()

	// Version-inapplicable targets short-circuit before the DSC read, so a
	// cluster without any DSC still yields no error.
	canApply, err := chk.CanApply(t.Context(), testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		CurrentVersion: "3.4.0",
		TargetVersion:  "3.5.0",
	}))

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(canApply).To(BeFalse())
}

func TestRenamingCheck_CanApply_NoDSC(t *testing.T) {
	g := NewWithT(t)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		CurrentVersion: testCurrentVersion,
		TargetVersion:  testTargetVersion,
	})

	chk := modelregistry.NewRenamingCheck()
	canApply, err := chk.CanApply(t.Context(), target)

	g.Expect(err).To(HaveOccurred())
	g.Expect(canApply).To(BeFalse())
}

func TestRenamingCheck_ManagedReportsRename(t *testing.T) {
	g := NewWithT(t)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{testutil.NewDSC(map[string]string{"modelregistry": "Managed"})},
		CurrentVersion: testCurrentVersion,
		TargetVersion:  testTargetVersion,
	})

	renamingCheck := modelregistry.NewRenamingCheck()
	dr, err := renamingCheck.Validate(t.Context(), target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(dr.ImpactedObjects).To(BeEmpty())
	g.Expect(dr.Status.Conditions).To(HaveLen(1))
	g.Expect(dr.Status.Conditions[0]).To(MatchFields(IgnoreExtras, Fields{
		"Condition": MatchFields(IgnoreExtras, Fields{
			"Type":   Equal(check.ConditionTypeCompatible),
			"Status": Equal(metav1.ConditionTrue),
			"Reason": Equal(check.ReasonComponentRenamed),
			"Message": And(
				ContainSubstring("renamed to AI Hub in DSC v3"),
				ContainSubstring("Managed"),
			),
		}),
		"Impact":      Equal(result.ImpactNone),
		"Remediation": ContainSubstring("No action required"),
	}))
	g.Expect(dr.Annotations).To(And(
		HaveKeyWithValue(check.AnnotationComponentManagementState, "Managed"),
		HaveKeyWithValue(check.AnnotationCheckTargetVersion, testTargetVersion),
	))
}

func TestRenamingCheck_MessageIsConcise(t *testing.T) {
	g := NewWithT(t)

	dsc := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": resources.DataScienceCluster.APIVersion(),
		"kind":       resources.DataScienceCluster.Kind,
		"metadata":   map[string]any{"name": testDSCName},
		"spec": map[string]any{"components": map[string]any{
			"modelregistry": map[string]any{
				"managementState":     "Managed",
				"registriesNamespace": "modelregistry-system",
			},
		}},
	}}
	original := dsc.DeepCopy()

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{dsc},
		CurrentVersion: testCurrentVersion,
		TargetVersion:  testTargetVersion,
	})

	renamingCheck := modelregistry.NewRenamingCheck()
	dr, err := renamingCheck.Validate(t.Context(), target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(dr.Status.Conditions).To(HaveLen(1))
	g.Expect(dr.Status.Conditions[0].Message).To(Equal(
		"Model Registry component (state: Managed) is renamed to AI Hub in DSC v3, available from RHOAI 3.6",
	))
	// The check is read-only and must not invent a default namespace.
	g.Expect(dsc.Object).To(Equal(original.Object))
}

func TestRenamingCheck_UnconfiguredStatesSkipRename(t *testing.T) {
	tests := []struct {
		name string
		dsc  *unstructured.Unstructured
	}{
		{name: "component absent", dsc: testutil.NewDSC(map[string]string{"kserve": "Managed"})},
		{name: "unmanaged", dsc: testutil.NewDSC(map[string]string{"modelregistry": "Unmanaged"})},
		{name: "removed", dsc: testutil.NewDSC(map[string]string{"modelregistry": "Removed"})},
		{name: "empty state", dsc: testutil.NewDSC(map[string]string{"modelregistry": ""})},
		{name: "unmatched state", dsc: testutil.NewDSC(map[string]string{"modelregistry": "Bogus"})},
		{name: "null state", dsc: &unstructured.Unstructured{Object: map[string]any{
			"apiVersion": resources.DataScienceCluster.APIVersion(),
			"kind":       resources.DataScienceCluster.Kind,
			"metadata":   map[string]any{"name": testDSCName},
			"spec": map[string]any{"components": map[string]any{
				"modelregistry": map[string]any{"managementState": nil},
			}},
		}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			target := testutil.NewTarget(t, testutil.TargetConfig{
				ListKinds:      listKinds,
				Objects:        []*unstructured.Unstructured{tt.dsc},
				CurrentVersion: testCurrentVersion,
				TargetVersion:  testTargetVersion,
			})

			renamingCheck := modelregistry.NewRenamingCheck()
			dr, err := renamingCheck.Validate(t.Context(), target)

			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(dr.Status.Conditions).To(HaveLen(1))
			g.Expect(dr.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
				"Type":   Equal(check.ConditionTypeConfigured),
				"Status": Equal(metav1.ConditionTrue),
				"Reason": Equal(check.ReasonRequirementsMet),
			}))
			for _, cond := range dr.Status.Conditions {
				g.Expect(cond.Reason).ToNot(Equal(check.ReasonComponentRenamed))
			}
		})
	}
}

func TestRenamingCheck_NonStringStateFails(t *testing.T) {
	g := NewWithT(t)

	dsc := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": resources.DataScienceCluster.APIVersion(),
		"kind":       resources.DataScienceCluster.Kind,
		"metadata":   map[string]any{"name": testDSCName},
		"spec": map[string]any{"components": map[string]any{
			"modelregistry": map[string]any{"managementState": int64(42)},
		}},
	}}

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		Objects:        []*unstructured.Unstructured{dsc},
		CurrentVersion: testCurrentVersion,
		TargetVersion:  testTargetVersion,
	})

	renamingCheck := modelregistry.NewRenamingCheck()
	dr, err := renamingCheck.Validate(t.Context(), target)

	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("managementState"))
	g.Expect(dr).To(BeNil())
}

func TestRenamingCheck_MissingDSC(t *testing.T) {
	g := NewWithT(t)

	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds:      listKinds,
		CurrentVersion: testCurrentVersion,
		TargetVersion:  testTargetVersion,
	})

	renamingCheck := modelregistry.NewRenamingCheck()
	dr, err := renamingCheck.Validate(t.Context(), target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(dr.Status.Conditions).To(HaveLen(1))
	g.Expect(dr.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Type":    Equal(check.ConditionTypeAvailable),
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonResourceNotFound),
		"Message": Equal("No DataScienceCluster found"),
	}))
	for _, cond := range dr.Status.Conditions {
		g.Expect(cond.Reason).ToNot(Equal(check.ReasonComponentRenamed))
	}
}

// newFailingListTarget builds a target whose DataScienceCluster list always
// fails with the given error, using the existing client test constructors.
func newFailingListTarget(t *testing.T, listErr error) check.Target {
	t.Helper()

	scheme := runtime.NewScheme()
	_ = metav1.AddMetaToScheme(scheme)

	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, listKinds)
	dynamicClient.PrependReactor("list", "datascienceclusters", func(_ k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, listErr
	})

	current := semver.MustParse(testCurrentVersion)
	target := semver.MustParse(testTargetVersion)

	return check.Target{
		Client:         client.NewForTesting(client.TestClientConfig{Dynamic: dynamicClient}),
		CurrentVersion: &current,
		TargetVersion:  &target,
	}
}

func TestRenamingCheck_ForbiddenListSuppressed(t *testing.T) {
	g := NewWithT(t)

	// Permission errors are suppressed by the shared client into an empty list,
	// which the builder reports as a missing resource with no error.
	forbidden := apierrors.NewForbidden(
		schema.GroupResource{Group: resources.DataScienceCluster.Group, Resource: "datascienceclusters"},
		"",
		errors.New("denied"),
	)
	target := newFailingListTarget(t, forbidden)

	renamingCheck := modelregistry.NewRenamingCheck()
	dr, err := renamingCheck.Validate(t.Context(), target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(dr.Status.Conditions).To(HaveLen(1))
	g.Expect(dr.Status.Conditions[0].Reason).To(Equal(check.ReasonResourceNotFound))
	for _, cond := range dr.Status.Conditions {
		g.Expect(cond.Reason).ToNot(Equal(check.ReasonComponentRenamed))
	}
}

func TestRenamingCheck_ListErrorPropagates(t *testing.T) {
	g := NewWithT(t)

	target := newFailingListTarget(t, errors.New("boom"))

	renamingCheck := modelregistry.NewRenamingCheck()
	dr, err := renamingCheck.Validate(t.Context(), target)

	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("DataScienceCluster"))
	g.Expect(dr).To(BeNil())
}
