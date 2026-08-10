package aipipelines

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/opendatahub-io/odh-cli/pkg/migrate/action"
	"github.com/opendatahub-io/odh-cli/pkg/migrate/action/result"
	"github.com/opendatahub-io/odh-cli/pkg/resources"
	"github.com/opendatahub-io/odh-cli/pkg/util/client"
)

// PodHealthState is the top-level snapshot saved to disk for pre→post comparison.
type PodHealthState struct {
	CapturedAt string      `json:"capturedAt"`
	DSPAs      []DSPAState `json:"dspas"`
}

type DSPAState struct {
	Name      string     `json:"dspaName"`
	Namespace string     `json:"namespace"`
	PodGroups []PodGroup `json:"podGroups"`
}

type PodGroup struct {
	Prefix     string    `json:"prefix"`
	PodsFound  bool      `json:"podsFound"`
	AllHealthy bool      `json:"allHealthy"`
	Pods       []PodInfo `json:"pods"`
}

type PodInfo struct {
	Name    string `json:"name"`
	Phase   string `json:"phase"`
	Ready   string `json:"ready"`
	Healthy bool   `json:"healthy"`
}

type podGroupStatus string

const (
	podGroupOK       podGroupStatus = "OK"
	podGroupFail     podGroupStatus = "FAIL"
	podGroupImproved podGroupStatus = "IMPROVED"
	podGroupWarn     podGroupStatus = "WARN"
)

type podGroupComparison struct {
	DSPAName  string
	Namespace string
	Prefix    string
	Status    podGroupStatus
	Current   PodGroup
}

func capturePodHealth(
	ctx context.Context,
	c client.Client,
	recorder action.StepRecorder,
	dspas []dspaInfo,
) (PodHealthState, error) {
	state := PodHealthState{
		CapturedAt: time.Now().UTC().Format(time.RFC3339),
		DSPAs:      make([]DSPAState, 0, len(dspas)),
	}

	for _, dspa := range dspas {
		step := recorder.Child(
			"capture-"+dspa.Namespace+"-"+dspa.Name,
			fmt.Sprintf("DSPA: %s | Namespace: %s", dspa.Name, dspa.Namespace),
		)

		pods, err := c.Dynamic().Resource(resources.Pod.GVR()).
			Namespace(dspa.Namespace).
			List(ctx, metav1.ListOptions{})
		if err != nil {
			step.Completef(result.StepFailed, "Failed to list pods: %v", err)

			return state, fmt.Errorf("listing pods in %s: %w", dspa.Namespace, err)
		}

		dspaState := DSPAState{
			Name:      dspa.Name,
			Namespace: dspa.Namespace,
			PodGroups: make([]PodGroup, 0, 2), //nolint:mnd
		}

		for _, prefix := range []string{podPrefixDSPipeline + dspa.Name, podPrefixMariaDB + dspa.Name} {
			group := getPodGroup(pods.Items, prefix)
			dspaState.PodGroups = append(dspaState.PodGroups, group)

			if !group.PodsFound {
				step.Recordf(prefix, "[WARN] No pods found", result.StepCompleted)
			} else if group.AllHealthy {
				step.Recordf(prefix, "[OK] All pods healthy", result.StepCompleted)
			} else {
				for _, pod := range group.Pods {
					if pod.Healthy {
						step.Recordf(pod.Name, "[OK] Running, Ready", result.StepCompleted)
					} else {
						step.Recordf(pod.Name, "[FAIL] Phase: %s, Ready: %s", result.StepFailed, pod.Phase, pod.Ready)
					}
				}
			}
		}

		state.DSPAs = append(state.DSPAs, dspaState)
		step.Completef(result.StepCompleted, "Captured %d pod groups", len(dspaState.PodGroups))
	}

	return state, nil
}

