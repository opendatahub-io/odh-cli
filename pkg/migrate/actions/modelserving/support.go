package modelserving

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"

	"github.com/opendatahub-io/odh-cli/pkg/migrate/action"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/action/result"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
	"github.com/opendatahub-io/odh-cli/pkg/util/jq"
)

const (
	// Annotation keys.
	annotationDeploymentMode      = "serving.kserve.io/deploymentMode"
	annotationEnableAuth          = "security.opendatahub.io/enable-auth"
	annotationManaged             = "opendatahub.io/managed"
	annotationHardwareProfileName = "opendatahub.io/hardware-profile-name"
	annotationHardwareProfileNS   = "opendatahub.io/hardware-profile-namespace"
	annotationRestartedAt         = "kubectl.kubernetes.io/restartedAt"

	// Deployment mode values.
	deploymentModeServerless    = "Serverless"
	deploymentModeModelMesh     = "ModelMesh"
	deploymentModeRawDeployment = "RawDeployment"

	// KServe constants.
	kserveContainerName = "kserve-container"

	// ConfigMap constants.
	inferenceServiceConfigName = "inferenceservice-config"
	inferenceServiceDataKey    = "inferenceService"

	// Deployment name for KServe controller.
	kserveControllerDeployment = "kserve-controller-manager"

	// Namespace label for ModelMesh.
	labelModelMeshEnabled = "modelmesh-enabled"

	// Managed annotation values.
	managedTrue  = "true"
	managedFalse = "false"

	// Auth resource naming conventions.
	authSASuffix          = "-sa"
	authRoleSuffix        = "-view-role"
	authRoleBindingSuffix = "-view"

	// Step messages.
	msgFoundISVCs                 = "Found %d InferenceServices with deploymentMode=%s"
	msgPatchDeploymentModeDryRun  = "Would patch InferenceService %s/%s deploymentMode from %s to %s"
	msgPatchDeploymentModeSuccess = "Patched InferenceService %s/%s deploymentMode to %s"
	msgPatchDeploymentModeFailed  = "Failed to patch InferenceService %s/%s: %v"
	msgRestartDeploymentDryRun    = "Would restart deployment %s/%s"
	msgRestartDeploymentSuccess   = "Restarted deployment %s/%s"
	msgRestartDeploymentFailed    = "Failed to restart deployment %s/%s: %v"
	msgGetConfigMapFailed         = "Failed to get ConfigMap %s/%s: %v"
	msgConfigMapNotFound          = "ConfigMap %s not found in namespace %s"
	msgGetAppNamespaceFailed      = "Failed to get applications namespace: %v"
	msgRemoveNSLabelSuccess       = "Removed %s label from namespace %s"
	msgRemoveNSLabelFailed        = "Failed to remove %s label from namespace %s: %v"
	msgRemoveNSLabelDryRun        = "Would remove %s label from namespace %s"
	msgAuthSkipped                = "Auth not enabled on InferenceService %s/%s (skipped auth resources)"
)

// inferenceServiceConfig preserves all fields in the inferenceService JSON
// while allowing targeted access to serviceAnnotationDisallowedList.
type inferenceServiceConfig map[string]json.RawMessage

const disallowedListKey = "serviceAnnotationDisallowedList"

func (c inferenceServiceConfig) disallowedList() ([]string, error) {
	raw, ok := c[disallowedListKey]
	if !ok {
		return nil, nil
	}

	var list []string
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", disallowedListKey, err)
	}

	return list, nil
}

func (c inferenceServiceConfig) setDisallowedList(list []string) error {
	raw, err := json.Marshal(list)
	if err != nil {
		return fmt.Errorf("marshaling %s: %w", disallowedListKey, err)
	}

	c[disallowedListKey] = raw

	return nil
}

// listISVCsByDeploymentMode lists InferenceServices filtered by deployment mode annotation.
func listISVCsByDeploymentMode(
	ctx context.Context,
	target action.Target,
	mode string,
) ([]*unstructured.Unstructured, error) {
	filter := func(obj *unstructured.Unstructured) (bool, error) {
		val, err := jq.Query[string](obj, ".metadata.annotations.\""+annotationDeploymentMode+"\"")
		if err != nil {
			return false, nil //nolint:nilerr // Missing annotation means not matching.
		}

		return val == mode, nil
	}

	return client.List[*unstructured.Unstructured](ctx, target.Client, resources.InferenceService, filter)
}

