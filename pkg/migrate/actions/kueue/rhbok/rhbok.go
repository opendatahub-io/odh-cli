package rhbok

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	platformcluster "github.com/opendatahub-io/odh-platform-utilities/pkg/cluster"
	platformolm "github.com/opendatahub-io/odh-platform-utilities/pkg/cluster/olm"
	operatorsv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	"github.com/spf13/pflag"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/opendatahub-io/odh-cli/pkg/constants"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/action"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/action/result"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
	"github.com/opendatahub-io/odh-cli/pkg/util/confirmation"
	"github.com/opendatahub-io/odh-cli/pkg/util/jq"
	"github.com/opendatahub-io/odh-cli/pkg/util/kube/olm"
)

const (
	actionID          = "kueue.rhbok.migrate"
	actionName        = "Migrate Kueue to Red Hat build of Kueue"
	actionDescription = "Migrates from OpenShift AI built-in Kueue to Red Hat Build of Kueue operator"

	// Operator constants.
	operatorNamespace   = "openshift-kueue-operator"
	subscriptionName    = "kueue-operator"
	subscriptionPackage = "kueue-operator"
	subscriptionSource  = "redhat-operators"
	sourceNamespace     = "openshift-marketplace"
	csvNamePrefix       = "kueue-operator"

	// Retry configuration for conflict resolution.
	retryInitialDuration = 500 * time.Millisecond
	retryFactor          = 2.0
	retryJitter          = 0.1
	retryMaxSteps        = 5
	operatorTimeout      = 5 * time.Minute
	operatorPollPeriod   = 10 * time.Second
	componentPollPeriod  = 10 * time.Second
	componentTimeout     = 5 * time.Minute

	// DataScienceCluster constants.
	kueueComponentPath = ".spec.components.kueue.managementState"
	kueueReadyType     = "KueueReady"

	// Embedded Kueue deployment.
	embeddedKueueDeployment = "kueue-controller-manager"

	// ConfigMap constants.
	configMapName            = "kueue-manager-config"
	applicationsNamespace    = "redhat-ods-applications"
	configMapAnnotationKey   = "opendatahub.io/managed"
	configMapAnnotationValue = "false"

	defaultQueueName = "default"
)

// RHBOKMigrationAction migrates embedded Kueue to the Red Hat build of Kueue operator.
type RHBOKMigrationAction struct {
	ClusterQueueName      string
	LocalQueueName        string
	QueueName             string
	Channel               string
	SkipRemoveEmbedded    bool
	ForceDeleteLegacyCRDs bool
	OLMMode               string
	ServiceAccount        string

	executeLabelingPlan *labelingPlan
	selectedOLMMode     olm.Mode
}

func (a *RHBOKMigrationAction) ID() string { return actionID }

func (a *RHBOKMigrationAction) Name() string { return actionName }

func (a *RHBOKMigrationAction) Description() string { return actionDescription }

func (a *RHBOKMigrationAction) Group() action.ActionGroup { return action.GroupMigration }

func (a *RHBOKMigrationAction) Phase() action.ActionPhase { return action.PhasePreUpgrade }

func (a *RHBOKMigrationAction) CanApply(target action.Target) bool {
	if target.CurrentVersion == nil {
		return false
	}

	return target.CurrentVersion.Major == 2 && target.CurrentVersion.Minor >= 25
}

func (a *RHBOKMigrationAction) AddFlags(fs *pflag.FlagSet) {
	fs.StringVar(&a.ClusterQueueName, "cluster-queue-name", "",
		"Custom default ClusterQueue name for DataScienceCluster (optional)")
	fs.StringVar(&a.LocalQueueName, "local-queue-name", "",
		"Custom default LocalQueue name for DataScienceCluster (optional)")
	fs.StringVar(&a.QueueName, "workload-queue-name", defaultQueueName,
		"Queue name value for the kueue.x-k8s.io/queue-name workload label")
	fs.StringVar(&a.Channel, "channel", "",
		"OLM channel for the Red Hat build of Kueue operator (default: resolved from catalog)")
	fs.BoolVar(&a.SkipRemoveEmbedded, "skip-remove-embedded", false,
		"Skip setting Kueue managementState to Removed before installing RHBOK (not recommended)")
	fs.BoolVar(&a.ForceDeleteLegacyCRDs, "force-delete-legacy-crds", false,
		"Delete legacy cohorts/topologies CRDs even when instances still exist")
	fs.StringVar(&a.OLMMode, "kueue-olm-mode", "auto",
		"RHBOK installation API: auto (defaults to v0 when both APIs are available), v0, or v1")
	fs.StringVar(&a.ServiceAccount, "kueue-service-account", "odh-cli-dependency-installer",
		"Preconfigured ServiceAccount for OLM v1 RHBOK installation")
}

