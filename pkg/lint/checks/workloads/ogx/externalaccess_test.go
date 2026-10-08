package ogx_test

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/opendatahub-io/odh-cli/pkg/lint/check"
	resultpkg "github.com/opendatahub-io/odh-cli/pkg/lint/check/result"
	"github.com/opendatahub-io/odh-cli/pkg/lint/check/testutil"
	"github.com/opendatahub-io/odh-cli/pkg/lint/checks/workloads/ogx"
	"github.com/opendatahub-io/odh-cli/pkg/resources"

	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

const (
	testNamespace   = "test-ns"
	testHostname    = "ogx.apps.example.com"
	testTLSSecret   = "ogx-tls" //nolint:gosec // Secret name in test data, not a credential.
	currentVersion  = "3.5.0"
	targetVersion   = "3.6.0"
	managedState    = "Managed"
	removedState    = "Removed"
	componentOGXKey = "ogx"
)

//nolint:gochecknoglobals // Shared immutable test specifications.
var (
	listKinds = map[schema.GroupVersionResource]string{
		resources.OGXServer.GVR():          resources.OGXServer.ListKind(),
		resources.DataScienceCluster.GVR(): resources.DataScienceCluster.ListKind(),
	}

	enabledNoHostnameNoTLSSpec = map[string]any{
		"network": map[string]any{
			"externalAccess": map[string]any{"enabled": true},
		},
	}
	enabledHostnameNoTLSSpec = map[string]any{
		"network": map[string]any{
			"externalAccess": map[string]any{"enabled": true, "hostname": testHostname},
		},
	}
	enabledHostnameAndTLSSpec = map[string]any{
		"network": map[string]any{
			"externalAccess": map[string]any{
				"enabled":  true,
				"hostname": testHostname,
				"tls":      map[string]any{"secretName": testTLSSecret},
			},
		},
	}
	disabledSpec = map[string]any{
		"network": map[string]any{
			"externalAccess": map[string]any{"enabled": false},
		},
	}
	noNetworkSpec = map[string]any{}
)

func managedOGXDSC() *unstructured.Unstructured {
	return testutil.NewDSC(map[string]string{componentOGXKey: managedState})
}

func newOGXServer(name string, spec map[string]any) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.OGXServer.APIVersion(),
			"kind":       resources.OGXServer.Kind,
			"metadata": map[string]any{
				"name":      name,
				"namespace": testNamespace,
			},
			"spec": spec,
		},
	}
}

func TestExternalAccessTLSCheck_Metadata(t *testing.T) {
	g := NewWithT(t)

	chk := ogx.NewExternalAccessTLSCheck()

	g.Expect(chk.ID()).To(Equal("workloads.ogx.externalaccess-tls"))
	g.Expect(chk.Name()).To(Equal("Workloads :: OGX :: External Access TLS (3.6+)"))
	g.Expect(chk.Group()).To(Equal(check.GroupWorkload))
	g.Expect(chk.CheckKind()).To(Equal(componentOGXKey))
	g.Expect(chk.Description()).ToNot(BeEmpty())
}

func TestExternalAccessTLSCheck_CanApply(t *testing.T) {
	tests := []struct {
		name           string
		currentVersion string
		targetVersion  string
		state          string
		want           bool
	}{
		{name: "upgrade to 3.6 with managed OGX", currentVersion: currentVersion, targetVersion: targetVersion, state: managedState, want: true},
		{name: "target below 3.6", currentVersion: currentVersion, targetVersion: currentVersion, state: managedState, want: false},
		{name: "current already 3.6", currentVersion: targetVersion, targetVersion: targetVersion, state: managedState, want: false},
		{name: "OGX removed", currentVersion: currentVersion, targetVersion: targetVersion, state: removedState, want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)
			target := testutil.NewTarget(t, testutil.TargetConfig{
				ListKinds:      listKinds,
				Objects:        []*unstructured.Unstructured{testutil.NewDSC(map[string]string{componentOGXKey: tc.state})},
				CurrentVersion: tc.currentVersion,
				TargetVersion:  tc.targetVersion,
			})

			canApply, err := ogx.NewExternalAccessTLSCheck().CanApply(t.Context(), target)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(canApply).To(Equal(tc.want))
		})
	}
}

func TestExternalAccessTLSCheck_MissingHostnameOrTLSIsAdvisory(t *testing.T) {
	g := NewWithT(t)
	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds: listKinds,
		Objects: []*unstructured.Unstructured{
			managedOGXDSC(),
			newOGXServer("no-hostname-no-tls", enabledNoHostnameNoTLSSpec),
			newOGXServer("hostname-no-tls", enabledHostnameNoTLSSpec),
			newOGXServer("hostname-and-tls", enabledHostnameAndTLSSpec),
		},
		CurrentVersion: currentVersion,
		TargetVersion:  targetVersion,
	})

	diagnostic, err := ogx.NewExternalAccessTLSCheck().Validate(t.Context(), target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(diagnostic.Status.Conditions).To(HaveLen(1))
	g.Expect(diagnostic.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Status":  Equal(metav1.ConditionFalse),
		"Reason":  Equal(check.ReasonWorkloadsImpacted),
		"Message": ContainSubstring("Found 2 OGXServer(s)"),
	}))
	g.Expect(diagnostic.Status.Conditions[0].Impact).To(Equal(resultpkg.ImpactAdvisory))
	g.Expect(diagnostic.ImpactedObjects).To(ConsistOf(
		HaveField("ObjectMeta.Name", "no-hostname-no-tls"),
		HaveField("ObjectMeta.Name", "hostname-no-tls"),
	))
}

func TestExternalAccessTLSCheck_CompliantOrDisabledDoNotMatch(t *testing.T) {
	g := NewWithT(t)
	target := testutil.NewTarget(t, testutil.TargetConfig{
		ListKinds: listKinds,
		Objects: []*unstructured.Unstructured{
			managedOGXDSC(),
			newOGXServer("hostname-and-tls", enabledHostnameAndTLSSpec),
			newOGXServer("disabled", disabledSpec),
			newOGXServer("no-network", noNetworkSpec),
		},
		CurrentVersion: currentVersion,
		TargetVersion:  targetVersion,
	})

	diagnostic, err := ogx.NewExternalAccessTLSCheck().Validate(t.Context(), target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(diagnostic.Status.Conditions).To(HaveLen(1))
	g.Expect(diagnostic.Status.Conditions[0].Condition).To(MatchFields(IgnoreExtras, Fields{
		"Status": Equal(metav1.ConditionTrue),
		"Reason": Equal(check.ReasonVersionCompatible),
	}))
	g.Expect(diagnostic.ImpactedObjects).To(BeEmpty())
}
