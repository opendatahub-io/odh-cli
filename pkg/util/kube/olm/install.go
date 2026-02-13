package olm

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/opendatahub-io/odh-cli/pkg/migrate/action"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/action/result"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
	"github.com/opendatahub-io/odh-cli/pkg/util/iostreams"
	operatorsv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
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

	checkStep := config.Recorder.Child("check-subscription",
		fmt.Sprintf("Checking if subscription '%s' exists in namespace '%s'", config.Name, config.Namespace))

	_, err := k8sClient.OLMClient().OperatorsV1alpha1().Subscriptions(config.Namespace).Get(ctx, config.Name, metav1.GetOptions{})

	switch {
	case err == nil:
		checkStep.Complete(result.StepCompleted, "Subscription already exists, skipping creation")

		// Verify CSV is ready (read-only operation, safe to run in dry-run)
		verifyStep := config.Recorder.Child("verify-csv",
			fmt.Sprintf("Verifying operator CSV '%s' is ready in namespace '%s'", config.CSVNamePrefix, config.Namespace))

		if err := WaitForCSV(ctx, k8sClient, config.Namespace, config.CSVNamePrefix, config.PollInterval, config.Timeout); err != nil {
			verifyStep.Complete(result.StepFailed, fmt.Sprintf("CSV verification failed: %v", err))

			return fmt.Errorf("failed waiting for operator CSV: %w", err)
		}

		verifyStep.Complete(result.StepCompleted, "Operator CSV is ready")

	case apierrors.IsNotFound(err):
		checkStep.Complete(result.StepCompleted, "Subscription not found")

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
		createStep.Complete(result.StepSkipped,
			fmt.Sprintf("Would create subscription %s/%s", config.Namespace, config.Name))

		waitStep := config.Recorder.Child("wait-csv",
			fmt.Sprintf("Would wait for CSV '%s' to reach 'Succeeded' phase", config.CSVNamePrefix))
		waitStep.Complete(result.StepSkipped, fmt.Sprintf("Timeout: %v", config.Timeout))

	default:
		checkStep.Complete(result.StepFailed, fmt.Sprintf("Failed to check subscription: %v", err))

		return fmt.Errorf("failed to check subscription: %w", err)
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

// WaitForCSV waits for a ClusterServiceVersion with the given name prefix to reach Succeeded phase.
func WaitForCSV(
	ctx context.Context,
	k8sClient client.Client,
	namespace string,
	csvNamePrefix string,
	pollInterval time.Duration,
	timeout time.Duration,
) error {
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