func (a *RHBOKMigrationAction) Prepare() action.Task {
	return &prepareTask{action: a}
}

func (a *RHBOKMigrationAction) Run() action.Task {
	return &runTask{action: a}
}

func (a *RHBOKMigrationAction) workloadQueueName() string {
	if a.QueueName == "" {
		return defaultQueueName
	}

	return a.QueueName
}

func (a *RHBOKMigrationAction) getKueueManagementState(
	ctx context.Context,
	c client.Client,
) (string, error) {
	dsc, err := client.GetSingleton(ctx, c, resources.DataScienceClusterV1)
	if err != nil {
		return "", fmt.Errorf("getting DataScienceCluster: %w", err)
	}

	state, err := jq.Query[string](dsc, kueueComponentPath)
	if err != nil {
		return "", fmt.Errorf("querying kueue managementState: %w", err)
	}

	if state == "" {
		return "", errors.New("kueue managementState is not configured")
	}

	switch state {
	case constants.ManagementStateManaged, constants.ManagementStateUnmanaged, constants.ManagementStateRemoved:
		return state, nil
	default:
		return "", fmt.Errorf("unsupported kueue managementState %q", state)
	}
}

func (a *RHBOKMigrationAction) isMigrationComplete(ctx context.Context, target action.Target) bool {
	state, err := a.getKueueManagementState(ctx, target.Client)
	if err != nil {
		return false
	}

	if state != constants.ManagementStateUnmanaged {
		return false
	}

	if a.selectedOLMMode == olm.ModeV1 {
		if !rhbokInstalledViaClusterExtension(ctx, target.Client) {
			return false
		}
	} else {
		installed, err := rhbokOperatorInstalled(ctx, target.Client)
		if err != nil || !installed {
			return false
		}
	}

	pods, err := target.Client.CoreV1().Pods(operatorNamespace).List(ctx, metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/name=kueue",
	})
	if err != nil || len(pods.Items) == 0 {
		return false
	}

	for i := range pods.Items {
		if !podReady(&pods.Items[i]) {
			return false
		}
	}

	return true
}

func rhbokOperatorInstalled(ctx context.Context, kubeClient client.Client) (bool, error) {
	sub, err := kubeClient.OLM().Subscriptions(operatorNamespace).Get(ctx, subscriptionName, metav1.GetOptions{})
	if err == nil {
		if sub.Status.InstalledCSV == "" {
			return false, nil
		}

		csv, csvErr := kubeClient.OLM().ClusterServiceVersions(operatorNamespace).
			Get(ctx, sub.Status.InstalledCSV, metav1.GetOptions{})
		if csvErr != nil {
			return false, fmt.Errorf("get RHBOK CSV %s: %w", sub.Status.InstalledCSV, csvErr)
		}

		return csv.Status.Phase == operatorsv1alpha1.CSVPhaseSucceeded, nil
	}

	if apierrors.IsForbidden(err) && rhbokInstalledViaClusterExtension(ctx, kubeClient) {
		return true, nil
	}

	if !apierrors.IsNotFound(err) && !meta.IsNoMatchError(err) {
		return false, fmt.Errorf("get RHBOK subscription: %w", err)
	}

	if kubeClient.ControllerRuntime() == nil {
		return false, fmt.Errorf("get RHBOK subscription: %w", err)
	}

	_, err = platformolm.OperatorExists(ctx, kubeClient.ControllerRuntime(), subscriptionPackage)
	if errors.Is(err, platformolm.ErrOperatorNotInstalled) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("check RHBOK operator: %w", err)
	}

	return true, nil
}

