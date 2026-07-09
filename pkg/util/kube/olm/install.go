package olm

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	operatorsv1 "github.com/operator-framework/api/pkg/operators/v1"
	operatorsv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/opendatahub-io/odh-cli/pkg/migrate/action"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/action/result"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
	"github.com/opendatahub-io/odh-cli/pkg/util/iostreams"
)

const (
	defaultPollInterval = 5 * time.Second
	defaultTimeout      = 5 * time.Minute

	csvPhaseSucceeded = "Succeeded"
)

// InstallConfig holds configuration for operator installation.
type InstallConfig struct {
	Name                string
	Namespace           string
	Package             string
	Channel             string
	Source              string
	SourceNamespace     string
	CSVNamePrefix       string
	OperatorGroupName   string
	TargetNamespaces    []string
	PollInterval        time.Duration
	Timeout             time.Duration
	StartingCSV         string
	InstallPlanApproval string
	DryRun              bool
	Recorder            action.StepRecorder
	IO                  iostreams.Interface
}

// EnsureOperatorInstalled ensures an operator is installed and ready.
// If the subscription doesn't exist, it creates it.
// Then it waits for the CSV to be ready.
func EnsureOperatorInstalled(
	ctx context.Context,
	k8sClient client.Client,
	config InstallConfig,
) error {
	if config.PollInterval == 0 {
		config.PollInterval = defaultPollInterval
	}
	if config.Timeout == 0 {
		config.Timeout = defaultTimeout
	}
	if config.InstallPlanApproval == "" {
		config.InstallPlanApproval = "Automatic"
	}

	if config.DryRun {
		return dryRunOperatorInstall(ctx, k8sClient, config)
	}

	if err := ensureNamespace(ctx, k8sClient, config.Namespace); err != nil {
		return fmt.Errorf("failed to ensure namespace: %w", err)
	}

	operatorGroupName := config.OperatorGroupName
	if operatorGroupName == "" {
		operatorGroupName = defaultOperatorGroupName(config.Namespace)
	}

	if err := ensureOperatorGroup(ctx, k8sClient, config.Namespace, operatorGroupName, config.TargetNamespaces); err != nil {
		return fmt.Errorf("failed to ensure operator group: %w", err)
	}

	// Check if subscription exists
	_, err := k8sClient.OLMClient().OperatorsV1alpha1().Subscriptions(config.Namespace).Get(ctx, config.Name, metav1.GetOptions{})

	switch {
	case err == nil:
		// Subscription already exists, skip creation
	case apierrors.IsNotFound(err):
		// Subscription doesn't exist, create it
		if err := createSubscription(ctx, k8sClient, config); err != nil {
			return fmt.Errorf("failed to create subscription: %w", err)
		}
	default:
		// Other error occurred
		return fmt.Errorf("failed to check subscription: %w", err)
	}

	// Wait for operator to be ready
	if err := WaitForCSV(ctx, k8sClient, config.Namespace, config.CSVNamePrefix, config.PollInterval, config.Timeout); err != nil {
		return fmt.Errorf("failed waiting for operator: %w", err)
	}

	return nil
}

