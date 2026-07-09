package olm

import (
	"errors"
	"testing"

	operatorsv1 "github.com/operator-framework/api/pkg/operators/v1"
	operatorsv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	operatorfake "github.com/operator-framework/operator-lifecycle-manager/pkg/api/client/clientset/versioned/fake"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	kubefake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"

	"github.com/opendatahub-io/odh-cli/pkg/util/client"

	. "github.com/onsi/gomega"
)

const (
	testOperatorNamespace = "openshift-kueue-operator"
)

func TestEnsureNamespace(t *testing.T) {
	t.Run("creates namespace when missing", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		kubeClient := kubefake.NewSimpleClientset() //nolint:staticcheck // fake client without apply configs
		k8sClient := client.NewForTesting(client.TestClientConfig{
			Kubernetes: kubeClient,
		})

		err := ensureNamespace(ctx, k8sClient, testOperatorNamespace)
		g.Expect(err).ToNot(HaveOccurred())

		ns, err := kubeClient.CoreV1().Namespaces().Get(ctx, testOperatorNamespace, metav1.GetOptions{})
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(ns.Name).To(Equal(testOperatorNamespace))
	})

	t.Run("succeeds when namespace already exists", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		kubeClient := kubefake.NewSimpleClientset(&corev1.Namespace{ //nolint:staticcheck // fake client without apply configs
			ObjectMeta: metav1.ObjectMeta{Name: testOperatorNamespace},
		})
		k8sClient := client.NewForTesting(client.TestClientConfig{
			Kubernetes: kubeClient,
		})

		err := ensureNamespace(ctx, k8sClient, testOperatorNamespace)
		g.Expect(err).ToNot(HaveOccurred())
	})

	t.Run("returns error when namespace get fails", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		kubeClient := kubefake.NewSimpleClientset() //nolint:staticcheck // fake client without apply configs
		kubeClient.PrependReactor("get", "namespaces", func(_ k8stesting.Action) (bool, runtime.Object, error) {
			return true, nil, errors.New("connection refused")
		})
		k8sClient := client.NewForTesting(client.TestClientConfig{
			Kubernetes: kubeClient,
		})

		err := ensureNamespace(ctx, k8sClient, testOperatorNamespace)
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("get namespace"))
	})
}

