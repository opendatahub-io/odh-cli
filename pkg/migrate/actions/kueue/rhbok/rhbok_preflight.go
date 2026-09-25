package rhbok

import (
	"context"
	"fmt"

	platformcluster "github.com/opendatahub-io/odh-platform-utilities/pkg/cluster"
	platformolm "github.com/opendatahub-io/odh-platform-utilities/pkg/cluster/olm"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/opendatahub-io/odh-cli/pkg/constants"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/action"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/action/result"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
	"github.com/opendatahub-io/odh-cli/pkg/util/kube/olm"
	"github.com/opendatahub-io/odh-cli/pkg/util/kube/rbac"
)

func preparePermissions() []rbac.PermissionCheck {
	checks := preparePermissionsV1()

	return append(checks,
		rbac.PermissionCheck{Verb: "list", Group: resources.PackageManifest.Group, Resource: resources.PackageManifest.Resource},
	)
}

func preparePermissionsV1() []rbac.PermissionCheck {
	return []rbac.PermissionCheck{
		{Verb: "get", Group: resources.DataScienceClusterV1.Group, Resource: resources.DataScienceClusterV1.Resource},
		{Verb: "list", Group: resources.DataScienceClusterV1.Group, Resource: resources.DataScienceClusterV1.Resource},
		{Verb: "list", Group: resources.ClusterQueue.Group, Resource: resources.ClusterQueue.Resource},
		{Verb: "list", Group: resources.LocalQueue.Group, Resource: resources.LocalQueue.Resource},
		{Verb: "get", Group: resources.ConfigMap.Group, Resource: resources.ConfigMap.Resource, Namespace: applicationsNamespace},
	}
}

func runPermissions(forceDeleteLegacyCRDs bool) []rbac.PermissionCheck {
	checks := append([]rbac.PermissionCheck{}, preparePermissions()...)
	checks = append(checks,
		rbac.PermissionCheck{Verb: "update", Group: resources.DataScienceClusterV1.Group, Resource: resources.DataScienceClusterV1.Resource},
		rbac.PermissionCheck{Verb: "get", Group: resources.Subscription.Group, Resource: resources.Subscription.Resource, Namespace: operatorNamespace},
		rbac.PermissionCheck{Verb: "create", Group: resources.Subscription.Group, Resource: resources.Subscription.Resource, Namespace: operatorNamespace},
		rbac.PermissionCheck{Verb: "list", Group: resources.OperatorGroup.Group, Resource: resources.OperatorGroup.Resource, Namespace: operatorNamespace},
		rbac.PermissionCheck{Verb: "create", Group: resources.OperatorGroup.Group, Resource: resources.OperatorGroup.Resource, Namespace: operatorNamespace},
		rbac.PermissionCheck{Verb: "list", Group: resources.ClusterServiceVersion.Group, Resource: resources.ClusterServiceVersion.Resource, Namespace: operatorNamespace},
		rbac.PermissionCheck{Verb: "create", Group: resources.Namespace.Group, Resource: resources.Namespace.Resource},
		rbac.PermissionCheck{Verb: "get", Group: resources.Namespace.Group, Resource: resources.Namespace.Resource},
		rbac.PermissionCheck{Verb: "patch", Group: resources.Namespace.Group, Resource: resources.Namespace.Resource},
		rbac.PermissionCheck{Verb: "update", Group: resources.ConfigMap.Group, Resource: resources.ConfigMap.Resource, Namespace: applicationsNamespace},
		rbac.PermissionCheck{Verb: "get", Group: resources.Deployment.Group, Resource: resources.Deployment.Resource, Namespace: applicationsNamespace},
		rbac.PermissionCheck{Verb: "list", Group: resources.Deployment.Group, Resource: resources.Deployment.Resource, Namespace: applicationsNamespace},
		rbac.PermissionCheck{Verb: "get", Group: resources.CustomResourceDefinition.Group, Resource: resources.CustomResourceDefinition.Resource},
		rbac.PermissionCheck{Verb: "delete", Group: resources.CustomResourceDefinition.Group, Resource: resources.CustomResourceDefinition.Resource},
		rbac.PermissionCheck{Verb: "list", Group: resources.Pod.Group, Resource: resources.Pod.Resource, Namespace: operatorNamespace},
	)
	if !forceDeleteLegacyCRDs {
		checks = append(checks,
			rbac.PermissionCheck{Verb: "list", Group: legacyCohortGroup, Resource: legacyCohortResource},
			rbac.PermissionCheck{Verb: "list", Group: legacyCohortGroup, Resource: legacyTopologyResource},
		)
	}

	for _, rt := range monitoredWorkloadResourceTypes() {
		checks = append(checks,
			rbac.PermissionCheck{Verb: "list", Group: rt.Group, Resource: rt.Resource},
			rbac.PermissionCheck{Verb: "patch", Group: rt.Group, Resource: rt.Resource},
		)
	}

	return checks
}