func dryRunOperatorInstall(
	ctx context.Context,
	k8sClient client.Client,
	config InstallConfig,
) error {
	if config.Recorder == nil {
		return errors.New("recorder required for dry-run mode")
	}

	recordDryRunPrerequisiteSteps(config)

	checkStep := config.Recorder.Child("check-subscription",
		fmt.Sprintf("Checking if subscription '%s' exists in namespace '%s'", config.Name, config.Namespace))

	_, err := k8sClient.OLMClient().OperatorsV1alpha1().Subscriptions(config.Namespace).Get(ctx, config.Name, metav1.GetOptions{})

	switch {
	case err == nil:
		checkStep.Completef(result.StepCompleted, "Subscription already exists, skipping creation")

		// Verify CSV is ready (read-only operation, safe to run in dry-run)
		verifyStep := config.Recorder.Child("verify-csv",
			fmt.Sprintf("Verifying operator CSV '%s' is ready in namespace '%s'", config.CSVNamePrefix, config.Namespace))

		if err := WaitForCSV(ctx, k8sClient, config.Namespace, config.CSVNamePrefix, config.PollInterval, config.Timeout); err != nil {
			verifyStep.Completef(result.StepFailed, "CSV verification failed: %v", err)

			return fmt.Errorf("failed waiting for operator CSV: %w", err)
		}

		verifyStep.Completef(result.StepCompleted, "Operator CSV is ready")

	case apierrors.IsNotFound(err):
		checkStep.Completef(result.StepCompleted, "Subscription not found")

		createStep := config.Recorder.Child("create-subscription", "Would create Subscription")
		createStep.AddDetail("name", config.Name)
		createStep.AddDetail("namespace", config.Namespace)
		createStep.AddDetail("package", config.Package)
		createStep.AddDetail("channel", config.Channel)
		createStep.AddDetail("source", config.Source)
		createStep.AddDetail("sourceNamespace", config.SourceNamespace)
		if config.StartingCSV != "" {
			createStep.AddDetail("startingCSV", config.StartingCSV)
		}
		createStep.Completef(result.StepSkipped,
			"Would create subscription %s/%s", config.Namespace, config.Name)

		waitStep := config.Recorder.Child("wait-csv",
			fmt.Sprintf("Would wait for CSV '%s' to reach 'Succeeded' phase", config.CSVNamePrefix))
		waitStep.Completef(result.StepSkipped, "Timeout: %v", config.Timeout)

	default:
		checkStep.Completef(result.StepFailed, "Failed to check subscription: %v", err)

		return fmt.Errorf("failed to check subscription: %w", err)
	}

	return nil
}

func recordDryRunPrerequisiteSteps(config InstallConfig) {
	nsStep := config.Recorder.Child("ensure-namespace",
		fmt.Sprintf("Would ensure namespace '%s' exists", config.Namespace))
	nsStep.Completef(result.StepSkipped, "Would create namespace if missing")

	ogStep := config.Recorder.Child("ensure-operatorgroup",
		fmt.Sprintf("Would ensure OperatorGroup exists in namespace '%s'", config.Namespace))
	ogStep.AddDetail("name", operatorGroupName(config))
	ogStep.Completef(result.StepSkipped, "Would create OperatorGroup if missing")
}

func operatorGroupName(config InstallConfig) string {
	if config.OperatorGroupName != "" {
		return config.OperatorGroupName
	}

	return defaultOperatorGroupName(config.Namespace)
}

func defaultOperatorGroupName(namespace string) string {
	return namespace + "-og"
}

func ensureOperatorGroup(
	ctx context.Context,
	k8sClient client.Client,
	namespace string,
	name string,
	targetNamespaces []string,
) error {
	list, err := k8sClient.OLMClient().OperatorsV1().OperatorGroups(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("list operatorgroups in %s: %w", namespace, err)
	}

	if len(list.Items) > 0 {
		return validateExistingOperatorGroups(list.Items, namespace, targetNamespaces)
	}

	og := &operatorsv1.OperatorGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}

	if len(targetNamespaces) > 0 {
		og.Spec.TargetNamespaces = append([]string(nil), targetNamespaces...)
	}

	_, err = k8sClient.OLMClient().OperatorsV1().OperatorGroups(namespace).Create(ctx, og, metav1.CreateOptions{})
	if err == nil {
		return nil
	}

	if apierrors.IsAlreadyExists(err) {
		list, listErr := k8sClient.OLMClient().OperatorsV1().OperatorGroups(namespace).List(ctx, metav1.ListOptions{})
		if listErr != nil {
			return fmt.Errorf("list operatorgroups in %s after already exists: %w", namespace, listErr)
		}

		return validateExistingOperatorGroups(list.Items, namespace, targetNamespaces)
	}

	return fmt.Errorf("create operatorgroup %s/%s: %w", namespace, name, err)
}

func validateExistingOperatorGroups(
	items []operatorsv1.OperatorGroup,
	namespace string,
	requested []string,
) error {
	if len(items) > 1 {
		return fmt.Errorf("expected at most one OperatorGroup in %s, found %d", namespace, len(items))
	}

	if len(items) == 0 {
		return fmt.Errorf("no OperatorGroup found in %s after create conflict", namespace)
	}

	existingTargets := items[0].Spec.TargetNamespaces
	if !slicesEqualUnordered(existingTargets, requested) {
		return fmt.Errorf(
			"operatorgroup %s/%s exists with targetNamespaces %v, but requested %v",
			namespace, items[0].Name, existingTargets, requested,
		)
	}

	return nil
}