// patchISVCDeploymentMode patches an InferenceService's deployment mode annotation.
func patchISVCDeploymentMode(
	ctx context.Context,
	target action.Target,
	isvc *unstructured.Unstructured,
	newMode string,
	step action.StepRecorder,
) {
	name := isvc.GetName()
	ns := isvc.GetNamespace()
	oldMode := getDeploymentMode(isvc)

	if target.DryRun {
		step.Completef(result.StepSkipped, msgPatchDeploymentModeDryRun, ns, name, oldMode, newMode)

		return
	}

	patchData := fmt.Sprintf(`{"metadata":{"annotations":{%q:%q}}}`, annotationDeploymentMode, newMode)

	_, err := target.Client.Dynamic().Resource(resources.InferenceService.GVR()).
		Namespace(ns).
		Patch(ctx, name, types.MergePatchType, []byte(patchData), metav1.PatchOptions{})

	if err != nil {
		step.Completef(result.StepFailed, msgPatchDeploymentModeFailed, ns, name, err)

		return
	}

	step.Completef(result.StepCompleted, msgPatchDeploymentModeSuccess, ns, name, newMode)
}

// getDeploymentMode returns the deployment mode annotation value, or empty string if not set.
func getDeploymentMode(obj *unstructured.Unstructured) string {
	val, err := jq.Query[string](obj, ".metadata.annotations.\""+annotationDeploymentMode+"\"")
	if err != nil {
		return ""
	}

	return val
}

// getInferenceServiceConfig gets the inferenceservice-config ConfigMap from the specified namespace.
func getInferenceServiceConfig(
	ctx context.Context,
	target action.Target,
	namespace string,
) (*unstructured.Unstructured, error) {
	cm, err := target.Client.Dynamic().Resource(resources.ConfigMap.GVR()).
		Namespace(namespace).
		Get(ctx, inferenceServiceConfigName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("getting %s ConfigMap: %w", inferenceServiceConfigName, err)
	}

	return cm, nil
}

// restartDeployment triggers a rolling restart of a deployment by patching the pod template annotation.
func restartDeployment(
	ctx context.Context,
	target action.Target,
	namespace string,
	name string,
	step action.StepRecorder,
) {
	if target.DryRun {
		step.Completef(result.StepSkipped, msgRestartDeploymentDryRun, namespace, name)

		return
	}

	patchData := fmt.Sprintf(
		`{"spec":{"template":{"metadata":{"annotations":{%q:%q}}}}}`,
		annotationRestartedAt,
		time.Now().Format(time.RFC3339),
	)

	_, err := target.Client.Dynamic().Resource(resources.Deployment.GVR()).
		Namespace(namespace).
		Patch(ctx, name, types.MergePatchType, []byte(patchData), metav1.PatchOptions{})

	if err != nil {
		step.Completef(result.StepFailed, msgRestartDeploymentFailed, namespace, name, err)

		return
	}

	step.Completef(result.StepCompleted, msgRestartDeploymentSuccess, namespace, name)
}

// getApplicationsNamespace retrieves the applications namespace from DSCI.
// Tries the v2 API first (RHOAI 3.x), then falls back to v1 (RHOAI 2.x).
func getApplicationsNamespace(ctx context.Context, target action.Target) (string, error) {
	ns, err := client.GetApplicationsNamespace(ctx, target.Client)
	if err == nil {
		return ns, nil
	}

	// Fall back to v1 DSCI for RHOAI 2.x clusters
	dsci, v1Err := client.GetSingleton(ctx, target.Client, resources.DSCInitializationV1)
	if v1Err != nil {
		return "", fmt.Errorf("getting applications namespace (v2: %w, v1: %w)", err, v1Err)
	}

	v1NS, queryErr := jq.Query[string](dsci, ".spec.applicationsNamespace")
	if queryErr != nil {
		return "", fmt.Errorf("getting applications namespace (v2: %w, v1 query: %w)", err, queryErr)
	}

	if v1NS == "" {
		return "", errors.New("getting applications namespace: applicationsNamespace not set in DSCI")
	}

	return v1NS, nil
}

