package training_test

import (
	"bytes"
	"errors"
	"testing"

	"github.com/blang/semver/v4"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8stesting "k8s.io/client-go/testing"

	"github.com/opendatahub-io/odh-cli/pkg/migrate/action"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/action/result"
	trainingaction "github.com/opendatahub-io/odh-cli/pkg/migrate/actions/training"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
	"github.com/opendatahub-io/odh-cli/pkg/util/iostreams"

	. "github.com/onsi/gomega"
)

const (
	testTimestamp             = "2025-01-01T00:00:00Z"
	targetVersion30           = "3.0.0"
	targetVersion36           = "3.6.0"
	trainingOperatorComponent = "trainingoperator"
	managementStateManaged    = "Managed"
	managementStateRemoved    = "Removed"
)

//nolint:gochecknoglobals // Test fixture
var listKinds = map[schema.GroupVersionResource]string{
	resources.PyTorchJob.GVR():                                resources.PyTorchJob.ListKind(),
	resources.TFJob.GVR():                                     resources.TFJob.ListKind(),
	resources.MPIJob.GVR():                                    resources.MPIJob.ListKind(),
	resources.XGBoostJob.GVR():                                resources.XGBoostJob.ListKind(),
	resources.TrainJob.GVR():                                  resources.TrainJob.ListKind(),
	resources.DataScienceCluster.GVR():                        resources.DataScienceCluster.ListKind(),
	resources.GetComponentCR(trainingOperatorComponent).GVR(): resources.GetComponentCR(trainingOperatorComponent).ListKind(),
}

func newTrainingJob(rt resources.ResourceType, name, namespace string, conditions []any) *unstructured.Unstructured {
	obj := map[string]any{
		"apiVersion": rt.APIVersion(),
		"kind":       rt.Kind,
		"metadata": map[string]any{
			"name":              name,
			"namespace":         namespace,
			"creationTimestamp": testTimestamp,
		},
	}

	if conditions != nil {
		obj["status"] = map[string]any{
			"conditions": conditions,
		}
	}

	return &unstructured.Unstructured{Object: obj}
}

func succeededConditions() []any {
	return []any{
		map[string]any{"type": "Created", "status": "True"},
		map[string]any{"type": "Running", "status": "False"},
		map[string]any{"type": "Succeeded", "status": "True"},
	}
}

func runningConditions() []any {
	return []any{
		map[string]any{"type": "Created", "status": "True"},
		map[string]any{"type": "Running", "status": "True"},
	}
}

func failedConditions() []any {
	return []any{
		map[string]any{"type": "Created", "status": "True"},
		map[string]any{"type": "Running", "status": "False"},
		map[string]any{"type": "Failed", "status": "True"},
	}
}

func createdConditions() []any {
	return []any{
		map[string]any{"type": "Created", "status": "True"},
	}
}

func newDSC(specState, statusState string) *unstructured.Unstructured {
	specComponents := map[string]any{}
	statusComponents := map[string]any{}

	if specState != "" {
		specComponents[trainingOperatorComponent] = map[string]any{"managementState": specState}
	}

	if statusState != "" {
		statusComponents[trainingOperatorComponent] = map[string]any{"managementState": statusState}
	}

	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resources.DataScienceCluster.APIVersion(),
			"kind":       resources.DataScienceCluster.Kind,
			"metadata": map[string]any{
				"name": "default-dsc",
			},
			"spec": map[string]any{
				"components": specComponents,
			},
			"status": map[string]any{
				"components": statusComponents,
			},
		},
	}
}

func newTrainingOperatorCR(name string) *unstructured.Unstructured {
	resourceType := *resources.GetComponentCR(trainingOperatorComponent)

	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": resourceType.APIVersion(),
			"kind":       resourceType.Kind,
			"metadata": map[string]any{
				"name": name,
			},
		},
	}
}

func newTestTarget(objects ...runtime.Object) action.Target {
	return newTestTargetForVersion(targetVersion30, objects...)
}

func newTestTargetForVersion(targetVersion string, objects ...runtime.Object) action.Target {
	scheme := runtime.NewScheme()
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, listKinds, objects...)
	testClient := client.NewForTesting(client.TestClientConfig{
		Dynamic: dynamicClient,
	})

	version := semver.MustParse(targetVersion)

	return action.Target{
		Client:         testClient,
		CurrentVersion: &version,
		TargetVersion:  &version,
		DryRun:         false,
		SkipConfirm:    true,
		Recorder:       action.NewRootRecorder(),
		IO:             iostreams.NewIOStreams(nil, &bytes.Buffer{}, &bytes.Buffer{}),
	}
}