func slicesEqualUnordered(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}

	counts := make(map[string]int, len(a))
	for _, value := range a {
		counts[value]++
	}

	for _, value := range b {
		counts[value]--
		if counts[value] < 0 {
			return false
		}
	}

	return true
}

func ensureNamespace(ctx context.Context, k8sClient client.Client, name string) error {
	_, err := k8sClient.CoreV1().Namespaces().Get(ctx, name, metav1.GetOptions{})
	if err == nil {
		return nil
	}

	if !apierrors.IsNotFound(err) {
		return fmt.Errorf("get namespace %s: %w", name, err)
	}

	_, err = k8sClient.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}, metav1.CreateOptions{})
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("create namespace %s: %w", name, err)
	}

	return nil
}

func createSubscription(
	ctx context.Context,
	k8sClient client.Client,
	config InstallConfig,
) error {
	subscription := &operatorsv1alpha1.Subscription{
		ObjectMeta: metav1.ObjectMeta{
			Name:      config.Name,
			Namespace: config.Namespace,
		},
		Spec: &operatorsv1alpha1.SubscriptionSpec{
			Channel:                config.Channel,
			InstallPlanApproval:    operatorsv1alpha1.Approval(config.InstallPlanApproval),
			Package:                config.Package,
			CatalogSource:          config.Source,
			CatalogSourceNamespace: config.SourceNamespace,
		},
	}

	if config.StartingCSV != "" {
		subscription.Spec.StartingCSV = config.StartingCSV
	}

	_, err := k8sClient.OLMClient().OperatorsV1alpha1().Subscriptions(config.Namespace).Create(ctx, subscription, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create subscription: %w", err)
	}

	return nil
}

// CSVReady reports whether a ClusterServiceVersion with the given name prefix has reached Succeeded phase.
func CSVReady(
	ctx context.Context,
	k8sClient client.Reader,
	namespace string,
	csvNamePrefix string,
) (bool, error) {
	if csvNamePrefix == "" {
		return false, errors.New("csv name prefix must not be empty")
	}

	csvList, err := k8sClient.OLM().ClusterServiceVersions(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return false, fmt.Errorf("listing CSVs in %s: %w", namespace, err)
	}

	for _, csv := range csvList.Items {
		if strings.HasPrefix(csv.Name, csvNamePrefix) && csv.Status.Phase == csvPhaseSucceeded {
			return true, nil
		}
	}

	return false, nil
}

// WaitForCSV waits for a ClusterServiceVersion with the given name prefix to reach Succeeded phase.
func WaitForCSV(
	ctx context.Context,
	k8sClient client.Client,
	namespace string,
	csvNamePrefix string,
	pollInterval time.Duration,
	timeout time.Duration,
) error {
	if csvNamePrefix == "" {
		return errors.New("csv name prefix must not be empty")
	}

	if pollInterval == 0 {
		pollInterval = defaultPollInterval
	}
	if timeout == 0 {
		timeout = defaultTimeout
	}

	err := wait.PollUntilContextTimeout(
		ctx,
		pollInterval,
		timeout,
		true,
		func(ctx context.Context) (bool, error) {
			csvList, err := k8sClient.OLMClient().OperatorsV1alpha1().ClusterServiceVersions(namespace).List(ctx, metav1.ListOptions{})

			if err != nil {
				if client.IsUnrecoverableError(err) {
					return false, fmt.Errorf("unrecoverable error listing CSVs: %w", err)
				}

				return false, nil
			}

			for _, csv := range csvList.Items {
				if strings.HasPrefix(csv.Name, csvNamePrefix) && csv.Status.Phase == csvPhaseSucceeded {
					return true, nil
				}
			}

			return false, nil
		},
	)
	if err != nil {
		return fmt.Errorf("timeout waiting for CSV %s to be ready: %w", csvNamePrefix, err)
	}

	return nil
}
