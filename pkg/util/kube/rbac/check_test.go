package rbac_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	authorizationv1 "k8s.io/api/authorization/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kubefake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"

	"github.com/opendatahub-io/odh-cli/pkg/util/kube/rbac"
)

func TestCheckPermissions_AllAllowed(t *testing.T) {
	client := kubefake.NewSimpleClientset()
	client.PrependReactor("create", "selfsubjectaccessreviews",
		func(action k8stesting.Action) (bool, runtime.Object, error) {
			return true, &authorizationv1.SelfSubjectAccessReview{
				Status: authorizationv1.SubjectAccessReviewStatus{Allowed: true},
			}, nil
		},
	)

	checks := []rbac.PermissionCheck{
		{Verb: "get", Group: "apps", Resource: "deployments"},
		{Verb: "list", Group: "", Resource: "configmaps", Namespace: "default"},
	}

	denied, err := rbac.CheckPermissions(context.Background(), client.AuthorizationV1(), checks)
	require.NoError(t, err)
	assert.Empty(t, denied)
}

func TestCheckPermissions_SomeDenied(t *testing.T) {
	client := kubefake.NewSimpleClientset()
	client.PrependReactor("create", "selfsubjectaccessreviews",
		func(action k8stesting.Action) (bool, runtime.Object, error) {
			createAction := action.(k8stesting.CreateAction)
			review := createAction.GetObject().(*authorizationv1.SelfSubjectAccessReview)
			allowed := review.Spec.ResourceAttributes.Verb != "create"

			return true, &authorizationv1.SelfSubjectAccessReview{
				Status: authorizationv1.SubjectAccessReviewStatus{Allowed: allowed},
			}, nil
		},
	)

	checks := []rbac.PermissionCheck{
		{Verb: "get", Group: "apps", Resource: "deployments"},
		{Verb: "create", Group: "operators.coreos.com", Resource: "subscriptions", Namespace: "openshift-kueue-operator"},
		{Verb: "list", Group: "", Resource: "configmaps"},
	}

	denied, err := rbac.CheckPermissions(context.Background(), client.AuthorizationV1(), checks)
	require.NoError(t, err)
	require.Len(t, denied, 1)
	assert.Equal(t, "create", denied[0].Verb)
	assert.Equal(t, "subscriptions", denied[0].Resource)
}

func TestCheckPermissions_AllDenied(t *testing.T) {
	client := kubefake.NewSimpleClientset()
	client.PrependReactor("create", "selfsubjectaccessreviews",
		func(action k8stesting.Action) (bool, runtime.Object, error) {
			return true, &authorizationv1.SelfSubjectAccessReview{
				Status: authorizationv1.SubjectAccessReviewStatus{Allowed: false},
			}, nil
		},
	)

	checks := []rbac.PermissionCheck{
		{Verb: "get", Group: "apps", Resource: "deployments"},
		{Verb: "create", Group: "apps", Resource: "deployments"},
	}

	denied, err := rbac.CheckPermissions(context.Background(), client.AuthorizationV1(), checks)
	require.NoError(t, err)
	assert.Len(t, denied, 2)
}

func TestCheckPermissions_APIError(t *testing.T) {
	client := kubefake.NewSimpleClientset()
	client.PrependReactor("create", "selfsubjectaccessreviews",
		func(action k8stesting.Action) (bool, runtime.Object, error) {
			return true, nil, assert.AnError
		},
	)

	checks := []rbac.PermissionCheck{
		{Verb: "get", Group: "apps", Resource: "deployments"},
	}

	denied, err := rbac.CheckPermissions(context.Background(), client.AuthorizationV1(), checks)
	require.Error(t, err)
	assert.Nil(t, denied)
	assert.Contains(t, err.Error(), "failed to check permission")
}

func TestCheckPermissions_EmptyChecks(t *testing.T) {
	client := kubefake.NewSimpleClientset()

	denied, err := rbac.CheckPermissions(context.Background(), client.AuthorizationV1(), nil)
	require.NoError(t, err)
	assert.Empty(t, denied)
}

func TestPermissionCheck_String(t *testing.T) {
	tests := []struct {
		name     string
		check    rbac.PermissionCheck
		expected string
	}{
		{
			name:     "cluster-scoped",
			check:    rbac.PermissionCheck{Verb: "list", Group: "kueue.x-k8s.io", Resource: "clusterqueues"},
			expected: "list clusterqueues.kueue.x-k8s.io [cluster]",
		},
		{
			name:     "namespaced",
			check:    rbac.PermissionCheck{Verb: "get", Group: "", Resource: "configmaps", Namespace: "default"},
			expected: "get configmaps [default]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.check.String())
		})
	}
}