func TestVerifyWorkloadsAction_Metadata(t *testing.T) {
	g := NewWithT(t)

	a := &trainingaction.VerifyWorkloadsAction{}
	g.Expect(a.ID()).To(Equal("training.verify-workloads"))
	g.Expect(a.Group()).To(Equal(action.GroupValidation))
	g.Expect(a.Phase()).To(Equal(action.PhasePostUpgrade))
	g.Expect(a.Prepare()).To(BeNil())
	g.Expect(a.Run()).ToNot(BeNil())
}

func TestVerifyWorkloadsAction_CanApply(t *testing.T) {
	t.Run("applies when target version is 3.x", func(t *testing.T) {
		g := NewWithT(t)

		a := &trainingaction.VerifyWorkloadsAction{}
		tv := semver.MustParse(targetVersion30)

		g.Expect(a.CanApply(action.Target{
			TargetVersion: &tv,
		})).To(BeTrue())
	})

	t.Run("applies when current version is nil", func(t *testing.T) {
		g := NewWithT(t)

		a := &trainingaction.VerifyWorkloadsAction{}
		tv := semver.MustParse(targetVersion30)

		g.Expect(a.CanApply(action.Target{
			TargetVersion: &tv,
		})).To(BeTrue())
	})

	t.Run("does not apply when target version is 2.x", func(t *testing.T) {
		g := NewWithT(t)

		a := &trainingaction.VerifyWorkloadsAction{}
		v := semver.MustParse("2.25.0")
		tv := semver.MustParse("2.25.0")

		g.Expect(a.CanApply(action.Target{
			CurrentVersion: &v,
			TargetVersion:  &tv,
		})).To(BeFalse())
	})

	t.Run("does not apply when target version is nil", func(t *testing.T) {
		g := NewWithT(t)

		a := &trainingaction.VerifyWorkloadsAction{}
		v := semver.MustParse("2.25.0")

		g.Expect(a.CanApply(action.Target{
			CurrentVersion: &v,
		})).To(BeFalse())
	})

	t.Run("does not apply for 2.x to 2.x upgrade", func(t *testing.T) {
		g := NewWithT(t)

		a := &trainingaction.VerifyWorkloadsAction{}
		v := semver.MustParse("2.16.0")
		tv := semver.MustParse("2.25.0")

		g.Expect(a.CanApply(action.Target{
			CurrentVersion: &v,
			TargetVersion:  &tv,
		})).To(BeFalse())
	})
}

func TestVerifyWorkloadsAction_RunAt36ReportsUnmanagedResources(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	running := newTrainingJob(resources.PyTorchJob, "running-job", "ns-a", runningConditions())
	target := newTestTargetForVersion(
		targetVersion36,
		newDSC("", managementStateRemoved),
		newTrainingOperatorCR("legacy-trainingoperator"),
		running,
	)

	a := &trainingaction.VerifyWorkloadsAction{}
	actionResult, err := a.Run().Execute(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(actionResult.HasFailedSteps()).To(BeFalse())
	g.Expect(actionResult.HasSkippedSteps()).To(BeTrue())

	dscStep := findStep(actionResult.Status.Steps, "trainingoperator-dsc")
	g.Expect(dscStep).ToNot(BeNil())
	g.Expect(dscStep.Status).To(Equal(result.StepCompleted))
	g.Expect(dscStep.Details["state"]).To(Equal(managementStateRemoved))
	g.Expect(dscStep.Message).To(ContainSubstring("No TrainingOperator installed or managed by RHOAI"))

	pytorchStep := findStep(actionResult.Status.Steps, resources.PyTorchJob.Resource)
	g.Expect(pytorchStep).ToNot(BeNil())
	g.Expect(pytorchStep.Status).To(Equal(result.StepSkipped))
	g.Expect(pytorchStep.Message).To(ContainSubstring("no longer managed by RHOAI"))

	summary := findStep(actionResult.Status.Steps, "summary")
	g.Expect(summary).ToNot(BeNil())
	g.Expect(summary.Status).To(Equal(result.StepSkipped))
	g.Expect(summary.Details["active"]).To(Equal(1))

	componentCRStep := findStep(actionResult.Status.Steps, "trainingoperator-cr")
	g.Expect(componentCRStep).ToNot(BeNil())
	g.Expect(componentCRStep.Status).To(Equal(result.StepSkipped))
	g.Expect(componentCRStep.Details["present"]).To(BeTrue())
	g.Expect(componentCRStep.Message).To(ContainSubstring("no longer managed by RHOAI"))
}

func TestVerifyWorkloadsAction_RunAt36WarnsWhenDSCManaged(t *testing.T) {
	tests := []struct {
		name string
		dsc  *unstructured.Unstructured
	}{
		{name: "managed", dsc: newDSC(managementStateManaged, managementStateRemoved)},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			g := NewWithT(t)
			ctx := t.Context()

			target := newTestTargetForVersion(targetVersion36, test.dsc)
			a := &trainingaction.VerifyWorkloadsAction{}
			actionResult, err := a.Run().Execute(ctx, target)

			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(actionResult.HasFailedSteps()).To(BeFalse())
			g.Expect(actionResult.HasSkippedSteps()).To(BeTrue())

			dscStep := findStep(actionResult.Status.Steps, "trainingoperator-dsc")
			g.Expect(dscStep).ToNot(BeNil())
			g.Expect(dscStep.Status).To(Equal(result.StepSkipped))
			g.Expect(dscStep.Message).To(ContainSubstring("still managed by RHOAI"))
		})
	}
}

