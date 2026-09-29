package rhbok_test

import (
	"encoding/json"
	"testing"

	crclient "sigs.k8s.io/controller-runtime/pkg/client"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/opendatahub-io/odh-cli/pkg/migrate/action"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/action/result"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/actions/kueue/rhbok"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/jq"

	. "github.com/onsi/gomega"
)

const (
	testRHBOKV1ServiceAccount = "rhbok-installer"
	testRHBOKV1Channel        = "stable-v1.2"
	testRHBOKV1Catalog        = "openshift-redhat-operators"
	testRHBOKV1NamespacePath  = ".spec.namespace"
	testRHBOKV1AccountPath    = ".spec.serviceAccount.name"
	testRHBOKV1PackagePath    = ".spec.source.catalog.packageName"
	testRHBOKV1CatalogPath    = `.spec.source.catalog.selector.matchLabels["olm.operatorframework.io/metadata.name"]`
	testRHBOKV1ChannelPath    = ".spec.source.catalog.channels[0]"
)

const (
	testRHBOKV1FailedMessage    = "bundle unpack failed"
	testRHBOKV1BlockedMessage   = "upgrade requires manual intervention"
	testRHBOKV1SuccessMessage   = "installed and ready"
	testRHBOKV1FailedCase       = "terminal installation failure"
	testRHBOKV1BlockedCase      = "blocked upgrade of installed bundle"
	testRHBOKV1SuccessCase      = "installed bundle"
	testRHBOKV1InstallStep      = "install-rhbok-operator"
	testRHBOKV1InstallDesc      = "Install Red Hat Build of Kueue Operator"
	testRHBOKV1FailedConditions = `[{"type":"Installed","status":"False","reason":"Failed","message":"` +
		testRHBOKV1FailedMessage + `"}]`
	testRHBOKV1BlockedConditions = `[{"type":"Installed","status":"True","reason":"Succeeded"},` +
		`{"type":"Progressing","status":"False","reason":"Blocked","message":"` + testRHBOKV1BlockedMessage + `"}]`
	testRHBOKV1SuccessConditions = `[{"type":"Installed","status":"True","reason":"Succeeded"}]`
)

func TestInstallRHBOKClusterExtensionStatus(t *testing.T) {
	tests := []struct {
		name        string
		conditions  string
		wantStatus  result.StepStatus
		wantMessage string
	}{
		{
			name:        testRHBOKV1FailedCase,
			conditions:  testRHBOKV1FailedConditions,
			wantStatus:  result.StepFailed,
			wantMessage: testRHBOKV1FailedMessage,
		},
		{
			name:        testRHBOKV1BlockedCase,
			conditions:  testRHBOKV1BlockedConditions,
			wantStatus:  result.StepFailed,
			wantMessage: testRHBOKV1BlockedMessage,
		},
		{
			name:        testRHBOKV1SuccessCase,
			conditions:  testRHBOKV1SuccessConditions,
			wantStatus:  result.StepCompleted,
			wantMessage: testRHBOKV1SuccessMessage,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)
			extension := newInstalledClusterExtension(
				rhbok.ExportSubscriptionName, rhbok.ExportSubscriptionName, testRHBOKOLMV1Version,
			)
			var conditions []any
			g.Expect(json.Unmarshal([]byte(tc.conditions), &conditions)).To(Succeed())
			g.Expect(unstructured.SetNestedSlice(extension.Object, conditions, "status", "conditions")).To(Succeed())

			target := newTarget(t, nil, targetOpts{controllerObjs: []crclient.Object{extension}})
			g.Expect(rhbok.ExportRHBOKInstalledViaClusterExtension(t.Context(), target)).To(
				Equal(tc.wantStatus == result.StepCompleted),
			)
			step := target.Recorder.Child(testRHBOKV1InstallStep, testRHBOKV1InstallDesc)
			rhbok.ExportInstallRHBOKClusterExtension(
				&rhbok.RHBOKMigrationAction{}, t.Context(), target, "", step,
			)

			res := target.Recorder.(action.RootRecorder).Build()
			g.Expect(res.Status.Steps).To(HaveLen(1))
			g.Expect(res.Status.Steps[0].Status).To(Equal(tc.wantStatus))
			g.Expect(res.Status.Steps[0].Message).To(ContainSubstring(tc.wantMessage))
		})
	}
}

func TestCreateRHBOKClusterExtension(t *testing.T) {
	g := NewWithT(t)
	account := &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{
		Name: testRHBOKV1ServiceAccount, Namespace: rhbok.ExportOperatorNamespace,
	}}
	target := newTarget(t, nil, targetOpts{
		olmV1Only:      true,
		controllerObjs: []crclient.Object{account},
	})
	a := &rhbok.RHBOKMigrationAction{ServiceAccount: testRHBOKV1ServiceAccount}

	g.Expect(rhbok.ExportCreateRHBOKExtension(a, t.Context(), target, testRHBOKV1Channel)).To(Succeed())

	extension := &unstructured.Unstructured{}
	extension.SetGroupVersionKind(resources.ClusterExtension.GVK())
	g.Expect(target.Client.ControllerRuntime().Get(t.Context(), crclient.ObjectKey{
		Name: rhbok.ExportSubscriptionName,
	}, extension)).To(Succeed())
	expectRHBOKExtensionField(g, extension, testRHBOKV1NamespacePath, rhbok.ExportOperatorNamespace)
	expectRHBOKExtensionField(g, extension, testRHBOKV1AccountPath, testRHBOKV1ServiceAccount)
	expectRHBOKExtensionField(g, extension, testRHBOKV1PackagePath, rhbok.ExportSubscriptionName)
	expectRHBOKExtensionField(g, extension, testRHBOKV1CatalogPath, testRHBOKV1Catalog)
	expectRHBOKExtensionField(g, extension, testRHBOKV1ChannelPath, testRHBOKV1Channel)
}

func expectRHBOKExtensionField(g *WithT, extension *unstructured.Unstructured, path string, want string) {
	got, err := jq.Query[string](extension, path)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(got).To(Equal(want))
}
