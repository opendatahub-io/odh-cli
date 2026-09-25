package rhbok_test

import (
	"testing"

	crclient "sigs.k8s.io/controller-runtime/pkg/client"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

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