func runPermissionsV1(forceDeleteLegacyCRDs bool) []rbac.PermissionCheck {
	checks := preparePermissionsV1()
	checks = append(checks,
		rbac.PermissionCheck{Verb: "update", Group: resources.DataScienceClusterV1.Group, Resource: resources.DataScienceClusterV1.Resource},
		rbac.PermissionCheck{Verb: "list", Group: resources.ClusterExtension.Group, Resource: resources.ClusterExtension.Resource},
		rbac.PermissionCheck{Verb: "get", Group: resources.ClusterExtension.Group, Resource: resources.ClusterExtension.Resource},
		rbac.PermissionCheck{Verb: "create", Group: resources.ClusterExtension.Group, Resource: resources.ClusterExtension.Resource},
		rbac.PermissionCheck{Verb: "get", Group: resources.ServiceAccount.Group, Resource: resources.ServiceAccount.Resource, Namespace: operatorNamespace},
		rbac.PermissionCheck{Verb: "get", Group: resources.Namespace.Group, Resource: resources.Namespace.Resource},
		rbac.PermissionCheck{Verb: "patch", Group: resources.Namespace.Group, Resource: resources.Namespace.Resource},
		rbac.PermissionCheck{Verb: "update", Group: resources.ConfigMap.Group, Resource: resources.ConfigMap.Resource, Namespace: applicationsNamespace},
		rbac.PermissionCheck{Verb: "get", Group: resources.Deployment.Group, Resource: resources.Deployment.Resource, Namespace: applicationsNamespace},
		rbac.PermissionCheck{Verb: "list", Group: resources.Deployment.Group, Resource: resources.Deployment.Resource, Namespace: applicationsNamespace},
		rbac.PermissionCheck{Verb: "get", Group: resources.CustomResourceDefinition.Group, Resource: resources.CustomResourceDefinition.Resource},
		rbac.PermissionCheck{Verb: "delete", Group: resources.CustomResourceDefinition.Group, Resource: resources.CustomResourceDefinition.Resource},
		rbac.PermissionCheck{Verb: "list", Group: resources.Pod.Group, Resource: resources.Pod.Resource, Namespace: operatorNamespace},
	)
	if !forceDeleteLegacyCRDs {
		checks = append(checks,
			rbac.PermissionCheck{Verb: "list", Group: legacyCohortGroup, Resource: legacyCohortResource},
			rbac.PermissionCheck{Verb: "list", Group: legacyCohortGroup, Resource: legacyTopologyResource},
		)
	}

	for _, rt := range monitoredWorkloadResourceTypes() {
		checks = append(checks,
			rbac.PermissionCheck{Verb: "list", Group: rt.Group, Resource: rt.Resource},
			rbac.PermissionCheck{Verb: "patch", Group: rt.Group, Resource: rt.Resource},
		)
	}

	return checks
}

func monitoredWorkloadResourceTypes() []resources.ResourceType {
	return []resources.ResourceType{
		resources.Notebook,
		resources.InferenceService,
		resources.LLMInferenceService,
		resources.RayCluster,
		resources.RayJob,
		resources.PyTorchJob,
	}
}

func (a *RHBOKMigrationAction) verifyRBAC(
	ctx context.Context,
	target action.Target,
	checks []rbac.PermissionCheck,
) {
	step := target.Recorder.Child(
		"verify-rbac",
		"Verify RBAC permissions",
	)

	denied, err := rbac.CheckPermissions(ctx, target.Client.AuthorizationV1(), checks)
	if err != nil {
		step.Completef(result.StepFailed, "Failed to verify RBAC permissions: %v", err)

		return
	}

	if len(denied) > 0 {
		for _, d := range denied {
			step.Child(
				fmt.Sprintf("denied-%s-%s", d.Verb, d.Resource),
				fmt.Sprintf("Missing permission: %s", d),
			).Completef(result.StepFailed, "Permission denied: %s", d)
		}

		step.Completef(result.StepFailed, "%d required permission(s) denied", len(denied))

		return
	}

	step.Completef(result.StepCompleted, "All %d required permissions verified", len(checks))
}

