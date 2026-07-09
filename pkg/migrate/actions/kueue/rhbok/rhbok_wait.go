package rhbok

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/opendatahub-io/odh-cli/pkg/constants"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/action"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/action/result"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
)

//nolint:gochecknoglobals // test-only poll overrides set via SetTestPollConfig in export_test.go
var (
	testComponentPollPeriod time.Duration
	testComponentTimeout    time.Duration
)

func podReady(pod *corev1.Pod) bool {
	if pod.Status.Phase == corev1.PodSucceeded {
		return true
	}

	if pod.Status.Phase != corev1.PodRunning {
		return false
	}

	for _, cond := range pod.Status.Conditions {
		if cond.Type == corev1.PodReady && cond.Status == corev1.ConditionTrue {
			return true
		}
	}

	return false
}

func pollPeriod() time.Duration {
	if testComponentPollPeriod > 0 {
		return testComponentPollPeriod
	}

	return componentPollPeriod
}

func pollTimeout() time.Duration {
	if testComponentTimeout > 0 {
		return testComponentTimeout
	}

	return componentTimeout
}

func (a *RHBOKMigrationAction) waitForEmbeddedRemoval(
	ctx context.Context,
	target action.Target,
) {
	step := target.Recorder.Child(
		"wait-embedded-removal",
		"Wait for embedded Kueue to be removed",
	)

	if a.SkipRemoveEmbedded {
		step.Completef(result.StepSkipped, "Skipped (--skip-remove-embedded)")

		return
	}

	if target.DryRun {
		step.Completef(result.StepSkipped, "Would wait for embedded Kueue removal")

		return
	}

	err := wait.PollUntilContextTimeout(ctx, pollPeriod(), pollTimeout(), true,
		func(ctx context.Context) (bool, error) {
			state, err := a.getKueueManagementState(ctx, target.Client)
			if err != nil {
				return false, err
			}

			if state != constants.ManagementStateRemoved {
				return false, nil
			}

			dsc, err := client.GetSingleton(ctx, target.Client, resources.DataScienceClusterV1)
			if err != nil {
				return false, fmt.Errorf("getting DataScienceCluster: %w", err)
			}

			cond, err := getDSCCondition(dsc, kueueReadyType)
			if err != nil {
				return false, err
			}

			if cond != nil && cond.Status == metav1.ConditionFalse && cond.Reason == constants.ManagementStateRemoved {
				return true, nil
			}

			_, err = target.Client.Dynamic().Resource(resources.Deployment.GVR()).
				Namespace(applicationsNamespace).
				Get(ctx, embeddedKueueDeployment, metav1.GetOptions{})
			if apierrors.IsNotFound(err) {
				return true, nil
			}

			if err != nil {
				return false, fmt.Errorf("getting embedded Kueue deployment: %w", err)
			}

			return false, nil
		})
	if err != nil {
		step.Completef(result.StepFailed, "Timed out waiting for embedded Kueue removal: %v", err)

		return
	}

	step.Completef(result.StepCompleted, "Embedded Kueue removed")
}

func (a *RHBOKMigrationAction) waitForKueueReady(
	ctx context.Context,
	target action.Target,
) {
	step := target.Recorder.Child(
		"wait-kueue-ready",
		"Wait for KueueReady condition",
	)

	if target.DryRun {
		step.Completef(result.StepSkipped, "Would wait for KueueReady=True")

		return
	}

	err := wait.PollUntilContextTimeout(ctx, pollPeriod(), pollTimeout(), true,
		func(ctx context.Context) (bool, error) {
			dsc, err := client.GetSingleton(ctx, target.Client, resources.DataScienceClusterV1)
			if err != nil {
				return false, fmt.Errorf("getting DataScienceCluster: %w", err)
			}

			cond, err := getDSCCondition(dsc, kueueReadyType)
			if err != nil {
				return false, err
			}

			if cond == nil || cond.Status != metav1.ConditionTrue {
				return false, nil
			}

			pods, err := target.Client.CoreV1().Pods(operatorNamespace).List(ctx, metav1.ListOptions{})
			if err != nil {
				return false, fmt.Errorf("listing RHBOK pods: %w", err)
			}

			if len(pods.Items) == 0 {
				return false, nil
			}

			for i := range pods.Items {
				if !podReady(&pods.Items[i]) {
					return false, nil
				}
			}

			return true, nil
		})
	if err != nil {
		step.Completef(result.StepFailed, "Timed out waiting for KueueReady: %v", err)

		return
	}

	step.Completef(result.StepCompleted, "KueueReady=True and RHBOK pods ready")
}