func TestEnsureOperatorGroup(t *testing.T) {
	t.Run("creates operator group when missing", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		olmClient := operatorfake.NewSimpleClientset() //nolint:staticcheck // apply configs not available in OLM fake
		k8sClient := client.NewForTesting(client.TestClientConfig{
			OLM: olmClient,
		})

		err := ensureOperatorGroup(ctx, k8sClient, testOperatorNamespace, "openshift-kueue-operator-og", nil)
		g.Expect(err).ToNot(HaveOccurred())

		list, err := olmClient.OperatorsV1().OperatorGroups(testOperatorNamespace).List(ctx, metav1.ListOptions{})
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(list.Items).To(HaveLen(1))
		g.Expect(list.Items[0].Name).To(Equal("openshift-kueue-operator-og"))
	})

	t.Run("succeeds when operator group already exists", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		existing := &operatorsv1.OperatorGroup{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "kueue-operators",
				Namespace: testOperatorNamespace,
			},
		}

		olmClient := operatorfake.NewSimpleClientset(existing) //nolint:staticcheck // apply configs not available in OLM fake
		k8sClient := client.NewForTesting(client.TestClientConfig{
			OLM: olmClient,
		})

		err := ensureOperatorGroup(ctx, k8sClient, testOperatorNamespace, "openshift-kueue-operator-og", nil)
		g.Expect(err).ToNot(HaveOccurred())
	})

	t.Run("succeeds when existing target namespaces match requested", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		existing := &operatorsv1.OperatorGroup{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "kueue-operators",
				Namespace: testOperatorNamespace,
			},
			Spec: operatorsv1.OperatorGroupSpec{
				TargetNamespaces: []string{"team-a", "team-b"},
			},
		}

		olmClient := operatorfake.NewSimpleClientset(existing) //nolint:staticcheck // apply configs not available in OLM fake
		k8sClient := client.NewForTesting(client.TestClientConfig{
			OLM: olmClient,
		})

		err := ensureOperatorGroup(ctx, k8sClient, testOperatorNamespace, "openshift-kueue-operator-og",
			[]string{"team-b", "team-a"})
		g.Expect(err).ToNot(HaveOccurred())
	})

	t.Run("returns error when target namespaces conflict", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		existing := &operatorsv1.OperatorGroup{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "kueue-operators",
				Namespace: testOperatorNamespace,
			},
			Spec: operatorsv1.OperatorGroupSpec{
				TargetNamespaces: []string{"other-ns"},
			},
		}

		olmClient := operatorfake.NewSimpleClientset(existing) //nolint:staticcheck // apply configs not available in OLM fake
		k8sClient := client.NewForTesting(client.TestClientConfig{
			OLM: olmClient,
		})

		err := ensureOperatorGroup(ctx, k8sClient, testOperatorNamespace, "openshift-kueue-operator-og", nil)
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("targetNamespaces"))
	})

	t.Run("treats AlreadyExists as success when operator group exists", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		existing := &operatorsv1.OperatorGroup{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "openshift-kueue-operator-og",
				Namespace: testOperatorNamespace,
			},
		}

		olmClient := operatorfake.NewSimpleClientset(existing) //nolint:staticcheck // apply configs not available in OLM fake
		olmClient.PrependReactor("create", "operatorgroups", func(_ k8stesting.Action) (bool, runtime.Object, error) {
			return true, nil, apierrors.NewAlreadyExists(
				schema.GroupResource{Group: "operators.coreos.com", Resource: "operatorgroups"},
				"openshift-kueue-operator-og",
			)
		})

		k8sClient := client.NewForTesting(client.TestClientConfig{
			OLM: olmClient,
		})

		err := ensureOperatorGroup(ctx, k8sClient, testOperatorNamespace, "openshift-kueue-operator-og", nil)
		g.Expect(err).ToNot(HaveOccurred())
	})

	t.Run("returns error when multiple operator groups exist", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		og1 := &operatorsv1.OperatorGroup{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "og-1",
				Namespace: testOperatorNamespace,
			},
		}
		og2 := &operatorsv1.OperatorGroup{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "og-2",
				Namespace: testOperatorNamespace,
			},
		}

		olmClient := operatorfake.NewSimpleClientset(og1, og2) //nolint:staticcheck // apply configs not available in OLM fake
		k8sClient := client.NewForTesting(client.TestClientConfig{
			OLM: olmClient,
		})

		err := ensureOperatorGroup(ctx, k8sClient, testOperatorNamespace, "openshift-kueue-operator-og", nil)
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("expected at most one OperatorGroup"))
	})
}

func TestCSVReady(t *testing.T) {
	t.Run("false when no succeeded CSV", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		olmClient := operatorfake.NewSimpleClientset() //nolint:staticcheck // apply configs not available in OLM fake
		k8sClient := client.NewForTesting(client.TestClientConfig{OLM: olmClient})

		ready, err := CSVReady(ctx, k8sClient, testOperatorNamespace, "kueue-operator")
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(ready).To(BeFalse())
	})

	t.Run("true when succeeded CSV exists", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		olmClient := operatorfake.NewSimpleClientset(&operatorsv1alpha1.ClusterServiceVersion{ //nolint:staticcheck // apply configs not available in OLM fake
			ObjectMeta: metav1.ObjectMeta{
				Name:      "kueue-operator.v1.0.0",
				Namespace: testOperatorNamespace,
			},
			Status: operatorsv1alpha1.ClusterServiceVersionStatus{
				Phase: operatorsv1alpha1.CSVPhaseSucceeded,
			},
		})
		k8sClient := client.NewForTesting(client.TestClientConfig{OLM: olmClient})

		ready, err := CSVReady(ctx, k8sClient, testOperatorNamespace, "kueue-operator")
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(ready).To(BeTrue())
	})

	t.Run("error when csv name prefix is empty", func(t *testing.T) {
		g := NewWithT(t)
		ctx := t.Context()

		olmClient := operatorfake.NewSimpleClientset() //nolint:staticcheck // apply configs not available in OLM fake
		k8sClient := client.NewForTesting(client.TestClientConfig{OLM: olmClient})

		ready, err := CSVReady(ctx, k8sClient, testOperatorNamespace, "")
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("csv name prefix must not be empty"))
		g.Expect(ready).To(BeFalse())
	})
}