func (a *RHBOKMigrationAction) checkCertManager(
	ctx context.Context,
	target action.Target,
) {
	step := target.Recorder.Child(
		"check-cert-manager",
		"Verify cert-manager is installed",
	)

	_, err := target.Client.APIExtensions().ApiextensionsV1().
		CustomResourceDefinitions().
		Get(ctx, "certificates.cert-manager.io", metav1.GetOptions{})
	if err == nil {
		step.Completef(result.StepCompleted, "cert-manager CRD found")

		return
	}

	if !apierrors.IsNotFound(err) && !apierrors.IsForbidden(err) {
		step.Completef(result.StepFailed, "Could not check cert-manager CRD: %v", err)

		return
	}

	_, err = target.Client.Get(ctx, resources.Namespace.GVR(), "cert-manager")
	if err == nil {
		step.Completef(result.StepCompleted, "cert-manager namespace found")

		return
	}

	if !apierrors.IsNotFound(err) {
		step.Completef(result.StepFailed, "Could not check cert-manager namespace: %v", err)

		return
	}

	_, err = target.Client.Get(ctx, resources.Namespace.GVR(), "openshift-cert-manager")
	if err == nil {
		step.Completef(result.StepCompleted, "openshift-cert-manager namespace found")

		return
	}

	if !apierrors.IsNotFound(err) {
		step.Completef(result.StepFailed, "Could not check openshift-cert-manager namespace: %v", err)

		return
	}

	step.Completef(result.StepFailed, "cert-manager not detected (required for RHBOK)")
}

func (a *RHBOKMigrationAction) checkCurrentKueueState(
	ctx context.Context,
	target action.Target,
) {
	step := target.Recorder.Child(
		"check-kueue-state",
		"Verify current Kueue state",
	)

	state, err := a.getKueueManagementState(ctx, target.Client)
	if err != nil {
		if apierrors.IsNotFound(err) {
			step.Completef(result.StepFailed, "DataScienceCluster not found - OpenShift AI may not be installed")

			return
		}

		step.Completef(result.StepFailed, "Failed to get Kueue management state: %v", err)

		return
	}

	switch state {
	case constants.ManagementStateRemoved:
		step.Completef(result.StepCompleted, "Kueue state is Removed; will resume migration")
	case constants.ManagementStateUnmanaged:
		step.Completef(result.StepCompleted, "Kueue state is Unmanaged")
	case constants.ManagementStateManaged:
		step.Completef(result.StepCompleted, "Kueue state is Managed; full migration required")
	}
}

func (a *RHBOKMigrationAction) checkNoRHBOKConflicts(
	ctx context.Context,
	target action.Target,
) {
	step := target.Recorder.Child(
		"check-rhbok-conflicts",
		"Check for Red Hat build of Kueue operator conflicts",
	)

	state, err := a.getKueueManagementState(ctx, target.Client)
	if err != nil {
		step.Completef(result.StepFailed, "Failed to get Kueue state: %v", err)

		return
	}
	if a.selectedOLMMode == olm.ModeV1 {
		a.checkNoRHBOKConflictsV1(ctx, target, step, state)

		return
	}
	a.checkNoRHBOKConflictsV0(ctx, target, step, state)
}

