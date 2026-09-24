package deps

import (
	"context"
	"fmt"

	platformcluster "github.com/opendatahub-io/odh-platform-utilities/pkg/cluster"
	operatorsv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
)

func requestedClusterExtension(ctx context.Context, reader crclient.Reader, packageName, namespace string) (bool, error) {
	requested, err := platformcluster.ClusterExtensionInstallsPackage(ctx, reader, packageName, namespace)
	if meta.IsNoMatchError(err) || apierrors.IsNotFound(err) {
		return false, nil
	}

	return requested, err //nolint:wrapcheck // Callers add dependency context.
}

func requestedV0Subscription(
	ctx context.Context, reader client.OLMReader, packageName, namespace string,
) (*operatorsv1alpha1.Subscription, error) {
	list, err := reader.Subscriptions(namespace).List(ctx, metav1.ListOptions{})
	if meta.IsNoMatchError(err) || apierrors.IsNotFound(err) {
		return nil, nil //nolint:nilnil // OLM v0 is absent.
	}
	if err != nil {
		return nil, fmt.Errorf("list subscriptions in %s: %w", namespace, err)
	}

	for i := range list.Items {
		if list.Items[i].Spec != nil && list.Items[i].Spec.Package == packageName {
			return &list.Items[i], nil
		}
	}

	return nil, nil //nolint:nilnil // No package request exists in this namespace.
}

type clusterExtensionInstallState struct {
	version   string
	requested bool
	installed bool
}

func findClusterExtensionInstallState(
	ctx context.Context, reader crclient.Reader, packageName, namespace string,
) (clusterExtensionInstallState, error) {
	var state clusterExtensionInstallState

	list := &unstructured.UnstructuredList{}
	list.SetGroupVersionKind(resources.ClusterExtension.GVK().GroupVersion().WithKind(resources.ClusterExtension.ListKind()))

	err := reader.List(ctx, list)
	if meta.IsNoMatchError(err) || apierrors.IsNotFound(err) {
		return state, nil
	}
	if err != nil {
		return state, fmt.Errorf("list ClusterExtensions: %w", err)
	}

	for i := range list.Items {
		extension := &list.Items[i]
		matches, matchErr := clusterExtensionMatches(extension, packageName, namespace)
		if matchErr != nil {
			return state, matchErr
		}
		if !matches {
			continue
		}
		state.requested = true

		installed, conditionErr := clusterExtensionInstalled(extension)
		if conditionErr != nil {
			return state, conditionErr
		}
		if !installed {
			continue
		}

		version, _, versionErr := unstructured.NestedString(extension.Object, "status", "install", "bundle", "version")
		if versionErr != nil {
			return state, fmt.Errorf("read ClusterExtension %s version: %w", extension.GetName(), versionErr)
		}

		state.version = version
		state.installed = true

		return state, nil
	}

	return state, nil
}

func clusterExtensionMatches(extension *unstructured.Unstructured, packageName, namespace string) (bool, error) {
	installNamespace, _, err := unstructured.NestedString(extension.Object, "spec", "namespace")
	if err != nil {
		return false, fmt.Errorf("read ClusterExtension %s namespace: %w", extension.GetName(), err)
	}
	if namespace != "" && installNamespace != namespace {
		return false, nil
	}

	sourceType, _, err := unstructured.NestedString(extension.Object, "spec", "source", "sourceType")
	if err != nil {
		return false, fmt.Errorf("read ClusterExtension %s source type: %w", extension.GetName(), err)
	}
	if sourceType != "Catalog" {
		return false, nil
	}

	installedPackage, _, err := unstructured.NestedString(extension.Object, "spec", "source", "catalog", "packageName")
	if err != nil {
		return false, fmt.Errorf("read ClusterExtension %s package: %w", extension.GetName(), err)
	}

	return installedPackage == packageName, nil
}

func clusterExtensionInstalled(extension *unstructured.Unstructured) (bool, error) {
	conditions, _, err := unstructured.NestedSlice(extension.Object, "status", "conditions")
	if err != nil {
		return false, fmt.Errorf("read ClusterExtension %s conditions: %w", extension.GetName(), err)
	}

	for _, condition := range conditions {
		values, ok := condition.(map[string]any)
		if !ok {
			return false, fmt.Errorf("ClusterExtension %s has malformed condition", extension.GetName())
		}
		if values["type"] == "Installed" {
			reason, _ := values["reason"].(string)
			if values["status"] == "False" && reason == "Failed" {
				message, _ := values["message"].(string)

				return false, fmt.Errorf(
					"ClusterExtension %s installation failed (%s): %s",
					extension.GetName(), reason, message,
				)
			}

			return values["status"] == "True" && values["reason"] == "Succeeded", nil
		}
	}

	return false, nil
}