func TestVerifyWorkloadsAction_RunAt36IgnoresDSCStatus(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	target := newTestTargetForVersion(targetVersion36, newDSC(managementStateRemoved, managementStateManaged))
	a := &trainingaction.VerifyWorkloadsAction{}
	actionResult, err := a.Run().Execute(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(actionResult.HasFailedSteps()).To(BeFalse())
	g.Expect(actionResult.HasSkippedSteps()).To(BeFalse())

	dscStep := findStep(actionResult.Status.Steps, "trainingoperator-dsc")
	g.Expect(dscStep).ToNot(BeNil())
	g.Expect(dscStep.Status).To(Equal(result.StepCompleted))
	g.Expect(dscStep.Details["state"]).To(Equal(managementStateRemoved))
	g.Expect(dscStep.Message).To(ContainSubstring("No TrainingOperator installed or managed by RHOAI"))

	summary := findStep(actionResult.Status.Steps, "summary")
	g.Expect(summary).ToNot(BeNil())
	g.Expect(summary.Status).To(Equal(result.StepCompleted))
	g.Expect(summary.Details["active"]).To(Equal(0))
}

func TestVerifyWorkloadsAction_RunAt36AllowsMissingTrainJobCRD(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	scheme := runtime.NewScheme()
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, listKinds, newDSC("", managementStateRemoved))
	dynamicClient.PrependReactor("list", "trainjobs", func(_ k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewNotFound(
			schema.GroupResource{Group: resources.TrainJob.Group, Resource: resources.TrainJob.Resource},
			"",
		)
	})

	testClient := client.NewForTesting(client.TestClientConfig{
		Dynamic: dynamicClient,
	})
	version := semver.MustParse(targetVersion36)
	target := action.Target{
		Client:         testClient,
		CurrentVersion: &version,
		TargetVersion:  &version,
		Recorder:       action.NewRootRecorder(),
		IO:             iostreams.NewIOStreams(nil, &bytes.Buffer{}, &bytes.Buffer{}),
	}

	a := &trainingaction.VerifyWorkloadsAction{}
	actionResult, err := a.Run().Execute(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(actionResult.HasFailedSteps()).To(BeFalse())
	g.Expect(actionResult.HasSkippedSteps()).To(BeFalse())

	crdStep := findStep(actionResult.Status.Steps, "trainjob-crd")
	g.Expect(crdStep).ToNot(BeNil())
	g.Expect(crdStep.Status).To(Equal(result.StepCompleted))
	g.Expect(crdStep.Message).To(ContainSubstring("v2 API not yet available"))

	summary := findStep(actionResult.Status.Steps, "summary")
	g.Expect(summary).ToNot(BeNil())
	g.Expect(summary.Status).To(Equal(result.StepCompleted))
	g.Expect(summary.Details["active"]).To(Equal(0))
}

func TestVerifyWorkloadsAction_RunAt36CompletesSummaryForTerminalWorkloads(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	terminal := newTrainingJob(resources.PyTorchJob, "terminal-job", "ns-a", succeededConditions())
	target := newTestTargetForVersion(
		targetVersion36,
		newDSC("", managementStateRemoved),
		terminal,
	)

	a := &trainingaction.VerifyWorkloadsAction{}
	actionResult, err := a.Run().Execute(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(actionResult.HasFailedSteps()).To(BeFalse())
	g.Expect(actionResult.HasSkippedSteps()).To(BeTrue())

	summary := findStep(actionResult.Status.Steps, "summary")
	g.Expect(summary).ToNot(BeNil())
	g.Expect(summary.Status).To(Equal(result.StepCompleted))
	g.Expect(summary.Details["active"]).To(Equal(0))
	g.Expect(summary.Details["terminal"]).To(Equal(1))
}

func TestVerifyWorkloadsAction_RunCategorizesBlockers(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	running := newTrainingJob(resources.PyTorchJob, "running-job", "ns-a", runningConditions())
	created := newTrainingJob(resources.TFJob, "created-job", "ns-b", createdConditions())

	target := newTestTarget(running, created)

	a := &trainingaction.VerifyWorkloadsAction{}
	actionResult, err := a.Run().Execute(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(actionResult).ToNot(BeNil())
	g.Expect(actionResult.Status.Completed).To(BeFalse())

	readiness := findStep(actionResult.Status.Steps, "migration-readiness")
	g.Expect(readiness).ToNot(BeNil())
	g.Expect(readiness.Status).To(Equal(result.StepFailed))
	g.Expect(readiness.Message).To(ContainSubstring("[BLOCKER]"))
	g.Expect(readiness.Message).To(ContainSubstring("2 active"))
	g.Expect(readiness.Details["blockers"]).To(Equal(2))
	g.Expect(readiness.Details["completable"]).To(Equal(0))
}

func TestVerifyWorkloadsAction_RunNoBlockers(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	succeeded := newTrainingJob(resources.PyTorchJob, "done-job", "ns-a", succeededConditions())
	failed := newTrainingJob(resources.TFJob, "failed-job", "ns-a", failedConditions())

	target := newTestTarget(succeeded, failed)

	a := &trainingaction.VerifyWorkloadsAction{}
	actionResult, err := a.Run().Execute(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(actionResult).ToNot(BeNil())

	readiness := findStep(actionResult.Status.Steps, "migration-readiness")
	g.Expect(readiness).ToNot(BeNil())
	g.Expect(readiness.Status).To(Equal(result.StepCompleted))
	g.Expect(readiness.Message).To(ContainSubstring("safe to proceed"))
	g.Expect(readiness.Details["blockers"]).To(Equal(0))
	g.Expect(readiness.Details["completable"]).To(Equal(2))
}

func TestVerifyWorkloadsAction_RunMixed(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	running := newTrainingJob(resources.PyTorchJob, "active-1", "ns-a", runningConditions())
	succeeded1 := newTrainingJob(resources.PyTorchJob, "done-1", "ns-a", succeededConditions())
	succeeded2 := newTrainingJob(resources.TFJob, "done-2", "ns-a", succeededConditions())
	failed := newTrainingJob(resources.MPIJob, "failed-1", "ns-b", failedConditions())

	target := newTestTarget(running, succeeded1, succeeded2, failed)

	a := &trainingaction.VerifyWorkloadsAction{}
	actionResult, err := a.Run().Execute(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())

	readiness := findStep(actionResult.Status.Steps, "migration-readiness")
	g.Expect(readiness).ToNot(BeNil())
	g.Expect(readiness.Status).To(Equal(result.StepFailed))
	g.Expect(readiness.Details["blockers"]).To(Equal(1))
	g.Expect(readiness.Details["completable"]).To(Equal(3))

	summary := findStep(actionResult.Status.Steps, "summary")
	g.Expect(summary).ToNot(BeNil())
	g.Expect(summary.Details["total"]).To(Equal(4))
	g.Expect(summary.Details["blockers"]).To(Equal(1))
	g.Expect(summary.Details["completable"]).To(Equal(3))
}

func TestVerifyWorkloadsAction_RunTrainJobCRDInstalled(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	target := newTestTarget()

	a := &trainingaction.VerifyWorkloadsAction{}
	actionResult, err := a.Run().Execute(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())

	crdStep := findStep(actionResult.Status.Steps, "trainjob-crd")
	g.Expect(crdStep).ToNot(BeNil())
	g.Expect(crdStep.Status).To(Equal(result.StepCompleted))
	g.Expect(crdStep.Details["trainjobCRDInstalled"]).To(BeTrue())
	g.Expect(crdStep.Message).To(ContainSubstring("v2 API available"))

	summary := findStep(actionResult.Status.Steps, "summary")
	g.Expect(summary).ToNot(BeNil())
	g.Expect(summary.Details["trainjobCRDInstalled"]).To(BeTrue())
}

func TestVerifyWorkloadsAction_RunTrainJobCRDNotInstalled(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	scheme := runtime.NewScheme()
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, listKinds)
	dynamicClient.PrependReactor("list", "trainjobs", func(_ k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewNotFound(
			schema.GroupResource{Group: resources.TrainJob.Group, Resource: resources.TrainJob.Resource},
			"",
		)
	})

	testClient := client.NewForTesting(client.TestClientConfig{
		Dynamic: dynamicClient,
	})

	v := semver.MustParse("2.25.0")
	tv := semver.MustParse("3.0.0")

	target := action.Target{
		Client:         testClient,
		CurrentVersion: &v,
		TargetVersion:  &tv,
		Recorder:       action.NewRootRecorder(),
		IO:             iostreams.NewIOStreams(nil, &bytes.Buffer{}, &bytes.Buffer{}),
	}

	a := &trainingaction.VerifyWorkloadsAction{}
	actionResult, err := a.Run().Execute(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())

	crdStep := findStep(actionResult.Status.Steps, "trainjob-crd")
	g.Expect(crdStep).ToNot(BeNil())
	g.Expect(crdStep.Status).To(Equal(result.StepCompleted))
	g.Expect(crdStep.Details["trainjobCRDInstalled"]).To(BeFalse())
	g.Expect(crdStep.Message).To(ContainSubstring("not installed"))
}

func TestVerifyWorkloadsAction_RunTrainJobCRDCheckError(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	scheme := runtime.NewScheme()
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, listKinds)
	dynamicClient.PrependReactor("list", "trainjobs", func(_ k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("connection refused")
	})

	testClient := client.NewForTesting(client.TestClientConfig{
		Dynamic: dynamicClient,
	})

	v := semver.MustParse("2.25.0")
	tv := semver.MustParse("3.0.0")

	target := action.Target{
		Client:         testClient,
		CurrentVersion: &v,
		TargetVersion:  &tv,
		Recorder:       action.NewRootRecorder(),
		IO:             iostreams.NewIOStreams(nil, &bytes.Buffer{}, &bytes.Buffer{}),
	}

	a := &trainingaction.VerifyWorkloadsAction{}
	actionResult, err := a.Run().Execute(ctx, target)

	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("checking TrainJob CRD"))

	crdStep := findStep(actionResult.Status.Steps, "trainjob-crd")
	g.Expect(crdStep).ToNot(BeNil())
	g.Expect(crdStep.Status).To(Equal(result.StepFailed))
	g.Expect(crdStep.Details["trainjobCRDInstalled"]).To(BeFalse())
}

func TestVerifyWorkloadsAction_RunMigrationMap(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	pytorch := newTrainingJob(resources.PyTorchJob, "pt-1", "ns-a", succeededConditions())
	tfjob := newTrainingJob(resources.TFJob, "tf-1", "ns-a", succeededConditions())

	target := newTestTarget(pytorch, tfjob)

	a := &trainingaction.VerifyWorkloadsAction{}
	actionResult, err := a.Run().Execute(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())

	summary := findStep(actionResult.Status.Steps, "summary")
	g.Expect(summary).ToNot(BeNil())

	migrationMap, ok := summary.Details["migrationMap"].(map[string]string)
	g.Expect(ok).To(BeTrue())
	g.Expect(migrationMap).To(HaveLen(2))
	g.Expect(migrationMap["PyTorchJob"]).To(ContainSubstring("torch"))
	g.Expect(migrationMap["TFJob"]).To(ContainSubstring("tensorflow"))
	g.Expect(migrationMap).ToNot(HaveKey("MPIJob"))
	g.Expect(migrationMap).ToNot(HaveKey("XGBoostJob"))
}

func TestVerifyWorkloadsAction_RunNoWorkloads(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	target := newTestTarget()

	a := &trainingaction.VerifyWorkloadsAction{}
	actionResult, err := a.Run().Execute(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(actionResult).ToNot(BeNil())
	g.Expect(actionResult.Status.Completed).To(BeTrue())

	readiness := findStep(actionResult.Status.Steps, "migration-readiness")
	g.Expect(readiness).ToNot(BeNil())
	g.Expect(readiness.Status).To(Equal(result.StepCompleted))
	g.Expect(readiness.Message).To(ContainSubstring("nothing to migrate"))

	summary := findStep(actionResult.Status.Steps, "summary")
	g.Expect(summary).ToNot(BeNil())
	g.Expect(summary.Details["total"]).To(Equal(0))
}

func TestVerifyWorkloadsAction_RunAllListsFail(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	scheme := runtime.NewScheme()
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, listKinds)
	dynamicClient.PrependReactor("list", "*", func(_ k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("connection refused")
	})
	dynamicClient.PrependReactor("list", "trainjobs", func(_ k8stesting.Action) (bool, runtime.Object, error) {
		return true, &unstructured.UnstructuredList{}, nil
	})

	testClient := client.NewForTesting(client.TestClientConfig{
		Dynamic: dynamicClient,
	})

	v := semver.MustParse("2.25.0")
	tv := semver.MustParse("3.0.0")

	target := action.Target{
		Client:         testClient,
		CurrentVersion: &v,
		TargetVersion:  &tv,
		Recorder:       action.NewRootRecorder(),
		IO:             iostreams.NewIOStreams(nil, &bytes.Buffer{}, &bytes.Buffer{}),
	}

	a := &trainingaction.VerifyWorkloadsAction{}
	_, err := a.Run().Execute(ctx, target)

	g.Expect(err).To(HaveOccurred())
	g.Expect(err.Error()).To(ContainSubstring("failed to list any training workload types"))
}

func TestVerifyWorkloadsAction_RunPartialFailure(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	tfjob := newTrainingJob(resources.TFJob, "tf-1", "ns-a", succeededConditions())

	scheme := runtime.NewScheme()
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, listKinds, tfjob)
	dynamicClient.PrependReactor("list", "pytorchjobs", func(_ k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("connection refused")
	})

	testClient := client.NewForTesting(client.TestClientConfig{
		Dynamic: dynamicClient,
	})

	v := semver.MustParse("2.25.0")
	tv := semver.MustParse("3.0.0")

	target := action.Target{
		Client:         testClient,
		CurrentVersion: &v,
		TargetVersion:  &tv,
		Recorder:       action.NewRootRecorder(),
		IO:             iostreams.NewIOStreams(nil, &bytes.Buffer{}, &bytes.Buffer{}),
	}

	a := &trainingaction.VerifyWorkloadsAction{}
	actionResult, err := a.Run().Execute(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(actionResult).ToNot(BeNil())
	g.Expect(actionResult.Status.Completed).To(BeFalse())

	pytorchStep := findStep(actionResult.Status.Steps, "pytorchjobs")
	g.Expect(pytorchStep).ToNot(BeNil())
	g.Expect(pytorchStep.Status).To(Equal(result.StepFailed))

	summary := findStep(actionResult.Status.Steps, "summary")
	g.Expect(summary).ToNot(BeNil())
	g.Expect(summary.Details["total"]).To(Equal(1))
}

func TestVerifyWorkloadsAction_ValidateSameAsExecute(t *testing.T) {
	g := NewWithT(t)
	ctx := t.Context()

	pytorch := newTrainingJob(resources.PyTorchJob, "pt-1", "ns-a", succeededConditions())
	target := newTestTarget(pytorch)

	a := &trainingaction.VerifyWorkloadsAction{}
	actionResult, err := a.Run().Validate(ctx, target)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(actionResult).ToNot(BeNil())
	g.Expect(actionResult.Status.Completed).To(BeTrue())
}

func findStep(steps []result.ActionStep, name string) *result.ActionStep {
	for i := range steps {
		if steps[i].Name == name {
			return &steps[i]
		}
	}

	return nil
}
