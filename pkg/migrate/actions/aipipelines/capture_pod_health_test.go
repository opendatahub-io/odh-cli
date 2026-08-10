package aipipelines_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/opendatahub-io/odh-cli/pkg/migrate/action"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/action/result"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/actions/aipipelines"
	"github.com/opendatahub-io/odh-cli/pkg/util/iostreams"

	. "github.com/onsi/gomega"
)

func newTestRecorder() action.RootRecorder {
	io := iostreams.NewIOStreams(&bytes.Buffer{}, &bytes.Buffer{}, &bytes.Buffer{})

	return action.NewVerboseRootRecorder(io)
}

func TestCaptureAndSavePodHealth(t *testing.T) {
	t.Run("captures and saves state when no DSPAs exist", func(t *testing.T) {
		g := NewWithT(t)

		dir := t.TempDir()
		statePath := filepath.Join(dir, "dspa_pre_upgrade_pods.json")
		cleanup := aipipelines.SetDefaultStatePath(statePath)
		defer cleanup()

		c := newFakeClient()
		recorder := newTestRecorder()

		aipipelines.CaptureAndSavePodHealth(context.Background(), c, recorder, false, false)

		_, err := os.Stat(statePath)
		g.Expect(err).ToNot(HaveOccurred())

		state, err := aipipelines.LoadPodHealthState(statePath)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(state.DSPAs).To(BeEmpty())
		g.Expect(state.CapturedAt).ToNot(BeEmpty())
	})

	t.Run("captures and saves state with DSPAs and pods", func(t *testing.T) {
		g := NewWithT(t)

		dir := t.TempDir()
		statePath := filepath.Join(dir, "dspa_pre_upgrade_pods.json")
		cleanup := aipipelines.SetDefaultStatePath(statePath)
		defer cleanup()

		pod := aipipelines.MakePodUnstructured("ds-pipeline-mydspa-abc", "ns1", "Running", "True")
		c := newFakeClient(
			makeDSPA("mydspa", "ns1", "v1"),
			&pod,
		)
		recorder := newTestRecorder()

		aipipelines.CaptureAndSavePodHealth(context.Background(), c, recorder, false, false)

		state, err := aipipelines.LoadPodHealthState(statePath)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(state.DSPAs).To(HaveLen(1))
		g.Expect(state.DSPAs[0].Name).To(Equal("mydspa"))
	})

	t.Run("dry-run does not write state file", func(t *testing.T) {
		g := NewWithT(t)

		dir := t.TempDir()
		statePath := filepath.Join(dir, "dspa_pre_upgrade_pods.json")
		cleanup := aipipelines.SetDefaultStatePath(statePath)
		defer cleanup()

		c := newFakeClient()
		recorder := newTestRecorder()

		aipipelines.CaptureAndSavePodHealth(context.Background(), c, recorder, true, false)

		_, err := os.Stat(statePath)
		g.Expect(os.IsNotExist(err)).To(BeTrue())

		res := recorder.Build()
		g.Expect(findStep(res, "save-state")).ToNot(BeNil())
		g.Expect(findStep(res, "save-state").Status).To(Equal(result.StepSkipped))
	})

	t.Run("skipIfExists skips when state file already exists", func(t *testing.T) {
		g := NewWithT(t)

		dir := t.TempDir()
		statePath := filepath.Join(dir, "dspa_pre_upgrade_pods.json")
		cleanup := aipipelines.SetDefaultStatePath(statePath)
		defer cleanup()

		existing := aipipelines.PodHealthState{CapturedAt: "2026-01-01T00:00:00Z"}
		err := aipipelines.SavePodHealthState(existing, statePath)
		g.Expect(err).ToNot(HaveOccurred())

		c := newFakeClient()
		recorder := newTestRecorder()

		aipipelines.CaptureAndSavePodHealth(context.Background(), c, recorder, false, true)

		state, err := aipipelines.LoadPodHealthState(statePath)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(state.CapturedAt).To(Equal("2026-01-01T00:00:00Z"))
	})

	t.Run("skipIfExists captures when no state file exists", func(t *testing.T) {
		g := NewWithT(t)

		dir := t.TempDir()
		statePath := filepath.Join(dir, "dspa_pre_upgrade_pods.json")
		cleanup := aipipelines.SetDefaultStatePath(statePath)
		defer cleanup()

		c := newFakeClient()
		recorder := newTestRecorder()

		aipipelines.CaptureAndSavePodHealth(context.Background(), c, recorder, false, true)

		_, err := os.Stat(statePath)
		g.Expect(err).ToNot(HaveOccurred())

		state, err := aipipelines.LoadPodHealthState(statePath)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(state.CapturedAt).ToNot(BeEmpty())
	})

	t.Run("prepare path always overwrites existing state", func(t *testing.T) {
		g := NewWithT(t)

		dir := t.TempDir()
		statePath := filepath.Join(dir, "dspa_pre_upgrade_pods.json")
		cleanup := aipipelines.SetDefaultStatePath(statePath)
		defer cleanup()

		existing := aipipelines.PodHealthState{CapturedAt: "2026-01-01T00:00:00Z"}
		err := aipipelines.SavePodHealthState(existing, statePath)
		g.Expect(err).ToNot(HaveOccurred())

		c := newFakeClient()
		recorder := newTestRecorder()

		aipipelines.CaptureAndSavePodHealth(context.Background(), c, recorder, false, false)

		state, err := aipipelines.LoadPodHealthState(statePath)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(state.CapturedAt).ToNot(Equal("2026-01-01T00:00:00Z"))
	})
}

func findStep(res *result.ActionResult, name string) *result.ActionStep {
	for i := range res.Status.Steps {
		if s := findStepRecursive(&res.Status.Steps[i], name); s != nil {
			return s
		}
	}

	return nil
}

func findStepRecursive(step *result.ActionStep, name string) *result.ActionStep {
	if step.Name == name {
		return step
	}

	for i := range step.Children {
		if s := findStepRecursive(&step.Children[i], name); s != nil {
			return s
		}
	}

	return nil
}