func (a *RHBOKMigrationAction) checkNoRHBOKConflictsV1(
	ctx context.Context,
	target action.Target,
	step action.StepRecorder,
	state string,
) {
	v0Requested, err := rhbokV0SubscriptionRequested(ctx, target.Client.ControllerRuntime())
	if err != nil {
		step.Completef(result.StepFailed, "Failed to check RHBOK OLM v0 Subscriptions: %v", err)

		return
	}
	if v0Requested {
		step.Completef(result.StepFailed,
			"Conflict: RHBOK is already requested through OLM v0; select --kueue-olm-mode=v0")

		return
	}

	requested, err := platformcluster.ClusterExtensionInstallsPackage(
		ctx, target.Client.ControllerRuntime(), subscriptionPackage, operatorNamespace,
	)
	if err != nil {
		step.Completef(result.StepFailed, "Failed to check RHBOK ClusterExtension: %v", err)

		return
	}
	if state == constants.ManagementStateManaged && requested {
		step.Completef(result.StepFailed,
			"Conflict: embedded Kueue is Managed but RHBOK operator is already requested")

		return
	}
	if requested {
		step.Completef(result.StepCompleted, "Red Hat build of Kueue ClusterExtension already requested")
	} else {
		step.Completef(result.StepCompleted, "No Red Hat build of Kueue conflicts detected")
	}
}

func rhbokV0SubscriptionRequested(ctx context.Context, reader crclient.Reader) (bool, error) {
	list := &unstructured.UnstructuredList{}
	list.SetGroupVersionKind(resources.Subscription.GVK().GroupVersion().WithKind(resources.Subscription.ListKind()))

	err := reader.List(ctx, list)
	if meta.IsNoMatchError(err) || apierrors.IsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("list OLM v0 Subscriptions: %w", err)
	}

	for i := range list.Items {
		packageName, _, err := unstructured.NestedString(list.Items[i].Object, "spec", "name")
		if err != nil {
			return false, fmt.Errorf("read Subscription %s package: %w", list.Items[i].GetName(), err)
		}
		if packageName == subscriptionPackage {
			return true, nil
		}
	}

	return false, nil
}

func (a *RHBOKMigrationAction) checkV1ServiceAccount(ctx context.Context, target action.Target) {
	step := target.Recorder.Child("check-rhbok-service-account", "Verify OLM v1 installer ServiceAccount")
	extension, err := findRHBOKClusterExtension(ctx, target.Client.ControllerRuntime())
	if err != nil {
		step.Completef(result.StepFailed, "Failed to check RHBOK ClusterExtension: %v", err)

		return
	}

	accountName := a.serviceAccountName()
	if extension != nil {
		installed, conditionErr := clusterExtensionInstalled(extension)
		if conditionErr != nil {
			step.Completef(result.StepFailed, "Failed to check RHBOK ClusterExtension status: %v", conditionErr)

			return
		}
		if installed {
			step.Completef(result.StepSkipped, "RHBOK ClusterExtension is already installed")

			return
		}

		accountName, _, err = unstructured.NestedString(extension.Object, "spec", "serviceAccount", "name")
		if err != nil {
			step.Completef(result.StepFailed,
				"Failed to read RHBOK ClusterExtension %s ServiceAccount: %v",
				extension.GetName(), err)

			return
		}
		if accountName == "" {
			step.Completef(result.StepSkipped,
				"Pending RHBOK ClusterExtension %s does not reference an installer ServiceAccount",
				extension.GetName())

			return
		}
	}

	account := &corev1.ServiceAccount{}
	err = target.Client.ControllerRuntime().Get(ctx, crclient.ObjectKey{
		Name: accountName, Namespace: operatorNamespace,
	}, account)
	if err != nil {
		step.Completef(result.StepFailed,
			"OLM v1 requires ServiceAccount %s in namespace %s: %v",
			accountName, operatorNamespace, err)

		return
	}

	step.Completef(result.StepCompleted, "OLM v1 installer ServiceAccount %s exists", accountName)
}

func findRHBOKClusterExtension(ctx context.Context, reader crclient.Reader) (*unstructured.Unstructured, error) {
	list := &unstructured.UnstructuredList{}
	list.SetGroupVersionKind(resources.ClusterExtension.GVK().GroupVersion().WithKind(resources.ClusterExtension.ListKind()))
	if err := reader.List(ctx, list); err != nil {
		return nil, fmt.Errorf("list ClusterExtensions: %w", err)
	}

	for i := range list.Items {
		extension := &list.Items[i]
		namespace, _, err := unstructured.NestedString(extension.Object, "spec", "namespace")
		if err != nil {
			return nil, fmt.Errorf("read ClusterExtension %s namespace: %w", extension.GetName(), err)
		}
		if namespace != operatorNamespace {
			continue
		}

		sourceType, _, err := unstructured.NestedString(extension.Object, "spec", "source", "sourceType")
		if err != nil {
			return nil, fmt.Errorf("read ClusterExtension %s source type: %w", extension.GetName(), err)
		}
		if sourceType != "Catalog" {
			continue
		}

		packageName, _, err := unstructured.NestedString(extension.Object, "spec", "source", "catalog", "packageName")
		if err != nil {
			return nil, fmt.Errorf("read ClusterExtension %s package: %w", extension.GetName(), err)
		}
		if packageName == subscriptionPackage {
			return extension, nil
		}
	}

	return nil, nil //nolint:nilnil // No RHBOK ClusterExtension request exists.
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
			return values["status"] == "True" && values["reason"] == "Succeeded", nil
		}
	}

	return false, nil
}