func rhbokInstalledViaClusterExtension(ctx context.Context, kubeClient client.Client) bool {
	if !rhbokRequestedViaClusterExtension(ctx, kubeClient) {
		return false
	}

	info, err := platformcluster.OperatorInstalledViaClusterExtension(
		ctx, kubeClient.ControllerRuntime(), subscriptionPackage,
	)

	return err == nil && info != nil
}

func rhbokRequestedViaClusterExtension(ctx context.Context, kubeClient client.Client) bool {
	if kubeClient.ControllerRuntime() == nil {
		return false
	}

	requested, err := platformcluster.ClusterExtensionInstallsPackage(
		ctx, kubeClient.ControllerRuntime(), subscriptionPackage, operatorNamespace,
	)

	return err == nil && requested
}

func (a *RHBOKMigrationAction) checkKueueManaged(
	ctx context.Context,
	target action.Target,
) bool {
	step := target.Recorder.Child(
		"check-kueue-managed",
		"Check if Kueue is managed by DataScienceCluster",
	)

	state, err := a.getKueueManagementState(ctx, target.Client)
	if err != nil {
		step.Completef(result.StepFailed, "Failed to get DataScienceCluster: %v", err)

		return false
	}

	if state == constants.ManagementStateManaged {
		step.Completef(result.StepCompleted, "Kueue is managed (managementState=%s)", state)

		return true
	}

	step.Completef(result.StepCompleted, "Kueue is not managed (managementState=%s)", state)

	return false
}

func (a *RHBOKMigrationAction) preserveKueueConfig(
	ctx context.Context,
	target action.Target,
) {
	step := target.Recorder.Child(
		"preserve-kueue-config",
		"Preserve Kueue ConfigMap for reference",
	)

	checkStep := step.Child(
		"check-configmap",
		fmt.Sprintf("Checking if ConfigMap '%s' exists in namespace '%s'", configMapName, applicationsNamespace),
	)

	configMap, err := target.Client.Dynamic().Resource(resources.ConfigMap.GVR()).
		Namespace(applicationsNamespace).
		Get(ctx, configMapName, metav1.GetOptions{})

	if err != nil {
		checkStep.Completef(result.StepCompleted, "ConfigMap not found")
		step.Completef(result.StepSkipped, "ConfigMap %s not found (skipped): %v", configMapName, err)

		return
	}

	checkStep.Completef(result.StepCompleted, "ConfigMap exists")

	annotateStep := step.Child(
		"apply-annotation",
		fmt.Sprintf("Apply annotation %s=%s", configMapAnnotationKey, configMapAnnotationValue),
	)

	if target.DryRun {
		annotateStep.Completef(result.StepSkipped, "Would annotate ConfigMap %s/%s", applicationsNamespace, configMapName)
		step.Completef(result.StepSkipped, "Dry-run: ConfigMap annotation skipped")

		return
	}

	annotations, err := jq.Query[map[string]any](configMap, ".metadata.annotations")
	if err != nil || annotations == nil {
		annotations = make(map[string]any)
	}

	annotations[configMapAnnotationKey] = configMapAnnotationValue

	annotationsJSON, err := json.Marshal(annotations)
	if err != nil {
		annotateStep.Completef(result.StepFailed, "Failed to marshal annotations: %v", err)
		step.Completef(result.StepFailed, "Failed to annotate ConfigMap")

		return
	}

	if err := jq.Transform(configMap, ".metadata.annotations = %s", annotationsJSON); err != nil {
		annotateStep.Completef(result.StepFailed, "Failed to set annotations: %v", err)
		step.Completef(result.StepFailed, "Failed to annotate ConfigMap")

		return
	}

	_, err = target.Client.Dynamic().Resource(resources.ConfigMap.GVR()).
		Namespace(applicationsNamespace).
		Update(ctx, configMap, metav1.UpdateOptions{})

	if err != nil {
		annotateStep.Completef(result.StepFailed, "Failed to update ConfigMap: %v", err)
		step.Completef(result.StepFailed, "Failed to annotate ConfigMap")

		return
	}

	annotateStep.Completef(result.StepCompleted, "Annotation applied successfully")
	step.Completef(result.StepCompleted, "ConfigMap %s annotated for preservation", configMapName)
}