func getPodGroup(pods []unstructured.Unstructured, prefix string) PodGroup {
	group := PodGroup{
		Prefix: prefix,
		Pods:   []PodInfo{},
	}

	for i := range pods {
		pod := &pods[i]
		name := pod.GetName()

		if !strings.HasPrefix(name, prefix) {
			continue
		}

		phase, _, _ := unstructured.NestedString(pod.Object, "status", "phase")
		if phase == "" {
			phase = "Unknown"
		}

		ready := "Unknown"

		conditions, found, _ := unstructured.NestedSlice(pod.Object, "status", "conditions")
		if found {
			for _, c := range conditions {
				cond, ok := c.(map[string]any)
				if !ok {
					continue
				}

				if condType, _ := cond["type"].(string); condType == "Ready" {
					if val, _ := cond["status"].(string); val != "" {
						ready = val
					}

					break
				}
			}
		}

		healthy := phase == "Running" && ready == "True"

		group.Pods = append(group.Pods, PodInfo{
			Name:    name,
			Phase:   phase,
			Ready:   ready,
			Healthy: healthy,
		})
	}

	group.PodsFound = len(group.Pods) > 0
	group.AllHealthy = group.PodsFound

	for _, pod := range group.Pods {
		if !pod.Healthy {
			group.AllHealthy = false

			break
		}
	}

	return group
}

func comparePodHealth(
	ctx context.Context,
	c client.Client,
	pre PodHealthState,
) ([]podGroupComparison, error) {
	var comparisons []podGroupComparison

	for _, dspa := range pre.DSPAs {
		pods, err := c.Dynamic().Resource(resources.Pod.GVR()).
			Namespace(dspa.Namespace).
			List(ctx, metav1.ListOptions{})
		if err != nil {
			return nil, fmt.Errorf("listing pods in %s: %w", dspa.Namespace, err)
		}

		for _, preGroup := range dspa.PodGroups {
			curGroup := getPodGroup(pods.Items, preGroup.Prefix)

			var status podGroupStatus

			switch {
			case curGroup.AllHealthy && preGroup.AllHealthy:
				status = podGroupOK
			case curGroup.AllHealthy && !preGroup.AllHealthy:
				status = podGroupImproved
			case !curGroup.AllHealthy && !preGroup.AllHealthy:
				status = podGroupWarn
			default:
				status = podGroupFail
			}

			comparisons = append(comparisons, podGroupComparison{
				DSPAName:  dspa.Name,
				Namespace: dspa.Namespace,
				Prefix:    preGroup.Prefix,
				Status:    status,
				Current:   curGroup,
			})
		}
	}

	return comparisons, nil
}

func hasAnyDegradation(comparisons []podGroupComparison) bool {
	for _, c := range comparisons {
		if c.Status == podGroupFail {
			return true
		}
	}

	return false
}

func savePodHealthState(state PodHealthState, paths ...string) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling state: %w", err)
	}

	for _, p := range paths {
		dir := filepath.Dir(p)
		if err := os.MkdirAll(dir, dirPermissions); err != nil {
			return fmt.Errorf("creating directory %s: %w", dir, err)
		}

		if err := os.WriteFile(p, data, filePermissions); err != nil {
			return fmt.Errorf("writing state to %s: %w", p, err)
		}
	}

	return nil
}

func loadPodHealthState(path string) (PodHealthState, error) {
	var state PodHealthState

	data, err := os.ReadFile(path) //nolint:gosec // path is from defaultStatePath(), not user input
	if err != nil {
		return state, fmt.Errorf("reading state from %s: %w", path, err)
	}

	if err := json.Unmarshal(data, &state); err != nil {
		return state, fmt.Errorf("unmarshaling state from %s: %w", path, err)
	}

	return state, nil
}

var defaultStatePathFunc = func() string { //nolint:gochecknoglobals // Overridden in tests via export_test.go
	return filepath.Join(defaultStateDir, stateFileName)
}

func defaultStatePath() string {
	return defaultStatePathFunc()
}