// parseISVCConfigData parses the inferenceService data key from the ConfigMap.
func parseISVCConfigData(configMap *unstructured.Unstructured) (inferenceServiceConfig, error) {
	dataJSON, err := jq.Query[string](configMap, ".data."+inferenceServiceDataKey)
	if err != nil {
		if errors.Is(err, jq.ErrNotFound) {
			return inferenceServiceConfig{}, nil
		}

		return nil, fmt.Errorf("reading %s: %w", inferenceServiceDataKey, err)
	}

	var cfg inferenceServiceConfig
	if err := json.Unmarshal([]byte(dataJSON), &cfg); err != nil {
		return nil, fmt.Errorf("parsing %s JSON: %w", inferenceServiceDataKey, err)
	}

	return cfg, nil
}

// ensureAuthResources creates the SA, Role, and RoleBinding for an InferenceService if they don't exist.
func ensureAuthResources(
	ctx context.Context,
	target action.Target,
	isvc *unstructured.Unstructured,
	step action.StepRecorder,
) {
	name := isvc.GetName()
	ns := isvc.GetNamespace()

	saName := name + authSASuffix
	roleName := name + authRoleSuffix
	roleBindingName := name + authRoleBindingSuffix

	ensureServiceAccount(ctx, target, ns, saName, step)
	ensureRole(ctx, target, ns, roleName, step)
	ensureRoleBinding(ctx, target, ns, roleBindingName, saName, roleName, step)
}

func ensureServiceAccount(
	ctx context.Context,
	target action.Target,
	namespace string,
	name string,
	step action.StepRecorder,
) {
	_, err := target.Client.Dynamic().Resource(resources.ServiceAccount.GVR()).
		Namespace(namespace).
		Get(ctx, name, metav1.GetOptions{})

	if err == nil {
		return
	}

	if !apierrors.IsNotFound(err) {
		step.Recordf("create-sa", "Failed to check ServiceAccount %s/%s: %v", result.StepFailed, namespace, name, err)

		return
	}

	if target.DryRun {
		step.Recordf("create-sa", "Would create ServiceAccount %s/%s", result.StepSkipped, namespace, name)

		return
	}

	sa := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.ServiceAccount.APIVersion(),
			"kind":       resources.ServiceAccount.Kind,
			"metadata": map[string]any{
				"name":      name,
				"namespace": namespace,
			},
		},
	}

	_, err = target.Client.Dynamic().Resource(resources.ServiceAccount.GVR()).
		Namespace(namespace).
		Create(ctx, sa, metav1.CreateOptions{})

	if err != nil && !apierrors.IsAlreadyExists(err) {
		step.Recordf("create-sa", "Failed to create ServiceAccount %s/%s: %v", result.StepFailed, namespace, name, err)

		return
	}

	step.Recordf("create-sa", "Created ServiceAccount %s/%s", result.StepCompleted, namespace, name)
}

func ensureRole(
	ctx context.Context,
	target action.Target,
	namespace string,
	name string,
	step action.StepRecorder,
) {
	_, err := target.Client.Dynamic().Resource(resources.Role.GVR()).
		Namespace(namespace).
		Get(ctx, name, metav1.GetOptions{})

	if err == nil {
		return
	}

	if !apierrors.IsNotFound(err) {
		step.Recordf("create-role", "Failed to check Role %s/%s: %v", result.StepFailed, namespace, name, err)

		return
	}

	if target.DryRun {
		step.Recordf("create-role", "Would create Role %s/%s", result.StepSkipped, namespace, name)

		return
	}

	role := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.Role.APIVersion(),
			"kind":       resources.Role.Kind,
			"metadata": map[string]any{
				"name":      name,
				"namespace": namespace,
			},
			"rules": []any{
				map[string]any{
					"apiGroups": []any{""},
					"resources": []any{"services"},
					"verbs":     []any{"get", "list", "watch"},
				},
			},
		},
	}

	_, err = target.Client.Dynamic().Resource(resources.Role.GVR()).
		Namespace(namespace).
		Create(ctx, role, metav1.CreateOptions{})

	if err != nil && !apierrors.IsAlreadyExists(err) {
		step.Recordf("create-role", "Failed to create Role %s/%s: %v", result.StepFailed, namespace, name, err)

		return
	}

	step.Recordf("create-role", "Created Role %s/%s", result.StepCompleted, namespace, name)
}