func (a *RHBOKMigrationAction) resolveSubscriptionChannel(
	ctx context.Context,
	target action.Target,
) (string, error) {
	if a.Channel != "" {
		return a.Channel, nil
	}

	if !target.DryRun {
		existing, err := target.Client.OLMClient().OperatorsV1alpha1().
			Subscriptions(operatorNamespace).
			Get(ctx, subscriptionName, metav1.GetOptions{})
		if err == nil && existing.Spec != nil && existing.Spec.Channel != "" {
			return existing.Spec.Channel, nil
		}

		if err != nil && !apierrors.IsNotFound(err) && !meta.IsNoMatchError(err) {
			return "", fmt.Errorf("checking existing subscription: %w", err)
		}
	}

	channel, err := olm.ResolveOperatorChannel(ctx, target.Client, olm.PackageQuery{
		PackageName:     subscriptionPackage,
		CatalogSource:   subscriptionSource,
		SourceNamespace: sourceNamespace,
	})
	if err != nil {
		target.IO.Errorf("Warning: could not resolve operator channel from catalog: %v; using fallback %s", err, olm.FallbackKueueOperatorChannel)

		return olm.FallbackKueueOperatorChannel, nil
	}

	return channel, nil
}

func (a *RHBOKMigrationAction) installRHBOKOperator(
	ctx context.Context,
	target action.Target,
) {
	step := target.Recorder.Child(
		"install-rhbok-operator",
		"Install Red Hat Build of Kueue Operator",
	)

	channel, err := a.operatorChannel(ctx, target)
	if err != nil {
		step.Completef(result.StepFailed, "Failed to resolve operator channel: %v", err)

		return
	}

	step.AddDetail("channel", channel)
	if a.selectedOLMMode == olm.ModeV1 {
		a.installRHBOKClusterExtension(ctx, target, channel, step)

		return
	}

	subscriptionExists := false
	if !target.DryRun {
		_, err := target.Client.OLMClient().OperatorsV1alpha1().Subscriptions(operatorNamespace).
			Get(ctx, subscriptionName, metav1.GetOptions{})
		subscriptionExists = err == nil
	}

	if !target.DryRun && !target.SkipConfirm && !subscriptionExists {
		target.IO.Fprintln()
		target.IO.Errorf("About to install Red Hat Build of Kueue Operator (channel: %s)", channel)
		if !confirmation.Prompt(target.IO, "Proceed with operator installation?") {
			step.Completef(result.StepSkipped, "User cancelled installation")

			return
		}
	}

	err = olm.EnsureOperatorInstalled(ctx, target.Client, olm.InstallConfig{
		Name:            subscriptionName,
		Namespace:       operatorNamespace,
		Package:         subscriptionPackage,
		Channel:         channel,
		Source:          subscriptionSource,
		SourceNamespace: sourceNamespace,
		CSVNamePrefix:   csvNamePrefix,
		PollInterval:    operatorPollPeriod,
		Timeout:         operatorTimeout,
		DryRun:          target.DryRun,
		Recorder:        step,
		IO:              target.IO,
	})

	if err != nil {
		step.Completef(result.StepFailed, "Failed to install operator: %v", err)

		return
	}

	if target.DryRun {
		step.Completef(result.StepSkipped, "Operator installation checks completed")
	} else if subscriptionExists {
		step.Completef(result.StepCompleted, "Red Hat build of Kueue operator already installed and ready")
	} else {
		step.Completef(result.StepCompleted, "Red Hat build of Kueue operator installed successfully")
	}
}

func getDSCCondition(dsc *unstructured.Unstructured, conditionType string) (*metav1.Condition, error) {
	conds, err := jq.Query[[]metav1.Condition](dsc, ".status.conditions // []")
	if err != nil {
		return nil, err
	}

	for i := range conds {
		if conds[i].Type == conditionType {
			return &conds[i], nil
		}
	}

	return nil, nil
}