func (a *RHBOKMigrationAction) checkNoRHBOKConflictsV0(
	ctx context.Context,
	target action.Target,
	step action.StepRecorder,
	state string,
) {
	info, err := olm.FindOperator(ctx, target.Client, func(sub *olm.SubscriptionInfo) bool {
		return sub.Name == subscriptionName
	})
	if apierrors.IsForbidden(err) && rhbokRequestedViaClusterExtension(ctx, target.Client) {
		info = &olm.SubscriptionInfo{Name: subscriptionName}
		err = nil
	}
	if err != nil && (apierrors.IsNotFound(err) || meta.IsNoMatchError(err)) && target.Client.ControllerRuntime() != nil {
		requested, requestErr := platformolm.OperatorPackageRequested(
			ctx,
			target.Client.ControllerRuntime(),
			subscriptionPackage,
		)
		if requestErr == nil && requested {
			info = &olm.SubscriptionInfo{Name: subscriptionName}
			err = nil
		} else {
			err = requestErr
		}
	}
	if err != nil {
		step.Completef(result.StepFailed, "Failed to check Red Hat build of Kueue subscription: %v", err)

		return
	}

	if state == constants.ManagementStateManaged && info.Found() {
		step.Completef(result.StepFailed,
			"Conflict: embedded Kueue is Managed but RHBOK operator is already installed")

		return
	}

	if info.Found() {
		step.Completef(result.StepCompleted, "Red Hat build of Kueue operator already installed")

		return
	}

	step.Completef(result.StepCompleted, "No Red Hat build of Kueue conflicts detected")
}

func (a *RHBOKMigrationAction) checkOperatorChannel(
	ctx context.Context,
	target action.Target,
) {
	step := target.Recorder.Child(
		"check-operator-channel",
		"Resolve Red Hat build of Kueue operator channel",
	)

	channel, err := a.operatorChannel(ctx, target)
	if err != nil {
		step.Completef(result.StepFailed, "Failed to resolve operator channel: %v", err)

		return
	}
	if a.selectedOLMMode == olm.ModeV1 && channel == "" {
		step.Completef(result.StepCompleted, "Existing RHBOK ClusterExtension has no single channel restriction")

		return
	}

	step.Completef(result.StepCompleted, "Will install from channel %s", channel)
}

func (a *RHBOKMigrationAction) verifyKueueResources(
	ctx context.Context,
	target action.Target,
) {
	step := target.Recorder.Child(
		"verify-kueue-resources",
		"Verify Kueue resources exist",
	)

	clusterQueues, err := target.Client.ListResources(ctx, resources.ClusterQueue.GVR())
	if err != nil {
		if apierrors.IsNotFound(err) || client.IsResourceTypeNotFound(err) {
			step.Completef(result.StepCompleted, "No ClusterQueue CRD found")

			return
		}

		step.Completef(result.StepFailed, "Failed to list ClusterQueues: %v", err)

		return
	}

	localQueues, err := target.Client.ListResources(ctx, resources.LocalQueue.GVR())
	if err != nil {
		if apierrors.IsNotFound(err) || client.IsResourceTypeNotFound(err) {
			step.Completef(result.StepCompleted,
				"Kueue resources found: %d ClusterQueues (LocalQueue CRD not found)",
				len(clusterQueues))

			return
		}

		step.Completef(result.StepFailed, "Failed to list LocalQueues: %v", err)

		return
	}

	step.Completef(result.StepCompleted,
		"Kueue resources found: %d ClusterQueues, %d LocalQueues",
		len(clusterQueues), len(localQueues))
}
