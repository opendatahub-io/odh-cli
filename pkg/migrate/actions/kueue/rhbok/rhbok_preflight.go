package rhbok

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/opendatahub-io/odh-cli/pkg/constants"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/action"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/action/result"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
	"github.com/opendatahub-io/odh-cli/pkg/util/kube/olm"
	"github.com/opendatahub-io/odh-cli/pkg/util/kube/rbac"
)

func preparePermissions() []rbac.PermissionCheck {
	return []rbac.PermissionCheck{
		{Verb: "get", Group: resources.DataScienceClusterV1.Group, Resource: resources.DataScienceClusterV1.Resource},
		{Verb: "list", Group: resources.DataScienceClusterV1.Group, Resource: resources.DataScienceClusterV1.Resource},
		{Verb: "list", Group: resources.ClusterQueue.Group, Resource: resources.ClusterQueue.Resource},
		{Verb: "list", Group: resources.LocalQueue.Group, Resource: resources.LocalQueue.Resource},
		{Verb: "get", Group: resources.ConfigMap.Group, Resource: resources.ConfigMap.Resource, Namespace: applicationsNamespace},
		{Verb: "list", Group: resources.PackageManifest.Group, Resource: resources.PackageManifest.Resource},
	}
}

func runPermissions() []rbac.PermissionCheck {
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

	info, err := olm.FindOperator(ctx, target.Client, func(sub *olm.SubscriptionInfo) bool {
		return sub.Name == subscriptionName
	})
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

	channel, err := a.resolveSubscriptionChannel(ctx, target)
	if err != nil {
		step.Completef(result.StepFailed, "Failed to resolve operator channel: %v", err)

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