func ensureRoleBinding(
	ctx context.Context,
	target action.Target,
	namespace string,
	name string,
	saName string,
	roleName string,
	step action.StepRecorder,
) {
	_, err := target.Client.Dynamic().Resource(resources.RoleBinding.GVR()).
		Namespace(namespace).
		Get(ctx, name, metav1.GetOptions{})

	if err == nil {
		return
	}

	if !apierrors.IsNotFound(err) {
		step.Recordf("create-rolebinding", "Failed to check RoleBinding %s/%s: %v", result.StepFailed, namespace, name, err)

		return
	}

	if target.DryRun {
		step.Recordf("create-rolebinding", "Would create RoleBinding %s/%s", result.StepSkipped, namespace, name)

		return
	}

	rb := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.RoleBinding.APIVersion(),
			"kind":       resources.RoleBinding.Kind,
			"metadata": map[string]any{
				"name":      name,
				"namespace": namespace,
			},
			"subjects": []any{
				map[string]any{
					"kind":      "ServiceAccount",
					"name":      saName,
					"namespace": namespace,
				},
			},
			"roleRef": map[string]any{
				"apiGroup": "rbac.authorization.k8s.io",
				"kind":     "Role",
				"name":     roleName,
			},
		},
	}

	_, err = target.Client.Dynamic().Resource(resources.RoleBinding.GVR()).
		Namespace(namespace).
		Create(ctx, rb, metav1.CreateOptions{})

	if err != nil && !apierrors.IsAlreadyExists(err) {
		step.Recordf("create-rolebinding", "Failed to create RoleBinding %s/%s: %v", result.StepFailed, namespace, name, err)

		return
	}

	step.Recordf("create-rolebinding", "Created RoleBinding %s/%s", result.StepCompleted, namespace, name)
}

// hasAuthEnabled returns true if the ISVC has the enable-auth annotation set to "true".
func hasAuthEnabled(isvc *unstructured.Unstructured) bool {
	val, err := jq.Query[string](isvc, ".metadata.annotations.\""+annotationEnableAuth+"\"")
	if err != nil {
		return false
	}

	return val == "true"
}

// removeModelMeshLabel removes the modelmesh-enabled label from a namespace.
func removeModelMeshLabel(
	ctx context.Context,
	target action.Target,
	namespace string,
	step action.StepRecorder,
) {
	if target.DryRun {
		step.Recordf(
			"remove-ns-label-"+namespace,
			msgRemoveNSLabelDryRun,
			result.StepSkipped,
			labelModelMeshEnabled, namespace,
		)

		return
	}

	patchData := fmt.Sprintf(`{"metadata":{"labels":{%q:null}}}`, labelModelMeshEnabled)

	_, err := target.Client.Dynamic().Resource(resources.Namespace.GVR()).
		Patch(ctx, namespace, types.MergePatchType, []byte(patchData), metav1.PatchOptions{})

	if err != nil {
		step.Recordf(
			"remove-ns-label-"+namespace,
			msgRemoveNSLabelFailed,
			result.StepFailed,
			labelModelMeshEnabled, namespace, err,
		)

		return
	}

	step.Recordf(
		"remove-ns-label-"+namespace,
		msgRemoveNSLabelSuccess,
		result.StepCompleted,
		labelModelMeshEnabled, namespace,
	)
}

// groupByNamespace groups unstructured objects by their namespace.
func groupByNamespace(objs []*unstructured.Unstructured) map[string][]*unstructured.Unstructured {
	grouped := make(map[string][]*unstructured.Unstructured)

	for _, obj := range objs {
		ns := obj.GetNamespace()
		grouped[ns] = append(grouped[ns], obj)
	}

	return grouped
}
