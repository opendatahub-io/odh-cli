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

const testTimestamp = "2025-01-01T00:00:00Z"

//nolint:gochecknoglobals // Test fixture
var listKinds = map[schema.GroupVersionResource]string{
	resources.PyTorchJob.GVR(): resources.PyTorchJob.ListKind(),
	resources.TFJob.GVR():      resources.TFJob.ListKind(),
	resources.MPIJob.GVR():     resources.MPIJob.ListKind(),
	resources.XGBoostJob.GVR(): resources.XGBoostJob.ListKind(),
	resources.TrainJob.GVR():   resources.TrainJob.ListKind(),
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

func newTestTarget(objects ...runtime.Object) action.Target {
	scheme := runtime.NewScheme()
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, listKinds, objects...)
	testClient := client.NewForTesting(client.TestClientConfig{
		Dynamic: dynamicClient,
	})

	v := semver.MustParse("2.25.0")
	tv := semver.MustParse("3.0.0")

	return action.Target{
		Client:         testClient,
		CurrentVersion: &v,
		TargetVersion:  &tv,
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
	t.Run("applies when current version is 2.x", func(t *testing.T) {
		g := NewWithT(t)

		a := &trainingaction.VerifyWorkloadsAction{}
		v := semver.MustParse("2.25.0")
		tv := semver.MustParse("3.0.0")

		g.Expect(a.CanApply(action.Target{
			CurrentVersion: &v,
			TargetVersion:  &tv,
		})).To(BeTrue())
	})

	t.Run("does not apply when current version is 3.x", func(t *testing.T) {
		g := NewWithT(t)

		a := &trainingaction.VerifyWorkloadsAction{}
		v := semver.MustParse("3.0.0")
		tv := semver.MustParse("3.1.0")

		g.Expect(a.CanApply(action.Target{
			CurrentVersion: &v,
			TargetVersion:  &tv,
		})).To(BeFalse())
	})

	t.Run("does not apply when current version is nil", func(t *testing.T) {
		g := NewWithT(t)

		a := &trainingaction.VerifyWorkloadsAction{}
		tv := semver.MustParse("3.0.0")

		g.Expect(a.CanApply(action.Target{
			TargetVersion: &tv,
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
